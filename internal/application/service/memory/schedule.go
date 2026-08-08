package memory

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const (
	// decaySweepInterval is how often every space is aged.
	//
	// Daily is the right granularity: half-lives are measured in months, so a
	// finer schedule would burn writes to move strengths by fractions of a
	// percent, and a coarser one would let an archived-worthy memory keep
	// occupying prompt budget for a week.
	decaySweepInterval = 24 * time.Hour
	// decaySweepStartupDelay lets the process finish booting before the first
	// sweep competes with startup work for the database.
	decaySweepStartupDelay = 5 * time.Minute
)

// RunDecaySchedule runs the memory retention sweep until the context ends.
//
// It lives outside the asynq scheduler on purpose. Lite has no Redis and
// therefore no periodic scheduler, and a memory store whose forgetting only
// works in one of the two deployment forms would quietly grow without bound in
// the other. A ticker behaves the same in both.
func RunDecaySchedule(ctx context.Context, writer interfaces.MemoryWriterService) {
	if writer == nil {
		return
	}
	timer := time.NewTimer(decaySweepStartupDelay)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		if err := writer.DecayAll(ctx); err != nil {
			logger.Warnf(ctx, "memory: decay sweep failed: %v", err)
		}
		timer.Reset(decaySweepInterval)
	}
}
