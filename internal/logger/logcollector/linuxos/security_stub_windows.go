//go:build windows
// +build windows

package linuxos

import (
	"context"

	"github.com/devpospicha/iagent/internal/config"
	"github.com/devpospicha/ishared/model"
)

// SecurityLogCollector is a no-op stub for Windows.
type SecurityLogCollector struct{}

// Name returns the collector name.
func (c *SecurityLogCollector) Name() string {
	return "security"
}

// NewSecurityLogCollector returns a disabled stub.
func NewSecurityLogCollector(cfg *config.Config) *SecurityLogCollector {
	return &SecurityLogCollector{}
}

// Collect returns no logs on Windows.
func (c *SecurityLogCollector) Collect(ctx context.Context) ([][]model.LogEntry, error) {
	return nil, nil
}

// Close is a no-op.
func (c *SecurityLogCollector) Close() {}
