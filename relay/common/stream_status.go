package common

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type StreamEndReason string

const (
	StreamEndReasonNone        StreamEndReason = ""
	StreamEndReasonDone        StreamEndReason = "done"
	StreamEndReasonTimeout     StreamEndReason = "timeout"
	StreamEndReasonClientGone  StreamEndReason = "client_gone"
	StreamEndReasonScannerErr  StreamEndReason = "scanner_error"
	StreamEndReasonHandlerStop StreamEndReason = "handler_stop"
	StreamEndReasonEOF         StreamEndReason = "eof"
	StreamEndReasonPanic       StreamEndReason = "panic"
	StreamEndReasonPingFail    StreamEndReason = "ping_fail"
)

const maxStreamErrorEntries = 20

type StreamErrorEntry struct {
	Message   string
	Timestamp time.Time
}

type StreamStatus struct {
	EndReason StreamEndReason
	EndError  error
	endOnce   sync.Once

	mu sync.Mutex
	// Completed records that the stream's terminal event (e.g.
	// response.completed / response.done) was delivered. A client disconnect
	// after the terminal event is a soft warning instead of an error.
	Completed  bool
	Errors     []StreamErrorEntry
	ErrorCount int
}

func NewStreamStatus() *StreamStatus {
	return &StreamStatus{}
}

func (s *StreamStatus) SetEndReason(reason StreamEndReason, err error) {
	if s == nil {
		return
	}
	s.endOnce.Do(func() {
		s.EndReason = reason
		s.EndError = err
	})
}

func (s *StreamStatus) RecordError(msg string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ErrorCount++
	if len(s.Errors) < maxStreamErrorEntries {
		s.Errors = append(s.Errors, StreamErrorEntry{
			Message:   msg,
			Timestamp: time.Now(),
		})
	}
}

// MarkCompleted records that the stream's terminal event (e.g.
// response.completed / response.done) was received and delivered, so a later
// client disconnect can be classified as a warning instead of an error.
func (s *StreamStatus) MarkCompleted() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.Completed = true
	s.mu.Unlock()
}

// IsCompleted reports whether the terminal event was delivered before the
// stream ended.
func (s *StreamStatus) IsCompleted() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Completed
}

func (s *StreamStatus) HasErrors() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ErrorCount > 0
}

func (s *StreamStatus) TotalErrorCount() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ErrorCount
}

func (s *StreamStatus) IsNormalEnd() bool {
	if s == nil {
		return true
	}
	return s.EndReason == StreamEndReasonDone ||
		s.EndReason == StreamEndReasonEOF ||
		s.EndReason == StreamEndReasonHandlerStop
}

// ToMap returns the admin-facing stream_status snapshot (status, end_reason
// and, when present, end_error / error_count / errors) that is written into
// usage log and conversation context records. A nil receiver yields an empty
// map so callers can skip nil checks.
func (s *StreamStatus) ToMap() map[string]interface{} {
	if s == nil {
		return map[string]interface{}{}
	}
	status := "ok"
	if s.EndReason == StreamEndReasonClientGone && s.IsCompleted() {
		// The terminal event was already delivered to the client; the client
		// merely dropped the connection afterwards. Not a real stream error.
		status = "warning"
	} else if !s.IsNormalEnd() || s.HasErrors() {
		status = "error"
	}
	info := map[string]interface{}{
		"status":     status,
		"end_reason": string(s.EndReason),
	}
	if s.EndError != nil {
		info["end_error"] = s.EndError.Error()
	}
	if s.ErrorCount > 0 {
		info["error_count"] = s.ErrorCount
		messages := make([]string, 0, len(s.Errors))
		for _, e := range s.Errors {
			messages = append(messages, e.Message)
		}
		info["errors"] = messages
	}
	return info
}

func (s *StreamStatus) Summary() string {
	if s == nil {
		return "StreamStatus<nil>"
	}
	b := &strings.Builder{}
	fmt.Fprintf(b, "reason=%s", s.EndReason)
	if s.EndError != nil {
		fmt.Fprintf(b, " end_error=%q", s.EndError.Error())
	}
	s.mu.Lock()
	if s.ErrorCount > 0 {
		fmt.Fprintf(b, " soft_errors=%d", s.ErrorCount)
	}
	s.mu.Unlock()
	return b.String()
}
