package metricrunner

import (
	"context"
	"fmt"
	"time"

	"github.com/devpospicha/iagent/internal/config"
	"github.com/devpospicha/iagent/internal/meta"
	"github.com/devpospicha/iagent/internal/metrics/metriccollector"
	"github.com/devpospicha/iagent/internal/metrics/metricsender"
	"github.com/devpospicha/ishared/model"
	"github.com/devpospicha/ishared/utils"
)

// MetricRunner is a struct that handles the collection and sending of metrics.
// It manages the metric collection interval, the task queue, and the
// metric sender. It implements the Run method to start the collection process
// and the Close method to clean up resources.
type MetricRunner struct {
	Config         *config.Config
	MetricSender   *metricsender.MetricSender
	MetricRegistry *metriccollector.MetricRegistry
	StartTime      time.Time
	Meta           *model.Meta
}

// NewRunner creates a new MetricRunner instance.
// It initializes the metric sender and sets up the context for the runner.
// It returns a pointer to the MetricRunner and an error if any occurs during initialization.
// The MetricRunner is responsible for collecting and sending metrics to the server.
func NewRunner(ctx context.Context, cfg *config.Config, baseMeta *model.Meta) (*MetricRunner, error) {

	// Init the collector registry
	metricRegistry := metriccollector.NewRegistry(cfg)

	// Init Metric Sender
	metricSender, err := metricsender.NewSender(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create sender: %v", err)
	}

	return &MetricRunner{
		Config:         cfg,
		MetricSender:   metricSender,
		MetricRegistry: metricRegistry,
		StartTime:      time.Now(),
		Meta:           baseMeta,
	}, nil
}

// Close closes the metric sender.
// It cleans up resources and ensures that the sender is properly closed.
// This is important to prevent resource leaks and ensure that all data is sent before shutting down.
func (r *MetricRunner) Close() {
	if r.MetricSender != nil {
		_ = r.MetricSender.Close()
	}
}

// RunAgent starts the agent's collection loop and sends tasks to the pool of workers.
// It collects metrics at the specified interval and sends them to the server.
// The method runs indefinitely until the context is done.
// It handles the collection of both host and container metrics.
// The host metrics are sent as a single payload, while container metrics are sent as separate payloads.
func (r *MetricRunner) Run(ctx context.Context) {

	defer r.MetricSender.Close()

	taskQueue := make(chan *model.MetricPayload, 500)
	go r.MetricSender.StartWorkerPool(ctx, taskQueue, r.Config.Agent.MetricCollection.Workers)

	ticker := time.NewTicker(r.Config.Agent.MetricCollection.Interval)
	defer ticker.Stop()

	utils.Info("MetricRunner started. Sending metrics every %v", r.Config.Agent.MetricCollection.Interval)

	for {
		select {
		case <-ctx.Done():
			utils.Warn("agent shutting down...")
			return
		case <-ticker.C:
			metrics, err := r.MetricRegistry.Collect(ctx)
			if err != nil {
				utils.Error("metric collection failed: %v", err)
				continue
			}

			var hostMetrics []model.Metric
			containerBatches := make(map[string][]model.Metric)
			containerMetas := make(map[string]*model.Meta)

			for _, m := range metrics {

				if len(m.Dimensions) > 0 && m.Dimensions["container_id"] != "" {
					id := m.Dimensions["container_id"]
					if id == "" {
						continue
					}
					// Add container metrics to containerBatches
					containerBatches[id] = append(containerBatches[id], m)

					// Initialize Meta only once per container ID
					containerMeta, exists := containerMetas[id]
					if !exists {
						containerMeta = meta.CloneMetaWithTags(r.Meta, nil)
						containerMetas[id] = containerMeta
					}

					// TODO metric runner add k8 namespace / cluster support
					// Populate meta with container-specific information
					for k, v := range m.Dimensions {
						switch k {
						case "container_id":
							containerMeta.ContainerID = v
						case "name", "container_name":
							containerMeta.ContainerName = v
						case "image_id":
							containerMeta.ContainerImageID = v
						case "image":
							containerMeta.ContainerImageName = v
						}
					}

					// Detect running status and apply tag
					if m.Name == "running" {
						if m.Value == 1 {
							containerMeta.Tags["status"] = "running"
						} else {
							containerMeta.Tags["status"] = "stopped"
						}
					}
					// Build tags for the container
					meta.BuildStandardTags(containerMeta, m, true, r.StartTime)

					// Set EndpointID for meta
					containerMeta.EndpointID = utils.GenerateEndpointID(containerMeta)

				} else {
					// Host metrics, collect them separately
					hostMetrics = append(hostMetrics, m)
				}
			}

			// Send host metrics as a single payload
			if len(hostMetrics) > 0 {

				// Build Host Meta
				hostMeta := meta.CloneMetaWithTags(r.Meta, nil)

				// Build tags
				meta.BuildStandardTags(hostMeta, hostMetrics[0], false, r.StartTime)

				// Set EndpointID for meta
				hostMeta.EndpointID = utils.GenerateEndpointID(hostMeta)

				payload := model.MetricPayload{
					AgentID:    hostMeta.AgentID,
					HostID:     hostMeta.HostID,
					Hostname:   hostMeta.Hostname,
					EndpointID: hostMeta.EndpointID,
					Timestamp:  time.Now(),
					Metrics:    hostMetrics,
					Meta:       hostMeta,
				}
				//utils.Info("META Payload for: %s - %v", payload.Host, payload.Meta)
				select {
				case taskQueue <- &payload:
				default:
					utils.Warn("Host task queue full! Dropping host metrics")
				}
			}

			// Send each container as a separate payload
			for id, metrics := range containerBatches {
				payload := model.MetricPayload{
					AgentID:    containerMetas[id].AgentID,
					HostID:     containerMetas[id].HostID,
					Hostname:   containerMetas[id].Hostname,
					EndpointID: containerMetas[id].EndpointID,
					Timestamp:  time.Now(),
					Metrics:    metrics,
					Meta:       containerMetas[id],
				}
				//utils.Info("META Payload for: %s - %s - %s - %v", payload.HostID, payload.AgentID, payload.Hostname, payload.Meta)

				select {
				case taskQueue <- &payload:
				default:
					utils.Warn("Task queue full! Dropping container metrics for %s", id)
				}
			}
		}
	}
}
