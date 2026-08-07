package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCaptureTestContext(method, path, body, contentType string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	c.Request = req
	c.Set(common.RequestIdKey, "test-request-123")
	c.Set("id", 1)
	return c, rec
}

func TestShouldCaptureConversation(t *testing.T) {
	cases := []struct {
		name        string
		format      types.RelayFormat
		path        string
		contentType string
		want        bool
	}{
		{"openai chat", types.RelayFormatOpenAI, "/v1/chat/completions", "application/json", true},
		{"openai chat charset", types.RelayFormatOpenAI, "/v1/chat/completions", "application/json; charset=utf-8", true},
		{"openai completions", types.RelayFormatOpenAI, "/v1/completions", "application/json", true},
		{"openai moderations skipped", types.RelayFormatOpenAI, "/v1/moderations", "application/json", false},
		{"claude messages", types.RelayFormatClaude, "/v1/messages", "application/json", true},
		{"gemini generate", types.RelayFormatGemini, "/v1beta/models/gemini-2.0-flash:generateContent", "application/json", true},
		{"openai responses", types.RelayFormatOpenAIResponses, "/v1/responses", "application/json", true},
		{"openai responses compact", types.RelayFormatOpenAIResponsesCompaction, "/v1/responses/compact", "application/json", true},
		{"openai multipart skipped", types.RelayFormatOpenAI, "/v1/chat/completions", "multipart/form-data; boundary=x", false},
		{"openai no content type skipped", types.RelayFormatOpenAI, "/v1/chat/completions", "", false},
		{"gemini embedContent skipped", types.RelayFormatGemini, "/v1beta/models/gemini-embedding-001:embedContent", "application/json", false},
		{"gemini batch embed skipped", types.RelayFormatGemini, "/v1beta/models/gemini-embedding-001:batchEmbedContents", "application/json", false},
		{"gemini engine embeddings skipped", types.RelayFormatGemini, "/v1/engines/embedding-model/embeddings", "application/json", false},
		{"openai embeddings skipped", types.RelayFormatEmbedding, "/v1/embeddings", "application/json", false},
		{"openai image skipped", types.RelayFormatOpenAIImage, "/v1/images/generations", "application/json", false},
		{"openai audio skipped", types.RelayFormatOpenAIAudio, "/v1/audio/transcriptions", "application/json", false},
		{"rerank skipped", types.RelayFormatRerank, "/v1/rerank", "application/json", false},
		{"realtime skipped", types.RelayFormatOpenAIRealtime, "/v1/realtime", "application/json", false},
		{"task skipped", types.RelayFormatTask, "/suno/submit/foo", "application/json", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newCaptureTestContext(http.MethodPost, tc.path, `{"model":"m"}`, tc.contentType)
			assert.Equal(t, tc.want, shouldCaptureConversation(c, tc.format))
		})
	}
}

func TestNewConversationCaptureWrapsWriter(t *testing.T) {
	c, _ := newCaptureTestContext(http.MethodPost, "/v1/chat/completions", `{"model":"gpt-4o","messages":[]}`, "application/json")
	capture := newConversationCapture(c, types.RelayFormatOpenAI)
	require.NotNil(t, capture)
	_, ok := c.Writer.(*conversationCaptureWriter)
	assert.True(t, ok, "c.Writer must be wrapped")
	capture.cleanup()
}

func TestNewConversationCaptureSkipsNonConversation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		format types.RelayFormat
		path   string
		ctype  string
	}{
		{"image", types.RelayFormatOpenAIImage, "/v1/images/generations", "application/json"},
		{"embedding", types.RelayFormatEmbedding, "/v1/embeddings", "application/json"},
		{"multipart chat", types.RelayFormatOpenAI, "/v1/chat/completions", "multipart/form-data; boundary=x"},
		{"realtime", types.RelayFormatOpenAIRealtime, "/v1/realtime", "application/json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newCaptureTestContext(http.MethodPost, tc.path, `{}`, tc.ctype)
			capture := newConversationCapture(c, tc.format)
			assert.Nil(t, capture)
			_, wrapped := c.Writer.(*conversationCaptureWriter)
			assert.False(t, wrapped, "c.Writer must stay untouched")
		})
	}
}

