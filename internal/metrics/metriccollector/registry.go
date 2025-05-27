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
			if cfg.Podman.Enabled {
				reg.Collectors["podman"] = container.NewPodmanCollectorWithSocket(cfg.Podman.Socket)
			}

		case "docker":
			if cfg.Docker.Enabled {
				reg.Collectors["docker"] = container.NewDockerCollector()
			}

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
			utils.Error(" Error collecting metric data %s: %v\n", name, err)
			continue
		}
		all = append(all, metrics...)
	}
	//utils.Info("MetricRegistry %vs ", all)
	return all, nil
}
