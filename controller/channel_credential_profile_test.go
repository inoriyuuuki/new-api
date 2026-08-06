package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCredentialProfileTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelCredentialProfile{}, &model.Log{}))
	return db
}

func newCredentialProfileRequest(t *testing.T, method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 1)
	ctx.Set("role", common.RoleRootUser)
	ctx.Set("username", "tester")
	return ctx, recorder
}

func setCredentialProfilePathParams(ctx *gin.Context, id int) {
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(id)}}
}

func decodeCredentialProfileResponse(t *testing.T, recorder *httptest.ResponseRecorder) (bool, map[string]any) {
	t.Helper()
	var resp struct {
		Success bool           `json:"success"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	return resp.Success, resp.Data
}

func decodeCredentialProfileArrayResponse(t *testing.T, recorder *httptest.ResponseRecorder) (bool, []any) {
	t.Helper()
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    []any  `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	return resp.Success, resp.Data
}

func TestCredentialProfileCreateAndListHideKey(t *testing.T) {
	db := setupCredentialProfileTestDB(t)

	ctx, recorder := newCredentialProfileRequest(t, http.MethodPost, "/api/channel/credential-profiles", `{"name":"shared","key":"sk-super-secret","base_url":"https://api.example.com"}`)
	CreateCredentialProfile(ctx)
	success, data := decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)
	assert.Equal(t, "shared", data["name"])
	assert.NotContains(t, recorder.Body.String(), "sk-super-secret")

	ctx, recorder = newCredentialProfileRequest(t, http.MethodGet, "/api/channel/credential-profiles", "")
	GetCredentialProfiles(ctx)
	success, items := decodeCredentialProfileArrayResponse(t, recorder)
	require.True(t, success)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	assert.Equal(t, float64(1), item["id"])
	assert.Equal(t, "shared", item["name"])
	assert.Equal(t, float64(0), item["bound_count"])
	assert.Equal(t, float64(0), item["out_of_sync_count"])
	assert.NotContains(t, recorder.Body.String(), "sk-super-secret")

	ctx, recorder = newCredentialProfileRequest(t, http.MethodGet, "/api/channel/credential-profiles/1", "")
	setCredentialProfilePathParams(ctx, 1)
	GetCredentialProfile(ctx)
	success, data = decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)
	assert.Equal(t, "shared", data["name"])
	assert.NotContains(t, recorder.Body.String(), "sk-super-secret")

	// the profile still stores the key (source of truth for apply)
	var stored model.ChannelCredentialProfile
	require.NoError(t, db.First(&stored, "id = ?", 1).Error)
	assert.Equal(t, "sk-super-secret", stored.Key)
}

