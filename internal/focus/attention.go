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

// GetState returns a copy of the current state
func (a *Attention) GetState() FocusState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}
