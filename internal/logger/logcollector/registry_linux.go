//go:build linux
// +build linux

package logcollector

import (
	"github.com/devpospicha/iagent/internal/config"
	linuxcollector "github.com/devpospicha/iagent/internal/logger/logcollector/linuxos"

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
			reg.LogCollectors["journald"] = linuxcollector.NewJournaldCollector(cfg)
		case "security":
			reg.LogCollectors["security"] = linuxcollector.NewSecurityLogCollector(cfg)
		default:
			utils.Warn("Unknown collector: %s (skipping)", name)
		}
	}

	utils.Info("Loaded %d log collectors", len(reg.LogCollectors))
	return reg
}
