package loki

import (
	"strings"
	"testing"
	"time"
)

// TestBatchPacksLinesIntoOneRequest checks the point of batching: a burst of log
// calls becomes a single push request.
func TestBatchPacksLinesIntoOneRequest(t *testing.T) {
	server := newRecordingServer(t)

	logger := New(Config{
		Address:   server.URL,
		Labels:    map[string]string{"app": "test"},
		BatchWait: time.Hour,
	})

	for range 3 {
		logger.Print("test log message")
	}

	if got := len(server.received()); got != 0 {
		t.Fatalf("Expected nothing sent before flush, got %d requests", got)
	}

	logger.Flush()

	payloads := server.received()
	if len(payloads) != 1 {
		t.Fatalf("Expected 1 request, got %d", len(payloads))
	}

	if got := len(payloads[0].Streams); got != 1 {
		t.Fatalf("Expected 1 stream, got %d", got)
	}

	if got := len(payloads[0].Streams[0].Values); got != 3 {
		t.Errorf("Expected 3 log lines in the request, got %d", got)
	}
}

// TestBatchFlushesWhenFull checks that a full batch is sent without waiting for
// BatchWait.
func TestBatchFlushesWhenFull(t *testing.T) {
	server := newRecordingServer(t)

	logger := New(Config{
		Address:         server.URL,
		BatchWait:       time.Hour,
		BatchMaxEntries: 2,
	})

	logger.Print("first")
	logger.Print("second")

	server.waitRequest(t)
	logger.Close()

	if got := server.lines(); got != 2 {
		t.Errorf("Expected 2 log lines delivered, got %d", got)
	}
}

// TestBatchFlushesOnWait checks that the background sender delivers a partial
// batch on its own.
func TestBatchFlushesOnWait(t *testing.T) {
	server := newRecordingServer(t)

	logger := New(Config{
		Address:   server.URL,
		BatchWait: 10 * time.Millisecond,
	})

	logger.Print("test log message")

	server.waitRequest(t)
	logger.Close()

	if got := server.lines(); got != 1 {
		t.Errorf("Expected 1 log line delivered, got %d", got)
	}
}

// TestBatchGroupsStreamsByLabels checks that lines with different label sets go
// into one request as separate streams.
func TestBatchGroupsStreamsByLabels(t *testing.T) {
	server := newRecordingServer(t)

	logger := New(Config{
		Address:   server.URL,
		Labels:    map[string]string{"app": "test"},
		BatchWait: time.Hour,
	})

	logger.Print(LabeledType{value: "labeled log"})
	logger.Print("plain log")
	logger.Print(LabeledType{value: "another labeled log"})

	logger.Flush()

	payloads := server.received()
	if len(payloads) != 1 {
		t.Fatalf("Expected 1 request, got %d", len(payloads))
	}

	streams := payloads[0].Streams
	if len(streams) != 2 {
		t.Fatalf("Expected 2 streams, got %d", len(streams))
	}

	linesByComponent := map[string]int{}
	for _, stream := range streams {
		if stream.Stream["app"] != "test" {
			t.Errorf("Expected label app=test in every stream, got %v", stream.Stream)
		}

		linesByComponent[stream.Stream["component"]] += len(stream.Values)
	}

	expected := map[string]int{"test": 2, "": 1}
	for component, count := range expected {
		if linesByComponent[component] != count {
			t.Errorf(
				"Expected %d lines for component %q, got %d",
				count, component, linesByComponent[component],
			)
		}
	}
}

// TestBatchSplitsOversizedBuffer checks that a buffer larger than one batch is
// delivered in several requests.
func TestBatchSplitsOversizedBuffer(t *testing.T) {
	server := newRecordingServer(t)

	logger := New(Config{
		Address:         server.URL,
		BatchWait:       time.Hour,
		BatchMaxEntries: 2,
		BatchQueueSize:  10,
	})

	// The buffer is filled while the first push is in flight, so that reaching
	// BatchMaxEntries does not send the lines one batch at a time.
	release := server.hold()

	logger.Print("first")
	logger.Print("second")
	server.waitRequest(t)

	for range 3 {
		logger.Print("more")
	}

	release()
	logger.Close()

	if got := len(server.received()); got != 3 {
		t.Errorf("Expected 3 requests for 5 lines with BatchMaxEntries 2, got %d", got)
	}

	if got := server.lines(); got != 5 {
		t.Errorf("Expected 5 log lines delivered, got %d", got)
	}
}

// TestBatchDropsOnOverflow checks that a queue overflow is reported and does not
// grow memory without a limit.
func TestBatchDropsOnOverflow(t *testing.T) {
	server := newRecordingServer(t)

	var errors []error
	logger := New(Config{
		Address:         server.URL,
		BatchWait:       time.Hour,
		BatchMaxEntries: 2,
		BatchQueueSize:  2,
		ErrorHandler:    func(err error) { errors = append(errors, err) },
	})

	release := server.hold()

	logger.Print("first")
	logger.Print("second")
	server.waitRequest(t)

	// The sender is busy with the first batch, so these fill the queue and overflow it.
	for range 4 {
		logger.Print("more")
	}

	release()
	logger.Close()

	if got := server.lines(); got != 4 {
		t.Errorf("Expected 4 log lines delivered, got %d", got)
	}

	if len(errors) != 1 {
		t.Fatalf("Expected 1 error about dropped lines, got %d: %v", len(errors), errors)
	}

	if !strings.Contains(errors[0].Error(), "2 log lines dropped") {
		t.Errorf("Expected an error about 2 dropped lines, got: %v", errors[0])
	}
}

// TestCloseFlushesBufferAndKeepsLoggerUsable checks that shutdown does not lose
// the buffer and that logging after Close still reaches Loki.
func TestCloseFlushesBufferAndKeepsLoggerUsable(t *testing.T) {
	server := newRecordingServer(t)

	logger := New(Config{
		Address:   server.URL,
		BatchWait: time.Hour,
	})

	logger.Print("before close")
	logger.Close()

	if got := server.lines(); got != 1 {
		t.Fatalf("Expected the buffer to be flushed on close, got %d lines", got)
	}

	logger.Close()

	logger.Print("after close")

	if got := server.lines(); got != 2 {
		t.Errorf("Expected a synchronous send after close, got %d lines", got)
	}
}
