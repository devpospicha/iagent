//go:build windows
// +build windows

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

// Package windows contains the EventViewer collector for Windows.
//
// This collector is used to collect logs from the Windows Event Viewer.
// It is a wrapper around the wevtapi.dll library.
//
// The EventViewer collector is used to collect logs from the Windows Event Viewer.
// It is a wrapper around the wevtapi.dll library.
//

package windowscollector

import (
	"context"
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/devpospicha/iagent/internal/config"
	"github.com/devpospicha/ishared/model"
	"github.com/devpospicha/ishared/utils"
	"golang.org/x/sys/windows"
)

// WindowsEvent represents the structure of a Windows Event Log entry
type WindowsEvent struct {
	XMLName xml.Name `xml:"Event"`
	System  struct {
		Provider struct {
			Name string `xml:"Name,attr"`
			Guid string `xml:"Guid,attr"`
		} `xml:"Provider"`
		EventID     int    `xml:"EventID"`
		Version     int    `xml:"Version"`
		Level       int    `xml:"Level"`
		Task        int    `xml:"Task"`
		Opcode      int    `xml:"Opcode"`
		Keywords    string `xml:"Keywords"`
		TimeCreated struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
		RecordID int    `xml:"EventRecordID"`
		Channel  string `xml:"Channel"`
		Computer string `xml:"Computer"`
		Security struct {
			UserID string `xml:"UserID,attr"`
		} `xml:"Security"`
		Execution struct {
			ProcessID int `xml:"ProcessID,attr"`
			ThreadID  int `xml:"ThreadID,attr"`
		} `xml:"Execution"`
	} `xml:"System"`
	EventData struct {
		Data []struct {
			Name  string `xml:"Name,attr"`
			Value string `xml:",chardata"`
		} `xml:"Data"`
	} `xml:"EventData"`
	UserData      interface{} `xml:"UserData"`
	RenderingInfo struct {
		Message string `xml:"Message"`
		Level   string `xml:"Level"`
		Task    string `xml:"Task"`
	} `xml:"RenderingInfo"`
}

var (
	modwevtapi             = windows.NewLazySystemDLL("wevtapi.dll")
	procEvtQuery           = modwevtapi.NewProc("EvtQuery")
	procEvtNext            = modwevtapi.NewProc("EvtNext")
	procEvtRender          = modwevtapi.NewProc("EvtRender")
	procEvtClose           = modwevtapi.NewProc("EvtClose")
	procEvtOpenChannelEnum = modwevtapi.NewProc("EvtOpenChannelEnum")
	procEvtNextChannelPath = modwevtapi.NewProc("EvtNextChannelPath")
	procEvtFormatMessage   = modwevtapi.NewProc("EvtFormatMessage")
)

const (
	EvtQueryChannelPath      = 0x1
	EvtQueryForwardDirection = 0x00000001
	EvtRenderEventXml        = 1
	EvtFormatMessageEvent    = 1
)

// channelCollector represents a collector for a single Windows Event Log channel
type channelCollector struct {
	channelName string
	handle      syscall.Handle
	lines       chan model.LogEntry
}

// EventViewerCollector struct implements the LogCollector interface.
type EventViewerCollector struct {
	collectors map[string]*channelCollector
	lines      chan model.LogEntry
	stop       chan struct{}
	wg         sync.WaitGroup
	mu         sync.Mutex // Add mutex for thread safety
	once       sync.Once  // Add once for single execution of Close
	batchSize  int
	maxSize    int
}

// getEventLogChannels returns a list of all available Windows Event Log channels
func getEventLogChannels() ([]string, error) {
	var channels []string

	// Open channel enumerator
	h, _, err := procEvtOpenChannelEnum.Call(0, 0)
	if h == 0 {
		return nil, err
	}
	defer procEvtClose.Call(h)

	// Buffer for channel name (using a reasonably large buffer)
	buffer := make([]uint16, 512)

	for {
		var dwRead uint32
		ret, _, _ := procEvtNextChannelPath.Call(
			h,
			uintptr(len(buffer)),
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(unsafe.Pointer(&dwRead)),
		)

		if ret == 0 {
			// No more channels or error
			break
		}

		channelName := syscall.UTF16ToString(buffer[:dwRead])
		channels = append(channels, channelName)
	}

	return channels, nil
}

