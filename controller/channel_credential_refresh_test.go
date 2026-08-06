package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newCredentialRefreshContext(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/credential-refresh", strings.NewReader(body))
	ctx.Set("id", 1)
	ctx.Set("role", common.RoleRootUser)
	ctx.Set("username", "tester")
	return ctx, recorder
}

func decodeCredentialRefreshResponse(t *testing.T, recorder *httptest.ResponseRecorder) (bool, map[string]any) {
	t.Helper()
	var resp struct {
		Success bool           `json:"success"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	return resp.Success, resp.Data
}

func reloadChannelByID(t *testing.T, id int) *model.Channel {
	t.Helper()
	channel, err := model.GetChannelById(id, true)
	require.NoError(t, err)
	return channel
}

func latestAuditLog(t *testing.T, db *gorm.DB) (model.Log, string, map[string]any) {
	t.Helper()
	var auditLog model.Log
	require.NoError(t, db.Order("id desc").First(&auditLog).Error)
	var auditData struct {
		Operation struct {
			Action string         `json:"action"`
			Params map[string]any `json:"params"`
		} `json:"op"`
	}
	require.NoError(t, common.UnmarshalJsonStr(auditLog.Other, &auditData))
	return auditLog, auditData.Operation.Action, auditData.Operation.Params
}

func TestMergeChannelKeys(t *testing.T) {
	openAI := constant.ChannelTypeOpenAI
	tests := []struct {
		name          string
		existingKey   string
		newKey        string
		isMultiKey    bool
		keyMode       string
		channelType   int
		vertexKeyType dto.VertexKeyType
		want          string
		wantErr       bool
	}{
		{
			name:        "single key append behaves as replace",
			existingKey: "old-key",
			newKey:      "new-key",
			isMultiKey:  false,
			keyMode:     credentialKeyModeAppend,
			channelType: openAI,
			want:        "new-key",
		},
		{
			name:        "multi key replace",
			existingKey: "k1\nk2",
			newKey:      "k9",
			isMultiKey:  true,
			keyMode:     credentialKeyModeReplace,
			channelType: openAI,
			want:        "k9",
		},
		{
			name:        "multi key append dedupes new keys",
			existingKey: "k1\nk2",
			newKey:      "k2\nk3",
			isMultiKey:  true,
			keyMode:     credentialKeyModeAppend,
			channelType: openAI,
			want:        "k1\nk2\nk3",
		},
		{
			name:        "multi key append with empty existing key",
			existingKey: "",
			newKey:      "k9",
			isMultiKey:  true,
			keyMode:     credentialKeyModeAppend,
			channelType: openAI,
			want:        "k9",
		},
		{
			name:          "vertex append parses JSON array keys",
			existingKey:   "v1",
			newKey:        `[{"key":"a"},{"key":"b"}]`,
			isMultiKey:    true,
			keyMode:       credentialKeyModeAppend,
			channelType:   constant.ChannelTypeVertexAi,
			vertexKeyType: dto.VertexKeyTypeJSON,
			want:          "v1\n{\"key\":\"a\"}\n{\"key\":\"b\"}",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := mergeChannelKeys(test.existingKey, test.newKey, test.isMultiKey, test.keyMode, test.channelType, test.vertexKeyType)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestRefreshChannelCredentialsRequestValidation(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	tests := []struct {
		name    string
		body    string
		message string
	}{
		{name: "empty body", body: `{}`},
		{name: "empty tag", body: `{"tag":"","key":"k"}`},
		{name: "missing tag", body: `{"key":"k"}`},
		{name: "neither key nor base_url", body: `{"tag":"shared"}`},
		{name: "explicit empty key", body: `{"tag":"shared","key":""}`},
		{name: "explicit null key", body: `{"tag":"shared","key":null}`},
		{name: "null base_url alone is not an effective field", body: `{"tag":"shared","base_url":null}`},
		{name: "invalid key_mode", body: `{"tag":"shared","key":"k","key_mode":"overwrite"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, recorder := newCredentialRefreshContext(t, test.body)
			RefreshChannelCredentials(ctx)

			success, _ := decodeCredentialRefreshResponse(t, recorder)
			assert.False(t, success)
			if test.message != "" {
				assert.Contains(t, recorder.Body.String(), test.message)
			}
		})
	}
}

