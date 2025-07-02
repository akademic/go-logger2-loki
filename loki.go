package loki

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type Logger struct {
	config Config
}

type logLabeler interface {
	stringer
	Labels() map[string]string
}

type stringer interface {
	String() string
}

func New(config Config) *Logger {
	return &Logger{
		config: config,
	}
}

func (l Logger) Print(v ...any) {
	logStr, addLabels := l.format(v...)

	err := l.send(logStr, addLabels)
	if err != nil {
		os.Stdout.Write([]byte(err.Error() + "\n"))
	}
}

func (l Logger) format(v ...any) (string, map[string]string) {
	addLabels := make(map[string]string)
	var logStr string

	for _, item := range v {
		var stringerObj stringer
		var ok bool

		itemLogStr := fmt.Sprintf("%v", item)

		if stringerObj, ok = item.(stringer); ok {
			itemLogStr = stringerObj.String()
		}

		if logobj, ok := item.(logLabeler); ok {
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

func (l Logger) send(logStr string, addLabels map[string]string) error {
	jsonData, err := l.makePayload(logStr, addLabels)
	if err != nil {
		return fmt.Errorf("makePayload error: %v", err)
	}

	req, err := http.NewRequest("POST", l._url(), bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("create post request to loki: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// send the HTTP request
	client := &http.Client{
		Timeout: l.config.Timeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send post request to loki: %w", err)
	}
	defer resp.Body.Close()

	// handle the HTTP response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("loki response: %d\t request was: %s", resp.StatusCode, string(jsonData))
	}

	return nil
}

func (l Logger) makePayload(logStr string, addLabels map[string]string) ([]byte, error) {
	type Stream struct {
		Stream map[string]string `json:"stream"`
		Values [][]string        `json:"values"`
	}

	type Payload struct {
		Streams []Stream `json:"streams"`
	}

	labels := make(map[string]string)
	for k, v := range l.config.Labels {
		labels[k] = v
	}

	if addLabels != nil {
		for k, v := range addLabels {
			labels[k] = v
		}
	}

	data := Payload{
		Streams: []Stream{
			{
				Stream: labels,
				Values: [][]string{
					{fmt.Sprintf("%d", time.Now().UnixNano()), logStr},
				},
			},
		},
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("can't marshal json to loki: %w", err)
	}

	return jsonData, err
}

func (l Logger) _url() string {
	return l.config.Address + "/loki/api/v1/push"
}
