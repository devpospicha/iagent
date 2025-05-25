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

// agent/processes/processsender/sender.go

package processsender

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/devpospicha/iagent/internal/config"
	grpcconn "github.com/devpospicha/iagent/internal/grpc"
	"github.com/devpospicha/iagent/internal/protohelper"
	"github.com/devpospicha/ishared/model"
	"github.com/devpospicha/ishared/proto"
	"github.com/devpospicha/ishared/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	goproto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	pauseDuration = 1 * time.Minute
	totalCap      = 15 * time.Minute
)

// ProcessSender handles streaming process snapshots
type ProcessSender struct {
	cfg    *config.Config
	ctx    context.Context
	cc     *grpc.ClientConn
	client proto.StreamServiceClient
	stream proto.StreamService_StreamClient
	wg     sync.WaitGroup
}

// NewSender initializes a new ProcessSender and starts the connection manager.
// It returns immediately and launches the background connection manager.
func NewSender(ctx context.Context, cfg *config.Config) (*ProcessSender, error) {
	s := &ProcessSender{ctx: ctx, cfg: cfg}
	go s.manageConnection()
	return s, nil
}

// manageConnection dials + opens the Stream, tears down on global disconnect,
// and retries with exponential backoff up to totalCap.
func (s *ProcessSender) manageConnection() {
	const (
		initial    = 1 * time.Second
		maxBackoff = 15 * time.Minute // Changed from 10s to 15min
		factor     = 2                // Multiplier for exponential backoff
	)

	backoff := initial
	var lastPause time.Time

	for {
		// If we've entered a new global pause window, tear down our stream
		pu := grpcconn.GetPauseUntil()
		if pu.After(lastPause) {
			utils.Info("Global disconnect: closing process stream")
			if s.stream != nil {
				_ = s.stream.CloseSend()
			}
			s.stream = nil
			backoff = initial
			lastPause = pu
		}

		// Sleep out the pause window if it's active
		grpcconn.WaitForResume()

		// Dial (or reuse) the gRPC connection
		cc, err := grpcconn.GetGRPCConn(s.cfg)
		if err != nil {
			utils.Info("Server offline (dial): retrying in %s", backoff)
			time.Sleep(backoff)
			// Calculate next backoff duration
			if backoff < maxBackoff {
				backoff = time.Duration(float64(backoff) * float64(factor))
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			continue
		}
		s.cc = cc
		s.client = proto.NewStreamServiceClient(cc)

		// Open the stream if we don't have one already
		if s.stream == nil {
			st, err := s.client.Stream(s.ctx)
			if err != nil {
				utils.Info("Server offline (stream): retrying in %s", backoff)
				time.Sleep(backoff)
				// Calculate next backoff duration
				if backoff < maxBackoff {
					backoff = time.Duration(float64(backoff) * float64(factor))
					if backoff > maxBackoff {
						backoff = maxBackoff
					}
				}
				continue
			}
			s.stream = st
			utils.Info("Process stream connected")
			// reset backoff now that we're actually online
			backoff = initial
		}

		// Short sleep so we can check for future disconnects
		time.Sleep(time.Second)
	}
}

// SendSnapshot sends a ProcessPayload; if stream is down, returns Unavailable.
func (s *ProcessSender) SendSnapshot(payload *model.ProcessPayload) error {
	if s.stream == nil {
		return status.Error(codes.Unavailable, "no active process stream")
	}

	// marshal process payload into protobuf
	pb := &proto.ProcessPayload{
		AgentId:    payload.AgentID,
		HostId:     payload.HostID,
		Hostname:   payload.Hostname,
		EndpointId: payload.EndpointID,
		Timestamp:  timestamppb.New(payload.Timestamp),
		Meta:       protohelper.ConvertMetaToProtoMeta(payload.Meta),
	}
	for _, p := range payload.Processes {
		pb.Processes = append(pb.Processes, &proto.ProcessInfo{
			Pid:        int32(p.PID),
			Ppid:       int32(p.PPID),
			User:       p.User,
			Executable: p.Executable,
			Cmdline:    p.Cmdline,
			CpuPercent: p.CPUPercent,
			MemPercent: p.MemPercent,
			Threads:    int32(p.Threads),
			StartTime:  timestamppb.New(p.StartTime),
			Tags:       p.Tags,
		})
	}
	b, err := goproto.Marshal(pb)
	if err != nil {
		return fmt.Errorf("marshal ProcessPayload: %w", err)
	}

	sp := &proto.StreamPayload{
		Payload: &proto.StreamPayload_Process{
			Process: &proto.ProcessWrapper{RawPayload: b},
		},
	}
	// send without additional retries
	if err := s.stream.Send(sp); err != nil {
		return fmt.Errorf("stream send failed: %w", err)
	}
	return nil
}

// Close waits for workers then closes the gRPC connection.
func (s *ProcessSender) Close() error {
	utils.Info("Closing ProcessSender...")
	s.wg.Wait()
	if s.cc != nil {
		return s.cc.Close()
	}
	return nil
}
