package system

import (
	"context"
	"time"

	agentutils "github.com/devpospicha/iagent/internal/utils"
	"github.com/devpospicha/ishared/model"
	"github.com/devpospicha/ishared/utils"
	"github.com/shirou/gopsutil/v4/net"
)

type NetworkCollector struct{}

// NewNetworkCollector creates a new NetworkCollector instance.
// It initializes the collector and returns a pointer to it.
// This collector gathers network interface I/O statistics using the gopsutil library.
// It collects metrics such as bytes sent, bytes received, packets sent, packets received,
// and errors in/out for each network interface on the system.
func NewNetworkCollector() *NetworkCollector {
	return &NetworkCollector{}
}

// Name returns the name of the collector.
// This is used to identify the collector in logs and metrics.
func (c *NetworkCollector) Name() string {
	return "network"
}

// Collect gathers network interface I/O statistics and returns them as a slice of model.Metric.
// It uses the gopsutil library to get network I/O counters for each interface.
// The metrics include bytes sent, bytes received, packets sent, packets received,
// and errors in/out for each network interface.
func (c *NetworkCollector) Collect(_ context.Context) ([]model.Metric, error) {
	now := time.Now()
	var metrics []model.Metric

	interfaces, err := net.IOCounters(true)
	if err != nil {
		utils.Error("❌ Failed to get network IO counters: %v", err)
		return nil, err
	}

	for _, iface := range interfaces {
		dims := map[string]string{"interface": iface.Name}

		metrics = append(metrics,
			agentutils.Metric("System", "Network", "bytes_sent", iface.BytesSent, "counter", "bytes", dims, now),
			agentutils.Metric("System", "Network", "bytes_recv", iface.BytesRecv, "counter", "bytes", dims, now),
			agentutils.Metric("System", "Network", "packets_sent", iface.PacketsSent, "counter", "count", dims, now),
			agentutils.Metric("System", "Network", "packets_recv", iface.PacketsRecv, "counter", "count", dims, now),
			agentutils.Metric("System", "Network", "err_in", iface.Errin, "counter", "count", dims, now),
			agentutils.Metric("System", "Network", "err_out", iface.Errout, "counter", "count", dims, now),
		)

	}

	return metrics, nil
}