// shouldCollectChannel determines if a channel should be collected based on configuration
func shouldCollectChannel(channel string, cfg *config.Config) bool {
	utils.Debug("Checking if channel %s should be collected...", channel)

	// First check exclusions (these apply regardless of collect_all setting)
	for _, pattern := range cfg.Agent.LogCollection.EventViewer.ExcludeChannels {
		if matchesPattern(channel, pattern) {
			utils.Debug("Channel %s matches exclude pattern %s, skipping", channel, pattern)
			return false
		}
	}

	// If collecting all channels, return true unless explicitly excluded above
	if cfg.Agent.LogCollection.EventViewer.CollectAll {
		utils.Debug("Channel %s will be collected (CollectAll=true)", channel)
		return true
	}

	// If not collecting all channels, check if it's in the explicit channels list
	channelLower := strings.ToLower(channel)
	for _, configuredChannel := range cfg.Agent.LogCollection.EventViewer.Channels {
		if strings.ToLower(configuredChannel) == channelLower {
			utils.Debug("Channel %s found in configured channels list", channel)
			return true
		}
	}

	utils.Debug("Channel %s not selected for collection (CollectAll=false and not in channels list)", channel)
	return false
}

// matchesPattern checks if a channel name matches a pattern (supporting basic wildcards)
func matchesPattern(channel, pattern string) bool {
	// Convert glob pattern to regex pattern
	pattern = strings.ReplaceAll(pattern, ".", "\\.")
	pattern = strings.ReplaceAll(pattern, "*", ".*")
	pattern = "^" + pattern + "$"

	matched, err := regexp.MatchString(pattern, channel)
	if err != nil {
		utils.Warn("Invalid pattern %s: %v", pattern, err)
		return false
	}
	return matched
}

// NewEventViewerCollector creates a new EventViewerCollector that monitors configured channels
func NewEventViewerCollector(cfg *config.Config) *EventViewerCollector {
	utils.Debug("Initializing EventViewer collector...")

	channels, err := getEventLogChannels()
	if err != nil {
		utils.Error("Failed to enumerate event log channels: %v", err)
		return nil
	}
	utils.Debug("Found %d total available Windows Event Log channels", len(channels))

	c := &EventViewerCollector{
		collectors: make(map[string]*channelCollector),
		lines:      make(chan model.LogEntry, cfg.Agent.LogCollection.BatchSize*10),
		stop:       make(chan struct{}),
		batchSize:  cfg.Agent.LogCollection.BatchSize,
		maxSize:    cfg.Agent.LogCollection.MessageMax,
	}

	selectedCount := 0
	skippedCount := 0
	// Initialize collectors for each selected channel
	for _, channel := range channels {
		if !shouldCollectChannel(channel, cfg) {
			utils.Debug("Skipping channel %s (not configured for collection)", channel)
			skippedCount++
			continue
		}
		selectedCount++

		namePtr, err := syscall.UTF16PtrFromString(channel)
		if err != nil {
			utils.Error("Invalid channel name %s: %v", channel, err)
			continue
		}

		// Create a query that gets events from 5 minutes ago onwards
		startTime := time.Now().UTC().Add(-5 * time.Minute).Format("2006-01-02T15:04:05.999Z")
		query := fmt.Sprintf("*[System[TimeCreated[@SystemTime>='%s']]]", startTime)
		queryPtr, err := syscall.UTF16PtrFromString(query)
		if err != nil {
			utils.Error("Failed to create query string for channel %s: %v", channel, err)
			continue
		}

		h, _, callErr := procEvtQuery.Call(
			0, // Local computer
			uintptr(unsafe.Pointer(namePtr)),
			uintptr(unsafe.Pointer(queryPtr)),
			uintptr(EvtQueryChannelPath|EvtQueryForwardDirection),
		)
		if h == 0 {
			utils.Error("EvtQuery failed for channel %s: %v", channel, callErr)
			continue
		}

		utils.Debug("Successfully opened channel %s with handle %v and query: %s", channel, h, query)

		collector := &channelCollector{
			channelName: channel,
			handle:      syscall.Handle(h),
			lines:       make(chan model.LogEntry, cfg.Agent.LogCollection.BatchSize*5),
		}

		c.collectors[channel] = collector
		c.wg.Add(1)
		go c.runReader(collector)
		utils.Info("Started collecting from Windows Event Log channel: %s (events from %s onwards)", channel, startTime)
	}

	utils.Info("EventViewer collector initialized with %d channels (selected) and %d channels (skipped). CollectAll=%v",
		selectedCount, skippedCount, cfg.Agent.LogCollection.EventViewer.CollectAll)
	return c
}

