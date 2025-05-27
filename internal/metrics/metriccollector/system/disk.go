package system

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	agentutils "github.com/devpospicha/iagent/internal/utils"
	"github.com/devpospicha/ishared/model"
	"github.com/shirou/gopsutil/v4/disk"
)

type DiskCollector struct{}

// NewDiskCollector creates a new DiskCollector instance.
// It uses the gopsutil library to gather disk metrics.
// The collector is platform-neutral and will skip virtual, pseudo, and temp filesystems on Linux/macOS.
// On Windows, it will skip reserved, empty, mapped, and unmounted drives.
// The collector gathers metrics such as total, used, free space, used percentage, inodes total, used, free, and used percentage.
// It also collects disk I/O metrics such as read/write counts, bytes, time, and merged counts.
// The metrics are returned as a slice of model.Metric.
func NewDiskCollector() *DiskCollector {
	return &DiskCollector{}
}

// Name returns the name of the collector.
// This is used for logging and debugging purposes.
func (c *DiskCollector) Name() string {
	return "disk"
}

// Collect gathers disk metrics using the gopsutil library.
// It retrieves information about disk partitions, usage, and I/O statistics.
// The metrics are filtered based on the platform (Linux/macOS/Windows) to exclude virtual, pseudo, and temp filesystems.
// The function returns a slice of model.Metric containing the collected metrics.
func (c *DiskCollector) Collect(_ context.Context) ([]model.Metric, error) {
	var metrics []model.Metric
	now := time.Now()

	partitions, err := disk.Partitions(false)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk partitions: %w", err)
	}

	for _, p := range partitions {
		// Platform-neutral filtering
		if runtime.GOOS != "windows" {
			// Linux/macOS: skip virtual, pseudo, temp filesystems
			if strings.HasPrefix(p.Mountpoint, "/sys") ||
				strings.HasPrefix(p.Mountpoint, "/proc") ||
				strings.HasPrefix(p.Mountpoint, "/run") ||
				p.Fstype == "tmpfs" || p.Fstype == "devtmpfs" || p.Fstype == "overlay" {
				continue
			}
		} else {
			// Windows: skip reserved/empty/mapped/unmounted drives
			if p.Fstype == "" || p.Mountpoint == "" {
				continue
			}
		}

		usage, err := disk.Usage(p.Mountpoint)
		if err != nil || usage == nil {
			continue
		}

		dims := map[string]string{
			"mountpoint": p.Mountpoint,                            // e.g. "/", "/data", or "C:\"
			"device":     strings.TrimPrefix(p.Device, "\\\\.\\"), /* Windows-style */
			"fstype":     p.Fstype,
		}

		metrics = append(metrics,
			agentutils.Metric("System", "Disk", "total", usage.Total, "gauge", "bytes", dims, now),
			agentutils.Metric("System", "Disk", "used", usage.Used, "gauge", "bytes", dims, now),
			agentutils.Metric("System", "Disk", "free", usage.Free, "gauge", "bytes", dims, now),
			agentutils.Metric("System", "Disk", "used_percent", usage.UsedPercent, "gauge", "percent", dims, now),
			agentutils.Metric("System", "Disk", "inodes_total", usage.InodesTotal, "gauge", "count", dims, now),
			agentutils.Metric("System", "Disk", "inodes_used", usage.InodesUsed, "gauge", "count", dims, now),
			agentutils.Metric("System", "Disk", "inodes_free", usage.InodesFree, "gauge", "count", dims, now),
			agentutils.Metric("System", "Disk", "inodes_used_percent", usage.InodesUsedPercent, "gauge", "percent", dims, now),
		)

	}

	if ioCounters, err := disk.IOCounters(); err == nil {
		for device, io := range ioCounters {
			dims := map[string]string{
				"device":        device,
				"serial_number": io.SerialNumber,
			}

			metrics = append(metrics,
				agentutils.Metric("System", "DiskIO", "read_count", io.ReadCount, "counter", "count", dims, now),
				agentutils.Metric("System", "DiskIO", "write_count", io.WriteCount, "counter", "count", dims, now),
				agentutils.Metric("System", "DiskIO", "read_bytes", io.ReadBytes, "counter", "bytes", dims, now),
				agentutils.Metric("System", "DiskIO", "write_bytes", io.WriteBytes, "counter", "bytes", dims, now),
				agentutils.Metric("System", "DiskIO", "read_time", io.ReadTime, "counter", "milliseconds", dims, now),
				agentutils.Metric("System", "DiskIO", "write_time", io.WriteTime, "counter", "milliseconds", dims, now),
				agentutils.Metric("System", "DiskIO", "io_time", io.IoTime, "counter", "milliseconds", dims, now),
				agentutils.Metric("System", "DiskIO", "merged_read_count", io.MergedReadCount, "counter", "count", dims, now),
				agentutils.Metric("System", "DiskIO", "merged_write_count", io.MergedWriteCount, "counter", "count", dims, now),
				agentutils.Metric("System", "DiskIO", "weighted_io", io.WeightedIO, "counter", "milliseconds", dims, now),
			)
		}
	}

	return metrics, nil
}
