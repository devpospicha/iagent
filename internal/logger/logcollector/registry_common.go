package logcollector

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/devpospicha/ishared/model"
	"github.com/devpospicha/ishared/utils"
)

// Collect calls Collect on all registered collectors and aggregates the results.
func (r *LogRegistry) Collect(ctx context.Context) ([][]model.LogEntry, error) {
	var allBatches [][]model.LogEntry
	for name, collector := range r.LogCollectors {
		batches, err := collector.Collect(ctx)
		if err != nil {
			utils.Error("Error collecting data %s: %v", name, err)
			continue
		}
		allBatches = append(allBatches, batches...)
	}
	return allBatches, nil
}

// Close calls Close on all collectors that implement io.Closer.
func (r *LogRegistry) Close() error {
	utils.Info("Closing log registry and collectors...")
	var errs []string

	for name, collector := range r.LogCollectors {
		if closer, ok := collector.(io.Closer); ok {
			utils.Debug("Closing collector: %s", name)
			if err := closer.Close(); err != nil {
				errMsg := fmt.Sprintf("Error closing collector %s: %v", name, err)
				utils.Error(errMsg)
				errs = append(errs, errMsg)
			}
		} else {
			utils.Debug("Collector %s does not implement io.Closer, skipping Close()", name)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing collectors: %s", strings.Join(errs, "; "))
	}

	utils.Info("All closable collectors closed.")
	return nil
}
