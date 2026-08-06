package controller

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// credentialProfileRequest is the create/update payload for a credential
// profile. Key and BaseURL are pointers so handlers can tell "absent" from
// "explicitly empty": an explicit empty key keeps the old key, an explicit
// empty base_url clears it, and base_url:null means "not provided".
type credentialProfileRequest struct {
	Name    string  `json:"name"`
	Key     *string `json:"key"`
	BaseURL *string `json:"base_url"`
	Remark  *string `json:"remark"`
}

// credentialProfileBindRequest carries the complete binding set for
// PUT /credential-profiles/:id/channels.
type credentialProfileBindRequest struct {
	ChannelIds []int `json:"channel_ids"`
}

// credentialProfileApplyRequest requires callers to explicitly choose preview or apply.
type credentialProfileApplyRequest struct {
	DryRun *bool `json:"dry_run"`
}

type credentialProfileApplyResult struct {
	ChannelID  int    `json:"channel_id"`
	Name       string `json:"name"`
	Type       int    `json:"type"`
	BaseURL    string `json:"base_url"`
	IsMultiKey bool   `json:"is_multi_key"`
	Success    bool   `json:"success"`
	Synced     bool   `json:"synced"` // key/base_url were (or would be) written
	Error      string `json:"error"`
}

type credentialProfileChannelView struct {
	Id           int    `json:"id"`
	Name         string `json:"name"`
	Type         int    `json:"type"`
	Status       int    `json:"status"`
	BaseURL      string `json:"base_url"`
	Models       string `json:"models"`
	Group        string `json:"group"`
	Tag          string `json:"tag"`
	IsMultiKey   bool   `json:"is_multi_key"`
	MultiKeySize int    `json:"multi_key_size"`
	InSync       bool   `json:"in_sync"`
}

// GetCredentialProfiles lists all profiles with bound_count and
// out_of_sync_count. Keys are never serialized (json:"-" on the model).
func GetCredentialProfiles(c *gin.Context) {
	summaries, err := model.GetCredentialProfileSummaries()
	if err != nil {
		common.SysError("failed to get credential profiles: " + err.Error())
		common.ApiErrorMsg(c, "获取凭据配置失败，请稍后重试")
		return
	}
	if summaries == nil {
		summaries = []*model.CredentialProfileSummary{}
	}
	common.ApiSuccess(c, summaries)
}

// GetCredentialProfile returns a single profile without its key.
func GetCredentialProfile(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	profile, err := model.GetCredentialProfileById(id, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorI18n(c, i18n.MsgNotFound)
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, profile)
}

// CreateCredentialProfile creates a profile. A non-empty key is required: a
// profile is the source of truth for credential materialization, so a key-less
// profile could wipe channel keys on apply.
func CreateCredentialProfile(c *gin.Context) {
	rawBody, err := c.GetRawData()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req credentialProfileRequest
	if err := common.Unmarshal(rawBody, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		common.ApiErrorI18n(c, i18n.MsgNameCannotBeEmpty)
		return
	}
	if req.Key == nil || strings.TrimSpace(*req.Key) == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	taken, err := model.CredentialProfileNameTaken(name, 0)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if taken {
		common.ApiErrorI18n(c, i18n.MsgAlreadyExists)
		return
	}
	profile := &model.ChannelCredentialProfile{
		Name:    name,
		Key:     strings.TrimSpace(*req.Key),
		BaseURL: normalizeOptionalStringPointer(req.BaseURL),
		Remark:  normalizeOptionalStringPointer(req.Remark),
	}
	if err := profile.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.credential_profile_create", map[string]interface{}{
		"id":   profile.Id,
		"name": profile.Name,
	})
	common.ApiSuccess(c, profile)
}

