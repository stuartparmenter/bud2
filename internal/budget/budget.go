package budget

import (
	"fmt"
)

// ThinkingBudget manages limits on autonomous Claude usage
type ThinkingBudget struct {
	tracker *SessionTracker

	// Limits
	DailyOutputTokens int // Max output tokens per 24h (default 1_000_000)
}

// NewThinkingBudget creates a new budget manager
func NewThinkingBudget(tracker *SessionTracker) *ThinkingBudget {
	return &ThinkingBudget{
		tracker:           tracker,
		DailyOutputTokens: 1_000_000, // 1M output tokens/day default
	}
}

// CanDoAutonomousWork checks if autonomous work is allowed
func (b *ThinkingBudget) CanDoAutonomousWork() (bool, string) {
	if b.tracker != nil {
		usage := b.tracker.TodayTokenUsage()
		if usage.OutputTokens >= b.DailyOutputTokens {
			return false, fmt.Sprintf("daily output token budget exceeded (%d/%d)",
				usage.OutputTokens, b.DailyOutputTokens)
		}
	}

	return true, ""
}
