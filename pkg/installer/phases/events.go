package phases

import (
	"time"
)

// EventType represents the type of event during installation.
type EventType int

const (
	EventPhaseStart    EventType = iota
	EventPhaseLog                // a log line from the current phase
	EventPhaseComplete           // phase finished successfully
	EventPhaseFailed             // phase finished with error
	EventPhaseSkipped            // phase was already completed (resume)
	EventInstallComplete
	EventRebootRequired
)

// PhaseEvent is sent from the phase runner to the TUI (or plain-text renderer).
type PhaseEvent struct {
	Type     EventType
	Phase    string        // phase name
	PhaseIdx int           // 0-based index
	Total    int           // total number of phases
	Message  string        // log line (for EventPhaseLog)
	Duration time.Duration // how long the phase took (for EventPhaseComplete)
	Err      error         // error detail (for EventPhaseFailed)
}
