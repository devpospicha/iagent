package metricsender

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/devpospicha/iagent/internal/command"
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

// MetricSender handles streaming metrics and control commands.
type MetricSender struct {
	client proto.StreamServiceClient
	cc     *grpc.ClientConn
	stream proto.StreamService_StreamClient
	wg     sync.WaitGroup
	cfg    *config.Config
	ctx    context.Context
}

// NewSender returns immediately and starts a background connection manager.
func NewSender(ctx context.Context, cfg *config.Config) (*MetricSender, error) {
	s := &MetricSender{
		ctx: ctx,
		cfg: cfg,
	}
	go s.manageConnection()
	return s, nil
}

// manageConnection dials/opens the stream with backoff, handles global disconnects.
// in metricsender/sender.go

func (s *MetricSender) manageConnection() {
	const (
		initial    = 1 * time.Second
		maxBackoff = 15 * time.Minute // Changed from 10s to 15min
		factor     = 2                // Multiplier for exponential backoff
	)

	backoff := initial

	for {
		// honor any global pause
		grpcconn.WaitForResume()

		// 2) handle a global disconnect command
		select {
		case <-grpcconn.DisconnectNotify():
			utils.Info("Global disconnect: closing metric stream")
			if s.stream != nil {
				_ = s.stream.CloseSend()
			}
			s.stream = nil
			// reset only *after* a full disconnect/reconnect cycle
			backoff = initial
			continue
		default:
		}

		// ensure we have a live ClientConn (will re-dial under the hood)
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

		// open the stream if we don't have one yet
		if s.stream == nil {
			stream, err := s.client.Stream(s.ctx)
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
			s.stream = stream
			utils.Info("Metrics stream connected")
			// now that we're actually connected, reset backoff
			backoff = initial
		}

		// block in the receive loop until error or next disconnect
		s.manageReceive()

		// 6) on exit, close just the stream
		if s.stream != nil {
			_ = s.stream.CloseSend()
		}
		s.stream = nil

		// 7) log and back off before the next full reconnect
		utils.Info("Metrics stream lost: retrying connect in %s", backoff)
		time.Sleep(backoff)
		// Calculate next backoff duration
		if backoff < maxBackoff {
			backoff = time.Duration(float64(backoff) * float64(factor))
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// SendMetrics marshals and sends a MetricPayload. If no stream, returns Unavailable.
func (s *MetricSender) SendMetrics(payload *model.MetricPayload) error {
	if s.stream == nil {
		return status.Error(codes.Unavailable, "no active metrics stream")
	}

	pbMetrics := make([]*proto.Metric, 0, len(payload.Metrics))
	for _, m := range payload.Metrics {
		pm := &proto.Metric{
			Name:              m.Name,
			Namespace:         m.Namespace,
			Subnamespace:      m.SubNamespace,
			Timestamp:         timestamppb.New(m.Timestamp),
			Value:             m.Value,
			Unit:              m.Unit,
			StorageResolution: int32(m.StorageResolution),
			Type:              m.Type,
			Dimensions:        m.Dimensions,
		}
		if sv := m.StatisticValues; sv != nil {
			pm.StatisticValues = &proto.StatisticValues{
				Minimum:     sv.Minimum,
				Maximum:     sv.Maximum,
				SampleCount: int32(sv.SampleCount),
				Sum:         sv.Sum,
			}
		}
		pbMetrics = append(pbMetrics, pm)
	}

	var metaProto *proto.Meta
	if payload.Meta != nil {
		metaProto = protohelper.ConvertMetaToProtoMeta(payload.Meta)
	}

	metricPayload := &proto.MetricPayload{
		AgentId:    payload.AgentID,
		HostId:     payload.HostID,
		Hostname:   payload.Hostname,
		EndpointId: payload.EndpointID,
		Timestamp:  timestamppb.New(payload.Timestamp),
		Metrics:    pbMetrics,
		Meta:       metaProto,
	}

	b, err := goproto.Marshal(metricPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal MetricPayload: %w", err)
	}

	streamPayload := &proto.StreamPayload{
		Payload: &proto.StreamPayload_Metric{
			Metric: &proto.MetricWrapper{RawPayload: b},
		},
	}
	// --- end conversion ---

	// send with timeout
	sendCtx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	// log.Info().Msgf("Sending metrics to server %v", streamPayload)
	go func() { errCh <- s.stream.Send(streamPayload) }()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("stream send failed: %w", err)
		}
		return nil
	case <-sendCtx.Done():
		return fmt.Errorf("stream send timeout")
	}
}

// manageReceive handles incoming commands; on a disconnect command, broadcasts global pause.
func (s *MetricSender) manageReceive() {
	for {
		resp, err := s.stream.Recv()
		if err != nil {
			if s.ctx.Err() != nil {
				utils.Info("Receive loop canceled")
			} else {
				utils.Error("Stream receive error: %v", err)
			}
			return
		}

		if cmd := resp.Command; cmd != nil &&
			cmd.CommandType == "control" &&
			cmd.Command == "disconnect" {

			utils.Info("Received global disconnect; pausing all senders for %v", pauseDuration)
			grpcconn.PauseConnections(pauseDuration)
			return
		}

		if resp.Command != nil {
			utils.Info("Handling command %s/%s", resp.Command.CommandType, resp.Command.Command)
			if result := command.HandleCommand(s.ctx, resp.Command); result != nil {
				s.sendCommandResponseWithRetry(result)
			}
		}
	}
}

// Close waits for any in-flight work then closes the connection.
func (s *MetricSender) Close() error {
	utils.Info("Closing MetricSender... waiting for workers")
	s.wg.Wait()
	utils.Info("All workers done")
	return s.cc.Close()
}

// reconnectStream re-dials and reopens the stream for sendCommandResponseWithRetry.
func (s *MetricSender) reconnectStream() error {
	if s.cc != nil {
		_ = s.cc.Close()
	}
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	conn, err := grpcconn.GetGRPCConn(s.cfg)
	if err != nil {
		return fmt.Errorf("failed to reconnect: %w", err)
	}
	s.cc = conn
	s.client = proto.NewStreamServiceClient(conn)

	stream, err := s.client.Stream(ctx)
	if err != nil {
		return fmt.Errorf("failed to reopen stream: %w", err)
	}
	s.stream = stream
	return nil
}

// sendCommandResponseWithRetry retries CommandResponse up to 3 times with backoff.
func (s *MetricSender) sendCommandResponseWithRetry(resp *proto.CommandResponse) {
	const maxAttempts = 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		sendCtx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		defer cancel()

		done := make(chan error, 1)
		go func() {
			done <- s.stream.Send(&proto.StreamPayload{
				Payload: &proto.StreamPayload_CommandResponse{CommandResponse: resp},
			})
		}()

		select {
		case err := <-done:
			if err != nil {
				utils.Warn("CommandResponse send attempt %d failed: %v", attempt, err)
				_ = s.reconnectStream()
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			utils.Debug("CommandResponse sent on attempt %d", attempt)
			return
		case <-sendCtx.Done():
			utils.Warn("CommandResponse send attempt %d timed out", attempt)
			_ = s.reconnectStream()
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	utils.Error("Failed to send CommandResponse after %d attempts", maxAttempts)
}
