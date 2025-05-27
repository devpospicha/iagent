package grpcconn

import (
	"log"
	"sync"
	"time"

	"github.com/devpospicha/iagent/internal/config"
	agentutils "github.com/devpospicha/iagent/internal/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/keepalive"
)

var (
	conn   *grpc.ClientConn
	connMu sync.Mutex
)

var (
	pauseMu    sync.Mutex
	pauseUntil time.Time

	disconnectMu sync.Mutex
	disconnectCh = make(chan struct{})
)

// GetGRPCConn returns the singleton ClientConn for the gRPC connection.
// It creates a new connection if one does not already exist.
// The connection is configured with TLS and various gRPC options.
// It is safe for concurrent use.
// Note: This function does not block until the connection is established.
func GetGRPCConn(cfg *config.Config) (*grpc.ClientConn, error) {
	connMu.Lock()
	defer connMu.Unlock()

	// If we have one already, check its health.
	if conn != nil {
		state := conn.GetState()
		if state == connectivity.Ready || state == connectivity.Idle {
			return conn, nil
		}
		// Otherwise it's broken; close and reset.
		_ = conn.Close()
		conn = nil
	}

	tlsCfg, err := agentutils.LoadTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                2 * time.Minute,
			Timeout:             20 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithInitialWindowSize(64 * 1024 * 1024),
		grpc.WithInitialConnWindowSize(128 * 1024 * 1024),
		grpc.WithReadBufferSize(8 * 1024 * 1024),
		grpc.WithWriteBufferSize(8 * 1024 * 1024),
		grpc.WithDefaultCallOptions(
			grpc.UseCompressor(gzip.Name),
			grpc.MaxCallRecvMsgSize(32*1024*1024),
			grpc.MaxCallSendMsgSize(32*1024*1024),
		),
	}
	log.Printf("gRPC connection established to %s", cfg.Agent.ServerURL)
	c, err := grpc.NewClient(cfg.Agent.ServerURL, opts...)
	if err != nil {
		return nil, err
	}

	conn = c
	return conn, nil
}

// CloseGRPCConn closes the connection (for shutdown)
func CloseGRPCConn() error {
	connMu.Lock()
	defer connMu.Unlock()
	if conn != nil {
		err := conn.Close()
		conn = nil
		return err
	}
	return nil
}

func GetPauseUntil() time.Time {
	pauseMu.Lock()
	pu := pauseUntil
	pauseMu.Unlock()
	return pu
}

// PauseConnections schedules a global pause of duration d,
// and broadcasts one “disconnect” event on disconnectCh.
func PauseConnections(d time.Duration) {
	// set the pause deadline
	pauseMu.Lock()
	pauseUntil = time.Now().Add(d)
	pauseMu.Unlock()

	// Immediately tear down the shared gRPC connection
	_ = CloseGRPCConn()

	disconnectMu.Lock()
	// non-blocking broadcast into the buffered channel
	select {
	case disconnectCh <- struct{}{}:
	default:
		// if the buffer is full, we’ve already signaled — no need to block
	}
	disconnectMu.Unlock()
}

// WaitForResume blocks until time.Now() ≥ pauseUntil.
// Even if pauseUntil was extended mid-sleep, this will re-check.
func WaitForResume() {
	for {
		pauseMu.Lock()
		pu := pauseUntil
		pauseMu.Unlock()

		now := time.Now()
		if now.Before(pu) {
			time.Sleep(pu.Sub(now))
			continue
		}
		return
	}
}

// DisconnectNotify returns the channel that will get
// one event each time PauseConnections is called.
func DisconnectNotify() <-chan struct{} {
	return disconnectCh
}
