package metriccollector

import (
	"context"

	"github.com/devpospicha/ishared/model"
)

// Collector is the interface that all metric collectors must implement.
type MetricCollector interface {
	Name() string
	Collect(ctx context.Context) ([]model.Metric, error)
}
