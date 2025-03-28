package loki

import (
	"encoding/json"
	"testing"
	"time"
)

// TestNew checks if the New function creates a Logger correctly
func TestNew(t *testing.T) {
	config := Config{
		Address: "http://test.loki.com",
		Labels:  map[string]string{"app": "test"},
		Timeout: 5 * time.Second,
	}
	logger := New(config)

	if logger == nil {
		t.Fatalf("New() returned nil logger")
	}

	if logger.config.Address != config.Address {
		t.Errorf("Expected address %s, got %s", config.Address, logger.config.Address)
	}
}

// TestMakePayload checks the payload creation logic
func TestMakePayload(t *testing.T) {
	config := Config{
		Labels: map[string]string{"app": "test"},
	}
	logger := &Logger{config: config}

	logStr := "test log message"
	additionalLabels := map[string]string{"env": "dev"}

	payload, err := logger.makePayload(logStr, additionalLabels)
	if err != nil {
		t.Fatalf("makePayload failed: %v", err)
	}

	var result map[string]any
	err = json.Unmarshal(payload, &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	streams, ok := result["streams"].([]any)
	if !ok || len(streams) == 0 {
		t.Fatal("No streams found in payload")
	}

	stream := streams[0].(map[string]any)

	// Check stream labels
	streamLabels, ok := stream["stream"].(map[string]any)
	if !ok {
		t.Fatal("Stream labels not found")
	}

	expectedLabels := map[string]string{
		"app": "test",
		"env": "dev",
	}
	for k, v := range expectedLabels {
		if streamLabels[k] != v {
			t.Errorf("Expected label %s=%s, got %v", k, v, streamLabels[k])
		}
	}

	// Check values
	values, ok := stream["values"].([]any)
	if !ok || len(values) == 0 {
		t.Fatal("No values found in stream")
	}

	value := values[0].([]any)
	if len(value) != 2 {
		t.Fatalf("Unexpected value format, got %v", value)
	}

	// Check log message
	if value[1] != logStr {
		t.Errorf("Expected log message %s, got %s", logStr, value[1])
	}
}
