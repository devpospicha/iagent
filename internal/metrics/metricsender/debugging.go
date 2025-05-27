package metricsender

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/devpospicha/ishared/model"
)

// AppendMetricsToFile appends the given MetricPayload to a file in JSON format.
// It creates the directory if it doesn't exist.
// The file is opened in append mode, and the payload is written as a newline-delimited JSON object.
// This is useful for debugging purposes, allowing you to inspect the metrics being sent.
func AppendMetricsToFile(payload *model.MetricPayload, filename string) error {
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = file.Write(append(data, '\n')) // newline-delimited JSON
	return err
}
