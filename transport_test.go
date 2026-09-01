package loki

import (
	"net/http"
	"testing"
	"time"
)

// TestPrintReusesConnection checks that the logger keeps one HTTP client for the
// whole process instead of creating one per log line.
func TestPrintReusesConnection(t *testing.T) {
	server := newCountingServer(t, http.StatusNoContent)

	logger := New(Config{
		Address: server.URL,
		Timeout: 5 * time.Second,
		Labels:  map[string]string{"app": "test"},
	})

	for range 5 {
		logger.Print("test log message")
	}

	if got := server.requests.Load(); got != 5 {
		t.Errorf("Expected 5 requests, got %d", got)
	}

	if got := server.connections.Load(); got != 1 {
		t.Errorf("Expected 1 connection for 5 log lines, got %d", got)
	}
}

// TestHTTPClientOverride checks that a caller supplied client is used as is.
func TestHTTPClientOverride(t *testing.T) {
	server := newCountingServer(t, http.StatusNoContent)

	client := &http.Client{Timeout: time.Second}
	logger := New(Config{Address: server.URL, HTTPClient: client})

	if logger.client != client {
		t.Fatal("Expected the client from config to be used")
	}

	logger.Print("test log message")

	if got := server.requests.Load(); got != 1 {
		t.Errorf("Expected 1 request, got %d", got)
	}
}

// TestClientPoolDefaults checks the connection pool settings the logger applies
// when it creates the client itself.
func TestClientPoolDefaults(t *testing.T) {
	testCases := []struct {
		name                        string
		config                      Config
		expectedIdleConnTimeout     time.Duration
		expectedMaxIdleConnsPerHost int
	}{
		{
			name:                        "zero values fall back to defaults",
			config:                      Config{},
			expectedIdleConnTimeout:     DefaultIdleConnTimeout,
			expectedMaxIdleConnsPerHost: DefaultMaxIdleConnsPerHost,
		},
		{
			name: "explicit values win",
			config: Config{
				IdleConnTimeout:     10 * time.Second,
				MaxIdleConnsPerHost: 7,
			},
			expectedIdleConnTimeout:     10 * time.Second,
			expectedMaxIdleConnsPerHost: 7,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logger := New(tc.config)

			transport, ok := logger.client.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("Expected *http.Transport, got %T", logger.client.Transport)
			}

			if transport.IdleConnTimeout != tc.expectedIdleConnTimeout {
				t.Errorf(
					"Expected IdleConnTimeout %s, got %s",
					tc.expectedIdleConnTimeout, transport.IdleConnTimeout,
				)
			}

			if transport.MaxIdleConnsPerHost != tc.expectedMaxIdleConnsPerHost {
				t.Errorf(
					"Expected MaxIdleConnsPerHost %d, got %d",
					tc.expectedMaxIdleConnsPerHost, transport.MaxIdleConnsPerHost,
				)
			}
		})
	}
}

// TestErrorHandlerReceivesDeliveryError checks that delivery errors go to the
// handler from the config instead of stdout.
func TestErrorHandlerReceivesDeliveryError(t *testing.T) {
	server := newCountingServer(t, http.StatusInternalServerError)

	var errors []error
	logger := New(Config{
		Address:      server.URL,
		ErrorHandler: func(err error) { errors = append(errors, err) },
	})

	logger.Print("test log message")

	if len(errors) != 1 {
		t.Fatalf("Expected 1 error, got %d: %v", len(errors), errors)
	}
}
