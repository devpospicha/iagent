// cmd/main.go - main entry point for agent.

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	iagent "github.com/devpospicha/iagent/internal/agent"
	"github.com/devpospicha/iagent/internal/bootstrap"
	"github.com/devpospicha/ishared/utils"
)

var Version = "dev" // default
// go build -ldflags "-X main.Version=0.3.2" -o gosight-agent ./cmd/agent

func main() {

	// Bootstrap config loading (flags -> env -> file)
	cfg := bootstrap.LoadAgentConfig()
	fmt.Printf("About to init logger with level = %s\n", cfg.Logs.LogLevel)

	bootstrap.SetupLogging(cfg)
	utils.Debug("debug logging is active from main.go")

	// Graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		utils.Warn("signal received, shutting down agent...")
		cancel()
	}()

	// Create Agent
	agent, err := iagent.NewAgent(ctx, cfg, Version)
	if err != nil {
		utils.Error("failed to initialize agent: %v", err)
		os.Exit(1)
	}

	// Start Agent
	agent.Start(ctx)

	<-ctx.Done()

	utils.Info("Context canceled, beginning agent shutdown...")

	agent.Close()
}
