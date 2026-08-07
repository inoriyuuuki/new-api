package common

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStreamStatus_SetEndReason_FirstWins(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	s.SetEndReason(StreamEndReasonDone, nil)
	s.SetEndReason(StreamEndReasonTimeout, nil)
	s.SetEndReason(StreamEndReasonClientGone, fmt.Errorf("context canceled"))

	assert.Equal(t, StreamEndReasonDone, s.EndReason)
	assert.Nil(t, s.EndError)
}

func TestStreamStatus_SetEndReason_WithError(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	expectedErr := fmt.Errorf("read: connection reset")
	s.SetEndReason(StreamEndReasonScannerErr, expectedErr)

	assert.Equal(t, StreamEndReasonScannerErr, s.EndReason)
	assert.Equal(t, expectedErr, s.EndError)
}

func TestStreamStatus_SetEndReason_NilSafe(t *testing.T) {
	t.Parallel()
	var s *StreamStatus
	s.SetEndReason(StreamEndReasonDone, nil)
}

func TestStreamStatus_SetEndReason_Concurrent(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	reasons := []StreamEndReason{
		StreamEndReasonDone,
		StreamEndReasonTimeout,
		StreamEndReasonClientGone,
		StreamEndReasonScannerErr,
		StreamEndReasonHandlerStop,
		StreamEndReasonEOF,
		StreamEndReasonPanic,
		StreamEndReasonPingFail,
	}

	var wg sync.WaitGroup
	for _, r := range reasons {
		wg.Add(1)
		go func(reason StreamEndReason) {
			defer wg.Done()
			s.SetEndReason(reason, nil)
		}(r)
	}
	wg.Wait()

	assert.NotEqual(t, StreamEndReasonNone, s.EndReason)
}

func TestStreamStatus_RecordError_Basic(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	s.RecordError("bad json")
	s.RecordError("another bad json")
	s.RecordError("client gone")

	assert.True(t, s.HasErrors())
	assert.Equal(t, 3, s.TotalErrorCount())
	assert.Len(t, s.Errors, 3)
}

func TestStreamStatus_RecordError_CapAtMax(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	for i := 0; i < 30; i++ {
		s.RecordError(fmt.Sprintf("error_%d", i))
	}

	assert.Equal(t, maxStreamErrorEntries, len(s.Errors))
	assert.Equal(t, 30, s.TotalErrorCount())
}

func TestStreamStatus_RecordError_NilSafe(t *testing.T) {
	t.Parallel()
	var s *StreamStatus
	s.RecordError("should not panic")
}

func TestStreamStatus_RecordError_Concurrent(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			s.RecordError(fmt.Sprintf("error_%d", idx))
		}(i)
	}
	wg.Wait()

	assert.Equal(t, 100, s.TotalErrorCount())
	assert.LessOrEqual(t, len(s.Errors), maxStreamErrorEntries)
}

func TestStreamStatus_HasErrors_Empty(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()
	assert.False(t, s.HasErrors())
	assert.Equal(t, 0, s.TotalErrorCount())
}

func TestStreamStatus_HasErrors_NilSafe(t *testing.T) {
	t.Parallel()
	var s *StreamStatus
	assert.False(t, s.HasErrors())
	assert.Equal(t, 0, s.TotalErrorCount())
}

func TestStreamStatus_IsNormalEnd(t *testing.T) {
	t.Parallel()
	tests := []struct {
		reason StreamEndReason
		normal bool
	}{
		{StreamEndReasonDone, true},
		{StreamEndReasonEOF, true},
		{StreamEndReasonHandlerStop, true},
		{StreamEndReasonTimeout, false},
		{StreamEndReasonClientGone, false},
		{StreamEndReasonScannerErr, false},
		{StreamEndReasonPanic, false},
		{StreamEndReasonPingFail, false},
		{StreamEndReasonNone, false},
	}
	for _, tt := range tests {
		s := NewStreamStatus()
		s.SetEndReason(tt.reason, nil)
		assert.Equal(t, tt.normal, s.IsNormalEnd(), "reason=%s", tt.reason)
	}
}

func TestStreamStatus_IsNormalEnd_NilSafe(t *testing.T) {
	t.Parallel()
	var s *StreamStatus
	assert.True(t, s.IsNormalEnd())
}

