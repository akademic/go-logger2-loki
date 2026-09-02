package loki

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testEntries builds the argument of makePayload/send for a single log line.
func testEntries(line string, labels map[string]string) []entry {
	return []entry{{timestamp: time.Now().UnixNano(), line: line, labels: labels}}
}

// countingServer is a Loki stub that counts requests and new TCP connections.
type countingServer struct {
	*httptest.Server
	requests    atomic.Int64
	connections atomic.Int64
}

func newCountingServer(t *testing.T, status int) *countingServer {
	t.Helper()

	cs := &countingServer{}

	cs.Server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cs.requests.Add(1)
		w.WriteHeader(status)
	}))

	cs.Server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			cs.connections.Add(1)
		}
	}

	cs.Server.Start()
	t.Cleanup(cs.Server.Close)

	return cs
}

type receivedStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

type receivedPayload struct {
	Streams []receivedStream `json:"streams"`
}

// recordingServer is a Loki stub that keeps every received payload. Requests block
// until block is closed, which lets a test hold a push in flight.
type recordingServer struct {
	*httptest.Server

	mu       sync.Mutex
	block    chan struct{}
	release  func()
	payloads []receivedPayload
	arrived  chan struct{}
}

func newRecordingServer(t *testing.T) *recordingServer {
	t.Helper()

	open := make(chan struct{})
	close(open)

	rs := &recordingServer{
		block:   open,
		release: func() {},
		arrived: make(chan struct{}, 1000),
	}

	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload receivedPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		rs.mu.Lock()
		rs.payloads = append(rs.payloads, payload)
		rs.mu.Unlock()

		select {
		case rs.arrived <- struct{}{}:
		default:
		}

		<-rs.currentBlock()

		w.WriteHeader(http.StatusNoContent)
	}))

	// A held request must be released before Close, which waits for outstanding
	// handlers: otherwise a failed assertion hangs the whole test binary.
	t.Cleanup(func() {
		rs.currentRelease()()
		rs.Server.Close()
	})

	return rs
}

// hold makes every request block until the returned function is called.
func (rs *recordingServer) hold() func() {
	block := make(chan struct{})
	release := sync.OnceFunc(func() { close(block) })

	rs.mu.Lock()
	rs.block = block
	rs.release = release
	rs.mu.Unlock()

	return release
}

func (rs *recordingServer) currentBlock() chan struct{} {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	return rs.block
}

func (rs *recordingServer) currentRelease() func() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	return rs.release
}

// waitRequest waits until a request reaches the handler.
func (rs *recordingServer) waitRequest(t *testing.T) {
	t.Helper()

	select {
	case <-rs.arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for a push request")
	}
}

func (rs *recordingServer) received() []receivedPayload {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	return append([]receivedPayload(nil), rs.payloads...)
}

func (rs *recordingServer) lines() int {
	total := 0

	for _, payload := range rs.received() {
		for _, stream := range payload.Streams {
			total += len(stream.Values)
		}
	}

	return total
}
