This is loki transport for https://github.com/akademic/go-logger2

## Usage

```go
transport := loki.New(loki.Config{
	Address: "http://loki:3100",
	Timeout: 5 * time.Second,
	Labels:  map[string]string{"app": "myapp"},
})
```

## Runtime labels

The label set from `Config.Labels` is the starting point and can be replaced at any
time after `New()`. Both methods are safe for concurrent use with logging.

```go
transport.SetLabels(map[string]string{"app": "myapp"})    // replace the whole set
current := transport.Labels()                             // copy of the current set
```

Per-message labels still take precedence: if a logged value implements
`Labels() map[string]string`, its labels are merged over the logger ones for that
message only.