func TestRefreshChannelCredentialsDryRunDoesNotWrite(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	first := &model.Channel{Name: "a", Key: "old-key", BaseURL: common.GetPointer("https://old.example"), Tag: common.GetPointer("shared"), Models: "gpt-4o", Group: "default"}
	second := &model.Channel{Name: "b", Key: "old-key", BaseURL: common.GetPointer("https://old.example"), Tag: common.GetPointer("shared"), Models: "gpt-4o", Group: "default"}
	require.NoError(t, db.Create(first).Error)
	require.NoError(t, db.Create(second).Error)

	ctx, recorder := newCredentialRefreshContext(t, `{"tag":"shared","key":"new-key","base_url":"https://new.example","dry_run":true}`)
	RefreshChannelCredentials(ctx)

	success, data := decodeCredentialRefreshResponse(t, recorder)
	require.True(t, success)
	assert.Equal(t, true, data["dry_run"])
	assert.Equal(t, float64(2), data["total"])
	assert.Equal(t, float64(2), data["succeeded"])
	assert.Equal(t, float64(0), data["failed"])

	for _, id := range []int{first.Id, second.Id} {
		channel := reloadChannelByID(t, id)
		assert.Equal(t, "old-key", channel.Key)
		assert.NotNil(t, channel.BaseURL)
		assert.Equal(t, "https://old.example", *channel.BaseURL)
	}
	// The new key must never leak into the response, even for a dry run.
	assert.NotContains(t, recorder.Body.String(), "new-key")
}

func TestRefreshChannelCredentialsApply(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	first := &model.Channel{Name: "a", Key: "old-key", BaseURL: common.GetPointer("https://old.example"), Tag: common.GetPointer("shared"), Models: "gpt-4o", Group: "default"}
	second := &model.Channel{Name: "b", Key: "old-key", BaseURL: common.GetPointer("https://old.example"), Tag: common.GetPointer("shared"), Models: "gpt-4o", Group: "default"}
	require.NoError(t, db.Create(first).Error)
	require.NoError(t, db.Create(second).Error)

	ctx, recorder := newCredentialRefreshContext(t, `{"tag":"shared","key":"new-key","base_url":"https://new.example"}`)
	RefreshChannelCredentials(ctx)

	success, data := decodeCredentialRefreshResponse(t, recorder)
	require.True(t, success)
	assert.Equal(t, false, data["dry_run"])
	assert.Equal(t, float64(2), data["total"])
	assert.Equal(t, float64(2), data["succeeded"])
	assert.Equal(t, float64(0), data["failed"])

	results, ok := data["results"].([]any)
	require.True(t, ok)
	require.Len(t, results, 2)
	for _, raw := range results {
		result, ok := raw.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, true, result["success"])
		// The result shape carries routing info but never the key.
		assert.NotContains(t, result, "key")
	}

	for _, id := range []int{first.Id, second.Id} {
		channel := reloadChannelByID(t, id)
		assert.Equal(t, "new-key", channel.Key)
		assert.NotNil(t, channel.BaseURL)
		assert.Equal(t, "https://new.example", *channel.BaseURL)
	}
	assert.NotContains(t, recorder.Body.String(), "new-key")
}

func TestRefreshChannelCredentialsBaseURLClear(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	channel := &model.Channel{Name: "a", Key: "keep-key", BaseURL: common.GetPointer("https://old.example"), Tag: common.GetPointer("shared"), Models: "gpt-4o", Group: "default"}
	require.NoError(t, db.Create(channel).Error)

	// key is absent, base_url is explicitly empty -> clear base_url, keep key.
	ctx, recorder := newCredentialRefreshContext(t, `{"tag":"shared","base_url":""}`)
	RefreshChannelCredentials(ctx)

	success, data := decodeCredentialRefreshResponse(t, recorder)
	require.True(t, success)
	assert.Equal(t, float64(1), data["succeeded"])

	reloaded := reloadChannelByID(t, channel.Id)
	assert.Equal(t, "keep-key", reloaded.Key)
	assert.NotNil(t, reloaded.BaseURL)
	assert.Equal(t, "", *reloaded.BaseURL)
}

func TestRefreshChannelCredentialsNullBaseURLIgnored(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	channel := &model.Channel{Name: "a", Key: "old-key", BaseURL: common.GetPointer("https://old.example"), Tag: common.GetPointer("shared"), Models: "gpt-4o", Group: "default"}
	require.NoError(t, db.Create(channel).Error)

	// base_url:null is treated as "not provided": the key is refreshed and the
	// base URL is left untouched.
	ctx, recorder := newCredentialRefreshContext(t, `{"tag":"shared","key":"new-key","base_url":null}`)
	RefreshChannelCredentials(ctx)

	success, data := decodeCredentialRefreshResponse(t, recorder)
	require.True(t, success)
	assert.Equal(t, float64(1), data["succeeded"])

	reloaded := reloadChannelByID(t, channel.Id)
	assert.Equal(t, "new-key", reloaded.Key)
	assert.NotNil(t, reloaded.BaseURL)
	assert.Equal(t, "https://old.example", *reloaded.BaseURL)
}

