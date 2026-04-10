package types

import "time"

// Percept is sensory input with automatic properties (no judgment)
type Percept struct {
	ID        string         `json:"id"`
	Source    string         `json:"source"`    // discord, github, calendar
	Type      string         `json:"type"`      // message, notification, event
	Intensity float64        `json:"intensity"` // 0.0-1.0, automatic
	Timestamp time.Time      `json:"timestamp"`
	Tags      []string       `json:"tags"` // [from:owner], [urgent], etc
	Data      map[string]any `json:"data"` // source-specific payload

	// TODO: Features may be dead code — only inbox.go sets it; verify if any reader consumes it before removing
	Features map[string]any `json:"features,omitempty"` // sense-defined clustering features
}

// ThreadStatus represents the logical state of a thread (conversation state)
type ThreadStatus string

const (
	StatusActive   ThreadStatus = "active"   // conversation is active
	StatusPaused   ThreadStatus = "paused"   // conversation paused
	StatusFrozen   ThreadStatus = "frozen"   // conversation frozen (deprecated, use SessionState)
	StatusComplete ThreadStatus = "complete" // conversation complete
)

// SessionState represents the runtime state of a thread's Claude session
type SessionState string

const (
	SessionFocused SessionState = "focused" // has attention, Claude running (limit: 1)
	SessionActive  SessionState = "active"  // Claude running in background (limit: 3)
	SessionFrozen  SessionState = "frozen"  // no Claude process, session on disk (unlimited)
	SessionNone    SessionState = ""        // no session yet
)

// Thread is a train of thought with computed salience
type Thread struct {
	ID          string       `json:"id"`
	Goal        string       `json:"goal"`
	Status      ThreadStatus `json:"status"`
	Salience    float64      `json:"salience"`     // computed from percepts + relevance
	Activation  float64      `json:"activation"`   // current activation level (for routing)
	PerceptRefs []string     `json:"percept_refs"` // many-to-many refs to percepts
	State       ThreadState  `json:"state"`
	CreatedAt   time.Time    `json:"created_at"`
	LastActive  time.Time    `json:"last_active"`
	ProcessedAt *time.Time   `json:"processed_at,omitempty"` // when last sent to executive

	// Session management
	SessionID    string       `json:"session_id,omitempty"`    // Claude session ID for resume
	SessionState SessionState `json:"session_state,omitempty"` // runtime state: focused/active/frozen
}

// ThreadState holds thread-specific context
type ThreadState struct {
	Phase    string         `json:"phase"`
	Context  map[string]any `json:"context"`
	NextStep string         `json:"next_step"`
}

// Action is a pending effector action
type Action struct {
	ID        string         `json:"id"`
	Effector  string         `json:"effector"` // discord, github
	Type      string         `json:"type"`     // send_message, comment
	Payload   map[string]any `json:"payload"`
	Status    string         `json:"status"` // pending, complete, failed
	Timestamp time.Time      `json:"timestamp"`
}

// Trace is a consolidated memory unit (compressed from percepts)
type Trace struct {
	ID         string    `json:"id"`
	Content    string    `json:"content"`    // summarized gist
	Embedding  []float64 `json:"embedding"`  // for similarity matching
	Activation float64   `json:"activation"` // current activation level (decays)
	Strength   int       `json:"strength"`   // reinforcement count
	Sources    []string  `json:"sources"`    // percept IDs that contributed
	IsCore     bool      `json:"is_core"`    // core identity traces (always activated)
	CreatedAt  time.Time `json:"created_at"`
	LastAccess time.Time `json:"last_access"`
}

// ImpulseSource identifies where an impulse came from
type ImpulseSource string

const (
	ImpulseTask     ImpulseSource = "task"     // from tasks.json - commitment due
	ImpulseIdea     ImpulseSource = "idea"     // from ideas.json - exploration urge
	ImpulseSchedule ImpulseSource = "schedule" // from schedule.json - recurring trigger
	ImpulseSystem   ImpulseSource = "system"   // system-generated (autonomous wake, etc)
)

// Impulse is an internal motivation (vs Percept which is external)
// Impulses and percepts are scored together by attention
type Impulse struct {
	ID          string         `json:"id"`
	Source      ImpulseSource  `json:"source"`    // task, idea, schedule, system
	Type        string         `json:"type"`      // due, triggered, explore, wake
	Intensity   float64        `json:"intensity"` // 0.0-1.0, based on urgency/priority
	Timestamp   time.Time      `json:"timestamp"`
	Description string         `json:"description"` // what this impulse is about
	Data        map[string]any `json:"data"`        // source-specific payload
}
