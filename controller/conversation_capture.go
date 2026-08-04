package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

const (
	// conversationCaptureMemoryThreshold is the in-memory capture buffer size.
	// Larger responses spill to a temporary file so long streaming responses
	// never exhaust heap.
	conversationCaptureMemoryThreshold = 256 << 10 // 256KB
	// conversationCaptureMaxRequestBodyBytes caps how much of the raw inbound
	// JSON body is persisted. If a request exceeds this the prefix is kept and
	// the capture status explicitly records the truncation (never silent).
	conversationCaptureMaxRequestBodyBytes = 16 << 20 // 16MB
	// conversationCaptureMaxResponseBytes caps the persisted response payload.
	conversationCaptureMaxResponseBytes = 64 << 20 // 64MB
	// conversationCaptureDBTimeout bounds the async DB-A write so a hung
	// database can never leak goroutines.
	conversationCaptureDBTimeout = 30 * time.Second
)

// CaptureStatus values persisted into ConversationContext.CaptureStatus.
const (
	captureStatusComplete           = "complete"
	captureStatusEmptyResponse      = "empty_response"
	captureStatusClientDisconnected = "client_disconnected"
	captureStatusRequestTooLarge    = "request_too_large"
	captureStatusResponseTooLarge   = "response_too_large"
	captureStatusFailed             = "capture_failed"
)

// conversationSpool captures a byte stream, keeping it in memory up to a
// threshold and then spilling to a temporary file so large streaming
// responses do not exhaust heap. It never silently truncates: once maxBytes
// is exceeded further writes are dropped and Overflow() reports it so the
// caller can persist an explicit capture status.
type conversationSpool struct {
	mu           sync.Mutex
	buf          bytes.Buffer
	file         *os.File
	filePath     string
	size         int64
	maxBytes     int64
	memThreshold int64
	overflow     bool
	closed       bool
}

func newConversationSpool(maxBytes int64, memThreshold int64) *conversationSpool {
	if maxBytes <= 0 {
		maxBytes = conversationCaptureMaxResponseBytes
	}
	if memThreshold <= 0 {
		memThreshold = conversationCaptureMemoryThreshold
	}
	return &conversationSpool{
		maxBytes:     maxBytes,
		memThreshold: memThreshold,
	}
}

func (s *conversationSpool) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, os.ErrClosed
	}
	originalLen := len(p)
	if s.size >= s.maxBytes {
		s.overflow = true
		// Accept the write on the capture side; the client stream is never
		// affected by the capture limit.
		return originalLen, nil
	}
	remaining := s.maxBytes - s.size
	if int64(len(p)) > remaining {
		s.overflow = true
		p = p[:int(remaining)]
	}
	if len(p) > 0 {
		if err := s.writeBytesLocked(p); err != nil {
			return 0, err
		}
		s.size += int64(len(p))
	}
	return originalLen, nil
}

func (s *conversationSpool) WriteString(p string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, os.ErrClosed
	}
	originalLen := len(p)
	if s.size >= s.maxBytes {
		s.overflow = true
		return originalLen, nil
	}
	remaining := s.maxBytes - s.size
	if int64(len(p)) > remaining {
		s.overflow = true
		p = p[:int(remaining)]
	}
	if len(p) > 0 {
		if err := s.writeStringLocked(p); err != nil {
			return 0, err
		}
		s.size += int64(len(p))
	}
	return originalLen, nil
}

func (s *conversationSpool) writeBytesLocked(p []byte) error {
	if s.file == nil && int64(s.buf.Len()+len(p)) <= s.memThreshold {
		_, err := s.buf.Write(p)
		return err
	}
	if s.file == nil {
		if err := s.spillLocked(); err != nil {
			return err
		}
	}
	_, err := s.file.Write(p)
	return err
}

func (s *conversationSpool) writeStringLocked(p string) error {
	if s.file == nil && int64(s.buf.Len()+len(p)) <= s.memThreshold {
		_, err := s.buf.WriteString(p)
		return err
	}
	if s.file == nil {
		if err := s.spillLocked(); err != nil {
			return err
		}
	}
	_, err := s.file.WriteString(p)
	return err
}

func (s *conversationSpool) spillLocked() error {
	f, err := os.CreateTemp("", "conversation-capture-*")
	if err != nil {
		return err
	}
	if _, err := f.Write(s.buf.Bytes()); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return err
	}
	s.file = f
	s.filePath = f.Name()
	s.buf.Reset()
	return nil
}

// String returns the full captured content. It is safe to call concurrently
// with Write, but the caller should normally call it only after the response
// finished streaming.
func (s *conversationSpool) String() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", os.ErrClosed
	}
	if s.file == nil {
		return s.buf.String(), nil
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	data, err := io.ReadAll(s.file)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *conversationSpool) Len() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.size
}

func (s *conversationSpool) Overflow() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.overflow
}

// Close releases the capture resources and removes the temporary file. After
// Close any Write returns os.ErrClosed.
func (s *conversationSpool) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.buf.Reset()
	if s.file != nil {
		path := s.filePath
		err := s.file.Close()
		s.file = nil
		s.filePath = ""
		if path != "" {
			_ = os.Remove(path)
		}
		return err
	}
	return nil
}

