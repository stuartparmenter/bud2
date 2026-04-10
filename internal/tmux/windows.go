// Package tmux manages tmux windows for agent observability.
// It opens a named window per executive wake and per subagent session,
// and periodically closes windows older than the configured age.
package tmux

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vthunder/bud2/internal/terminal"
)

const session = "bud"

// Manager is a tmux-based terminal window manager that satisfies the
// terminal.Manager interface.
type Manager struct{}

// NewManager creates a new tmux Manager.
func NewManager() *Manager {
	return &Manager{}
}

var _ terminal.Manager = (*Manager)(nil)

var execWindowOnce sync.Once

// EnsureExecWindow opens the persistent executive log window exactly once per
// process lifetime. Subsequent calls are no-ops.
func (m *Manager) EnsureExecWindow(logPath string) {
	execWindowOnce.Do(func() {
		openWindow("exec", "exec-persistent", "tail -f "+logPath)
	})
}

// OpenExecWindow opens a tmux window showing the executive session event log.
// Uses tail -F (uppercase) so the window waits for the file to appear if not yet created.
func (m *Manager) OpenExecWindow(focusID, logPath string) {
	openWindow("exec", focusID, "tail -F "+logPath)
}

// OpenSubagentWindow opens a tmux window tailing the subagent session log file.
// Uses tail -F so the window waits for the file to appear if it hasn't been created yet.
func (m *Manager) OpenSubagentWindow(sessionID, logPath string) {
	openWindow("sub", sessionID, "tail -F "+logPath)
}

func openWindow(windowType, id, command string) {
	if err := ensureSession(); err != nil {
		log.Printf("[tmux] cannot ensure session %q: %v", session, err)
		return
	}
	epoch := time.Now().Unix()
	shortID := id
	if len(shortID) > 6 {
		shortID = shortID[:6]
	}
	windowName := fmt.Sprintf("bud-%s-%d-%s", windowType, epoch, shortID)
	if err := exec.Command("tmux", "new-window", "-t", session+":", "-n", windowName, command).Run(); err != nil {
		log.Printf("[tmux] failed to open window %q: %v", windowName, err)
	}
}

func ensureSession() error {
	if exec.Command("tmux", "has-session", "-t", session).Run() == nil {
		return nil
	}
	return exec.Command("tmux", "new-session", "-d", "-s", session).Run()
}

// CloseOld removes windows from the bud tmux session created more than
// maxAge ago. Silently returns 0 if tmux is not running or the session doesn't exist.
func (m *Manager) CloseOld(maxAge time.Duration) int {
	out, err := exec.Command("tmux", "list-windows", "-t", session, "-F", "#{window_index}:#{window_name}").Output()
	if err != nil {
		return 0
	}
	cutoff := time.Now().Add(-maxAge).Unix()
	var toKill []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		idx, name, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		segs := strings.Split(name, "-")
		if len(segs) < 4 || segs[0] != "bud" {
			continue
		}
		epoch, err := strconv.ParseInt(segs[2], 10, 64)
		if err != nil {
			continue
		}
		if epoch < cutoff {
			toKill = append(toKill, idx)
		}
	}
	for i := len(toKill) - 1; i >= 0; i-- {
		if err := exec.Command("tmux", "kill-window", "-t", session+":"+toKill[i]).Run(); err != nil {
			log.Printf("[tmux] failed to kill window %s: %v", toKill[i], err)
		}
	}
	if n := len(toKill); n > 0 {
		log.Printf("[tmux] closed %d old window(s)", n)
	}
	return len(toKill)
}

// StartCleanupLoop runs CloseOld on the given interval in a background goroutine.
func (m *Manager) StartCleanupLoop(interval, maxAge time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			m.CloseOld(maxAge)
		}
	}()
}
