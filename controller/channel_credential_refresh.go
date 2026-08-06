package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
)

// credentialRefreshMaxChannels caps how many channels a single credential
// refresh may touch. The operation rewrites secrets across many channels, so
// the batch stays bounded even though it is admin-only and audited. It is a
// var (not a const) so tests can exercise the cap cheaply.
var credentialRefreshMaxChannels = 500

const (
	credentialKeyModeReplace = "replace"
	credentialKeyModeAppend  = "append"
)

// credentialRefreshRequest is the payload for RefreshChannelCredentials. Key
// and BaseURL are pointers so the handler can distinguish "field absent" from
// "field explicitly present but empty": an explicit empty key is rejected,
// while an explicit empty base_url clears the channel base URL.
type credentialRefreshRequest struct {
	Tag     string  `json:"tag"`
	Key     *string `json:"key"`
	BaseURL *string `json:"base_url"`
	KeyMode string  `json:"key_mode"`
	DryRun  bool    `json:"dry_run"`
}

type credentialRefreshResult struct {
	ChannelID  int    `json:"channel_id"`
	Name       string `json:"name"`
	Type       int    `json:"type"`
	BaseURL    string `json:"base_url"`
	IsMultiKey bool   `json:"is_multi_key"`
	Success    bool   `json:"success"`
	Error      string `json:"error"`
}

// mergeChannelKeys computes the key string to persist for a channel edit under
// the given key mode. It mirrors the multi-key append/replace semantics of the
// single-channel edit path (UpdateChannel): append only merges for multi-key
// channels (deduplicating new keys against the existing list), and single-key
// channels always replace. Vertex AI non-API-key channels parse appended keys
// as a JSON array.
func mergeChannelKeys(existingKey, newKey string, isMultiKey bool, keyMode string, channelType int, vertexKeyType dto.VertexKeyType) (string, error) {
	if !isMultiKey || keyMode != credentialKeyModeAppend || existingKey == "" {
		return newKey, nil
	}

	var existingKeys []string
	if strings.HasPrefix(strings.TrimSpace(existingKey), "[") {
		var arr []json.RawMessage
		if err := common.Unmarshal([]byte(strings.TrimSpace(existingKey)), &arr); err == nil {
			existingKeys = make([]string, len(arr))
			for i, v := range arr {
				existingKeys[i] = string(v)
			}
		}
	} else {
		existingKeys = strings.Split(strings.Trim(existingKey, "\n"), "\n")
	}

	var newKeys []string
	if channelType == constant.ChannelTypeVertexAi && vertexKeyType != dto.VertexKeyTypeAPIKey {
		if strings.HasPrefix(strings.TrimSpace(newKey), "[") {
			array, err := getVertexArrayKeys(newKey)
			if err != nil {
				return "", err
			}
			newKeys = array
		} else {
			newKeys = []string{newKey}
		}
	} else {
		for _, key := range strings.Split(newKey, "\n") {
			if key = strings.TrimSpace(key); key != "" {
				newKeys = append(newKeys, key)
			}
		}
	}

	seen := make(map[string]struct{}, len(existingKeys)+len(newKeys))
	for _, key := range existingKeys {
		if key = strings.TrimSpace(key); key != "" {
			seen[key] = struct{}{}
		}
	}
	dedupedNewKeys := make([]string, 0, len(newKeys))
	for _, key := range newKeys {
		if key = strings.TrimSpace(key); key != "" {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			dedupedNewKeys = append(dedupedNewKeys, key)
		}
	}
	return strings.Join(append(existingKeys, dedupedNewKeys...), "\n"), nil
}

