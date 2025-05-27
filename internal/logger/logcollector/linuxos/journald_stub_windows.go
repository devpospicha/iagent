//go:build windows
// +build windows

package linuxos

import (
	"context"
	"io"

	"github.com/devpospicha/iagent/internal/config"
	"github.com/devpospicha/ishared/model"
)

// JournaldCollector is a no-op stub for Windows.
type JournaldCollector struct{}

// Name returns the name of the stub collector.
func (j *JournaldCollector) Name() string {
	return "journald"
}

// NewJournaldCollector returns a disabled stub collector.
func NewJournaldCollector(cfg *config.Config) *JournaldCollector {
	return &JournaldCollector{}
}

// Collect returns no logs on Windows.
func (j *JournaldCollector) Collect(ctx context.Context) ([][]model.LogEntry, error) {
	return nil, nil
}

// Close is a no-op on Windows.
func (j *JournaldCollector) Close() error {
	return nil
}

// Ensure JournaldCollector implements io.Closer
var _ io.Closer = (*JournaldCollector)(nil)