func TestRefreshChannelCredentialsKeyModes(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	t.Run("multi key replace trims key list and status", func(t *testing.T) {
		channel := &model.Channel{
			Name:   "multi",
			Key:    "k1\nk2",
			Tag:    common.GetPointer("shared"),
			Models: "gpt-4o",
			Group:  "default",
			ChannelInfo: model.ChannelInfo{
				IsMultiKey:         true,
				MultiKeySize:       2,
				MultiKeyMode:       constant.MultiKeyModeRandom,
				MultiKeyStatusList: map[int]int{0: 1, 1: 1, 2: 1},
			},
		}
		require.NoError(t, db.Create(channel).Error)

		ctx, recorder := newCredentialRefreshContext(t, `{"tag":"shared","key":"k9"}`)
		RefreshChannelCredentials(ctx)

		success, _ := decodeCredentialRefreshResponse(t, recorder)
		require.True(t, success)

		reloaded := reloadChannelByID(t, channel.Id)
		assert.Equal(t, "k9", reloaded.Key)
		assert.True(t, reloaded.ChannelInfo.IsMultiKey)
		assert.Equal(t, 1, reloaded.ChannelInfo.MultiKeySize)
		assert.Equal(t, map[int]int{0: 1}, reloaded.ChannelInfo.MultiKeyStatusList)
	})

	t.Run("multi key append merges and dedupes", func(t *testing.T) {
		channel := &model.Channel{
			Name:   "multi",
			Key:    "k1\nk2",
			Tag:    common.GetPointer("shared"),
			Models: "gpt-4o",
			Group:  "default",
			ChannelInfo: model.ChannelInfo{
				IsMultiKey:   true,
				MultiKeySize: 2,
				MultiKeyMode: constant.MultiKeyModeRandom,
			},
		}
		require.NoError(t, db.Create(channel).Error)

		ctx, recorder := newCredentialRefreshContext(t, `{"tag":"shared","key":"k2\nk3","key_mode":"append"}`)
		RefreshChannelCredentials(ctx)

		success, _ := decodeCredentialRefreshResponse(t, recorder)
		require.True(t, success)

		reloaded := reloadChannelByID(t, channel.Id)
		assert.Equal(t, "k1\nk2\nk3", reloaded.Key)
		assert.Equal(t, 3, reloaded.ChannelInfo.MultiKeySize)
	})

	t.Run("single key append replaces", func(t *testing.T) {
		channel := &model.Channel{Name: "single", Key: "old-key", Tag: common.GetPointer("shared"), Models: "gpt-4o", Group: "default"}
		require.NoError(t, db.Create(channel).Error)

		ctx, recorder := newCredentialRefreshContext(t, `{"tag":"shared","key":"k9","key_mode":"append"}`)
		RefreshChannelCredentials(ctx)

		success, _ := decodeCredentialRefreshResponse(t, recorder)
		require.True(t, success)

		reloaded := reloadChannelByID(t, channel.Id)
		assert.Equal(t, "k9", reloaded.Key)
	})
}

func TestRefreshChannelCredentialsPartialFailure(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	openAI := &model.Channel{Name: "openai", Type: constant.ChannelTypeOpenAI, Key: "ok", Tag: common.GetPointer("mixed"), Models: "gpt-4o", Group: "default"}
	codex := &model.Channel{Name: "codex", Type: constant.ChannelTypeCodex, Key: `{"access_token":"at","account_id":"ac"}`, Tag: common.GetPointer("mixed"), Models: "gpt-5-codex", Group: "default"}
	require.NoError(t, db.Create(openAI).Error)
	require.NoError(t, db.Create(codex).Error)

	// A non-JSON key is valid for the OpenAI channel but fails Codex validation.
	ctx, recorder := newCredentialRefreshContext(t, `{"tag":"mixed","key":"not-a-json-key"}`)
	RefreshChannelCredentials(ctx)

	success, data := decodeCredentialRefreshResponse(t, recorder)
	require.True(t, success)
	assert.Equal(t, float64(2), data["total"])
	assert.Equal(t, float64(1), data["succeeded"])
	assert.Equal(t, float64(1), data["failed"])

	results, ok := data["results"].([]any)
	require.True(t, ok)
	require.Len(t, results, 2)
	var openAIResult, codexResult map[string]any
	for _, raw := range results {
		result, ok := raw.(map[string]any)
		require.True(t, ok)
		switch result["channel_id"] {
		case float64(openAI.Id):
			openAIResult = result
		case float64(codex.Id):
			codexResult = result
		}
	}
	require.NotNil(t, openAIResult)
	require.NotNil(t, codexResult)
	assert.Equal(t, true, openAIResult["success"])
	assert.Equal(t, false, codexResult["success"])
	assert.Contains(t, codexResult["error"], "Codex key must be a valid JSON object")

	assert.Equal(t, "not-a-json-key", reloadChannelByID(t, openAI.Id).Key)
	assert.Equal(t, `{"access_token":"at","account_id":"ac"}`, reloadChannelByID(t, codex.Id).Key)
	assert.NotContains(t, recorder.Body.String(), "not-a-json-key")

	// The audit trail records the tag, counts and failing channel IDs only:
	// no error text, key material or base URL may leak into the log.
	auditLog, action, params := latestAuditLog(t, db)
	assert.Equal(t, "channel.credential_refresh", action)
	assert.Equal(t, "mixed", params["tag"])
	assert.Equal(t, []any{float64(codex.Id)}, params["failed_channel_ids"])
	assert.Equal(t, float64(1), params["failed"])
	assert.NotContains(t, auditLog.Content, "not-a-json-key")
	assert.NotContains(t, auditLog.Content, "Codex key must be a valid JSON object")
	assert.NotContains(t, auditLog.Other, "not-a-json-key")
	assert.NotContains(t, auditLog.Other, "Codex key must be a valid JSON object")
}