// conversationCaptureWriter transparently passes every response through to
// the wrapped gin.ResponseWriter while capturing the payload. It embeds the
// interface and only overrides Write/WriteString, so Flush/CloseNotify/
// Hijack/Pusher/Status/Written behavior is inherited unchanged from the
// underlying writer.
type conversationCaptureWriter struct {
	gin.ResponseWriter
	spool *conversationSpool

	mu                 sync.Mutex
	clientDisconnected bool
	captureErr         error
}

func (w *conversationCaptureWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if n > 0 {
		w.capture(p[:n])
	}
	if err != nil {
		w.markClientDisconnected()
	}
	return n, err
}

func (w *conversationCaptureWriter) WriteString(s string) (int, error) {
	n, err := io.WriteString(w.ResponseWriter, s)
	if n > 0 {
		w.captureString(s[:n])
	}
	if err != nil {
		w.markClientDisconnected()
	}
	return n, err
}

func (w *conversationCaptureWriter) capture(p []byte) {
	if _, err := w.spool.Write(p); err != nil {
		w.setCaptureFailed(err)
	}
}

func (w *conversationCaptureWriter) captureString(s string) {
	if _, err := w.spool.WriteString(s); err != nil {
		w.setCaptureFailed(err)
	}
}

func (w *conversationCaptureWriter) markClientDisconnected() {
	w.mu.Lock()
	w.clientDisconnected = true
	w.mu.Unlock()
}

func (w *conversationCaptureWriter) setCaptureFailed(err error) {
	if err == nil {
		return
	}
	w.mu.Lock()
	if w.captureErr == nil {
		w.captureErr = err
	}
	w.mu.Unlock()
}

func (w *conversationCaptureWriter) isClientDisconnected() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.clientDisconnected
}

func (w *conversationCaptureWriter) captureError() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.captureErr
}

// conversationCapture owns one request's context capture lifecycle: wrapping
// the response writer, reading the raw request body, and persisting the final
// record asynchronously into DB A.
type conversationCapture struct {
	writer *conversationCaptureWriter
	spool  *conversationSpool

	relayFormat types.RelayFormat
	relayInfo   *relaycommon.RelayInfo

	requestBody  string
	requestTrunc bool

	// done is closed in finalize to stop the CloseNotify watcher goroutine.
	done chan struct{}
}

// newConversationCapture decides whether the request qualifies for context
// capture and, when it does, wraps c.Writer with the capture writer. It
// returns nil for formats/payloads that must not be captured (WebSocket
// realtime, images, audio, embeddings, rerank, multipart, etc.), in which
// case c.Writer is left untouched.
func newConversationCapture(c *gin.Context, relayFormat types.RelayFormat) *conversationCapture {
	if !shouldCaptureConversation(c, relayFormat) {
		return nil
	}
	spool := newConversationSpool(conversationCaptureMaxResponseBytes, conversationCaptureMemoryThreshold)
	capture := &conversationCapture{
		writer: &conversationCaptureWriter{
			ResponseWriter: c.Writer,
			spool:          spool,
		},
		spool:       spool,
		relayFormat: relayFormat,
		done:        make(chan struct{}),
	}
	c.Writer = capture.writer
	capture.watchDisconnect()
	return capture
}

// watchDisconnect marks the capture as client-disconnected as soon as the
// underlying connection closes, so long-running SSE streams get an accurate
// status even when the handler never observes a write error.
func (cap *conversationCapture) watchDisconnect() {
	// CloseNotify is not implemented by every ResponseWriter (for example
	// httptest.ResponseRecorder panics), so guard it and treat the capture as
	// still connected when the underlying writer cannot report disconnects.
	notify := func() (ch <-chan bool) {
		defer func() { _ = recover() }()
		return cap.writer.ResponseWriter.CloseNotify()
	}()
	if notify == nil {
		return
	}
	gopool.Go(func() {
		select {
		case <-notify:
			cap.writer.markClientDisconnected()
		case <-cap.done:
		}
	})
}

// readRequestBody snapshots the user's original inbound JSON body from the
// BodyStorage cache. It always rewinds the storage so the relay retry loop and
// downstream handlers can re-read the same body. Failures are best-effort and
// only logged; they never affect the API response.
func (cap *conversationCapture) readRequestBody(c *gin.Context) {
	if c.Request == nil {
		return
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		logger.LogDebug(c, "conversation capture: request body unavailable: %s", err.Error())
		return
	}
	defer func() {
		if _, seekErr := storage.Seek(0, io.SeekStart); seekErr != nil {
			logger.LogDebug(c, "conversation capture: failed to rewind body storage: %s", seekErr.Error())
		}
	}()
	data, err := io.ReadAll(io.LimitReader(storage, conversationCaptureMaxRequestBodyBytes+1))
	if err != nil {
		logger.LogDebug(c, "conversation capture: failed to read request body: %s", err.Error())
		return
	}
	if int64(len(data)) > conversationCaptureMaxRequestBodyBytes {
		cap.requestTrunc = true
		data = data[:conversationCaptureMaxRequestBodyBytes]
	}
	cap.requestBody = string(data)
}