func TestCredentialProfileValidationAndUniqueness(t *testing.T) {
	setupCredentialProfileTestDB(t)

	tests := []struct {
		name string
		body string
	}{
		{name: "missing key", body: `{"name":"no-key"}`},
		{name: "empty key", body: `{"name":"no-key","key":""}`},
		{name: "empty name", body: `{"name":"","key":"k"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, recorder := newCredentialProfileRequest(t, http.MethodPost, "/api/channel/credential-profiles", test.body)
			CreateCredentialProfile(ctx)
			success, _ := decodeCredentialProfileResponse(t, recorder)
			assert.False(t, success)
		})
	}

	ctx, recorder := newCredentialProfileRequest(t, http.MethodPost, "/api/channel/credential-profiles", `{"name":"shared","key":"k1"}`)
	CreateCredentialProfile(ctx)
	success, _ := decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)

	ctx, recorder = newCredentialProfileRequest(t, http.MethodPost, "/api/channel/credential-profiles", `{"name":"shared","key":"k2"}`)
	CreateCredentialProfile(ctx)
	success, _ = decodeCredentialProfileResponse(t, recorder)
	assert.False(t, success)
	assert.Contains(t, recorder.Body.String(), "common.already_exists")
}

func TestCredentialProfileUpdatePresenceSemantics(t *testing.T) {
	db := setupCredentialProfileTestDB(t)
	profile := &model.ChannelCredentialProfile{Name: "shared", Key: "old-key", BaseURL: common.GetPointer("https://old.example")}
	require.NoError(t, db.Create(profile).Error)

	// empty key keeps the old key; explicit empty base_url clears it.
	ctx, recorder := newCredentialProfileRequest(t, http.MethodPut, "/api/channel/credential-profiles/1", `{"name":"shared2","key":"","base_url":"","remark":"note"}`)
	setCredentialProfilePathParams(ctx, profile.Id)
	UpdateCredentialProfile(ctx)
	success, _ := decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)

	reloaded, err := model.GetCredentialProfileById(profile.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "shared2", reloaded.Name)
	assert.Equal(t, "old-key", reloaded.Key)
	require.NotNil(t, reloaded.BaseURL)
	assert.Equal(t, "", *reloaded.BaseURL)
	require.NotNil(t, reloaded.Remark)
	assert.Equal(t, "note", *reloaded.Remark)
	assert.NotContains(t, recorder.Body.String(), "old-key")

	// null base_url means "not provided": the cleared value stays cleared.
	ctx, recorder = newCredentialProfileRequest(t, http.MethodPut, "/api/channel/credential-profiles/1", `{"base_url":null}`)
	setCredentialProfilePathParams(ctx, profile.Id)
	UpdateCredentialProfile(ctx)
	success, _ = decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)
	reloaded, err = model.GetCredentialProfileById(profile.Id, true)
	require.NoError(t, err)
	require.NotNil(t, reloaded.BaseURL)
	assert.Equal(t, "", *reloaded.BaseURL)

	// non-empty key updates.
	ctx, recorder = newCredentialProfileRequest(t, http.MethodPut, "/api/channel/credential-profiles/1", `{"key":"new-key"}`)
	setCredentialProfilePathParams(ctx, profile.Id)
	UpdateCredentialProfile(ctx)
	success, _ = decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)
	reloaded, err = model.GetCredentialProfileById(profile.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "new-key", reloaded.Key)

	// rename to an existing name is rejected.
	other := &model.ChannelCredentialProfile{Name: "other", Key: "k"}
	require.NoError(t, db.Create(other).Error)
	ctx, recorder = newCredentialProfileRequest(t, http.MethodPut, "/api/channel/credential-profiles/1", `{"name":"other"}`)
	setCredentialProfilePathParams(ctx, profile.Id)
	UpdateCredentialProfile(ctx)
	success, _ = decodeCredentialProfileResponse(t, recorder)
	assert.False(t, success)
	assert.Contains(t, recorder.Body.String(), "common.already_exists")
}

func TestCredentialProfileDeleteGuard(t *testing.T) {
	db := setupCredentialProfileTestDB(t)

	profile := &model.ChannelCredentialProfile{Name: "shared", Key: "k"}
	require.NoError(t, db.Create(profile).Error)
	ctx, recorder := newCredentialProfileRequest(t, http.MethodDelete, "/api/channel/credential-profiles/1", "")
	setCredentialProfilePathParams(ctx, profile.Id)
	DeleteCredentialProfile(ctx)
	success, _ := decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)

	bound := &model.ChannelCredentialProfile{Name: "bound", Key: "k2"}
	require.NoError(t, db.Create(bound).Error)
	channel := &model.Channel{Name: "a", Key: "chan-key", Models: "gpt-4o", Group: "default", CredentialProfileId: common.GetPointer(bound.Id)}
	require.NoError(t, db.Create(channel).Error)

	ctx, recorder = newCredentialProfileRequest(t, http.MethodDelete, "/api/channel/credential-profiles/2", "")
	setCredentialProfilePathParams(ctx, bound.Id)
	DeleteCredentialProfile(ctx)
	success, _ = decodeCredentialProfileResponse(t, recorder)
	assert.False(t, success)
	assert.Contains(t, recorder.Body.String(), "解绑")
}

func TestCredentialProfileBindAndSync(t *testing.T) {
	db := setupCredentialProfileTestDB(t)
	profile := &model.ChannelCredentialProfile{Name: "shared", Key: "profile-key", BaseURL: common.GetPointer("https://profile.example")}
	require.NoError(t, db.Create(profile).Error)

	first := &model.Channel{Name: "a", Key: "old-a", BaseURL: common.GetPointer("https://old.example"), Models: "gpt-4o", Group: "default"}
	second := &model.Channel{Name: "b", Key: "old-b", Models: "gpt-4o", Group: "default"}
	require.NoError(t, db.Create(first).Error)
	require.NoError(t, db.Create(second).Error)

	body := fmt.Sprintf(`{"channel_ids":[%d,%d]}`, first.Id, second.Id)
	ctx, recorder := newCredentialProfileRequest(t, http.MethodPut, "/api/channel/credential-profiles/1/channels", body)
	setCredentialProfilePathParams(ctx, profile.Id)
	SetCredentialProfileChannels(ctx)
	success, data := decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)
	assert.Equal(t, float64(2), data["total"])
	assert.Equal(t, float64(2), data["succeeded"])
	assert.Equal(t, float64(0), data["failed"])
	assert.Equal(t, float64(2), data["synced"])
	assert.Equal(t, float64(0), data["removed"])

	for _, id := range []int{first.Id, second.Id} {
		channel := reloadChannelByID(t, id)
		require.NotNil(t, channel.CredentialProfileId)
		assert.Equal(t, profile.Id, *channel.CredentialProfileId)
		assert.Equal(t, "profile-key", channel.Key)
		require.NotNil(t, channel.BaseURL)
		assert.Equal(t, "https://profile.example", *channel.BaseURL)
	}
	assert.NotContains(t, recorder.Body.String(), "profile-key")

	var bindAudit model.Log
	require.NoError(t, db.Where("content = ?", fmt.Sprintf(
		"Bound credential profile %s (ID: %d) to 2 channels and unbound 0",
		profile.Name,
		profile.Id,
	)).First(&bindAudit).Error)

	// per-channel view: both in sync, key never returned.
	ctx, recorder = newCredentialProfileRequest(t, http.MethodGet, "/api/channel/credential-profiles/1/channels", "")
	setCredentialProfilePathParams(ctx, profile.Id)
	GetCredentialProfileChannels(ctx)
	success, items := decodeCredentialProfileArrayResponse(t, recorder)
	require.True(t, success)
	require.Len(t, items, 2)
	for _, raw := range items {
		item := raw.(map[string]any)
		assert.Equal(t, true, item["in_sync"])
		assert.NotContains(t, item, "key")
	}
	assert.NotContains(t, recorder.Body.String(), "profile-key")

	// unbinding the second channel leaves its key/base_url untouched.
	ctx, recorder = newCredentialProfileRequest(t, http.MethodPut, "/api/channel/credential-profiles/1/channels", fmt.Sprintf(`{"channel_ids":[%d]}`, first.Id))
	setCredentialProfilePathParams(ctx, profile.Id)
	SetCredentialProfileChannels(ctx)
	success, data = decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)
	assert.Equal(t, float64(0), data["total"])
	assert.Equal(t, float64(0), data["succeeded"])
	assert.Equal(t, float64(0), data["failed"])
	assert.Equal(t, float64(1), data["removed"])

	unbound := reloadChannelByID(t, second.Id)
	assert.Nil(t, unbound.CredentialProfileId)
	assert.Equal(t, "profile-key", unbound.Key)
	require.NotNil(t, unbound.BaseURL)
	assert.Equal(t, "https://profile.example", *unbound.BaseURL)
}

func TestCredentialProfileBindReportsMissingChannel(t *testing.T) {
	setupCredentialProfileTestDB(t)
	profile := &model.ChannelCredentialProfile{Name: "shared", Key: "k"}
	require.NoError(t, model.DB.Create(profile).Error)

	ctx, recorder := newCredentialProfileRequest(t, http.MethodPut, "/api/channel/credential-profiles/1/channels", `{"channel_ids":[999]}`)
	setCredentialProfilePathParams(ctx, profile.Id)
	SetCredentialProfileChannels(ctx)
	success, data := decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)
	assert.Equal(t, float64(1), data["total"])
	assert.Equal(t, float64(0), data["succeeded"])
	assert.Equal(t, float64(1), data["failed"])
	// total must equal succeeded + failed.
	assert.Equal(t, data["total"], data["succeeded"].(float64)+data["failed"].(float64))
	results, ok := data["results"].([]any)
	require.True(t, ok)
	require.Len(t, results, 1)
	assert.Equal(t, "channel not found", results[0].(map[string]any)["error"])
}

func TestCredentialProfileApplyRequiresExplicitDryRun(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing body", body: ""},
		{name: "empty object", body: `{}`},
		{name: "null dry run", body: `{"dry_run":null}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupCredentialProfileTestDB(t)
			profile := &model.ChannelCredentialProfile{Name: "shared", Key: "new-profile-key"}
			require.NoError(t, db.Create(profile).Error)
			channel := &model.Channel{
				Name:                "a",
				Key:                 "old-key",
				Models:              "gpt-4o",
				Group:               "default",
				CredentialProfileId: common.GetPointer(profile.Id),
			}
			require.NoError(t, db.Create(channel).Error)

			ctx, recorder := newCredentialProfileRequest(t, http.MethodPost, "/api/channel/credential-profiles/1/apply", test.body)
			setCredentialProfilePathParams(ctx, profile.Id)
			ApplyCredentialProfile(ctx)

			success, _ := decodeCredentialProfileResponse(t, recorder)
			assert.False(t, success)
			assert.Equal(t, "old-key", reloadChannelByID(t, channel.Id).Key)
		})
	}
}

