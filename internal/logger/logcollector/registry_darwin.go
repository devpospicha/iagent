//go:build darwin
// +build darwin

package logcollector

import (
	"github.com/devpospicha/iagent/internal/config"
	macos "github.com/devpospicha/iagent/internal/logger/logcollector/macos"
	"github.com/devpospicha/ishared/utils"
)

type LogRegistry struct {
	LogCollectors map[string]Collector
}

func NewRegistry(cfg *config.Config) *LogRegistry {
	reg := &LogRegistry{LogCollectors: make(map[string]Collector)}

	for _, name := range cfg.Agent.LogCollection.Sources {
		switch name {
		case "macos":
			reg.LogCollectors["macos"] = macos.NewMacOSCollector(cfg)
		default:
			utils.Warn("Unknown collector on macOS: %s (skipping)", name)
		}
	}

	utils.Info("Loaded %d log collectors on macOS", len(reg.LogCollectors))
	return reg
}
