package loki

import (
	"fmt"
	"sync"
	"time"
)

// batcher accumulates log lines so that a burst of log calls does not turn into a
// burst of HTTP requests: a batch of a thousand lines is one push request instead
// of a thousand
type batcher struct {
	logger     *Logger
	wait       time.Duration
	maxEntries int
	queueSize  int

	mu            sync.Mutex
	entries       []entry
	lastTimestamp int64
	dropped       int
	closed        bool

	// flushMu serializes pushes, so that the final flush of close() waits for a
	// flush started elsewhere instead of racing with it.
	flushMu sync.Mutex

	flushNow  chan struct{}
	done      chan struct{}
	stopped   chan struct{}
	closeOnce sync.Once
}

func newBatcher(logger *Logger, config Config) *batcher {
	b := &batcher{
		logger:     logger,
		wait:       config.BatchWait,
		maxEntries: config.BatchMaxEntries,
		queueSize:  config.BatchQueueSize,
		flushNow:   make(chan struct{}, 1),
		done:       make(chan struct{}),
		stopped:    make(chan struct{}),
	}

	go b.run()

	return b
}

// add buffers the log line and reports whether the batcher took it over. It
// returns false after close, so that the caller falls back to a synchronous send
func (b *batcher) add(line string, labels map[string]string) bool {
	b.mu.Lock()

	if b.closed {
		b.mu.Unlock()

		return false
	}

	if len(b.entries) >= b.queueSize {
		b.dropped++
		b.mu.Unlock()

		return true
	}

	b.entries = append(b.entries, entry{
		timestamp: b.nextTimestamp(),
		line:      line,
		labels:    labels,
	})
	full := len(b.entries) >= b.maxEntries

	b.mu.Unlock()

	if full {
		select {
		case b.flushNow <- struct{}{}:
		default:
		}
	}

	return true
}

// Must be called with b.mu held.
func (b *batcher) nextTimestamp() int64 {
	timestamp := time.Now().UnixNano()

	if timestamp <= b.lastTimestamp {
		timestamp = b.lastTimestamp + 1
	}

	b.lastTimestamp = timestamp

	return timestamp
}

func (b *batcher) run() {
	defer close(b.stopped)

	ticker := time.NewTicker(b.wait)
	defer ticker.Stop()

	for {
		select {
		case <-b.done:
			b.flush()

			return
		case <-ticker.C:
			b.flush()
		case <-b.flushNow:
			b.flush()
		}
	}
}

// flush sends the whole buffer, splitting it into requests of maxEntries lines.
func (b *batcher) flush() {
	b.flushMu.Lock()
	defer b.flushMu.Unlock()

	for {
		entries, dropped := b.take()

		if dropped > 0 {
			b.logger.reportError(fmt.Errorf("loki buffer overflow: %d log lines dropped", dropped))
		}

		if len(entries) == 0 {
			return
		}

		if err := b.logger.send(entries); err != nil {
			b.logger.reportError(err)
		}
	}
}

// take detaches up to maxEntries buffered lines together with the number of lines
// dropped since the previous call.
func (b *batcher) take() ([]entry, int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	dropped := b.dropped
	b.dropped = 0

	if len(b.entries) == 0 {
		return nil, dropped
	}

	if len(b.entries) <= b.maxEntries {
		entries := b.entries
		b.entries = nil

		return entries, dropped
	}

	entries := b.entries[:b.maxEntries:b.maxEntries]
	b.entries = b.entries[b.maxEntries:]

	return entries, dropped
}

func (b *batcher) close() {
	b.closeOnce.Do(func() {
		b.mu.Lock()
		b.closed = true
		b.mu.Unlock()

		close(b.done)
	})

	<-b.stopped
}