func TestStreamStatus_Summary(t *testing.T) {
	t.Parallel()

	s := NewStreamStatus()
	s.SetEndReason(StreamEndReasonDone, nil)
	summary := s.Summary()
	assert.Contains(t, summary, "reason=done")
	assert.NotContains(t, summary, "soft_errors")

	s2 := NewStreamStatus()
	s2.SetEndReason(StreamEndReasonTimeout, nil)
	s2.RecordError("bad json")
	s2.RecordError("write failed")
	summary2 := s2.Summary()
	assert.Contains(t, summary2, "reason=timeout")
	assert.Contains(t, summary2, "soft_errors=2")
}

func TestStreamStatus_Summary_NilSafe(t *testing.T) {
	t.Parallel()
	var s *StreamStatus
	assert.Equal(t, "StreamStatus<nil>", s.Summary())
}

func TestStreamStatus_ToMap(t *testing.T) {
	t.Parallel()

	t.Run("nil safe", func(t *testing.T) {
		var s *StreamStatus
		assert.Empty(t, s.ToMap())
	})

	t.Run("normal end", func(t *testing.T) {
		s := NewStreamStatus()
		s.SetEndReason(StreamEndReasonDone, nil)
		got := s.ToMap()
		assert.Equal(t, "ok", got["status"])
		assert.Equal(t, string(StreamEndReasonDone), got["end_reason"])
		assert.NotContains(t, got, "end_error")
		assert.NotContains(t, got, "error_count")
		assert.NotContains(t, got, "errors")
	})

	t.Run("abnormal end with error", func(t *testing.T) {
		s := NewStreamStatus()
		s.SetEndReason(StreamEndReasonClientGone, fmt.Errorf("context canceled"))
		got := s.ToMap()
		assert.Equal(t, "error", got["status"])
		assert.Equal(t, string(StreamEndReasonClientGone), got["end_reason"])
		assert.Equal(t, "context canceled", got["end_error"])
	})

	t.Run("soft errors", func(t *testing.T) {
		s := NewStreamStatus()
		s.SetEndReason(StreamEndReasonDone, nil)
		s.RecordError("bad json")
		s.RecordError("write failed")
		got := s.ToMap()
		assert.Equal(t, "error", got["status"], "soft errors must flip status to error")
		assert.Equal(t, 2, got["error_count"])
		assert.Equal(t, []string{"bad json", "write failed"}, got["errors"])
	})
}

func TestStreamStatus_Completed(t *testing.T) {
	t.Parallel()

	s := NewStreamStatus()
	assert.False(t, s.IsCompleted())
	s.MarkCompleted()
	assert.True(t, s.IsCompleted())

	var nilStatus *StreamStatus
	nilStatus.MarkCompleted()
	assert.False(t, nilStatus.IsCompleted())
}

func TestStreamStatus_ToMap_ClientGoneWarning(t *testing.T) {
	t.Parallel()

	t.Run("client gone after terminal event is warning", func(t *testing.T) {
		s := NewStreamStatus()
		s.MarkCompleted()
		s.SetEndReason(StreamEndReasonClientGone, fmt.Errorf("context canceled"))
		got := s.ToMap()
		assert.Equal(t, "warning", got["status"])
		assert.Equal(t, string(StreamEndReasonClientGone), got["end_reason"])
		assert.Equal(t, "context canceled", got["end_error"])
	})

	t.Run("client gone without terminal event stays error", func(t *testing.T) {
		s := NewStreamStatus()
		s.SetEndReason(StreamEndReasonClientGone, fmt.Errorf("context canceled"))
		got := s.ToMap()
		assert.Equal(t, "error", got["status"])
	})

	t.Run("terminal event alone stays ok", func(t *testing.T) {
		s := NewStreamStatus()
		s.MarkCompleted()
		s.SetEndReason(StreamEndReasonEOF, nil)
		got := s.ToMap()
		assert.Equal(t, "ok", got["status"])
	})

	t.Run("other abnormal ends stay error even when completed", func(t *testing.T) {
		s := NewStreamStatus()
		s.MarkCompleted()
		s.SetEndReason(StreamEndReasonScannerErr, fmt.Errorf("response body closed"))
		got := s.ToMap()
		assert.Equal(t, "error", got["status"])
	})
}