func TestCapturePlainJSON(t *testing.T) {
	reqBody := `{"model":"gpt-4o","api_key":"sk-secret","messages":[{"role":"user","content":"hi"}]}`
	c, rec := newCaptureTestContext(http.MethodPost, "/v1/chat/completions?api_key=sk-query-secret", reqBody, "application/json")
	c.Request.Header.Set("Authorization", "Bearer sk-header-secret")
	c.Request.Header.Set("X-Goog-Api-Key", "AIzaSyAAAaUooTUni8AdaOkSRMda30n_Q4vrV70")
	capture := newConversationCapture(c, types.RelayFormatOpenAI)
	require.NotNil(t, capture)

	capture.readRequestBody(c)
	capture.setRelayInfo(&relaycommon.RelayInfo{OriginModelName: "gpt-4o", IsStream: false})

	w := c.Writer.(*conversationCaptureWriter)
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(`{"id":"chatcmpl-1","choices":[]}`))
	require.NoError(t, err)

	record := capture.buildRecord(c)
	require.NotNil(t, record)
	assert.Equal(t, "test-request-123", record.RequestID)
	assert.Equal(t, 1, record.UserID)
	assert.Equal(t, "gpt-4o", record.ModelName)
	assert.Equal(t, "/v1/chat/completions", record.RequestPath)
	assert.Equal(t, string(types.RelayFormatOpenAI), record.RelayFormat)
	assert.Equal(t, `{"api_key":"***","messages":[{"content":"hi","role":"user"}],"model":"gpt-4o"}`, record.RequestBody)
	assert.Equal(t, `{"id":"chatcmpl-1","choices":[]}`, record.ResponseBody)
	require.NotEmpty(t, record.RequestMeta)
	assert.Contains(t, record.RequestMeta, `"method":"POST"`)
	assert.Contains(t, record.RequestMeta, `"url":"http://***/v1/chat/completions?api_key=***"`)
	assert.Contains(t, record.RequestMeta, `"Authorization":"***"`)
	assert.Contains(t, record.RequestMeta, `"X-Goog-Api-Key":"***"`)
	assert.NotContains(t, record.RequestMeta, "sk-secret")
	assert.NotContains(t, record.RequestMeta, "sk-query-secret")
	assert.NotContains(t, record.RequestMeta, "sk-header-secret")
	assert.NotContains(t, record.RequestMeta, "AIzaSyAAAaUooTUni8AdaOkSRMda30n_Q4vrV70")
	assert.Equal(t, http.StatusOK, record.ResponseStatus)
	assert.False(t, record.IsStream)
	assert.Equal(t, captureStatusComplete, record.CaptureStatus)

	// Passthrough: the client received the exact same bytes and status.
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, `{"id":"chatcmpl-1","choices":[]}`, rec.Body.String())
	capture.cleanup()
}

func TestCaptureTruncatedJSONBodyNotPersisted(t *testing.T) {
	// A request body cut at the capture limit is invalid JSON; the raw
	// prefix must never be written to the conversation context.
	reqBody := `{"model":"gpt-4o","api_key":"sk-secret","messages":[]}`
	truncated := reqBody[:20]
	c, _ := newCaptureTestContext(http.MethodPost, "/v1/chat/completions", truncated, "application/json")
	capture := newConversationCapture(c, types.RelayFormatOpenAI)
	require.NotNil(t, capture)
	capture.readRequestBody(c)

	record := capture.buildRecord(c)
	require.NotNil(t, record)
	assert.Equal(t, "", record.RequestBody)
	require.NotEmpty(t, record.RequestMeta)
	assert.NotContains(t, record.RequestMeta, "sk-secret")
	capture.cleanup()
}

func TestCaptureSSEChunks(t *testing.T) {
	c, rec := newCaptureTestContext(http.MethodPost, "/v1/chat/completions", `{"model":"gpt-4o","messages":[],"stream":true}`, "application/json")
	capture := newConversationCapture(c, types.RelayFormatOpenAI)
	require.NotNil(t, capture)
	capture.setRelayInfo(&relaycommon.RelayInfo{OriginModelName: "gpt-4o", IsStream: true})

	w := c.Writer.(*conversationCaptureWriter)
	chunks := []string{
		"data: {\"id\":\"1\",\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n",
		"data: {\"id\":\"1\",\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n",
		"data: [DONE]\n\n",
	}
	for _, ch := range chunks {
		n, err := w.WriteString(ch)
		require.NoError(t, err)
		assert.Equal(t, len(ch), n)
		w.Flush()
	}

	record := capture.buildRecord(c)
	require.NotNil(t, record)
	assert.True(t, record.IsStream)
	assert.Equal(t, strings.Join(chunks, ""), record.ResponseBody)
	assert.Equal(t, captureStatusComplete, record.CaptureStatus)
	assert.Equal(t, http.StatusOK, rec.Code)
	capture.cleanup()
}

func TestCaptureErrorDeferPassthrough(t *testing.T) {
	// Mirrors the Relay pattern: finalize is deferred first, then a deferred
	// error writer runs on exit and must be captured and passed through.
	c, rec := newCaptureTestContext(http.MethodPost, "/v1/chat/completions", `{"model":"gpt-4o","messages":[]}`, "application/json")
	capture := newConversationCapture(c, types.RelayFormatOpenAI)
	require.NotNil(t, capture)

	// Nested scope mirrors Relay: the error-writer defer runs before finalize
	// (finalize was registered first), so the error JSON is captured.
	func() {
		defer c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "invalid request",
				"type":    "invalid_request_error",
			},
		})
	}()

	record := capture.buildRecord(c)
	require.NotNil(t, record)
	assert.Equal(t, http.StatusBadRequest, record.ResponseStatus)
	assert.Contains(t, record.ResponseBody, "invalid request")
	assert.Contains(t, record.ResponseBody, "invalid_request_error")
	assert.Equal(t, captureStatusComplete, record.CaptureStatus)

	// Passthrough: the error JSON actually reached the client.
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid request")
	capture.cleanup()
}

