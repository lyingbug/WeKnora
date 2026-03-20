package agent

import (
	"errors"
	"sync"
)

// ErrBudgetExhausted is returned when the token budget has been exceeded
var ErrBudgetExhausted = errors.New("token budget exhausted")

// TokenBudget provides thread-safe token budget management for sub-agent execution.
// All sub-agents spawned by a parent agent share the same TokenBudget instance.
type TokenBudget struct {
	mu       sync.Mutex
	total    int // Total budget
	consumed int // Consumed tokens
}

// NewTokenBudget creates a new token budget with the given total
func NewTokenBudget(total int) *TokenBudget {
	return &TokenBudget{total: total}
}

// Check returns true if the budget has at least `required` tokens remaining
func (b *TokenBudget) Check(required int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.consumed+required <= b.total
}

// Deduct subtracts the given number of tokens from the budget.
// Returns ErrBudgetExhausted if the budget is exceeded after deduction.
func (b *TokenBudget) Deduct(tokens int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.consumed += tokens
	if b.consumed > b.total {
		return ErrBudgetExhausted
	}
	return nil
}

// Remaining returns the number of tokens remaining in the budget
func (b *TokenBudget) Remaining() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.total - b.consumed
}

// Stats returns the current budget statistics
func (b *TokenBudget) Stats() TokenBudgetStats {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.total - b.consumed
	var usageRate float64
	if b.total > 0 {
		usageRate = float64(b.consumed) / float64(b.total)
	}
	return TokenBudgetStats{
		Total:     b.total,
		Consumed:  b.consumed,
		Remaining: remaining,
		UsageRate: usageRate,
	}
}

// TokenBudgetStats contains budget usage statistics
type TokenBudgetStats struct {
	Total     int     `json:"total"`
	Consumed  int     `json:"consumed"`
	Remaining int     `json:"remaining"`
	UsageRate float64 `json:"usage_rate"`
}

// EstimateTokensFromAgentState estimates token consumption from agent execution state.
// Uses a rough heuristic of ~4 characters per token.
func EstimateTokensFromAgentState(finalAnswer string, roundSteps interface{}) int {
	total := len(finalAnswer) / 4

	// Try to extract step data if available
	type stepLike struct {
		Thought   string
		ToolCalls []struct {
			Result *struct {
				Output string
			}
		}
	}

	// We receive []types.AgentStep but to avoid import cycles,
	// we just estimate from the final answer length with a multiplier
	// A typical agent run consumes ~3x the final answer in total tokens
	if total < 1024 {
		total = 1024 // Minimum estimate
	}
	return total * 3
}
