package terminal

import "time"

type Manager interface {
	EnsureExecWindow(logPath string)
	OpenExecWindow(focusID, logPath string)
	OpenSubagentWindow(sessionID, logPath string)
	StartCleanupLoop(interval, maxAge time.Duration)
	CloseOld(maxAge time.Duration) int
}
