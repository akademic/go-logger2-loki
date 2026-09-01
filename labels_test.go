package loki

import (
	"encoding/json"
	"reflect"
	"sync"
	"testing"
)

func payloadLabels(t *testing.T, payload []byte) map[string]string {
	t.Helper()

	var result struct {
		Streams []struct {
			Stream map[string]string `json:"stream"`
		} `json:"streams"`
	}

	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	if len(result.Streams) == 0 {
		t.Fatal("No streams found in payload")
	}

	return result.Streams[0].Stream
}

func TestLabelsMutations(t *testing.T) {
	testCases := []struct {
		name     string
		initial  map[string]string
		mutate   func(l *Logger)
		expected map[string]string
	}{
		{
			name:     "SetLabels replaces the whole set",
			initial:  map[string]string{"app": "test", "env": "dev"},
			mutate:   func(l *Logger) { l.SetLabels(map[string]string{"host": "node-1"}) },
			expected: map[string]string{"host": "node-1"},
		},
		{
			name:     "SetLabels with nil clears the set",
			initial:  map[string]string{"app": "test"},
			mutate:   func(l *Logger) { l.SetLabels(nil) },
			expected: map[string]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logger := New(Config{Labels: tc.initial})

			tc.mutate(logger)

			if got := logger.Labels(); !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("Expected labels %v, got %v", tc.expected, got)
			}
		})
	}
}

// TestLabelsAreDetachedFromCaller checks that the logger does not share label maps
// with the caller, neither on creation nor on set nor on read.
func TestLabelsAreDetachedFromCaller(t *testing.T) {
	configLabels := map[string]string{"app": "config"}
	logger := New(Config{Labels: configLabels})

	configLabels["app"] = "changed"

	setLabels := map[string]string{"app": "test"}
	logger.SetLabels(setLabels)

	setLabels["app"] = "changed"
	setLabels["env"] = "dev"

	returned := logger.Labels()
	returned["app"] = "changed"

	expected := map[string]string{"app": "test"}
	if got := logger.Labels(); !reflect.DeepEqual(got, expected) {
		t.Errorf("Expected labels %v, got %v", expected, got)
	}
}

// TestPayloadUsesRuntimeLabels checks that labels changed after New() get into the payload.
func TestPayloadUsesRuntimeLabels(t *testing.T) {
	logger := New(Config{Labels: map[string]string{"app": "test"}})

	logger.SetLabels(map[string]string{"host": "node-1"})

	payload, err := logger.makePayload(testEntries("test log message", map[string]string{"env": "dev"}))
	if err != nil {
		t.Fatalf("makePayload failed: %v", err)
	}

	labels := payloadLabels(t, payload)

	expected := map[string]string{"host": "node-1", "env": "dev"}
	if !reflect.DeepEqual(labels, expected) {
		t.Errorf("Expected labels %v, got %v", expected, labels)
	}
}

// TestIgnoreMessageLabels checks that with IgnoreMessageLabels the stream carries
// only the labels of the logger, so that a high cardinality value reported by a
// logged type can not create a stream per log line.
func TestIgnoreMessageLabels(t *testing.T) {
	server := newRecordingServer(t)

	logger := New(Config{
		Address:             server.URL,
		Labels:              map[string]string{"app": "test"},
		IgnoreMessageLabels: true,
	})

	logger.Print(LabeledType{value: "labeled log"})

	payloads := server.received()
	if len(payloads) != 1 {
		t.Fatalf("Expected 1 payload, got %d", len(payloads))
	}

	expected := map[string]string{"app": "test"}
	if got := payloads[0].Streams[0].Stream; !reflect.DeepEqual(got, expected) {
		t.Errorf("Expected labels %v, got %v", expected, got)
	}

	if got := payloads[0].Streams[0].Values[0][1]; got != "labeled log" {
		t.Errorf("Expected log line %q, got %q", "labeled log", got)
	}
}

// TestLabelsConcurrentAccess checks that label mutations and log sending are race free.
// Meaningful under -race.
func TestLabelsConcurrentAccess(t *testing.T) {
	logger := New(Config{Labels: map[string]string{"app": "test"}})

	const goroutines = 20

	wg := sync.WaitGroup{}
	wg.Add(goroutines * 3)

	for i := range goroutines {
		go func() {
			defer wg.Done()
			logger.SetLabels(map[string]string{"app": "test", "host": "node"})
			logger.SetLabels(map[string]string{"app": "test"})
		}()

		go func() {
			defer wg.Done()
			logger.Labels()
		}()

		go func() {
			defer wg.Done()
			if _, err := logger.makePayload(testEntries("log", map[string]string{"i": string(rune(i))})); err != nil {
				t.Errorf("makePayload failed: %v", err)
			}
		}()
	}

	wg.Wait()
}
