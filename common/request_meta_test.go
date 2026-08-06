package common

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaskRequestHeaderValue(t *testing.T) {
	cases := []struct {
		key   string
		value string
		want  string
	}{
		{key: "Authorization", value: "Bearer sk-secret", want: "***"},
		{key: "X-Api-Key", value: "sk-prod-123456", want: "***"},
		{key: "X-Goog-Api-Key", value: "AIzaSyAAAaUooTUni8AdaOkSRMda30n_Q4vrV70", want: "***"},
		{key: "Proxy-Authorization", value: "Basic dXNlcjpwYXNz", want: "***"},
		{key: "X-Auth-Token", value: "tok-abc-123", want: "***"},
		{key: "OPENAI_API_KEY", value: "sk-proj-abcdefghij", want: "***"},
		{key: "X-Amz-Security-Token", value: "IQoJb3JpZ2luX2Vj", want: "***"},
		{key: "Cookie", value: "session=abc", want: "***"},
		{key: "Content-Type", value: "application/json", want: "application/json"},
		{key: "Origin", value: "https://api.internal.example.com", want: "https://***.com"},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			assert.Equal(t, tc.want, MaskRequestHeaderValue(tc.key, tc.value))
		})
	}
}

func TestMaskQueryParams(t *testing.T) {
	assert.Equal(t, "api_key=***&model=***", MaskQueryParams("model=gpt-4o&api_key=sk-secret"))
	assert.Equal(t, "key=***", MaskQueryParams("key=alpha%26beta"))
	assert.Equal(t, "", MaskQueryParams(""))
}

func TestMaskRequestURL(t *testing.T) {
	u := parseTestURL("https://api.gateway.internal:8443/v1/chat/completions?model=gpt-4o&key=sk-secret")
	masked := MaskRequestURL(u)
	assert.Equal(t, "https://***.internal/v1/chat/completions?key=***&model=***", masked)
}

func TestMaskJSONSensitiveValues(t *testing.T) {
	body := []byte(`{
		"model":"gpt-4o",
		"api_key":"sk-secret",
		"key":"sk-alternate",
		"auth":"basic-credential",
		"messages":[{
			"role":"user",
			"content":"visit https://example.com/v1/users/42?token=abc with Bearer sk-bearer-secret and api.example.com/v1?key=sk-123"
		}],
		"nested":{"access_token":"tok-123","keep":42}
	}`)
	masked := MaskJSONSensitiveValues(body)
	var got map[string]interface{}
	require.NoError(t, Unmarshal(masked, &got))
	assert.Equal(t, "gpt-4o", got["model"])
	assert.Equal(t, "***", got["api_key"])
	assert.Equal(t, "***", got["key"])
	assert.Equal(t, "***", got["auth"])
	assert.Equal(t, "***", got["nested"].(map[string]interface{})["access_token"])
	assert.Equal(t, float64(42), got["nested"].(map[string]interface{})["keep"])
	messages := got["messages"].([]interface{})
	content := messages[0].(map[string]interface{})["content"].(string)
	assert.NotContains(t, content, "example.com")
	assert.NotContains(t, content, "token=abc")
	assert.NotContains(t, content, "sk-bearer-secret")
	assert.NotContains(t, content, "sk-123")
}

func TestMaskJSONSensitiveValuesRejectsInvalidJSON(t *testing.T) {
	// Truncated JSON (for example a request body cut at the capture limit)
	// must never fall back to storing the raw payload.
	assert.Empty(t, MaskJSONSensitiveValues([]byte(`{"api_key":"sk-secret`)))
	assert.Empty(t, MaskJSONSensitiveValues([]byte(``)))
}

func TestBuildRequestMeta(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	c.Request = newTestRequest()
	meta := BuildRequestMeta(c)
	require.NotEmpty(t, meta)
	var got map[string]interface{}
	require.NoError(t, Unmarshal([]byte(meta), &got))
	assert.Equal(t, "POST", got["method"])
	assert.Equal(t, "https://***.internal/v1/chat/completions?api_key=***", got["url"])
	headers, ok := got["headers"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "***", headers["Authorization"])
	assert.Equal(t, "***", headers["X-Api-Key"])
	assert.Equal(t, "***", headers["X-Goog-Api-Key"])
	assert.Equal(t, "application/json", headers["Content-Type"])
}

func parseTestURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

func newTestRequest() *http.Request {
	req, err := http.NewRequest(
		http.MethodPost,
		"https://api.gateway.internal/v1/chat/completions?api_key=sk-secret",
		nil,
	)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-secret")
	req.Header.Set("X-Api-Key", "sk-prod-123")
	req.Header.Set("X-Goog-Api-Key", "AIzaSyAAAaUooTUni8AdaOkSRMda30n_Q4vrV70")
	return req
}