// UpdateCredentialProfile updates a profile. The existing profile is loaded
// (with its key) and mutated, so omitted fields naturally keep their values:
// key present-but-empty keeps the old key, base_url present-but-empty clears
// it, base_url:null means "not provided".
func UpdateCredentialProfile(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	rawBody, err := c.GetRawData()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req credentialProfileRequest
	if err := common.Unmarshal(rawBody, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	var requestData map[string]any
	if err := common.Unmarshal(rawBody, &requestData); err != nil {
		common.ApiError(c, err)
		return
	}
	profile, err := model.GetCredentialProfileById(id, true)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorI18n(c, i18n.MsgNotFound)
			return
		}
		common.ApiError(c, err)
		return
	}
	if _, ok := requestData["name"]; ok {
		name := strings.TrimSpace(req.Name)
		if name == "" {
			common.ApiErrorI18n(c, i18n.MsgNameCannotBeEmpty)
			return
		}
		if name != profile.Name {
			taken, err := model.CredentialProfileNameTaken(name, profile.Id)
			if err != nil {
				common.ApiError(c, err)
				return
			}
			if taken {
				common.ApiErrorI18n(c, i18n.MsgAlreadyExists)
				return
			}
		}
		profile.Name = name
	}
	if req.Key != nil && strings.TrimSpace(*req.Key) != "" {
		profile.Key = strings.TrimSpace(*req.Key)
	}
	if _, ok := requestData["base_url"]; ok && req.BaseURL != nil {
		profile.BaseURL = trimStringPointer(req.BaseURL)
	}
	if _, ok := requestData["remark"]; ok {
		profile.Remark = normalizeOptionalStringPointer(req.Remark)
	}
	if err := profile.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.credential_profile_update", map[string]interface{}{
		"id":   profile.Id,
		"name": profile.Name,
	})
	common.ApiSuccess(c, profile)
}

// DeleteCredentialProfile deletes a profile. Profiles with bound channels are
// rejected so a profile cannot silently stop managing channels.
func DeleteCredentialProfile(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	profile, err := model.GetCredentialProfileById(id, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorI18n(c, i18n.MsgNotFound)
			return
		}
		common.ApiError(c, err)
		return
	}
	if err := model.DeleteCredentialProfileTx(id); err != nil {
		if errors.Is(err, model.ErrCredentialProfileBound) {
			common.ApiErrorMsg(c, "无法删除：仍有渠道引用该凭据配置，请先解绑")
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorI18n(c, i18n.MsgNotFound)
			return
		}
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.credential_profile_delete", map[string]interface{}{
		"id":   id,
		"name": profile.Name,
	})
	common.ApiSuccess(c, gin.H{})
}

// GetCredentialProfileChannels lists the channels bound to a profile with their
// in_sync state. Keys are never returned; only a boolean comparison result.
func GetCredentialProfileChannels(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	profile, err := model.GetCredentialProfileById(id, true)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorI18n(c, i18n.MsgNotFound)
			return
		}
		common.ApiError(c, err)
		return
	}
	channels, err := model.GetChannelsByCredentialProfile(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]credentialProfileChannelView, 0, len(channels))
	for _, channel := range channels {
		tag := ""
		if channel.Tag != nil {
			tag = *channel.Tag
		}
		items = append(items, credentialProfileChannelView{
			Id:           channel.Id,
			Name:         channel.Name,
			Type:         channel.Type,
			Status:       channel.Status,
			BaseURL:      channelBaseURLString(channel.BaseURL),
			Models:       channel.Models,
			Group:        channel.Group,
			Tag:          tag,
			IsMultiKey:   channel.ChannelInfo.IsMultiKey,
			MultiKeySize: channel.ChannelInfo.MultiKeySize,
			InSync:       model.ProfileAndChannelInSync(profile, channel),
		})
	}
	common.ApiSuccess(c, items)
}

