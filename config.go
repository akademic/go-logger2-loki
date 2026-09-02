package loki

import (
	"net/http"
	"time"
)

const (
	// DefaultIdleConnTimeout must stay below the keepalive timeout of any reverse
	// proxy in front of Loki (nginx keepalive_timeout is 65s by default). A
	// connection already closed by the server but still kept in the client pool
	// gives EOF on the next request, and net/http can not retry a POST
	DefaultIdleConnTimeout = 30 * time.Second

	// DefaultMaxIdleConnsPerHost replaces the net/http default of 2, which makes
	// log delivery from many goroutines reopen connections all the time
	DefaultMaxIdleConnsPerHost = 100

	// DefaultBatchMaxEntries is the number of log lines packed into one push
	// request when batching is on
	DefaultBatchMaxEntries = 1000

	// DefaultBatchQueueSize is the number of log lines kept in memory while a push
	// request is in flight. Lines above the limit are dropped
	DefaultBatchQueueSize = 10000
)

type Config struct {
	Address string
	Timeout time.Duration
	Labels  map[string]string

	// IgnoreMessageLabels drops labels reported by logged values via
	// Labels() map[string]string, leaving only the label set of the logger itself
	IgnoreMessageLabels bool

	// IdleConnTimeout and MaxIdleConnsPerHost configure the connection pool of the
	// HTTP client the logger creates. Zero values mean the defaults above
	IdleConnTimeout     time.Duration
	MaxIdleConnsPerHost int

	// HTTPClient replaces the client the logger would create itself. When it is
	// set, Timeout, IdleConnTimeout and MaxIdleConnsPerHost are ignored
	HTTPClient *http.Client

	// BatchWait enables batching: log lines are accumulated and pushed in one
	// request every BatchWait, or earlier once BatchMaxEntries is reached. Zero
	// means no batching — every log line is a separate synchronous push request
	//
	// Batching is also what keeps entries ordered: lines are stamped and buffered
	// under one lock and pushed one request at a time, so Loki sees the timestamps
	// of the process in ascending order. Concurrent single line pushes can not
	// give that guarantee
	//
	// With batching on, delivery happens in a background goroutine, so the last
	// lines of a process are lost unless Close() is called on shutdown.
	BatchWait time.Duration

	// BatchMaxEntries is the size of one push request. Zero means
	// DefaultBatchMaxEntries.
	BatchMaxEntries int

	// BatchQueueSize is how many lines may wait in memory. Zero means
	// DefaultBatchQueueSize. Once the queue is full, lines are dropped and the
	// number of dropped lines is reported through ErrorHandler
	BatchQueueSize int

	// ErrorHandler receives delivery errors. Nil means writing them to stdout
	ErrorHandler func(error)
}

func (c Config) withDefaults() Config {
	if c.IdleConnTimeout <= 0 {
		c.IdleConnTimeout = DefaultIdleConnTimeout
	}

	if c.MaxIdleConnsPerHost <= 0 {
		c.MaxIdleConnsPerHost = DefaultMaxIdleConnsPerHost
	}

	if c.BatchMaxEntries <= 0 {
		c.BatchMaxEntries = DefaultBatchMaxEntries
	}

	if c.BatchQueueSize <= 0 {
		c.BatchQueueSize = DefaultBatchQueueSize
	}

	if c.BatchQueueSize < c.BatchMaxEntries {
		c.BatchQueueSize = c.BatchMaxEntries
	}

	return c
}

func (c Config) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.IdleConnTimeout = c.IdleConnTimeout
	transport.MaxIdleConnsPerHost = c.MaxIdleConnsPerHost

	if transport.MaxIdleConns < c.MaxIdleConnsPerHost {
		transport.MaxIdleConns = c.MaxIdleConnsPerHost
	}

	return &http.Client{
		Timeout:   c.Timeout,
		Transport: transport,
	}
}