func (cap *conversationCapture) setRelayInfo(info *relaycommon.RelayInfo) {
	cap.relayInfo = info
}

// finalize is registered as a deferred call in Relay *before* the deferred
// error writer, so any error response written on exit is captured too. It
// snapshots the response, builds the record and persists it asynchronously;
// a database failure is only logged and never affects the API response.
func (cap *conversationCapture) finalize(c *gin.Context) {
	close(cap.done)
	defer cap.cleanup()

	record := cap.buildRecord(c)
	if record.RequestID == "" || record.UserID <= 0 {
		// Nothing to link or own: skip persistence.
		return
	}
	gopool.Go(func() {
		ctx, cancel := context.WithTimeout(context.Background(), conversationCaptureDBTimeout)
		defer cancel()
		if err := model.UpsertConversationContext(ctx, record); err != nil {
			common.SysError(fmt.Sprintf("conversation capture: failed to persist context for request %s: %s", record.RequestID, err.Error()))
		}
	})
}

func (cap *conversationCapture) buildRecord(c *gin.Context) *model.ConversationContext {
	responseBody, err := cap.spool.String()
	if err != nil {
		// Keep whatever was captured so far and record the failure explicitly.
		cap.writer.setCaptureFailed(err)
		responseBody = ""
	}
	status := http.StatusOK
	if st := cap.writer.ResponseWriter.Status(); st != 0 {
		status = st
	}
	requestPath := ""
	if c.Request != nil && c.Request.URL != nil {
		requestPath = c.Request.URL.Path
	}
	return &model.ConversationContext{
		RequestID:      c.GetString(common.RequestIdKey),
		UserID:         c.GetInt("id"),
		CreatedAt:      common.GetTimestamp(),
		RequestPath:    requestPath,
		RelayFormat:    string(cap.relayFormat),
		ModelName:      cap.resolveModelName(c),
		RequestBody:    cap.requestBody,
		ResponseBody:   responseBody,
		ResponseStatus: status,
		IsStream:       cap.resolveIsStream(c),
		CaptureStatus:  cap.resolveStatus(),
	}
}

func (cap *conversationCapture) resolveModelName(c *gin.Context) string {
	if cap.relayInfo != nil && cap.relayInfo.OriginModelName != "" {
		return cap.relayInfo.OriginModelName
	}
	return common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
}

func (cap *conversationCapture) resolveIsStream(c *gin.Context) bool {
	if cap.relayInfo != nil {
		return cap.relayInfo.IsStream
	}
	return common.GetContextKeyBool(c, constant.ContextKeyIsStream)
}

// resolveStatus picks the most severe explicit capture status so truncation or
// capture failure is never silent.
func (cap *conversationCapture) resolveStatus() string {
	if cap.writer.captureError() != nil {
		return captureStatusFailed
	}
	if cap.requestTrunc {
		return captureStatusRequestTooLarge
	}
	if cap.spool.Overflow() {
		return captureStatusResponseTooLarge
	}
	if cap.writer.isClientDisconnected() {
		return captureStatusClientDisconnected
	}
	if cap.spool.Len() == 0 {
		return captureStatusEmptyResponse
	}
	return captureStatusComplete
}

func (cap *conversationCapture) cleanup() {
	_ = cap.spool.Close()
}

// shouldCaptureConversation restricts capture to JSON conversation traffic.
// Only OpenAI/Claude/Gemini/OpenAI Responses formats with a JSON content-type
// are captured; embeddings, image, audio, rerank, task, MJ and WebSocket
// realtime requests are never captured.
func shouldCaptureConversation(c *gin.Context, relayFormat types.RelayFormat) bool {
	switch relayFormat {
	case types.RelayFormatOpenAI,
		types.RelayFormatClaude,
		types.RelayFormatGemini,
		types.RelayFormatOpenAIResponses,
		types.RelayFormatOpenAIResponsesCompaction:
	default:
		return false
	}
	if c.Request == nil || c.Request.Header == nil || c.Request.URL == nil {
		return false
	}
	contentType := strings.ToLower(strings.TrimSpace(c.Request.Header.Get("Content-Type")))
	if !strings.HasPrefix(contentType, "application/json") {
		return false
	}
	path := c.Request.URL.Path
	// Gemini exposes embeddings under RelayFormatGemini (":embedContent",
	// ":batchEmbedContents", "/engines/:model/embeddings"); those are not
	// conversations and must not be captured.
	if strings.Contains(path, "embed") {
		return false
	}
	// Claude's native Messages endpoint (/v1/messages) is not covered by
	// Path2RelayMode (it resolves to RelayModeUnknown), so handle it here.
	if relayFormat == types.RelayFormatClaude && path == "/v1/messages" {
		return true
	}
	switch relayconstant.Path2RelayMode(path) {
	case relayconstant.RelayModeChatCompletions,
		relayconstant.RelayModeCompletions,
		relayconstant.RelayModeResponses,
		relayconstant.RelayModeResponsesCompact,
		relayconstant.RelayModeGemini:
		return true
	default:
		return false
	}
}