// Name returns the name of the collector
func (e *EventViewerCollector) Name() string {
	return "eventviewer"
}

// runReader is a helper function that reads logs from a specific channel
func (e *EventViewerCollector) runReader(collector *channelCollector) {
	utils.Debug("Starting reader for channel: %s", collector.channelName)
	defer e.wg.Done()
	defer func() {
		e.mu.Lock()
		if collector.handle != 0 {
			procEvtClose.Call(uintptr(collector.handle))
			collector.handle = 0
		}
		e.mu.Unlock()
		utils.Debug("Reader stopped for channel: %s", collector.channelName)
	}()

	buffer := make([]uint16, 65536)   // 64KB
	noMoreItems := syscall.Errno(259) // ERROR_NO_MORE_ITEMS

	for {
		// Check stop signal before starting a new batch
		select {
		case <-e.stop:
			utils.Debug("Stopping reader for channel: %s", collector.channelName)
			return
		default:
		}

		var returned uint32
		eventHandles := make([]syscall.Handle, 10)

		// Use a shorter timeout (100ms) to be more responsive to shutdown
		r, _, err := procEvtNext.Call(uintptr(collector.handle), 10, uintptr(unsafe.Pointer(&eventHandles[0])), 100, 0, uintptr(unsafe.Pointer(&returned)))

		if r == 0 {
			if err != nil && err != noMoreItems {
				utils.Debug("Channel %s: EvtNext error: %v", collector.channelName, err)
			}
			// Use shorter sleep and check stop signal
			select {
			case <-e.stop:
				return
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}

		if returned == 0 {
			utils.Debug("Channel %s: No new events found in this iteration", collector.channelName)
			// Use shorter sleep and check stop signal
			select {
			case <-e.stop:
				return
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}

		utils.Debug("Channel %s: Retrieved %d new events", collector.channelName, returned)

		// Process events with frequent stop checks
		for i := uint32(0); i < returned; i++ {
			select {
			case <-e.stop:
				// Clean up remaining handles before returning
				for j := i; j < returned; j++ {
					procEvtClose.Call(uintptr(eventHandles[j]))
				}
				return
			default:
			}

			var used, props uint32
			ret, _, err := procEvtRender.Call(0, uintptr(eventHandles[i]), EvtRenderEventXml, uintptr(len(buffer)*2), uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&props)))
			if ret == 0 {
				utils.Debug("Channel %s: Failed to render event %d: %v", collector.channelName, i, err)
				procEvtClose.Call(uintptr(eventHandles[i]))
				continue
			}

			xml := syscall.UTF16ToString(buffer[:used/2])
			entry := buildLogEntry(xml, e.maxSize, collector.channelName)

			utils.Debug("Channel %s: Processing event ID %s with level %s", collector.channelName, entry.Meta.EventID, entry.Level)

			// Try to send the entry with a timeout
			select {
			case e.lines <- entry:
				utils.Debug("Channel %s: Successfully queued event ID %s", collector.channelName, entry.Meta.EventID)
			case <-e.stop:
				procEvtClose.Call(uintptr(eventHandles[i]))
				// Clean up remaining handles before returning
				for j := i + 1; j < returned; j++ {
					procEvtClose.Call(uintptr(eventHandles[j]))
				}
				return
			default:
				utils.Warn("EventViewer log buffer full for channel %s. Dropping entry.", collector.channelName)
			}
			procEvtClose.Call(uintptr(eventHandles[i]))
		}
	}
}