func TestCaptureClientDisconnectedStatus(t *testing.T) {
	c, _ := newCaptureTestContext(http.MethodPost, "/v1/chat/completions", `{"model":"gpt-4o","messages":[]}`, "application/json")
	capture := newConversationCapture(c, types.RelayFormatOpenAI)
	require.NotNil(t, capture)

	w := c.Writer.(*conversationCaptureWriter)
	_, err := w.Write([]byte("partial"))
	require.NoError(t, err)
	w.markClientDisconnected()

	record := capture.buildRecord(c)
	require.NotNil(t, record)
	assert.Equal(t, "partial", record.ResponseBody)
	assert.Equal(t, captureStatusClientDisconnected, record.CaptureStatus)
	capture.cleanup()
}

func TestCaptureEmptyResponseStatus(t *testing.T) {
	c, _ := newCaptureTestContext(http.MethodPost, "/v1/chat/completions", `{"model":"gpt-4o","messages":[]}`, "application/json")
	capture := newConversationCapture(c, types.RelayFormatOpenAI)
	require.NotNil(t, capture)
	// No bytes written to the response.
	record := capture.buildRecord(c)
	require.NotNil(t, record)
	assert.Equal(t, "", record.ResponseBody)
	assert.Equal(t, captureStatusEmptyResponse, record.CaptureStatus)
	capture.cleanup()
}

func TestConversationSpoolDiskSpillAndContent(t *testing.T) {
	// Force a spill to a temp file with a tiny memory threshold.
	spool := newConversationSpool(1<<20, 8)
	payload := strings.Repeat("a", 1000)
	n, err := spool.Write([]byte(payload))
	require.NoError(t, err)
	assert.Equal(t, len(payload), n)
	assert.Equal(t, int64(len(payload)), spool.Len())
	assert.False(t, spool.Overflow())
	got, err := spool.String()
	require.NoError(t, err)
	assert.Equal(t, payload, got)
	require.NoError(t, spool.Close())
}

func TestConversationSpoolOverflowIsExplicit(t *testing.T) {
	spool := newConversationSpool(10, 10)
	n, err := spool.Write([]byte("0123456789ABCDEF"))
	require.NoError(t, err)
	assert.Equal(t, 16, n) // capture side accepts the write, never fails the client
	assert.True(t, spool.Overflow())
	assert.Equal(t, int64(10), spool.Len())
	got, err := spool.String()
	require.NoError(t, err)
	assert.Equal(t, "0123456789", got)
	require.NoError(t, spool.Close())
}

func TestConversationSpoolClosedRejectsWrites(t *testing.T) {
	spool := newConversationSpool(1<<20, 8)
	require.NoError(t, spool.Close())
	_, err := spool.Write([]byte("x"))
	assert.Error(t, err)
	_, err = spool.String()
	assert.Error(t, err)
}

func TestCaptureStreamStatusPersisted(t *testing.T) {
	c, _ := newCaptureTestContext(http.MethodPost, "/v1/responses", `{"model":"m"}`, "application/json")
	capture := newConversationCapture(c, types.RelayFormatOpenAIResponses)
	require.NotNil(t, capture)

	info := &relaycommon.RelayInfo{IsStream: true}
	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, fmt.Errorf("context canceled"))
	info.StreamStatus.RecordError("bad json")
	capture.setRelayInfo(info)

	record := capture.buildRecord(c)
	require.NotNil(t, record)
	require.NotEmpty(t, record.StreamStatus)

	var parsed map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(record.StreamStatus, &parsed))
	assert.Equal(t, "error", parsed["status"])
	assert.Equal(t, "client_gone", parsed["end_reason"])
	assert.Equal(t, "context canceled", parsed["end_error"])
	assert.Equal(t, float64(1), parsed["error_count"])
	capture.cleanup()
}

func TestCaptureStreamStatusEmptyForNonStream(t *testing.T) {
	c, _ := newCaptureTestContext(http.MethodPost, "/v1/chat/completions", `{"model":"m"}`, "application/json")
	capture := newConversationCapture(c, types.RelayFormatOpenAI)
	require.NotNil(t, capture)

	info := &relaycommon.RelayInfo{IsStream: false}
	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	capture.setRelayInfo(info)

	record := capture.buildRecord(c)
	require.NotNil(t, record)
	assert.Empty(t, record.StreamStatus)
	capture.cleanup()
}
