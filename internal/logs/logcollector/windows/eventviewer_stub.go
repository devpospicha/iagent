//go:build !windows
// +build !windows

package windowscollector

import (
	"context"

	"github.com/devpospicha/iagent/internal/config"
	"github.com/devpospicha/ishared/model"
)

type EventViewerCollector struct{}

func NewEventViewerCollector(_ *config.Config, _ string) *EventViewerCollector {
	return &EventViewerCollector{}
}

func (e *EventViewerCollector) Name() string {
	return "eventviewer (disabled)"
}

func (e *EventViewerCollector) Collect(_ context.Context) ([][]model.LogEntry, error) {
	return nil, nil
}

func (e *EventViewerCollector) Close() error {
	return nil
}
