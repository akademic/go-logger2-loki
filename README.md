This is loki transport for https://github.com/akademic/go-logger2

## Usage

```go
transport := loki.New(loki.Config{
	Address: "http://loki:3100",
	Timeout: 5 * time.Second,
	Labels:  map[string]string{"app": "myapp"},
})
```

The logger creates one HTTP client for its whole lifetime, so connections to Loki
are reused. The pool can be tuned, and `HTTPClient` replaces the client entirely
(then `Timeout`, `IdleConnTimeout` and `MaxIdleConnsPerHost` are ignored):

```go
transport := loki.New(loki.Config{
	Address:             "http://loki:3100",
	Timeout:             10 * time.Second,
	IdleConnTimeout:     30 * time.Second, // default, see below
	MaxIdleConnsPerHost: 100,              // default, net/http itself allows 2
	Labels:              map[string]string{"app": "myapp"},
})
```

## Runtime labels

The label set from `Config.Labels` is the starting point and can be replaced at any
time after `New()`. Both methods are safe for concurrent use with logging.

```go
transport.SetLabels(map[string]string{"app": "myapp"})    // replace the whole set
current := transport.Labels()                             // copy of the current set
```

Per-message labels take precedence: if a logged value implements
`Labels() map[string]string`, its labels are merged over the logger ones for that
message only. `go-logger2` uses this to report the component of every log line.

`IgnoreMessageLabels` turns per-message labels off:

```go
transport := loki.New(loki.Config{
	Address:             "http://loki:3100",
	Labels:              map[string]string{"app": "myapp"},
	IgnoreMessageLabels: true,
})
```

## Batching

Without batching every log line is a separate synchronous POST. A service that
processes a batch of a thousand items and writes three lines per item therefore
makes three thousand requests. `BatchWait` turns batching on: lines are collected
and pushed in one request every `BatchWait`, or earlier once `BatchMaxEntries`
lines pile up. Lines with different label sets go into one request as separate
streams.

```go
transport := loki.New(loki.Config{
	Address:         "http://loki:3100",
	Labels:          map[string]string{"app": "myapp"},
	BatchWait:       time.Second,
	BatchMaxEntries: 1000,  // default, size of one push request
	BatchQueueSize:  10000, // default, lines kept in memory while a push is in flight
})
defer transport.Close()
```

Delivery moves to a background goroutine, so on shutdown the service must call
`Close()`: it flushes the buffer, stops the sender and returns when everything is
delivered. The logger stays usable afterwards and sends every line synchronously,
so logs written during shutdown are not lost. `Flush()` delivers the buffer without
stopping the sender.

Once `BatchQueueSize` is reached (Loki is down or too slow) new lines are dropped,
and the number of dropped lines is reported as an error.

## Delivery errors

By default delivery errors are written to stdout. `ErrorHandler` receives them
instead — for example to count them in a metric:

```go
transport := loki.New(loki.Config{
	Address:      "http://loki:3100",
	ErrorHandler: func(err error) { metrics.LokiErrors.Inc() },
})
```
