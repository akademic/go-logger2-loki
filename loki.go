package loki

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxErrorResponseLen = 512
	maxErrorRequestLen  = 1024
)

type Logger struct {
	config Config
	client *http.Client

	labelsMu sync.RWMutex
	labels   map[string]string

	batcher *batcher
}

type logLabeler interface {
	stringer
	Labels() map[string]string
}

type stringer interface {
	String() string
}

type entry struct {
	timestamp time.Time
	line      string
	labels    map[string]string
}

func New(config Config) *Logger {
	config = config.withDefaults()

	logger := &Logger{
		config: config,
		client: config.httpClient(),
		labels: copyLabels(config.Labels),
	}

	if config.BatchWait > 0 {
		logger.batcher = newBatcher(logger, config)
	}

	return logger
}

func (l *Logger) Print(v ...any) {
	logStr, addLabels := l.format(v...)

	logEntry := entry{
		timestamp: time.Now(),
		line:      logStr,
		labels:    addLabels,
	}

	if l.batcher != nil && l.batcher.add(logEntry) {
		return
	}

	if err := l.send([]entry{logEntry}); err != nil {
		l.reportError(err)
	}
}

// Flush delivers everything buffered by batching and returns when it is done.
// Without batching there is nothing to flush.
func (l *Logger) Flush() {
	if l.batcher == nil {
		return
	}

	l.batcher.flush()
}

// Close stops the background sender after delivering what is left in the buffer.
// The logger stays usable afterwards and sends every line synchronously, so logs
// written during shutdown are not lost. Safe to call more than once.
func (l *Logger) Close() {
	if l.batcher == nil {
		return
	}

	l.batcher.close()
}

func (l *Logger) format(v ...any) (string, map[string]string) {
	addLabels := make(map[string]string)
	var logStr string

	for _, item := range v {
		var stringerObj stringer
		var ok bool

		itemLogStr := fmt.Sprintf("%v", item)

		if stringerObj, ok = item.(stringer); ok {
			itemLogStr = stringerObj.String()
		}

		if logobj, ok := item.(logLabeler); ok && !l.config.IgnoreMessageLabels {
			itemLabels := logobj.Labels()
			for k, v := range itemLabels {
				addLabels[k] = v
			}
		}

		if logStr == "" {
			logStr = itemLogStr
		} else {
			logStr += " " + itemLogStr
		}
	}

	return logStr, addLabels
}

func (l *Logger) send(entries []entry) error {
	jsonData, err := l.makePayload(entries)
	if err != nil {
		return fmt.Errorf("makePayload error: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, l._url(), bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("create post request to loki: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		return fmt.Errorf("send post request to loki: %w", err)
	}
	defer resp.Body.Close()

	// The body has to be read to the end, otherwise the connection is not returned
	// to the pool and the next request opens a new one.
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseLen))
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf(
			"loki response: %d %s\t request was: %s",
			resp.StatusCode,
			strings.TrimSpace(string(respBody)),
			truncate(string(jsonData), maxErrorRequestLen),
		)
	}

	return nil
}

func (l *Logger) makePayload(entries []entry) ([]byte, error) {
	type Stream struct {
		Stream map[string]string `json:"stream"`
		Values [][]string        `json:"values"`
	}

	type Payload struct {
		Streams []Stream `json:"streams"`
	}

	slices.SortStableFunc(entries, func(a, b entry) int {
		return a.timestamp.Compare(b.timestamp)
	})

	baseLabels := l.currentLabels()

	streams := make([]Stream, 0, 1)
	streamByLabels := make(map[string]int, 1)

	for _, logEntry := range entries {
		// The base label set is the same for the whole payload, so per-message
		// labels alone identify the stream.
		key := labelsKey(logEntry.labels)

		index, ok := streamByLabels[key]
		if !ok {
			labels := copyLabels(baseLabels)
			for k, v := range logEntry.labels {
				labels[k] = v
			}

			index = len(streams)
			streamByLabels[key] = index
			streams = append(streams, Stream{Stream: labels})
		}

		streams[index].Values = append(
			streams[index].Values,
			[]string{strconv.FormatInt(logEntry.timestamp.UnixNano(), 10), logEntry.line},
		)
	}

	data := Payload{
		Streams: streams,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("can't marshal json to loki: %w", err)
	}

	return jsonData, err
}

func (l *Logger) reportError(err error) {
	if l.config.ErrorHandler != nil {
		l.config.ErrorHandler(err)

		return
	}

	os.Stdout.Write([]byte(err.Error() + "\n"))
}

func (l *Logger) _url() string {
	return l.config.Address + "/loki/api/v1/push"
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}

	return s[:limit] + "..."
}
