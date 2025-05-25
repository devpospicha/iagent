/*
SPDX-License-Identifier: GPL-3.0-or-later

Copyright (C) 2025 Aaron Mathis aaron.mathis@gmail.com

This file is part of GoSight.

GoSight is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

GoSight is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with GoSight. If not, see https://www.gnu.org/licenses/.
*/

// gosight/agent/internal/logs/logcollector/registry.go
// registry.go - loads and initializes all enabled log collectors at runtime.

package logcollector

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/devpospicha/iagent/internal/config"
	linuxcollector "github.com/devpospicha/iagent/internal/logs/logcollector/linux"
	windowscollector "github.com/devpospicha/iagent/internal/logs/logcollector/windows"

	"github.com/devpospicha/ishared/model"
	"github.com/devpospicha/ishared/utils"
)

// LogRegistry holds active log collectors keyed by name
// It is responsible for managing the lifecycle of log collectors,
// including their initialization, collection of logs, and closing them when no longer needed.
type LogRegistry struct {
	LogCollectors map[string]Collector
}

// NewRegistry initializes and registers enabled log collectors based on the configuration.
// It creates a new LogRegistry instance and populates it with the specified collectors.
func NewRegistry(cfg *config.Config) *LogRegistry {
	reg := &LogRegistry{LogCollectors: make(map[string]Collector)}

	for _, name := range cfg.Agent.LogCollection.Sources {
		switch name {
		case "journald":
			if runtime.GOOS != "linux" {
				utils.Warn("journald collector is only supported on Linux (skipping) \n")
				continue
			}
			reg.LogCollectors["journald"] = linuxcollector.NewJournaldCollector(cfg)
		case "security":
			if runtime.GOOS != "linux" {
				utils.Warn("journald collector is only supported on Linux (skipping) \n")
				continue
			}
			reg.LogCollectors["security"] = linuxcollector.NewSecurityLogCollector(cfg)
		case "eventviewer":
			if runtime.GOOS == "windows" {
				reg.LogCollectors["eventviewer"] = windowscollector.NewEventViewerCollector(cfg)
			}
		default:
			utils.Warn("Unknown collector: %s (skipping) \n", name)
		}
	}
	utils.Info("Loaded %d log collectors", len(reg.LogCollectors))

	return reg
}

// Collect runs all active collectors and returns all collected metrics as a slice.
// It iterates over the registered collectors, calls their Collect method,
// and appends the resulting metrics to a slice.
func (r *LogRegistry) Collect(ctx context.Context) ([][]model.LogEntry, error) {
	var allBatches [][]model.LogEntry

	for name, collector := range r.LogCollectors {
		logBatches, err := collector.Collect(ctx)
		if err != nil {
			utils.Error("Error collecting %s: %v\n", name, err)
			continue
		}
		allBatches = append(allBatches, logBatches...)
		//utils.Debug("LogRegistry returned %d batches", len(logBatches))
	}

	return allBatches, nil
}

// Close cleans up the resources used by the LogRegistry.
// It closes all log collectors and handles any errors that occur during the closing process.
// It should be called when the LogRegistry is no longer needed.
func (r *LogRegistry) Close() error {
	utils.Info("Closing log registry and collectors...")
	var errs []string // Collect errors non-blockingly

	for name, collector := range r.LogCollectors {
		// Check if the collector implements io.Closer
		if closer, ok := collector.(io.Closer); ok {
			utils.Debug("Closing collector: %s", name)
			if err := closer.Close(); err != nil {
				errMsg := fmt.Sprintf("Error closing collector %s: %v", name, err)
				utils.Error(errMsg)
				errs = append(errs, errMsg)
			}
		} else {
			utils.Debug("Collector %s does not implement io.Closer, skipping Close() call.", name)
		}
	}

	if len(errs) > 0 {
		// Return an aggregated error
		return fmt.Errorf("encountered errors closing collectors: %s", strings.Join(errs, "; "))
	}

	utils.Info("All closable collectors closed.")
	return nil
}
