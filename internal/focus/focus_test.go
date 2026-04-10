package focus

import (
	"testing"
)

// TestFocusSuspension tests that focus can be suspended and resumed
func TestFocusSuspension(t *testing.T) {
	attention := New()

	// Focus on a task
	task1 := &PendingItem{
		ID:      "task-1",
		Content: "Working on feature",
	}
	attention.Focus(task1)

	// Verify current focus
	current := attention.GetState().CurrentItem
	if current == nil || current.ID != "task-1" {
		t.Error("Expected task-1 to be current focus")
	}

	// Interrupt with higher priority
	interrupt := &PendingItem{
		ID:      "msg-1",
		Content: "Urgent message",
	}
	attention.Focus(interrupt)

	// msg-1 should be current, task-1 should be suspended
	state := attention.GetState()
	if state.CurrentItem == nil || state.CurrentItem.ID != "msg-1" {
		t.Error("Expected msg-1 to be current focus after interrupt")
	}
	if len(state.Suspended) != 1 || state.Suspended[0].ID != "task-1" {
		t.Error("Expected task-1 to be in suspended stack")
	}

	// Complete the interrupt
	attention.Complete()

	// task-1 should be automatically resumed
	state = attention.GetState()
	if state.CurrentItem == nil || state.CurrentItem.ID != "task-1" {
		t.Error("Expected task-1 to be resumed after completing interrupt")
	}
}

// TestPriorityString tests priority string representation
func TestPriorityString(t *testing.T) {
	tests := []struct {
		priority Priority
		expected string
	}{
		{P0Critical, "P0:Critical"},
		{P1UserInput, "P1:UserInput"},
		{P2DueTask, "P2:DueTask"},
		{P3ActiveWork, "P3:ActiveWork"},
		{P4Exploration, "P4:Exploration"},
		{Priority(99), "Unknown"},
	}

	for _, tt := range tests {
		result := tt.priority.String()
		if result != tt.expected {
			t.Errorf("Priority(%d).String() = %q, want %q", tt.priority, result, tt.expected)
		}
	}
}
