// devpospicha/agent/internal/logs/logcollector/collector.go
// Package logcollector provides the Collector interface for all metric collectors.
package logcollector

import (
	"context"

	"github.com/devpospicha/ishared/model"
)

// Collector is an interface that defines the methods for a log collector.
// It includes methods for collecting logs and returning them in a specific format.
type Collector interface {
	Name() string
	Collect(ctx context.Context) ([][]model.LogEntry, error)
	Close() error // add this to ensure each collector implements a close method
}