func TestCredentialProfileApplyDryRunAndApply(t *testing.T) {
	db := setupCredentialProfileTestDB(t)
	profile := &model.ChannelCredentialProfile{Name: "shared", Key: "new-profile-key", BaseURL: common.GetPointer("https://new.example")}
	require.NoError(t, db.Create(profile).Error)
	channel := &model.Channel{Name: "a", Key: "old-key", BaseURL: common.GetPointer("https://old.example"), Models: "gpt-4o", Group: "default", CredentialProfileId: common.GetPointer(profile.Id)}
	require.NoError(t, db.Create(channel).Error)

	// dry run previews without writing and without leaking the key.
	ctx, recorder := newCredentialProfileRequest(t, http.MethodPost, "/api/channel/credential-profiles/1/apply", `{"dry_run":true}`)
	setCredentialProfilePathParams(ctx, profile.Id)
	ApplyCredentialProfile(ctx)
	success, data := decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)
	assert.Equal(t, true, data["dry_run"])
	assert.Equal(t, float64(1), data["succeeded"])
	reloaded := reloadChannelByID(t, channel.Id)
	assert.Equal(t, "old-key", reloaded.Key)
	assert.NotContains(t, recorder.Body.String(), "new-profile-key")

	// real apply materializes key/base_url only when explicitly requested.
	ctx, recorder = newCredentialProfileRequest(t, http.MethodPost, "/api/channel/credential-profiles/1/apply", `{"dry_run":false}`)
	setCredentialProfilePathParams(ctx, profile.Id)
	ApplyCredentialProfile(ctx)
	success, data = decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)
	assert.Equal(t, false, data["dry_run"])
	assert.Equal(t, float64(1), data["succeeded"])
	reloaded = reloadChannelByID(t, channel.Id)
	assert.Equal(t, "new-profile-key", reloaded.Key)
	require.NotNil(t, reloaded.BaseURL)
	assert.Equal(t, "https://new.example", *reloaded.BaseURL)
	assert.NotContains(t, recorder.Body.String(), "new-profile-key")

	// list reflects in-sync state.
	ctx, recorder = newCredentialProfileRequest(t, http.MethodGet, "/api/channel/credential-profiles", "")
	GetCredentialProfiles(ctx)
	success, items := decodeCredentialProfileArrayResponse(t, recorder)
	require.True(t, success)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	assert.Equal(t, float64(1), item["bound_count"])
	assert.Equal(t, float64(0), item["out_of_sync_count"])
}

