package logcollector

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/devpospicha/iagent/internal/config"
	linuxcollector "github.com/devpospicha/iagent/internal/logger/logcollector/linuxos"
	macos "github.com/devpospicha/iagent/internal/logger/logcollector/macos"
	windowscollector "github.com/devpospicha/iagent/internal/logger/logcollector/windows"

	"github.com/devpospicha/ishared/model"
	"github.com/devpospicha/ishared/utils"
)

type LogRegistry struct {
	LogCollectors map[string]Collector
}

func NewRegistry(cfg *config.Config) *LogRegistry {
	reg := &LogRegistry{LogCollectors: make(map[string]Collector)}

	for _, name := range cfg.Agent.LogCollection.Sources {
		switch name {
		case "journald":
			if runtime.GOOS == "linux" {
				reg.LogCollectors["journald"] = linuxcollector.NewJournaldCollector(cfg)
			} else {
				utils.Warn("journald collector is only supported on Linux (skipping)")
			}
		case "security":
			if runtime.GOOS == "linux" {
				reg.LogCollectors["security"] = linuxcollector.NewSecurityLogCollector(cfg)
			} else {
				utils.Warn("security collector is only supported on Linux (skipping)")
			}
		case "eventviewer":
			if runtime.GOOS == "windows" {
				reg.LogCollectors["eventviewer"] = windowscollector.NewEventViewerCollector(cfg)
			} else {
				utils.Warn("eventviewer collector is only supported on Windows (skipping)")
			}
		case "macos":
			if runtime.GOOS == "darwin" {
				reg.LogCollectors["macos"] = macos.NewMacOSCollector(cfg)
			} else {
				utils.Warn("macos collector is only supported on macOS (skipping)")
			}
		default:
			utils.Warn("Unknown collector: %s (skipping)", name)
		}
	}

	utils.Info("Loaded %d log collectors", len(reg.LogCollectors))
	return reg
}

func (r *LogRegistry) Collect(ctx context.Context) ([][]model.LogEntry, error) {
	var allBatches [][]model.LogEntry
	for name, collector := range r.LogCollectors {
		batches, err := collector.Collect(ctx)
		if err != nil {
			utils.Error("Error collecting %s: %v", name, err)
			continue
		}
		allBatches = append(allBatches, batches...)
	}
	return allBatches, nil
}

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
