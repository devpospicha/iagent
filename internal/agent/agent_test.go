//go:build linux
// +build linux

package iagent

import (
	"context"
	"reflect"
	"testing"

	"github.com/devpospicha/iagent/internal/config"
	metricrunner "github.com/devpospicha/iagent/internal/metrics/metricrunner"
	"github.com/devpospicha/iagent/internal/processes/processrunner"
	"github.com/devpospicha/ishared/model"
)

func TestNewAgent(t *testing.T) {
	type args struct {
		ctx          context.Context
		cfg          *config.Config
		agentVersion string
	}
	tests := []struct {
		name    string
		args    args
		want    *Agent
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewAgent(tt.args.ctx, tt.args.cfg, tt.args.agentVersion)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAgent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewAgent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgent_Start(t *testing.T) {
	type fields struct {
		Config       *config.Config
		MetricRunner *metricrunner.MetricRunner
		AgentID      string
		AgentVersion string
		//	LogRunner     *logrunner.LogRunner
		ProcessRunner *processrunner.ProcessRunner
		Meta          *model.Meta
		Ctx           context.Context
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Agent{
				Config:       tt.fields.Config,
				MetricRunner: tt.fields.MetricRunner,
				AgentID:      tt.fields.AgentID,
				AgentVersion: tt.fields.AgentVersion,
				//	LogRunner:     tt.fields.LogRunner,
				ProcessRunner: tt.fields.ProcessRunner,
				Meta:          tt.fields.Meta,
				Ctx:           tt.fields.Ctx,
			}
			a.Start(tt.args.ctx)
		})
	}
}

func TestAgent_Close(t *testing.T) {
	type fields struct {
		Config       *config.Config
		MetricRunner *metricrunner.MetricRunner
		AgentID      string
		AgentVersion string
		//	LogRunner     *logrunner.LogRunner
		ProcessRunner *processrunner.ProcessRunner
		Meta          *model.Meta
		Ctx           context.Context
	}
	tests := []struct {
		name   string
		fields fields
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Agent{
				Config:       tt.fields.Config,
				MetricRunner: tt.fields.MetricRunner,
				AgentID:      tt.fields.AgentID,
				AgentVersion: tt.fields.AgentVersion,
				//		LogRunner:     tt.fields.LogRunner,
				ProcessRunner: tt.fields.ProcessRunner,
				Meta:          tt.fields.Meta,
				Ctx:           tt.fields.Ctx,
			}
			a.Close()
		})
	}
}
