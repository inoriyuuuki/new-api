package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// ChannelCredentialProfile is a named, reusable credential preset (API key +
// base URL) that channels can reference. A profile is the management source of
// truth: applying it materializes key/base_url onto the bound channels, while
// the relay continues to read only model.Channel. The Key column is as
// sensitive as Channel.Key and is never serialized (json:"-") nor returned by
// any list/detail endpoint.
//
// BaseURL semantics: nil means the profile does not manage base_url (apply and
// sync comparisons ignore it), a non-nil value (including "") is managed and
// materialized onto bound channels.
type ChannelCredentialProfile struct {
	Id          int     `json:"id" gorm:"primaryKey"`
	Name        string  `json:"name" gorm:"type:varchar(64);uniqueIndex;not null"`
	Key         string  `json:"-" gorm:"not null"`
	BaseURL     *string `json:"base_url"`
	Remark      *string `json:"remark" gorm:"type:varchar(255)"`
	CreatedTime int64   `json:"created_time" gorm:"bigint"`
	UpdatedTime int64   `json:"updated_time" gorm:"bigint"`
}

// CredentialProfileSummary augments a profile with derived counts for list
// views. Key stays hidden (json:"-") even though the embedded profile was
// loaded with its key to compute out-of-sync counts.
type CredentialProfileSummary struct {
	ChannelCredentialProfile
	BoundCount     int `json:"bound_count"`
	OutOfSyncCount int `json:"out_of_sync_count"`
}

// CredentialProfileConflictError reports that one or more desired channels are
// already bound to a different credential profile. Only channel IDs are
// carried; never any key material.
type CredentialProfileConflictError struct {
	ChannelIds []int
}

func (e *CredentialProfileConflictError) Error() string {
	return fmt.Sprintf("channels are already bound to another credential profile: %v", e.ChannelIds)
}

// ErrCredentialProfileBound is returned when deleting a profile that still has
// bound channels.
var ErrCredentialProfileBound = errors.New("credential profile still has bound channels")

// GetCredentialProfiles returns all profiles ordered by id. When selectKey is
// false the key column is omitted from the query as defense in depth; when
// true the caller must be an internal path that needs the key to compute
// sync state and must never serialize it.
func GetCredentialProfiles(selectKey bool) ([]*ChannelCredentialProfile, error) {
	var profiles []*ChannelCredentialProfile
	query := DB.Order("id asc")
	if !selectKey {
		query = query.Omit("key")
	}
	err := query.Find(&profiles).Error
	return profiles, err
}

