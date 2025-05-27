//go:build windows
// +build windows

package logcollector

import (
	"github.com/devpospicha/iagent/internal/config"
	windowscollector "github.com/devpospicha/iagent/internal/logger/logcollector/windows"

	"github.com/devpospicha/ishared/utils"
)

type LogRegistry struct {
	LogCollectors map[string]Collector
}

func NewRegistry(cfg *config.Config) *LogRegistry {
	reg := &LogRegistry{LogCollectors: make(map[string]Collector)}

	for _, name := range cfg.Agent.LogCollection.Sources {
		switch name {
		case "eventviewer":
			reg.LogCollectors["eventviewer"] = windowscollector.NewEventViewerCollector(cfg)
		default:
			utils.Warn("Unknown collector: %s (skipping)", name)
		}
	}

	utils.Info("Loaded %d log collectors", len(reg.LogCollectors))
	return reg
}
