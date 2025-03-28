package loki

import (
	"testing"
)

type StringableType struct {
	value string
}

func (s StringableType) String() string {
	return s.value
}

type LabeledType struct {
	value string
}

func (l LabeledType) String() string {
	return l.value
}

func (l LabeledType) Labels() map[string]string {
	return map[string]string{"component": "test"}
}

// TestFormat checks the format function
func TestFormat(t *testing.T) {
	config := Config{
		Labels: map[string]string{"app": "test"},
	}
	logger := New(config)

	testCases := []struct {
		name           string
		inputs         []any
		expectedLogStr string
		expectedLabels map[string]string
	}{
		{
			name:           "Simple string",
			inputs:         []any{"hello world"},
			expectedLogStr: "hello world",
			expectedLabels: map[string]string{},
		},
		{
			name:           "Multiple inputs",
			inputs:         []any{"log", 42, true},
			expectedLogStr: "log 42 true",
			expectedLabels: map[string]string{},
		},
		{
			name:           "With Stringer",
			inputs:         []any{StringableType{value: "custom string"}},
			expectedLogStr: "custom string",
			expectedLabels: map[string]string{},
		},
		{
			name:           "With Labeler",
			inputs:         []any{LabeledType{value: "labeled log"}},
			expectedLogStr: "labeled log",
			expectedLabels: map[string]string{"component": "test"},
		},
		{
			name:           "Mixed types",
			inputs:         []any{StringableType{value: "custom"}, LabeledType{value: "log"}, 123},
			expectedLogStr: "custom log 123",
			expectedLabels: map[string]string{"component": "test"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logStr, addLabels := logger.format(tc.inputs...)

			if logStr != tc.expectedLogStr {
				t.Errorf("Expected log string %q, got %q", tc.expectedLogStr, logStr)
			}

			for k, v := range tc.expectedLabels {
				if val, exists := addLabels[k]; !exists || val != v {
					t.Errorf("Expected label %s=%s, got %v", k, v, addLabels[k])
				}
			}
		})
	}
}
