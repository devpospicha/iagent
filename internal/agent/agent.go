package iagent

import (
	"context"
	"fmt"

	"github.com/devpospicha/iagent/internal/config"
	grpcconn "github.com/devpospicha/iagent/internal/grpc"
	agentidentity "github.com/devpospicha/iagent/internal/identity"

	"github.com/devpospicha/iagent/internal/logger/logrunner"
	"github.com/devpospicha/iagent/internal/meta"
	metricrunner "github.com/devpospicha/iagent/internal/metrics/metricrunner"
	"github.com/devpospicha/iagent/internal/processes/processrunner"
	"github.com/devpospicha/ishared/model"
	"github.com/devpospicha/ishared/utils"
)

// Agent is a struct that represents the GoSight agent.
// It contains the configuration, metric runner, log runner, process runner,
// agent ID, agent version, and metadata.
type Agent struct {
	Config        *config.Config
	MetricRunner  *metricrunner.MetricRunner
	AgentID       string
	AgentVersion  string
	LogRunner     *logrunner.LogRunner
	ProcessRunner *processrunner.ProcessRunner
	Meta          *model.Meta
	Ctx           context.Context
}

// NewAgent creates a new instance of the GoSight agent.
// It initializes the agent with the provided configuration, context, and agent version.
// It retrieves the agent ID and builds the base metadata for the agent.
// It also creates instances of the metric runner, log runner, and process runner.
// If any of these steps fail, it returns an error.
func NewAgent(ctx context.Context, cfg *config.Config, agentVersion string) (*Agent, error) {

	// Retrieve (or set) the agent ID
	agentID, err := agentidentity.LoadOrCreateAgentID()
	if err != nil {
		utils.Fatal("Failed to get agent ID: %v", err)
	}

	// Build base metadata for the agent and cache it in the Agent struct
	baseMeta := meta.BuildMeta(cfg, nil, agentID, agentVersion)

	metricRunner, err := metricrunner.NewRunner(ctx, cfg, baseMeta)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric runner: %v", err)
	}
	logRunner, err := logrunner.NewRunner(ctx, cfg, baseMeta)
	if err != nil {
		return nil, fmt.Errorf("failed to create log runner: %v", err)
	}

	processRunner, err := processrunner.NewRunner(ctx, cfg, baseMeta)
	if err != nil {
		return nil, fmt.Errorf("failed to create process runner: %v", err)
	}

	return &Agent{
		Ctx:           ctx,
		Config:        cfg,
		MetricRunner:  metricRunner,
		AgentID:       agentID,
		AgentVersion:  agentVersion,
		LogRunner:     logRunner,
		ProcessRunner: processRunner,
		Meta:          baseMeta,
	}, nil
}

// Start initializes and starts the metric, log, and process runners.
// It runs each runner in a separate goroutine.
// The context is used to manage the lifecycle of the runners.
// The function logs the start of each runner and handles any errors that may occur.
func (a *Agent) Start(ctx context.Context) {

	// Start runner.
	utils.Debug("Agent attempting to start metricrunner.")
	go a.MetricRunner.Run(ctx)

	utils.Debug("Agent attempting to start metricrunner.")
	//go a.LogRunner.Run(ctx)

	utils.Debug("Agent attempting to start processrunner.")
	go a.ProcessRunner.Run(ctx)

}

// Close stops all runners and closes the gRPC connection.
// It waits for all runners to finish before closing the connection.
func (a *Agent) Close() {
	// Stop All Runners
	a.MetricRunner.Close()
	//a.LogRunner.Close()
	a.ProcessRunner.Close()

	err := grpcconn.CloseGRPCConn()
	if err != nil {
		utils.Warn("Failed to close gRPC connection cleanly: %v", err)
	}
	utils.Info("Agent shutdown complete")

}
