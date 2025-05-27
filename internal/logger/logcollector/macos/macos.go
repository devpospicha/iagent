//go:build darwin
// +build darwin

package macos

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/devpospicha/iagent/internal/config"
	"github.com/devpospicha/ishared/model"
	"github.com/devpospicha/ishared/utils"
)

// MacOSCollector streams logs from macOS unified logging system using 'log stream' command.
type MacOSCollector struct {
	Config    *config.Config
	lines     chan model.LogEntry
	cmd       *exec.Cmd
	stop      chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	once      sync.Once
	batchSize int
	maxSize   int
}

// Name returns the name of the collector.
func (m *MacOSCollector) Name() string {
	return "macos_log"
}

// NewMacOSCollector initializes a new MacOSCollector.
func NewMacOSCollector(cfg *config.Config) *MacOSCollector {
	utils.Info("Initializing macOS log collector...")

	collector := &MacOSCollector{
		Config:    cfg,
		lines:     make(chan model.LogEntry, cfg.Agent.LogCollection.BatchSize*10),
		stop:      make(chan struct{}),
		batchSize: cfg.Agent.LogCollection.BatchSize,
		maxSize:   cfg.Agent.LogCollection.MessageMax,
	}

	collector.wg.Add(1)
	go collector.runLogStream()

	utils.Info("macOS log collector initialized and log stream started.")
	return collector
}

// runLogStream executes 'log stream' and reads logs line by line.
func (m *MacOSCollector) runLogStream() {
	defer m.wg.Done()

	m.cmd = exec.Command("log", "stream", "--style", "json")

	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		utils.Error("Failed to get stdout pipe for 'log stream': %v", err)
		return
	}

	if err := m.cmd.Start(); err != nil {
		utils.Error("Failed to start 'log stream' command: %v", err)
		return
	}

	reader := bufio.NewReader(stdout)

	for {
		select {
		case <-m.stop:
			m.cmd.Process.Kill()
			m.cmd.Wait()
			close(m.lines)
			return
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				utils.Warn("Error reading from 'log stream': %v", err)
				continue
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			entry, err := parseLogStreamJSON(line)
			if err != nil {
				utils.Warn("Failed to parse macOS log line JSON: %v", err)
				continue
			}

			logEntry := buildLogEntry(entry, m.maxSize)
			m.lines <- logEntry
		}
	}
}

// parseLogStreamJSON parses a single JSON log line from 'log stream' output.
func parseLogStreamJSON(line string) (map[string]interface{}, error) {
	var entry map[string]interface{}
	err := json.Unmarshal([]byte(line), &entry)
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// buildLogEntry builds model.LogEntry from the parsed JSON map.
func buildLogEntry(entry map[string]interface{}, maxSize int) model.LogEntry {
	// Extract timestamp (nanoseconds since epoch)
	ts := time.Now()
	if t, ok := entry["timestamp"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339Nano, t); err == nil {
			ts = parsed
		}
	}

	msg := ""
	if m, ok := entry["eventMessage"].(string); ok {
		msg = m
	}
	if !utf8.ValidString(msg) {
		msg = sanitizeUTF8(msg)
	}
	if len(msg) > maxSize && maxSize > 0 {
		msg = msg[:maxSize] + " [truncated]"
	}

	// Basic fields extraction with fallback to empty strings
	category := ""
	if subsys, ok := entry["subsystem"].(string); ok {
		category = subsys
	}

	pid := 0
	if p, ok := entry["processID"].(float64); ok {
		pid = int(p)
	}

	procName := ""
	if pn, ok := entry["process"].(string); ok {
		procName = pn
	}

	// Extract user info if available (limited info from macOS log)
	user := ""
	if u, ok := entry["userID"].(float64); ok {
		user = strconv.Itoa(int(u))
	}

	fields := map[string]string{
		"SUBSYSTEM": category,
		"PROCESS":   procName,
	}

	tags := map[string]string{}

	meta := &model.LogMeta{
		Platform:    "macos",
		AppName:     procName,
		Unit:        category,
		User:        user,
		Executable:  procName,
		Extra:       map[string]string{}, // no raw fields parsed in detail
		ContainerID: "",
	}

	return model.LogEntry{
		Timestamp: ts,
		Level:     "info", // macOS logs don’t have explicit priority in this simplified example
		Message:   msg,
		Source:    "macos_log",
		Category:  category,
		PID:       pid,
		Fields:    fields,
		Tags:      tags,
		Meta:      meta,
	}
}

// sanitizeUTF8 ensures that the string is valid UTF-8.
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "\uFFFD")
}

// Collect drains the internal lines channel and returns batched logs.
func (m *MacOSCollector) Collect(ctx context.Context) ([][]model.LogEntry, error) {
	m.mu.Lock()
	if m.cmd == nil {
		m.mu.Unlock()
		return nil, nil // Collector not started
	}
	m.mu.Unlock()

	var allBatches [][]model.LogEntry
	var currentBatch []model.LogEntry

collectLoop:
	for {
		select {
		case entry, ok := <-m.lines:
			if !ok {
				break collectLoop
			}
			currentBatch = append(currentBatch, entry)
			if len(currentBatch) >= m.batchSize {
				allBatches = append(allBatches, currentBatch)
				currentBatch = make([]model.LogEntry, 0, m.batchSize)
			}
		case <-ctx.Done():
			if len(currentBatch) > 0 {
				allBatches = append(allBatches, currentBatch)
			}
			return allBatches, ctx.Err()
		default:
			break collectLoop
		}
	}

	if len(currentBatch) > 0 {
		allBatches = append(allBatches, currentBatch)
	}

	return allBatches, nil
}

// Close stops the log streaming process.
func (m *MacOSCollector) Close() error {
	m.once.Do(func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		close(m.stop)
		if m.cmd != nil && m.cmd.Process != nil {
			m.cmd.Process.Kill()
			m.cmd.Wait()
		}
		m.wg.Wait()
		close(m.lines)
		utils.Info("macOS log collector closed.")
	})
	return nil
}

// Ensure MacOSCollector implements io.Closer
var _ io.Closer = (*MacOSCollector)(nil)