func TestCredentialProfileOutOfSyncDetection(t *testing.T) {
	db := setupCredentialProfileTestDB(t)
	profile := &model.ChannelCredentialProfile{Name: "shared", Key: "profile-key", BaseURL: common.GetPointer("https://p.example")}
	require.NoError(t, db.Create(profile).Error)
	channel := &model.Channel{Name: "a", Key: "profile-key", BaseURL: common.GetPointer("https://p.example"), Models: "gpt-4o", Group: "default", CredentialProfileId: common.GetPointer(profile.Id)}
	require.NoError(t, db.Create(channel).Error)

	// A direct channel edit drifts away from the profile.
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("key", "drifted-key").Error)

	ctx, recorder := newCredentialProfileRequest(t, http.MethodGet, "/api/channel/credential-profiles", "")
	GetCredentialProfiles(ctx)
	success, items := decodeCredentialProfileArrayResponse(t, recorder)
	require.True(t, success)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	assert.Equal(t, float64(1), item["bound_count"])
	assert.Equal(t, float64(1), item["out_of_sync_count"])
	assert.NotContains(t, recorder.Body.String(), "profile-key")

	ctx, recorder = newCredentialProfileRequest(t, http.MethodGet, "/api/channel/credential-profiles/1/channels", "")
	setCredentialProfilePathParams(ctx, profile.Id)
	GetCredentialProfileChannels(ctx)
	success, items = decodeCredentialProfileArrayResponse(t, recorder)
	require.True(t, success)
	require.Len(t, items, 1)
	assert.Equal(t, false, items[0].(map[string]any)["in_sync"])
}

