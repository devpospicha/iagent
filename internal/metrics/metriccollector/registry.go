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

// gosight/agent/internal/collector/registry.go
// registry.go - loads and initializes all enabled collectors at runtime.

package metriccollector

import (
	"context"

	"github.com/devpospicha/iagent/internal/config"
	"github.com/devpospicha/iagent/internal/metrics/metriccollector/container"
	"github.com/devpospicha/iagent/internal/metrics/metriccollector/system"
	"github.com/devpospicha/ishared/model"
	"github.com/devpospicha/ishared/utils"
)

// Registry holds active collectors keyed by name
type MetricRegistry struct {
	Collectors map[string]MetricCollector
}

// NewRegistry initializes and registers enabled collectors based on the configuration.
// It creates a new MetricRegistry instance and populates it with the specified collectors.
// The collectors are created based on the configuration settings and are stored in a map.
// The function returns a pointer to the MetricRegistry instance.
// It also logs the number of loaded collectors for debugging purposes.
func NewRegistry(cfg *config.Config) *MetricRegistry {
	reg := &MetricRegistry{Collectors: make(map[string]MetricCollector)}

	for _, name := range cfg.Agent.MetricCollection.Sources {
		switch name {
		case "cpu":
			reg.Collectors["cpu"] = system.NewCPUCollector(cfg.Agent.MetricCollection.Interval)
		case "mem":
			reg.Collectors["mem"] = system.NewMemCollector()
		case "disk":
			reg.Collectors["disk"] = system.NewDiskCollector()
		case "host":
			reg.Collectors["host"] = system.NewHostCollector()
		case "net":
			reg.Collectors["net"] = system.NewNetworkCollector()
		case "podman":
			reg.Collectors["podman"] = container.NewPodmanCollectorWithSocket(cfg.Podman.Socket)
		case "docker":
			reg.Collectors["docker"] = container.NewDockerCollector()
		default:
			utils.Warn(" Unknown collector: %s (skipping) \n", name)
		}
	}
	utils.Info("Loaded %d metric collectors", len(reg.Collectors))

	return reg
}

// Collect runs all active collectors and returns all collected metrics
func (r *MetricRegistry) Collect(ctx context.Context) ([]model.Metric, error) {
	var all []model.Metric

	for name, collector := range r.Collectors {
		metrics, err := collector.Collect(ctx)
		if err != nil {
			utils.Error(" Error collecting %s: %v\n", name, err)
			continue
		}
		all = append(all, metrics...)
	}

	return all, nil
}