// SetCredentialProfileChannels replaces the profile's binding set. Unbinding
// never touches a channel's key/base_url; newly bound channels are validated
// and materialized immediately (per-item results). Channels that stay bound
// are left untouched.
func SetCredentialProfileChannels(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	profile, err := model.GetCredentialProfileById(id, true)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorI18n(c, i18n.MsgNotFound)
			return
		}
		common.ApiError(c, err)
		return
	}
	rawBody, err := c.GetRawData()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req credentialProfileBindRequest
	if err := common.Unmarshal(rawBody, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	seen := make(map[int]struct{}, len(req.ChannelIds))
	newIds := make([]int, 0, len(req.ChannelIds))
	for _, channelId := range req.ChannelIds {
		if _, ok := seen[channelId]; ok {
			continue
		}
		seen[channelId] = struct{}{}
		newIds = append(newIds, channelId)
	}
	if len(newIds) > credentialRefreshMaxChannels {
		common.ApiErrorMsg(c, "绑定渠道数量过多，单次最多 "+strconv.Itoa(credentialRefreshMaxChannels)+" 个")
		return
	}
	// Atomically replace the binding set: read current bindings, reject any
	// desired channel already bound to another profile (no silent re-binding),
	// then unbind removed / bind added. Any failure rolls back everything.
	addedIds, removedIds, err := model.ReplaceCredentialProfileBindings(id, newIds)
	if err != nil {
		var conflictErr *model.CredentialProfileConflictError
		if errors.As(err, &conflictErr) {
			common.ApiErrorMsg(c, "以下渠道已绑定其他凭据配置，无法绑定："+intsToCSV(conflictErr.ChannelIds))
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorI18n(c, i18n.MsgNotFound)
			return
		}
		common.ApiError(c, err)
		return
	}

	results := make([]credentialProfileApplyResult, 0, len(addedIds))
	succeeded, syncedOK, failed := 0, 0, 0
	failedChannelIds := make([]int, 0)
	// Load the newly bound channels with their keys for the sync comparison.
	channels, err := model.GetChannelsByIds(addedIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channelById := make(map[int]*model.Channel, len(channels))
	for _, channel := range channels {
		channelById[channel.Id] = channel
	}
	for _, channelId := range addedIds {
		result := credentialProfileApplyResult{ChannelID: channelId}
		channel, ok := channelById[channelId]
		if !ok {
			failed++
			failedChannelIds = append(failedChannelIds, channelId)
			result.Error = "channel not found"
			results = append(results, result)
			continue
		}
		result.Name = channel.Name
		result.Type = channel.Type
		result.IsMultiKey = channel.ChannelInfo.IsMultiKey
		// The transaction above already persisted the binding, so this channel
		// stays bound even if the immediate materialization fails (it then
		// shows as out of sync until the next apply).
		synced, baseURL, syncErr := syncChannelFromCredentialProfile(profile, channel, false)
		result.BaseURL = baseURL
		if syncErr != nil {
			failed++
			failedChannelIds = append(failedChannelIds, channelId)
			result.Error = syncErr.Error()
			results = append(results, result)
			continue
		}
		result.Synced = synced
		result.Success = true
		if synced {
			syncedOK++
		}
		succeeded++
		results = append(results, result)
	}
	if syncedOK > 0 {
		model.InitChannelCache()
	}
	recordManageAudit(c, "channel.credential_profile_bind", map[string]interface{}{
		"id":                  profile.Id,
		"name":                profile.Name,
		"added":               len(addedIds),
		"removed":             len(removedIds),
		"added_channel_ids":   addedIds,
		"removed_channel_ids": removedIds,
		"failed_channel_ids":  failedChannelIds,
	})
	common.ApiSuccess(c, gin.H{
		"total":     len(addedIds),
		"succeeded": succeeded,
		"failed":    failed,
		"synced":    syncedOK,
		"removed":   len(removedIds),
		"results":   results,
	})
}

// ApplyCredentialProfile materializes the profile's key/base_url onto every
// bound channel (replace semantics, matching the single-channel edit path).
// dry_run previews per-channel results without writing. One failing channel
// never blocks the rest, and the response/audit never contain key material.
func ApplyCredentialProfile(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	profile, err := model.GetCredentialProfileById(id, true)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorI18n(c, i18n.MsgNotFound)
			return
		}
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(profile.Key) == "" {
		common.ApiErrorMsg(c, "凭据配置缺少密钥，无法应用")
		return
	}
	var req credentialProfileApplyRequest
	rawBody, err := c.GetRawData()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(string(rawBody)) == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := common.Unmarshal(rawBody, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.DryRun == nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	dryRun := *req.DryRun

	channels, err := model.GetChannelsByCredentialProfile(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(channels) > credentialRefreshMaxChannels {
		common.ApiErrorMsg(c, "绑定渠道数量过多，单次最多 "+strconv.Itoa(credentialRefreshMaxChannels)+" 个")
		return
	}

	results := make([]credentialProfileApplyResult, 0, len(channels))
	succeeded, failed, syncedCount := 0, 0, 0
	for _, channel := range channels {
		result := credentialProfileApplyResult{
			ChannelID:  channel.Id,
			Name:       channel.Name,
			Type:       channel.Type,
			IsMultiKey: channel.ChannelInfo.IsMultiKey,
		}
		synced, baseURL, syncErr := syncChannelFromCredentialProfile(profile, channel, dryRun)
		result.BaseURL = baseURL
		if syncErr != nil {
			failed++
			result.Error = syncErr.Error()
			results = append(results, result)
			continue
		}
		result.Synced = synced
		result.Success = true
		if synced {
			syncedCount++
		}
		succeeded++
		results = append(results, result)
	}
	if !dryRun && syncedCount > 0 {
		// One cache refresh after all writes; base_url writes may change the
		// proxy setting only if the channel itself changes it, which apply
		// never does, so no proxy client invalidation is needed here.
		model.InitChannelCache()
	}

	auditAction := "channel.credential_profile_apply"
	if dryRun {
		auditAction = "channel.credential_profile_preview"
	}
	recordManageAudit(c, auditAction, map[string]interface{}{
		"id":        profile.Id,
		"name":      profile.Name,
		"dry_run":   dryRun,
		"total":     len(channels),
		"succeeded": succeeded,
		"failed":    failed,
		"synced":    syncedCount,
	})
	common.ApiSuccess(c, gin.H{
		"dry_run":   dryRun,
		"total":     len(channels),
		"succeeded": succeeded,
		"failed":    failed,
		"synced":    syncedCount,
		"results":   results,
	})
}

// syncChannelFromCredentialProfile computes the materialized channel state for
// the profile (replace key merge + base_url override), validates it, and — when
// not a dry run and something actually changed — persists it via
// Channel.Update() so multi-key invariants are maintained. It returns whether
// a write happened (or would happen for a dry run), the resulting base URL for
// display, and any error.
func syncChannelFromCredentialProfile(profile *model.ChannelCredentialProfile, channel *model.Channel, dryRun bool) (synced bool, baseURL string, err error) {
	working := *channel
	mergedKey, mergeErr := mergeChannelKeys(channel.Key, profile.Key, channel.ChannelInfo.IsMultiKey, credentialKeyModeReplace, channel.Type, channel.GetOtherSettings().VertexKeyType)
	if mergeErr != nil {
		return false, channelBaseURLString(channel.BaseURL), mergeErr
	}
	working.Key = mergedKey
	if profile.BaseURL != nil {
		working.BaseURL = profile.BaseURL
	}
	if err := validateChannel(&working, false); err != nil {
		return false, channelBaseURLString(working.BaseURL), err
	}
	keyChanged := working.Key != channel.Key
	baseURLChanged := !equalStringPtr(working.BaseURL, channel.BaseURL)
	if dryRun {
		return keyChanged || baseURLChanged, channelBaseURLString(working.BaseURL), nil
	}
	if !keyChanged && !baseURLChanged {
		return false, channelBaseURLString(working.BaseURL), nil
	}
	if err := working.Update(); err != nil {
		return false, channelBaseURLString(working.BaseURL), err
	}
	return true, channelBaseURLString(working.BaseURL), nil
}

func trimStringPointer(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	return &trimmed
}

// normalizeOptionalStringPointer trims a string pointer and collapses an empty
// result to nil. Used for optional profile fields (base_url on create, remark).
func normalizeOptionalStringPointer(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// intsToCSV renders int ids as a comma-separated list for user-facing error
// messages. It never carries key material.
func intsToCSV(ids []int) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.Itoa(id))
	}
	return strings.Join(parts, ",")
}
