package service

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type skillExecutionRecorder struct {
	repo interfaces.SkillExecutionRunRepository
}

func (r skillExecutionRecorder) RecordSkillExecution(ctx context.Context, run *types.SkillExecutionRun) error {
	if r.repo == nil {
		return nil
	}
	return r.repo.CreateSkillExecutionRun(ctx, run)
}