// GetCredentialProfileById returns a single profile. See GetCredentialProfiles
// for the selectKey semantics.
func GetCredentialProfileById(id int, selectKey bool) (*ChannelCredentialProfile, error) {
	profile := &ChannelCredentialProfile{Id: id}
	query := DB
	if !selectKey {
		query = query.Omit("key")
	}
	err := query.First(profile, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return profile, nil
}

// Insert persists a new profile. The unique index on Name is enforced by the
// database on every supported dialect.
func (profile *ChannelCredentialProfile) Insert() error {
	now := common.GetTimestamp()
	profile.CreatedTime = now
	profile.UpdatedTime = now
	return DB.Create(profile).Error
}

// Update persists the editable fields (name/key/base_url/remark). Callers load
// the existing profile with its key first and mutate it, so "keep the old key"
// falls out naturally instead of being re-encoded here.
func (profile *ChannelCredentialProfile) Update() error {
	profile.UpdatedTime = common.GetTimestamp()
	return DB.Model(profile).Select("name", "key", "base_url", "remark", "updated_time").Updates(profile).Error
}

// GetChannelsByCredentialProfile returns channels bound to the profile,
// including disabled ones (disabled channels still need credential rotation).
func GetChannelsByCredentialProfile(profileId int, selectKey bool) ([]*Channel, error) {
	var channels []*Channel
	query := DB.Where("credential_profile_id = ?", profileId)
	if !selectKey {
		query = query.Omit("key")
	}
	err := query.Find(&channels).Error
	return channels, err
}

// ReplaceCredentialProfileBindings atomically replaces the channel binding set
// of a profile. Inside one transaction it locks the profile row (the SQLite
// branch of lockForUpdate skips FOR UPDATE, which SQLite does not support),
// reads the current bindings, rejects the whole request if any desired channel
// is already bound to a different profile (no silent re-binding), then unbinds
// removed and binds added channels. Any error rolls back every change so the
// controller never issues two independent UPDATEs.
//
// It returns the newly bound channel ids and the unbound channel ids. A
// conflicting channel set is reported through *CredentialProfileConflictError
// (see its ChannelIds field).
func ReplaceCredentialProfileBindings(profileId int, desiredIds []int) (addedIds, removedIds []int, err error) {
	err = DB.Transaction(func(tx *gorm.DB) error {
		var profile ChannelCredentialProfile
		if err := lockForUpdate(tx).First(&profile, "id = ?", profileId).Error; err != nil {
			return err
		}

		var currentIds []int
		if err := tx.Model(&Channel{}).Where("credential_profile_id = ?", profileId).Pluck("id", &currentIds).Error; err != nil {
			return err
		}
		currentSet := make(map[int]struct{}, len(currentIds))
		for _, id := range currentIds {
			currentSet[id] = struct{}{}
		}

		// Lock desired channel rows before checking ownership. Locking only the
		// profile row does not serialize requests for different profiles that
		// target the same channel, allowing both conflict checks to pass.
		if len(desiredIds) > 0 {
			var desiredChannels []Channel
			if err := lockForUpdate(tx).
				Select("id", "credential_profile_id").
				Where("id IN ?", desiredIds).
				Order("id asc").
				Find(&desiredChannels).Error; err != nil {
				return err
			}

			conflictIds := make([]int, 0)
			for _, channel := range desiredChannels {
				if channel.CredentialProfileId != nil && *channel.CredentialProfileId != profileId {
					conflictIds = append(conflictIds, channel.Id)
				}
			}
			if len(conflictIds) > 0 {
				return &CredentialProfileConflictError{ChannelIds: conflictIds}
			}
		}

		desiredSet := make(map[int]struct{}, len(desiredIds))
		for _, id := range desiredIds {
			desiredSet[id] = struct{}{}
		}
		addedIds = make([]int, 0, len(desiredIds))
		for _, id := range desiredIds {
			if _, ok := currentSet[id]; !ok {
				addedIds = append(addedIds, id)
			}
		}
		removedIds = make([]int, 0, len(currentIds))
		for _, id := range currentIds {
			if _, ok := desiredSet[id]; !ok {
				removedIds = append(removedIds, id)
			}
		}

		if len(removedIds) > 0 {
			if err := tx.Model(&Channel{}).Where("id IN ?", removedIds).Update("credential_profile_id", nil).Error; err != nil {
				return err
			}
		}
		if len(addedIds) > 0 {
			if err := tx.Model(&Channel{}).Where("id IN ?", addedIds).Update("credential_profile_id", profileId).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return addedIds, removedIds, err
}

// DeleteCredentialProfileTx deletes a profile while holding a lock on its row
// (skipped on SQLite). Profiles with bound channels are rejected via
// ErrCredentialProfileBound, so a profile can never silently stop managing
// channels.
func DeleteCredentialProfileTx(id int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var profile ChannelCredentialProfile
		if err := lockForUpdate(tx).First(&profile, "id = ?", id).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&Channel{}).Where("credential_profile_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrCredentialProfileBound
		}
		return tx.Delete(&ChannelCredentialProfile{}, "id = ?", id).Error
	})
}

// GetCredentialProfileSummaries returns all profiles with their derived
// bound_count and out_of_sync_count. A bound channel is in sync when its
// materialized key (and base_url, only when the profile manages it) equals the
// profile's; direct channel edits that drift away show up as out of sync.
//
// NOTE: this loads every bound channel's key/base_url to compute the counts.
// A join/aggregation (or a dedicated out-of-sync counter) can replace the
// full scan later if profile counts grow large; profile lists are expected to
// be small, so the current read is kept for simplicity and exactness.
func GetCredentialProfileSummaries() ([]*CredentialProfileSummary, error) {
	profiles, err := GetCredentialProfiles(true)
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return nil, nil
	}
	var channels []*Channel
	if err := DB.Select("id", "credential_profile_id", "key", "base_url").Where("credential_profile_id IS NOT NULL").Find(&channels).Error; err != nil {
		return nil, err
	}
	byProfile := make(map[int][]*Channel)
	for _, channel := range channels {
		if channel.CredentialProfileId == nil {
			continue
		}
		byProfile[*channel.CredentialProfileId] = append(byProfile[*channel.CredentialProfileId], channel)
	}
	summaries := make([]*CredentialProfileSummary, 0, len(profiles))
	for _, profile := range profiles {
		bound := byProfile[profile.Id]
		outOfSync := 0
		for _, channel := range bound {
			if !ProfileAndChannelInSync(profile, channel) {
				outOfSync++
			}
		}
		summaries = append(summaries, &CredentialProfileSummary{
			ChannelCredentialProfile: *profile,
			BoundCount:               len(bound),
			OutOfSyncCount:           outOfSync,
		})
	}
	return summaries, nil
}

// ProfileAndChannelInSync reports whether a bound channel's materialized
// credential state still matches the profile. When the profile does not manage
// base_url (BaseURL == nil), only the key is compared; when it does, both key
// and base_url must match.
func ProfileAndChannelInSync(profile *ChannelCredentialProfile, channel *Channel) bool {
	if channel.Key != profile.Key {
		return false
	}
	if profile.BaseURL == nil {
		return true
	}
	if channel.BaseURL == nil {
		return false
	}
	return *channel.BaseURL == *profile.BaseURL
}

// CredentialProfileNameTaken reports whether another profile already uses the
// given name. excludeId > 0 skips that profile for update flows.
func CredentialProfileNameTaken(name string, excludeId int) (bool, error) {
	var count int64
	query := DB.Model(&ChannelCredentialProfile{}).Where("name = ?", name)
	if excludeId > 0 {
		query = query.Where("id != ?", excludeId)
	}
	err := query.Count(&count).Error
	return count > 0, err
}
