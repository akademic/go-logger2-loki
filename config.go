package loki

import "time"

type Config struct {
	Address string
	Timeout time.Duration
	Labels  map[string]string
}