// Collect collects logs from all channels
func (e *EventViewerCollector) Collect(ctx context.Context) ([][]model.LogEntry, error) {
	var all [][]model.LogEntry
	var batch []model.LogEntry
	batchCount := 0
	totalLogs := 0

	utils.Debug("Starting collection cycle for EventViewer...")

collect:
	for {
		select {
		case log, ok := <-e.lines:
			if !ok {
				utils.Debug("EventViewer lines channel closed")
				break collect
			}

			// Don't add endpoint metadata to individual log entries
			// This will be added at the LogPayload level by the LogRunner
			batch = append(batch, log)
			totalLogs++
			if len(batch) >= e.batchSize {
				all = append(all, batch)
				batchCount++
				utils.Debug("Completed batch %d with %d logs", batchCount, len(batch))
				batch = nil
			}
		case <-ctx.Done():
			utils.Debug("Collection cycle cancelled by context")
			break collect
		default:
			break collect
		}
	}

	if len(batch) > 0 {
		all = append(all, batch)
		batchCount++
	}

	utils.Debug("Collection cycle complete. Total logs: %d, Batches: %d", totalLogs, batchCount)
	return all, nil
}

// Close closes the EventViewerCollector
func (e *EventViewerCollector) Close() error {
	var err error
	e.once.Do(func() {
		utils.Debug("Closing EventViewer collector...")

		// Signal all readers to stop by closing stop channel
		e.mu.Lock()
		if e.stop != nil {
			close(e.stop)
			e.stop = nil
		}
		e.mu.Unlock()

		// Wait for goroutines with a timeout
		done := make(chan struct{})
		go func() {
			e.wg.Wait()
			close(done)
		}()

		// Wait up to 5 seconds for graceful shutdown
		select {
		case <-done:
			utils.Debug("All EventViewer readers stopped gracefully")
		case <-time.After(5 * time.Second):
			utils.Warn("Timeout waiting for EventViewer readers to stop")
		}

		// Cleanup resources regardless of timeout
		e.mu.Lock()
		for name, collector := range e.collectors {
			if collector.handle != 0 {
				utils.Debug("Force closing handle for channel: %s", name)
				procEvtClose.Call(uintptr(collector.handle))
				collector.handle = 0
			}
		}
		e.collectors = nil
		e.mu.Unlock()

		utils.Debug("EventViewer collector closed")
	})
	return err
}

// mapEventLevel maps Windows Event Log levels to standardized GoSight levels
func mapEventLevel(level string) string {
	switch strings.ToLower(level) {
	case "critical", "error":
		return "error"
	case "warning":
		return "warning"
	case "information":
		return "info"
	case "verbose":
		return "debug"
	default:
		return "info" // default to info for unknown levels
	}
}