func TestCredentialProfileApplyMultiKeyReplace(t *testing.T) {
	db := setupCredentialProfileTestDB(t)
	profile := &model.ChannelCredentialProfile{Name: "shared", Key: "single-profile-key"}
	require.NoError(t, db.Create(profile).Error)
	channel := &model.Channel{
		Name:   "a",
		Key:    "k1\nk2",
		Models: "gpt-4o",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeySize:       2,
			MultiKeyStatusList: map[int]int{0: 1, 1: 2, 2: 1},
		},
		CredentialProfileId: common.GetPointer(profile.Id),
	}
	require.NoError(t, db.Create(channel).Error)

	ctx, recorder := newCredentialProfileRequest(t, http.MethodPost, "/api/channel/credential-profiles/1/apply", `{"dry_run":false}`)
	setCredentialProfilePathParams(ctx, profile.Id)
	ApplyCredentialProfile(ctx)
	success, _ := decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)

	reloaded := reloadChannelByID(t, channel.Id)
	assert.Equal(t, "single-profile-key", reloaded.Key)
	assert.Equal(t, 1, reloaded.ChannelInfo.MultiKeySize)
	assert.NotContains(t, reloaded.ChannelInfo.MultiKeyStatusList, 2)
}

func TestCredentialProfileApplyPartialFailure(t *testing.T) {
	db := setupCredentialProfileTestDB(t)
	profile := &model.ChannelCredentialProfile{Name: "shared", Key: "sk-plain"}
	require.NoError(t, db.Create(profile).Error)
	good := &model.Channel{Name: "good", Key: "old", Models: "gpt-4o", Group: "default", CredentialProfileId: common.GetPointer(profile.Id)}
	bad := &model.Channel{Name: "bad", Key: "old", Type: constant.ChannelTypeCodex, Models: "gpt-4o", Group: "default", CredentialProfileId: common.GetPointer(profile.Id)}
	require.NoError(t, db.Create(good).Error)
	require.NoError(t, db.Create(bad).Error)

	ctx, recorder := newCredentialProfileRequest(t, http.MethodPost, "/api/channel/credential-profiles/1/apply", `{"dry_run":false}`)
	setCredentialProfilePathParams(ctx, profile.Id)
	ApplyCredentialProfile(ctx)
	success, data := decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)
	assert.Equal(t, float64(1), data["succeeded"])
	assert.Equal(t, float64(1), data["failed"])

	reloadedGood := reloadChannelByID(t, good.Id)
	assert.Equal(t, "sk-plain", reloadedGood.Key)
	reloadedBad := reloadChannelByID(t, bad.Id)
	assert.Equal(t, "old", reloadedBad.Key)
	assert.NotContains(t, recorder.Body.String(), "sk-plain")
}

func TestCredentialProfileAuditDoesNotContainKey(t *testing.T) {
	db := setupCredentialProfileTestDB(t)
	profile := &model.ChannelCredentialProfile{Name: "shared", Key: "sk-audit-secret"}
	require.NoError(t, db.Create(profile).Error)
	channel := &model.Channel{Name: "a", Key: "old", Models: "gpt-4o", Group: "default", CredentialProfileId: common.GetPointer(profile.Id)}
	require.NoError(t, db.Create(channel).Error)

	ctx, recorder := newCredentialProfileRequest(t, http.MethodPost, "/api/channel/credential-profiles/1/apply", `{"dry_run":false}`)
	setCredentialProfilePathParams(ctx, profile.Id)
	ApplyCredentialProfile(ctx)
	success, _ := decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)

	auditLog, action, params := latestAuditLog(t, db)
	assert.Equal(t, "channel.credential_profile_apply", action)
	assert.Equal(t, "shared", params["name"])
	assert.Equal(t, float64(1), params["succeeded"])
	assert.NotContains(t, auditLog.Other, "sk-audit-secret")
	assert.NotContains(t, recorder.Body.String(), "sk-audit-secret")
}

