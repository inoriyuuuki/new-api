package common

import (
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"

	"github.com/gin-gonic/gin"
)

// sensitiveRequestKeys are JSON/header/query keys whose values must never be
// persisted into conversation context or logs. Matching is case-insensitive,
// normalizes separators to "_", and matches both the whole key and a
// "_"+candidate suffix, so Authorization, Api-Key, X-Goog-Api-Key,
// Proxy-Authorization, OPENAI_API_KEY, accessToken and client_secret all hit
// the same bucket while total_tokens/max_tokens stay unmasked.
var sensitiveRequestKeys = []string{
	"authorization",
	"apikey",
	"api_key",
	"xapikey",
	"accesstoken",
	"clientsecret",
	"clientid",
	"password",
	"passwd",
	"pwd",
	"secret",
	"token",
	"credential",
	"cookie",
	"session",
	"signature",
	"sign",
	"privatekey",
	"refreshtoken",
	"idtoken",
	"setcookie",
	"key",
	"auth",
}

func normalizeSensitiveKey(key string) string {
	var b strings.Builder
	b.Grow(len(key))
	pendingSep := false
	for _, r := range strings.ToLower(key) {
		if r == '-' || r == '_' || r == '.' || r == ' ' {
			pendingSep = b.Len() > 0
			continue
		}
		if pendingSep {
			b.WriteByte('_')
			pendingSep = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isSensitiveRequestKey(key string) bool {
	normalized := normalizeSensitiveKey(key)
	for _, candidate := range sensitiveRequestKeys {
		if normalized == candidate || strings.HasSuffix(normalized, "_"+candidate) {
			return true
		}
	}
	return false
}

// credentialLikePattern masks common credential encodings (OpenAI-style
// sk- keys, Google AIza keys and Bearer tokens) even when they appear inside
// free-form strings instead of dedicated JSON/header fields.
var credentialLikePattern = regexp.MustCompile(`(?i)\b(sk-[a-z0-9_-]{3,}|AIza[0-9a-z_-]{20,}|Bearer\s+[a-z0-9._~+/=-]{4,})\b`)

func MaskCredentialLikeValues(s string) string {
	return credentialLikePattern.ReplaceAllString(s, "***")
}

// MaskRequestHeaderValue masks credential-bearing headers entirely and runs
// the generic URL/domain/IP masker over everything else.
func MaskRequestHeaderValue(key, value string) string {
	if value == "" {
		return ""
	}
	if isSensitiveRequestKey(key) {
		return "***"
	}
	return MaskCredentialLikeValues(MaskSensitiveInfo(value))
}

// MaskRequestURL masks the request address (scheme/host) while preserving the
// path and masking query values. The path itself is deliberately kept because
// it is the endpoint being debugged and is already visible separately.
func MaskRequestURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		scheme = "http"
	}
	host := kitutil.MaskHostForURL(u.Hostname())
	if host == "" {
		host = "***"
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	result := scheme + "://" + host + path
	if maskedQuery := MaskQueryParams(u.RawQuery); maskedQuery != "" {
		result += "?" + maskedQuery
	}
	return result
}

// MaskQueryParams preserves query keys but replaces every value with ***, so
// debugging still shows which parameters were present without leaking their
// contents (API keys, tokens, signatures, etc.).
func MaskQueryParams(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "***"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	masked := make([]string, 0, len(keys))
	for _, key := range keys {
		masked = append(masked, key+"=***")
	}
	return strings.Join(masked, "&")
}

// MaskJSONSensitiveValues recursively redacts sensitive JSON key values and
// masks URLs/domains/IPs inside remaining strings. The JSON structure is
// preserved so the frontend can still render the request/response as
// conversation messages; only the sensitive contents change.
func MaskJSONSensitiveValues(data []byte) []byte {
	var root interface{}
	if err := Unmarshal(data, &root); err != nil {
		// Never persist an unmasked payload: malformed or truncated JSON
		// cannot be walked safely, and the caller must not fall back to raw.
		return nil
	}
	masked := maskJSONValue(root)
	out, err := Marshal(masked)
	if err != nil {
		return nil
	}
	return out
}

func maskJSONValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, item := range v {
			if isSensitiveRequestKey(key) {
				out[key] = "***"
				continue
			}
			out[key] = maskJSONValue(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = maskJSONValue(item)
		}
		return out
	case string:
		return MaskCredentialLikeValues(MaskSensitiveInfo(v))
	default:
		return value
	}
}

// BuildRequestMeta snapshots the inbound HTTP request into a single masked
// JSON object: method, path with masked query parameters, and headers with
// credential values redacted. The result is stored in
// ConversationContext.RequestMeta so admins can inspect every request-side
// parameter without exposing request addresses, API keys, or other secrets.
func BuildRequestMeta(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ""
	}
	method := strings.ToUpper(c.Request.Method)
	if method == "" {
		method = http.MethodGet
	}
	path := c.Request.URL.Path
	if path == "" {
		path = "/"
	}
	rawQuery := c.Request.URL.RawQuery
	if maskedQuery := MaskQueryParams(rawQuery); maskedQuery != "" {
		path += "?" + maskedQuery
	}
	meta := map[string]interface{}{
		"method": method,
		"url":    MaskRequestURL(c.Request.URL),
	}
	headers := make(map[string]string, len(c.Request.Header))
	for key, values := range c.Request.Header {
		headers[key] = MaskRequestHeaderValue(key, strings.Join(values, ", "))
	}
	if len(headers) > 0 {
		meta["headers"] = headers
	}
	return GetJsonString(meta)
}
