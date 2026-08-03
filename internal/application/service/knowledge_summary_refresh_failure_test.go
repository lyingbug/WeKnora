package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type summaryRefreshStatusRepo struct {
	interfaces.KnowledgeRepository
	columnWrites map[string]interface{}
	updateErr    error
}

func (r *summaryRefreshStatusRepo) UpdateKnowledgeColumn(
	_ context.Context, _ string, column string, value interface{},
) error {
	if r.columnWrites == nil {
		r.columnWrites = map[string]interface{}{}
	}
	r.columnWrites[column] = value
	return r.updateErr
}

func TestHandleSummaryRefreshFailure(t *testing.T) {
	llmErr := errors.New("upstream 502")

	t.Run("retryable delivery leaves status untouched", func(t *testing.T) {
		repo := &summaryRefreshStatusRepo{}
		svc := &knowledgeService{repo: repo}
		ctx := types.WithTaskRetryMetadata(context.Background(), 1, 3)

		err := svc.handleSummaryRefreshFailure(ctx, "knowledge-1", llmErr)
		if !errors.Is(err, llmErr) {
			t.Fatalf("error = %v, want the generation error so the executor retries", err)
		}
		if len(repo.columnWrites) != 0 {
			t.Fatalf("retryable delivery wrote %v, want no status write", repo.columnWrites)
		}
	})

	t.Run("terminal delivery marks summary failed", func(t *testing.T) {
		repo := &summaryRefreshStatusRepo{}
		svc := &knowledgeService{repo: repo}
		ctx := types.WithTaskRetryMetadata(context.Background(), 3, 3)

		err := svc.handleSummaryRefreshFailure(ctx, "knowledge-1", llmErr)
		if !errors.Is(err, llmErr) {
			t.Fatalf("error = %v, want the generation error", err)
		}
		if got := repo.columnWrites["summary_status"]; got != types.SummaryStatusFailed {
			t.Fatalf("summary_status = %v, want %q", got, types.SummaryStatusFailed)
		}
	})

	t.Run("wrapped terminal failure still marks summary failed", func(t *testing.T) {
		repo := &summaryRefreshStatusRepo{}
		svc := &knowledgeService{repo: repo}
		ctx := types.WithTaskRetryMetadata(context.Background(), 3, 3)

		wrapped := fmt.Errorf("get chat model: %w", llmErr)
		if err := svc.handleSummaryRefreshFailure(ctx, "knowledge-1", wrapped); err == nil {
			t.Fatal("expected the failure to propagate")
		}
		if got := repo.columnWrites["summary_status"]; got != types.SummaryStatusFailed {
			t.Fatalf("summary_status = %v, want %q", got, types.SummaryStatusFailed)
		}
	})

	t.Run("stale refresh is swallowed without a status write", func(t *testing.T) {
		repo := &summaryRefreshStatusRepo{}
		svc := &knowledgeService{repo: repo}
		ctx := types.WithTaskRetryMetadata(context.Background(), 3, 3)

		if err := svc.handleSummaryRefreshFailure(ctx, "knowledge-1", ErrSummaryRefreshStale); err != nil {
			t.Fatalf("stale refresh should not be retried or reported, got %v", err)
		}
		if len(repo.columnWrites) != 0 {
			t.Fatalf("stale refresh wrote %v, want the newer refresh to own the status", repo.columnWrites)
		}
	})

	t.Run("insufficient content is terminal without an extra write", func(t *testing.T) {
		repo := &summaryRefreshStatusRepo{}
		svc := &knowledgeService{repo: repo}
		ctx := types.WithTaskRetryMetadata(context.Background(), 0, 3)

		if err := svc.handleSummaryRefreshFailure(
			ctx, "knowledge-1", errInsufficientSummaryContent,
		); err != nil {
			t.Fatalf("insufficient content should not be retried, got %v", err)
		}
		if len(repo.columnWrites) != 0 {
			t.Fatalf("insufficient content wrote %v, want the caller's write to stand", repo.columnWrites)
		}
	})

	t.Run("status write failure does not mask the generation error", func(t *testing.T) {
		repo := &summaryRefreshStatusRepo{updateErr: errors.New("database unavailable")}
		svc := &knowledgeService{repo: repo}
		ctx := types.WithTaskRetryMetadata(context.Background(), 3, 3)

		if err := svc.handleSummaryRefreshFailure(ctx, "knowledge-1", llmErr); !errors.Is(err, llmErr) {
			t.Fatalf("error = %v, want the generation error", err)
		}
	})
}