// TestCredentialProfileBaseURLNotManagedStaysInSync covers the P1 semantics:
// when a profile does not manage base_url (BaseURL nil), a bound channel is in
// sync by key alone even if it carries its own base_url.
func TestCredentialProfileBaseURLNotManagedStaysInSync(t *testing.T) {
	db := setupCredentialProfileTestDB(t)

	// Profile without base_url; the bound channel keeps its own base_url.
	profile := &model.ChannelCredentialProfile{Name: "shared", Key: "profile-key"}
	require.NoError(t, db.Create(profile).Error)
	channel := &model.Channel{Name: "a", Key: "profile-key", BaseURL: common.GetPointer("https://own.example"), Models: "gpt-4o", Group: "default", CredentialProfileId: common.GetPointer(profile.Id)}
	require.NoError(t, db.Create(channel).Error)

	ctx, recorder := newCredentialProfileRequest(t, http.MethodGet, "/api/channel/credential-profiles", "")
	GetCredentialProfiles(ctx)
	success, items := decodeCredentialProfileArrayResponse(t, recorder)
	require.True(t, success)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	assert.Equal(t, float64(1), item["bound_count"])
	assert.Equal(t, float64(0), item["out_of_sync_count"])

	ctx, recorder = newCredentialProfileRequest(t, http.MethodGet, "/api/channel/credential-profiles/1/channels", "")
	setCredentialProfilePathParams(ctx, profile.Id)
	GetCredentialProfileChannels(ctx)
	success, items = decodeCredentialProfileArrayResponse(t, recorder)
	require.True(t, success)
	require.Len(t, items, 1)
	assert.Equal(t, true, items[0].(map[string]any)["in_sync"])

	// A profile that manages base_url still flags a channel with a different
	// (or missing) base_url as out of sync.
	managed := &model.ChannelCredentialProfile{Name: "managed", Key: "profile-key", BaseURL: common.GetPointer("https://p.example")}
	require.NoError(t, db.Create(managed).Error)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("credential_profile_id", managed.Id).Error)

	ctx, recorder = newCredentialProfileRequest(t, http.MethodGet, "/api/channel/credential-profiles/2/channels", "")
	setCredentialProfilePathParams(ctx, managed.Id)
	GetCredentialProfileChannels(ctx)
	success, items = decodeCredentialProfileArrayResponse(t, recorder)
	require.True(t, success)
	require.Len(t, items, 1)
	assert.Equal(t, false, items[0].(map[string]any)["in_sync"])
}

// TestCredentialProfileBindConflictRejected covers the P2 no-silent-steal rule:
// binding a channel that already references another profile fails the whole
// request and changes no bindings.
func TestCredentialProfileBindConflictRejected(t *testing.T) {
	db := setupCredentialProfileTestDB(t)
	first := &model.ChannelCredentialProfile{Name: "first", Key: "k1"}
	second := &model.ChannelCredentialProfile{Name: "second", Key: "k2"}
	require.NoError(t, db.Create(first).Error)
	require.NoError(t, db.Create(second).Error)
	channel := &model.Channel{Name: "a", Key: "old", Models: "gpt-4o", Group: "default", CredentialProfileId: common.GetPointer(first.Id)}
	require.NoError(t, db.Create(channel).Error)

	// Trying to bind the channel to the second profile must fail outright.
	ctx, recorder := newCredentialProfileRequest(t, http.MethodPut, "/api/channel/credential-profiles/2/channels", fmt.Sprintf(`{"channel_ids":[%d]}`, channel.Id))
	setCredentialProfilePathParams(ctx, second.Id)
	SetCredentialProfileChannels(ctx)
	success, _ := decodeCredentialProfileResponse(t, recorder)
	assert.False(t, success)
	assert.Contains(t, recorder.Body.String(), "已绑定其他凭据配置")
	assert.Contains(t, recorder.Body.String(), strconv.Itoa(channel.Id))

	// No binding changed: still bound to the first profile.
	reloaded := reloadChannelByID(t, channel.Id)
	require.NotNil(t, reloaded.CredentialProfileId)
	assert.Equal(t, first.Id, *reloaded.CredentialProfileId)
	var count int64
	require.NoError(t, db.Model(&model.Channel{}).Where("credential_profile_id = ?", second.Id).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

// TestAddChannelDoesNotPersistCredentialProfileId covers the P2 rule that a
// channel created through the normal create endpoint never carries a profile
// reference, even if the client sends one.
func TestAddChannelDoesNotPersistCredentialProfileId(t *testing.T) {
	db := setupCredentialProfileTestDB(t)

	body := `{"mode":"single","channel":{"name":"a","key":"k","models":"gpt-4o","group":"default","credential_profile_id":123}}`
	ctx, recorder := newCredentialProfileRequest(t, http.MethodPost, "/api/channel", body)
	AddChannel(ctx)
	success, _ := decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)

	var created model.Channel
	require.NoError(t, db.Where("name = ?", "a").First(&created).Error)
	assert.Nil(t, created.CredentialProfileId)
}

// TestCopyChannelDoesNotInheritCredentialProfile covers the P2 rule that a
// copied channel starts unbound: the reference is not inherited.
func TestCopyChannelDoesNotInheritCredentialProfile(t *testing.T) {
	db := setupCredentialProfileTestDB(t)
	profile := &model.ChannelCredentialProfile{Name: "shared", Key: "k"}
	require.NoError(t, db.Create(profile).Error)
	src := &model.Channel{Name: "src", Key: "k", Models: "gpt-4o", Group: "default", CredentialProfileId: common.GetPointer(profile.Id)}
	require.NoError(t, db.Create(src).Error)

	ctx, recorder := newCredentialProfileRequest(t, http.MethodPost, fmt.Sprintf("/api/channel/copy/%d", src.Id), "")
	setCredentialProfilePathParams(ctx, src.Id)
	CopyChannel(ctx)
	success, data := decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)
	newID := int(data["id"].(float64))

	var clone model.Channel
	require.NoError(t, db.First(&clone, "id = ?", newID).Error)
	assert.Nil(t, clone.CredentialProfileId)
	assert.Equal(t, "k", clone.Key)
}