// RefreshChannelCredentials batch-refreshes key/base_url of every channel
// carrying the given tag. dry_run and apply share the same per-channel
// validation and key merge logic; dry_run never writes. Each channel is
// validated and updated independently so one failing channel does not block the
// rest. The response and audit trail never contain key material.
func RefreshChannelCredentials(c *gin.Context) {
	rawBody, err := c.GetRawData()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	req := credentialRefreshRequest{}
	if err := common.Unmarshal(rawBody, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	var requestData map[string]any
	if err := common.Unmarshal(rawBody, &requestData); err != nil {
		common.ApiError(c, err)
		return
	}

	tag := strings.TrimSpace(req.Tag)
	if tag == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	_, keyFieldPresent := requestData["key"]
	_, baseURLFieldPresent := requestData["base_url"]
	// An explicitly present key must be non-empty; key:null is rejected too.
	if keyFieldPresent && (req.Key == nil || strings.TrimSpace(*req.Key) == "") {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	// base_url:null is treated as "not provided"; only a non-null string
	// counts as an effective field, so {tag, base_url:null} is rejected.
	keyProvided := req.Key != nil
	baseURLProvided := baseURLFieldPresent && req.BaseURL != nil
	if !keyProvided && !baseURLProvided {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	keyMode := strings.TrimSpace(req.KeyMode)
	if keyMode == "" {
		keyMode = credentialKeyModeReplace
	}
	if keyMode != credentialKeyModeReplace && keyMode != credentialKeyModeAppend {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	channels, err := model.GetChannelsByTag(tag, true, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(channels) == 0 {
		common.ApiError(c, fmt.Errorf("no channels found with tag: %s", tag))
		return
	}
	if len(channels) > credentialRefreshMaxChannels {
		common.ApiError(c, fmt.Errorf("too many channels (%d) for tag: %s, max %d", len(channels), tag, credentialRefreshMaxChannels))
		return
	}

	newKey := ""
	if keyProvided {
		newKey = *req.Key
	}
	// An explicit null base_url is treated as "not provided"; only an explicit
	// empty string clears the channel base URL.
	var newBaseURL *string
	if baseURLProvided {
		newBaseURL = req.BaseURL
	}

	results := make([]credentialRefreshResult, 0, len(channels))
	keyChanged, baseURLChanged := false, false
	succeeded, failed := 0, 0
	failedChannelIDs := make([]int, 0, len(channels))

	for _, channel := range channels {
		result := credentialRefreshResult{
			ChannelID:  channel.Id,
			Name:       channel.Name,
			Type:       channel.Type,
			IsMultiKey: channel.ChannelInfo.IsMultiKey,
		}
		working := *channel
		if keyProvided {
			mergedKey, mergeErr := mergeChannelKeys(channel.Key, newKey, channel.ChannelInfo.IsMultiKey, keyMode, channel.Type, channel.GetOtherSettings().VertexKeyType)
			if mergeErr != nil {
				failed++
				failedChannelIDs = append(failedChannelIDs, channel.Id)
				result.Error = mergeErr.Error()
				results = append(results, result)
				continue
			}
			working.Key = mergedKey
		}
		if newBaseURL != nil {
			working.BaseURL = newBaseURL
		}
		result.BaseURL = channelBaseURLString(working.BaseURL)
		// The same validation guards dry_run and apply so a dry run faithfully
		// previews which channels would fail.
		if err := validateChannel(&working, false); err != nil {
			failed++
			failedChannelIDs = append(failedChannelIDs, channel.Id)
			result.Error = err.Error()
			results = append(results, result)
			continue
		}
		if req.DryRun {
			// Dry run: a channel that passed validation counts toward
			// changed_fields even though nothing was written.
			if keyProvided && working.Key != channel.Key {
				keyChanged = true
			}
			if newBaseURL != nil && !equalStringPtr(working.BaseURL, channel.BaseURL) {
				baseURLChanged = true
			}
			succeeded++
			result.Success = true
			results = append(results, result)
			continue
		}
		// Update() keeps multi-key invariants (MultiKeySize, per-key status
		// trimming) and refreshes abilities, matching the single-channel edit path.
		if err := working.Update(); err != nil {
			failed++
			failedChannelIDs = append(failedChannelIDs, channel.Id)
			result.Error = err.Error()
			results = append(results, result)
			continue
		}
		// Apply: changed_fields only reflects channels whose Update succeeded.
		if keyProvided && working.Key != channel.Key {
			keyChanged = true
		}
		if newBaseURL != nil && !equalStringPtr(working.BaseURL, channel.BaseURL) {
			baseURLChanged = true
		}
		succeeded++
		result.Success = true
		results = append(results, result)
	}

	if !req.DryRun && succeeded > 0 {
		// Refresh the relay cache exactly once after all writes. The refresh
		// never touches a channel's proxy setting, so no proxy client
		// invalidation is needed here.
		model.InitChannelCache()
	}

	changedFields := make([]string, 0, 2)
	if keyChanged {
		changedFields = append(changedFields, "key")
	}
	if baseURLChanged {
		changedFields = append(changedFields, "base_url")
	}
	// Audit the tag, changed fields and counts only; key/base_url content and
	// per-channel error text are never logged. Dry runs use a distinct action
	// so the audit trail does not claim credentials were already refreshed.
	auditAction := "channel.credential_refresh"
	if req.DryRun {
		auditAction = "channel.credential_refresh_preview"
	}
	recordManageAudit(c, auditAction, map[string]interface{}{
		"tag":                tag,
		"dry_run":            req.DryRun,
		"changed_fields":     changedFields,
		"total":              len(channels),
		"succeeded":          succeeded,
		"failed":             failed,
		"failed_channel_ids": failedChannelIDs,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"dry_run":   req.DryRun,
			"total":     len(channels),
			"succeeded": succeeded,
			"failed":    failed,
			"results":   results,
		},
	})
}

func channelBaseURLString(baseURL *string) string {
	if baseURL == nil {
		return ""
	}
	return *baseURL
}