// eventCategoryMap defines mappings from Windows Event Log channels/providers to GoSight categories
var eventCategoryMap = []struct {
	MatchKey string // lowercased channel or provider name
	Category string // GoSight standard category
}{
	// System category
	{"system", "system"},
	{"microsoft-windows-kernel", "system"},
	{"microsoft-windows-systemevents", "system"},
	{"microsoft-windows-diagnostics", "system"},

	// Auth category
	{"security", "auth"},
	{"microsoft-windows-security-authentication", "auth"},
	{"microsoft-windows-security-auditing", "auth"},
	{"microsoft-windows-security-logon", "auth"},

	// Security category
	{"microsoft-windows-windows defender", "security"},
	{"microsoft-windows-firewall", "security"},
	{"microsoft-windows-applocker", "security"},
	{"microsoft-windows-codeintegrity", "security"},

	// Network category
	{"microsoft-windows-networking", "network"},
	{"microsoft-windows-dhcp", "network"},
	{"microsoft-windows-dns", "network"},
	{"microsoft-windows-tcpip", "network"},
	{"microsoft-windows-networkprofile", "network"},

	// App category
	{"application", "app"},
	{"microsoft-windows-iis", "app"},
	{"microsoft-windows-applicationhost", "app"},
	{"microsoft-windows-appxdeployment", "app"},

	// Container category
	{"microsoft-windows-containers", "container"},
	{"microsoft-windows-hyper-v", "container"},
	{"docker", "container"},

	// Config category
	{"microsoft-windows-grouppolicy", "config"},
	{"microsoft-windows-windowsupdateclient", "config"},
	{"microsoft-windows-servicing", "config"},

	// Audit category
	{"microsoft-windows-audit", "audit"},
	{"microsoft-windows-ntlm", "audit"},
	{"microsoft-windows-certificateservicesclient", "audit"},

	// Scheduler category
	{"microsoft-windows-taskscheduler", "scheduler"},
	{"microsoft-windows-windowsbackup", "scheduler"},
	{"microsoft-windows-bits", "scheduler"},
}

// classifyEventCategory determines the GoSight category based on the Windows Event Log channel and provider
func classifyEventCategory(channel, provider string) string {
	inputs := []string{
		strings.ToLower(channel),
		strings.ToLower(provider),
	}

	for _, input := range inputs {
		for _, mapping := range eventCategoryMap {
			if strings.Contains(input, mapping.MatchKey) {
				return mapping.Category
			}
		}
	}

	return "system" // default fallback category
}

// formatWindowsEvent creates a human-readable message from a Windows Event
func formatWindowsEvent(evt *WindowsEvent) string {
	var parts []string

	// Add basic event information
	parts = append(parts, fmt.Sprintf("Event ID: %d", evt.System.EventID))

	if evt.RenderingInfo.Message != "" {
		// Use the pre-rendered message if available
		parts = append(parts, "Message: "+evt.RenderingInfo.Message)
	} else {
		// Add relevant EventData fields
		for _, data := range evt.EventData.Data {
			if data.Name != "" {
				parts = append(parts, fmt.Sprintf("%s: %s", data.Name, data.Value))
			} else if data.Value != "" {
				parts = append(parts, data.Value)
			}
		}
	}

	// Add process information if available
	if evt.System.Execution.ProcessID > 0 {
		parts = append(parts, fmt.Sprintf("Process: %d", evt.System.Execution.ProcessID))
	}

	// Add user information if available
	if evt.System.Security.UserID != "" {
		parts = append(parts, fmt.Sprintf("User: %s", evt.System.Security.UserID))
	}

	return strings.Join(parts, " | ")
}

