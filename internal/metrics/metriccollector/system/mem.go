package system

import (
	"context"
	"math"
	"time"

	agentutils "github.com/devpospicha/iagent/internal/utils"
	"github.com/devpospicha/ishared/model"
	"github.com/devpospicha/ishared/utils"
	"github.com/shirou/gopsutil/v4/mem"
)

type MEMCollector struct{}

// NewMemCollector creates a new MEMCollector instance.
// It initializes the collector and returns a pointer to it.
func NewMemCollector() *MEMCollector {
	return &MEMCollector{}
}

// Name returns the name of the collector.
// This is used to identify the collector in logs and metrics.
func (c *MEMCollector) Name() string {
	return "mem"
}

// Collect gathers memory metrics and returns them as a slice of model.Metric.
// It uses the gopsutil library to get virtual and swap memory information.
// The metrics include total, available, used memory, and swap memory details.
func (c *MEMCollector) Collect(_ context.Context) ([]model.Metric, error) {
	var metrics []model.Metric
	now := time.Now()

	// --- Virtual Memory ---
	memory, err := mem.VirtualMemory()
	if err != nil {
		utils.Warn("Error getting memory info: %v", err)
	} else if memory != nil {
		dims := map[string]string{"source": "physical"}

		metrics = append(metrics,
			agentutils.Metric("System", "Memory", "total", memory.Total, "gauge", "bytes", dims, now),
			agentutils.Metric("System", "Memory", "available", memory.Available, "gauge", "bytes", dims, now),
			agentutils.Metric("System", "Memory", "used", memory.Used, "gauge", "bytes", dims, now),
			agentutils.Metric("System", "Memory", "used_percent", memory.UsedPercent, "gauge", "percent", dims, now),
		)
	}

	// --- Swap Memory ---
	swap, err := mem.SwapMemory()
	if err != nil {
		utils.Warn("Error getting swap memory info: %v", err)
	} else if swap != nil && swap.Total > 0 {
		dims := map[string]string{"source": "swap"}

		usedPercent := swap.UsedPercent
		if usedPercent <= 0 {
			usedPercent = float64(swap.Total-swap.Free) / float64(swap.Total) * 100
		}

		if math.IsNaN(usedPercent) || math.IsInf(usedPercent, 0) {

			usedPercent = 0
		}

		metrics = append(metrics,
			agentutils.Metric("System", "Memory", "swap_total", swap.Total, "gauge", "bytes", dims, now),
			agentutils.Metric("System", "Memory", "swap_used", swap.Used, "gauge", "bytes", dims, now),
			agentutils.Metric("System", "Memory", "swap_free", swap.Free, "gauge", "bytes", dims, now),
			agentutils.Metric("System", "Memory", "swap_used_percent", usedPercent, "gauge", "percent", dims, now),
		)
	} else {
		utils.Debug("Swap metrics skipped — no swap memory available.")
	}

	return metrics, nil
}
