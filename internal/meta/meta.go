package meta

import (
	"os"
	"runtime"

	"github.com/devpospicha/iagent/internal/config"
	"github.com/devpospicha/ishared/model"
	"github.com/devpospicha/ishared/utils"
	"github.com/shirou/gopsutil/v4/host"
)

// BuildMeta constructs the metadata for the agent, including system information and custom tags.
// It retrieves the hostname, local IP address, and host information using the gopsutil library.
// The metadata includes the agent ID, version, host ID, hostname, IP address, OS details,
// and any additional tags provided in the configuration or as arguments.

func BuildMeta(cfg *config.Config, addTags map[string]string, agentID, agentVersion string) *model.Meta {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
		utils.Warn("Failed to get hostname: %v", err)
	}

	ip := utils.GetLocalIP()
	if ip == "" {
		ip = "unknown"
		utils.Warn("Failed to get local IP address")
	}
	hostInfo, err := host.Info()
	if err != nil {
		utils.Warn("Failed to get host info: %v", err)
		hostInfo = &host.InfoStat{}
	}

	tags := utils.MergeMaps(cfg.CustomTags, addTags)

	meta := &model.Meta{
		AgentID:              agentID,
		AgentVersion:         agentVersion,
		HostID:               hostInfo.HostID,
		Hostname:             hostname,
		IPAddress:            ip,
		OS:                   hostInfo.OS,
		OSVersion:            hostInfo.PlatformVersion,
		Platform:             hostInfo.Platform,
		PlatformFamily:       hostInfo.PlatformFamily,
		PlatformVersion:      hostInfo.PlatformVersion,
		KernelArchitecture:   hostInfo.KernelArch,
		VirtualizationSystem: hostInfo.VirtualizationSystem,
		VirtualizationRole:   hostInfo.VirtualizationRole,
		KernelVersion:        hostInfo.KernelVersion,
		Architecture:         runtime.GOARCH,
		Tags:                 tags,
	}

	return meta
}

// CloneMetaWithTags returns a shallow copy of the base Meta
// but optionally overrides or adds new Tags.
func CloneMetaWithTags(base *model.Meta, extraTags map[string]string) *model.Meta {
	if base == nil {
		return nil
	}

	// Shallow copy the struct
	clone := *base

	// Deep copy and merge the Tags map
	clone.Tags = utils.MergeMaps(base.Tags, extraTags)

	return &clone
}

// BuildContainerMeta builds a container-specific meta object
// It includes additional fields relevant to containerized environments
// such as container ID, image name, and runtime information.
func BuildContainerMeta(cfg *config.Config, addTags map[string]string, agentID, agentVersion string) *model.Meta {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
		utils.Warn("Failed to get hostname: %v", err)
	}

	ip := utils.GetLocalIP()
	if ip == "" {
		ip = "unknown"
		utils.Warn("Failed to get local IP address")
	}

	hostInfo, err := host.Info()
	if err != nil {
		utils.Warn("Failed to get host info: %v", err)
		hostInfo = &host.InfoStat{}
	}

	tags := utils.MergeMaps(cfg.CustomTags, addTags)

	return &model.Meta{
		AgentID:              agentID,
		AgentVersion:         agentVersion,
		HostID:               hostInfo.HostID,
		Hostname:             hostname,
		IPAddress:            ip,
		OS:                   hostInfo.OS,
		OSVersion:            hostInfo.PlatformVersion,
		Platform:             hostInfo.Platform,
		PlatformFamily:       hostInfo.PlatformFamily,
		PlatformVersion:      hostInfo.PlatformVersion,
		KernelArchitecture:   hostInfo.KernelArch,
		VirtualizationSystem: hostInfo.VirtualizationSystem,
		VirtualizationRole:   hostInfo.VirtualizationRole,
		KernelVersion:        hostInfo.KernelVersion,
		Architecture:         runtime.GOARCH,
		Tags:                 tags,
	}
}
