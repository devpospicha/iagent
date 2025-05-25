package processsender

import (
	"context"
	"fmt"
	"time"

	"github.com/devpospicha/ishared/model"
	"github.com/devpospicha/ishared/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// StartWorkerPool starts a pool of workers to process incoming process payloads.
// Each worker will attempt to send the payload to the gRPC server.
// The number of workers is determined by workerCount.
// Workers exit when the sender’s context is done or the queue is closed.
func (s *ProcessSender) StartWorkerPool(ctx context.Context, queue <-chan *model.ProcessPayload, workerCount int) {
	for i := 0; i < workerCount; i++ {
		s.wg.Add(1)
		go func(id int) {
			defer s.wg.Done()
			for {
				// 1) Exit on shutdown
				select {
				case <-ctx.Done():
					utils.Info("Process worker %d shutting down", id)
					return
				default:
				}

				// If we’re not yet connected, back off and loop
				if s.stream == nil {
					time.Sleep(500 * time.Millisecond)
					continue
				}

				// We have a stream—grab the next payload
				var payload *model.ProcessPayload
				select {
				case payload = <-queue:
				case <-ctx.Done():
					utils.Info("Process worker %d shutting down", id)
					return
				}

				// Try to send it
				if err := s.SendSnapshot(payload); err != nil {
					utils.Warn("Process worker %d failed to send payload: %v", id, err)
				}
			}
		}(i + 1)
	}
}

// trySendWithBackoff attempts to send the process payload using exponential backoff.
// It retries on Unavailable, DeadlineExceeded, or ResourceExhausted errors.
// After 5 attempts, returns the last error.
func (s *ProcessSender) trySendWithBackoff(payload *model.ProcessPayload) error {
	var err error
	backoff := 500 * time.Millisecond
	maxBackoff := 10 * time.Second

	for attempt := 1; attempt <= 1; attempt++ {
		err = s.SendSnapshot(payload)
		if err == nil {
			return nil
		}

		st, ok := status.FromError(err)
		if ok {
			switch st.Code() {
			case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
				utils.Warn("Transient process send error (%s) — retrying in %v [attempt %d/5]", st.Code(), backoff, attempt)
			default:
				utils.Error("Permanent process send error (%s): %v", st.Code(), err)
				return err
			}
		} else {
			utils.Warn("Unknown process send error — retrying in %v [attempt %d/5]: %v", backoff, attempt, err)
		}

		time.Sleep(backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return fmt.Errorf("process send failed after 5 attempts: %w", err)
}