// parseEventXML extracts relevant information from the Windows Event XML
func parseEventXML(xmlStr string) (level, provider, channel string, fields map[string]string, timestamp time.Time, message string) {
	fields = make(map[string]string)

	var evt WindowsEvent
	if err := xml.Unmarshal([]byte(xmlStr), &evt); err != nil {
		utils.Warn("Failed to parse event XML: %v", err)
		return "info", "", "", fields, time.Now(), xmlStr
	}

	// Map the level
	level = strconv.Itoa(evt.System.Level)
	provider = evt.System.Provider.Name
	channel = evt.System.Channel

	// Parse timestamp
	if evt.System.TimeCreated.SystemTime != "" {
		if t, err := time.Parse(time.RFC3339Nano, evt.System.TimeCreated.SystemTime); err == nil {
			timestamp = t
		} else {
			timestamp = time.Now()
			utils.Warn("Failed to parse event timestamp: %v", err)
		}
	} else {
		timestamp = time.Now()
	}

	// Add fields
	fields["event_id"] = strconv.Itoa(evt.System.EventID)
	fields["computer"] = evt.System.Computer
	if evt.System.Provider.Guid != "" {
		fields["provider_guid"] = evt.System.Provider.Guid
	}
	if evt.System.Security.UserID != "" {
		fields["user_id"] = evt.System.Security.UserID
	}
	if evt.System.Execution.ProcessID > 0 {
		fields["process_id"] = strconv.Itoa(evt.System.Execution.ProcessID)
	}

	// Extract event data fields
	for _, data := range evt.EventData.Data {
		if data.Name != "" && data.Value != "" {
			fields[strings.ToLower(data.Name)] = data.Value
		}
	}

	// Create human-readable message
	var parts []string

	// Add provider and event ID
	parts = append(parts, fmt.Sprintf("[%s] Event ID %d", provider, evt.System.EventID))

	// Add event data in a readable format
	for _, data := range evt.EventData.Data {
		if data.Name != "" && data.Value != "" {
			parts = append(parts, fmt.Sprintf("%s: %s", data.Name, data.Value))
		}
	}

	// Add process information if available
	if evt.System.Execution.ProcessID > 0 {
		parts = append(parts, fmt.Sprintf("Process: %d", evt.System.Execution.ProcessID))
	}

	// Add user information if available
	if evt.System.Security.UserID != "" {
		parts = append(parts, fmt.Sprintf("User: %s", evt.System.Security.UserID))
	}

	message = strings.Join(parts, " | ")

	return
}

// buildLogEntry builds a LogEntry from an XML string
func buildLogEntry(xml string, maxSize int, channelName string) model.LogEntry {
	// Parse the event XML to extract key information
	level, provider, channel, fields, timestamp, message := parseEventXML(xml)

	// Ensure message is valid UTF-8 and respect size limits
	if !utf8.ValidString(message) {
		message = strings.ToValidUTF8(message, "\uFFFD")
	}
	if maxSize > 0 && len(message) > maxSize {
		message = message[:maxSize] + " [truncated]"
	}

	// Add standard fields
	fields["provider"] = provider
	fields["channel"] = channel

	// Determine the appropriate category
	category := classifyEventCategory(channel, provider)

	// Create tags for quick filtering
	tags := make(map[string]string)
	if eventID, ok := fields["event_id"]; ok {
		tags["event_id"] = eventID
	}
	if computer, ok := fields["computer"]; ok {
		tags["computer"] = computer
	}

	// Add Windows-specific metadata
	meta := &model.LogMeta{
		Platform: "eventviewer",
		Service:  channelName,
		AppName:  provider,
		EventID:  fields["event_id"],
		User:     fields["user_id"],
		Extra: map[string]string{
			"channel":  channel,
			"provider": provider,
		},
	}

	// If there's a process ID, add it to both fields and metadata
	var pid int
	if pidStr, ok := fields["process_id"]; ok {
		if pidInt, err := strconv.Atoi(pidStr); err == nil {
			pid = pidInt
			meta.Extra["process_id"] = pidStr
			if exe, ok := fields["executable"]; ok {
				meta.Executable = exe
			}
		}
	}

	// Add any additional metadata from fields
	if meta.Extra == nil {
		meta.Extra = make(map[string]string)
	}
	for k, v := range fields {
		if _, exists := meta.Extra[k]; !exists {
			meta.Extra[k] = v
		}
	}

	return model.LogEntry{
		Timestamp: timestamp,
		Level:     mapEventLevel(level),
		Message:   message,
		Source:    "eventviewer",
		Category:  category,
		PID:       pid,
		Fields:    fields,
		Tags:      tags,
		Meta:      meta,
	}
}
