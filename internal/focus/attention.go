package focus

import (
	"sync"
)

// Attention manages focus with a suspended-item stack.
// Focus() pushes the current item onto the suspend stack; Complete() pops it back.
type Attention struct {
	mu    sync.RWMutex
	state FocusState
}

// New creates a new attention system
func New() *Attention {
	return &Attention{
		state: FocusState{},
	}
}

// Focus sets the current focus item, suspending any existing focus.
func (a *Attention) Focus(item *PendingItem) {
	a.mu.Lock()
	if a.state.CurrentItem != nil {
		a.state.Suspended = append([]*PendingItem{a.state.CurrentItem}, a.state.Suspended...)
	}
	a.state.CurrentItem = item
	a.mu.Unlock()
}

// Complete marks the current focus as complete, resuming the most recent suspended item.
func (a *Attention) Complete() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state.CurrentItem = nil

	if len(a.state.Suspended) > 0 {
		a.state.CurrentItem = a.state.Suspended[0]
		a.state.Suspended = a.state.Suspended[1:]
	}
}

// GetState returns a copy of the current state
func (a *Attention) GetState() FocusState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}
