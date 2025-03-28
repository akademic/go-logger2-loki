package loki

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSendSuccess checks if the send function works correctly
func TestSendSuccess(t *testing.T) {
	// Create a server that successfully responds to requests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		// Check the request body
		var payload map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&payload)
		if err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		if payload == nil {
			t.Error("Payload is nil")
		}
	}))
	defer server.Close()

	config := Config{
		Address: server.URL,
		Labels:  map[string]string{"app": "test"},
		Timeout: 5 * time.Second,
	}
	logger := New(config)

	err := logger.send("test log message", map[string]string{"env": "dev"})
	if err != nil {
		t.Errorf("Unexpected error during send: %v", err)
	}
}

// TestSendError checks error handling in the send function
func TestSendError(t *testing.T) {
	testCases := []struct {
		name          string
		serverStatus  int
		expectedError string
	}{
		{
			name:          "Internal Server Error",
			serverStatus:  http.StatusInternalServerError,
			expectedError: "loki response: 500",
		},
		{
			name:          "Bad Request",
			serverStatus:  http.StatusBadRequest,
			expectedError: "loki response: 400",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.serverStatus)
			}))
			defer server.Close()

			config := Config{
				Address: server.URL,
				Labels:  map[string]string{"app": "test"},
				Timeout: 5 * time.Second,
			}
			logger := New(config)

			err := logger.send("test error log", nil)

			if err == nil {
				t.Error("Expected an error, got nil")
			}

			if err != nil && !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("Expected error containing %q, got: %v", tc.expectedError, err)
			}
		})
	}
}
