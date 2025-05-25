package logsender

import (
	"context"
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

// LogSender holds the gRPC client, connection, and stream.
type LogSender struct {
	client proto.LogServiceClient
	cc     *grpc.ClientConn
	stream proto.LogService_SubmitStreamClient
	wg     sync.WaitGroup
	cfg    *config.Config
	ctx    context.Context
}

// NewSender initializes a new LogSender and starts the connection manager.
// It returns immediately and launches the background connection manager.
func NewSender(ctx context.Context, cfg *config.Config) (*LogSender, error) {
	s := &LogSender{ctx: ctx, cfg: cfg}
	go s.manageConnection()
	return s, nil
}

// manageConnection dials & opens the SubmitStream, tears it down on global disconnect,
// and retries with exponential backoff up to totalCap, then fixed-interval.
// in logsender/sender.go
func (s *LogSender) manageConnection() {
	const (
		initial    = 1 * time.Second
		maxBackoff = 15 * time.Minute // Changed from 10s to 15min
		factor     = 2                // Multiplier for exponential backoff
	)

	backoff := initial
	var lastPause time.Time

	for {
		// If we've just been told to pause (disconnect), tear down both stream & conn once
		pu := grpcconn.GetPauseUntil()
		if pu.After(lastPause) {
			utils.Info("Global disconnect: closing log stream and connection")
			if s.stream != nil {
				_ = s.stream.CloseSend()
			}

			s.stream = nil
			backoff = initial
			lastPause = pu
		}

		// Wait out the pause window (returns when pauseUntil ≤ now)
		grpcconn.WaitForResume()

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
		s.client = proto.NewLogServiceClient(cc)

		// Open the SubmitStream if we don't already have one
		if s.stream == nil {
			st, err := s.client.SubmitStream(s.ctx)
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
			utils.Info("Log stream connected")
			// reset on successful stream open
			backoff = initial
		}

		// Brief pause to catch any new disconnects
		time.Sleep(time.Second)
	}
}

// SendLogs marshals the LogPayload and sends it.
// If no active stream, returns Unavailable so your worker backoff kicks in.
func (s *LogSender) SendLogs(payload *model.LogPayload) error {
	if s.stream == nil {
		return status.Error(codes.Unavailable, "no active log stream")
	}
	// --- begin conversion ---
	// Convert LogPayload to proto.LogPayload
	pbLogs := make([]*proto.LogEntry, 0, len(payload.Logs))
	for _, log := range payload.Logs {
		pb := &proto.LogEntry{
			Timestamp: timestamppb.New(log.Timestamp),
			Level:     log.Level,
			Message:   log.Message,
			Source:    log.Source,
			Category:  log.Category,
			Pid:       int32(log.PID),
			Fields:    log.Fields,
			Tags:      log.Tags,
			Meta:      protohelper.ConvertLogMetaToProto(log.Meta),
		}
		pbLogs = append(pbLogs, pb)
	}
	var metaProto *proto.Meta
	if payload.Meta != nil {
		metaProto = protohelper.ConvertMetaToProtoMeta(payload.Meta)
	}
	req := &proto.LogPayload{
		AgentId:    payload.AgentID,
		HostId:     payload.HostID,
		Hostname:   payload.Hostname,
		EndpointId: payload.EndpointID,
		Timestamp:  timestamppb.New(payload.Timestamp),
		Logs:       pbLogs,
		Meta:       metaProto,
	}
	// --- end conversion ---

	// verify marshal
	if _, err := goproto.Marshal(req); err != nil {
		utils.Error("Failed to marshal LogPayload: %v", err)
		return err
	}

	// send
	utils.Info("Sending %d logs to server", len(pbLogs))
	if err := s.stream.Send(req); err != nil {
		utils.Warn("Log stream send failed: %v", err)
		return err
	}
	utils.Debug("Streamed %d logs", len(pbLogs))
	return nil
}

// Close shuts down worker pool and closes the gRPC connection.
func (s *LogSender) Close() error {
	utils.Info("Closing LogSender... waiting for workers")
	s.wg.Wait()
	utils.Info("All LogSender workers finished")
	return s.cc.Close()
}