// TestCredentialProfileCreateNormalizesEmptyBaseURL covers the review item that
// an empty base_url on create becomes nil (profile does not manage base_url)
// and remark is trimmed.
func TestCredentialProfileCreateNormalizesEmptyBaseURL(t *testing.T) {
	db := setupCredentialProfileTestDB(t)

	ctx, recorder := newCredentialProfileRequest(t, http.MethodPost, "/api/channel/credential-profiles", `{"name":"shared","key":"k","base_url":"   ","remark":"  note  "}`)
	CreateCredentialProfile(ctx)
	success, data := decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)
	assert.Nil(t, data["base_url"])

	var stored model.ChannelCredentialProfile
	require.NoError(t, db.First(&stored, "id = ?", 1).Error)
	assert.Nil(t, stored.BaseURL)
	require.NotNil(t, stored.Remark)
	assert.Equal(t, "note", *stored.Remark)
}

// TestCredentialProfileBindPartialSyncFailureCounts verifies the unified bind
// response counts: total = succeeded + failed even when one channel fails
// validation, and the failed channel stays bound (out of sync).
func TestCredentialProfileBindPartialSyncFailureCounts(t *testing.T) {
	db := setupCredentialProfileTestDB(t)
	profile := &model.ChannelCredentialProfile{Name: "shared", Key: "sk-plain"}
	require.NoError(t, db.Create(profile).Error)
	good := &model.Channel{Name: "good", Key: "old", Models: "gpt-4o", Group: "default"}
	bad := &model.Channel{Name: "bad", Key: "old", Type: constant.ChannelTypeCodex, Models: "gpt-4o", Group: "default"}
	require.NoError(t, db.Create(good).Error)
	require.NoError(t, db.Create(bad).Error)

	body := fmt.Sprintf(`{"channel_ids":[%d,%d]}`, good.Id, bad.Id)
	ctx, recorder := newCredentialProfileRequest(t, http.MethodPut, "/api/channel/credential-profiles/1/channels", body)
	setCredentialProfilePathParams(ctx, profile.Id)
	SetCredentialProfileChannels(ctx)
	success, data := decodeCredentialProfileResponse(t, recorder)
	require.True(t, success)
	assert.Equal(t, float64(2), data["total"])
	assert.Equal(t, float64(1), data["succeeded"])
	assert.Equal(t, float64(1), data["failed"])
	assert.Equal(t, data["total"], data["succeeded"].(float64)+data["failed"].(float64))

	// Both are bound (the failed one is flagged as out of sync, not unbound).
	reloadedGood := reloadChannelByID(t, good.Id)
	require.NotNil(t, reloadedGood.CredentialProfileId)
	assert.Equal(t, "sk-plain", reloadedGood.Key)
	reloadedBad := reloadChannelByID(t, bad.Id)
	require.NotNil(t, reloadedBad.CredentialProfileId)
	assert.Equal(t, "old", reloadedBad.Key)
}