func TestRefreshChannelCredentialsDryRunMatchesApplyValidation(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	openAI := &model.Channel{Name: "openai", Type: constant.ChannelTypeOpenAI, Key: "ok", Tag: common.GetPointer("mixed"), Models: "gpt-4o", Group: "default"}
	codex := &model.Channel{Name: "codex", Type: constant.ChannelTypeCodex, Key: `{"access_token":"at","account_id":"ac"}`, Tag: common.GetPointer("mixed"), Models: "gpt-5-codex", Group: "default"}
	require.NoError(t, db.Create(openAI).Error)
	require.NoError(t, db.Create(codex).Error)

	ctx, recorder := newCredentialRefreshContext(t, `{"tag":"mixed","key":"not-a-json-key","dry_run":true}`)
	RefreshChannelCredentials(ctx)

	success, data := decodeCredentialRefreshResponse(t, recorder)
	require.True(t, success)
	assert.Equal(t, true, data["dry_run"])
	assert.Equal(t, float64(1), data["succeeded"])
	assert.Equal(t, float64(1), data["failed"])

	results, ok := data["results"].([]any)
	require.True(t, ok)
	for _, raw := range results {
		result, ok := raw.(map[string]any)
		require.True(t, ok)
		if result["channel_id"] == float64(codex.Id) {
			assert.Equal(t, false, result["success"])
			assert.Contains(t, result["error"], "Codex key must be a valid JSON object")
		}
	}

	// Dry run must not write anything.
	assert.Equal(t, "ok", reloadChannelByID(t, openAI.Id).Key)
	assert.Equal(t, `{"access_token":"at","account_id":"ac"}`, reloadChannelByID(t, codex.Id).Key)

	// A dry run is audited under its own action and must not claim the
	// credentials were refreshed.
	auditLog, action, params := latestAuditLog(t, db)
	assert.Equal(t, "channel.credential_refresh_preview", action)
	assert.Equal(t, "mixed", params["tag"])
	assert.Equal(t, []any{float64(codex.Id)}, params["failed_channel_ids"])
	assert.NotContains(t, auditLog.Content, "not-a-json-key")
}

func TestRefreshChannelCredentialsNoChannels(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	ctx, recorder := newCredentialRefreshContext(t, `{"tag":"none","key":"k"}`)
	RefreshChannelCredentials(ctx)

	success, _ := decodeCredentialRefreshResponse(t, recorder)
	assert.False(t, success)
	assert.Contains(t, recorder.Body.String(), "no channels found")
}

func TestRefreshChannelCredentialsMaxChannels(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	originalMax := credentialRefreshMaxChannels
	credentialRefreshMaxChannels = 2
	t.Cleanup(func() { credentialRefreshMaxChannels = originalMax })

	for i := 0; i < 3; i++ {
		channel := &model.Channel{Name: "c", Key: "k", Tag: common.GetPointer("many"), Models: "gpt-4o", Group: "default"}
		require.NoError(t, db.Create(channel).Error)
	}

	ctx, recorder := newCredentialRefreshContext(t, `{"tag":"many","key":"new"}`)
	RefreshChannelCredentials(ctx)

	success, _ := decodeCredentialRefreshResponse(t, recorder)
	assert.False(t, success)
	assert.Contains(t, recorder.Body.String(), "too many channels")
}
