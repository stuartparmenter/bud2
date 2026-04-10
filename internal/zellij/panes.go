// Package zellij manages zellij panes for agent observability.
// It opens a named pane per executive wake and per subagent session inside a
// "Bud Sessions" tab in the existing "bud" zellij session.
package zellij

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/vthunder/bud2/internal/terminal"
)

// zellijBin returns the path to the zellij binary, checking common install
// locations that may not be in the launchd PATH.
func zellijBin() string {
	if path, err := exec.LookPath("zellij"); err == nil {
		return path
	}
	home := os.Getenv("HOME")
	candidates := []string{
		home + "/.cargo/bin/zellij",
		"/opt/homebrew/bin/zellij",
		"/usr/local/bin/zellij",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "zellij"
}

const (
	zellijSession = "bud"
	tabName       = "Bud Sessions"
)

// Manager is a zellij-based terminal window manager that satisfies the
// terminal.Manager interface.
type Manager struct{}

// NewManager creates a new zellij Manager.
func NewManager() *Manager {
	return &Manager{}
}

var _ terminal.Manager = (*Manager)(nil)

// execWindowOnce ensures EnsureExecWindow only opens one pane per process lifetime.
var execWindowOnce sync.Once

// EnsureExecWindow opens the persistent executive log pane exactly once per
// process lifetime. Subsequent calls are no-ops.
func (m *Manager) EnsureExecWindow(logPath string) {
	execWindowOnce.Do(func() {
		openPane("bud-exec", "tail -n +1 -F "+logPath)
	})
}

// OpenExecWindow opens a zellij pane tailing the executive session event log.
func (m *Manager) OpenExecWindow(focusID, logPath string) {
	shortID := focusID
	if len(shortID) > 6 {
		shortID = shortID[:6]
	}
	epoch := time.Now().Unix()
	paneName := fmt.Sprintf("exec-%d-%s", epoch, shortID)
	openPane(paneName, "tail -n +1 -F "+logPath)
}

// OpenSubagentWindow opens a zellij pane tailing the subagent session log file.
func (m *Manager) OpenSubagentWindow(sessionID, logPath string) {
	shortID := sessionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	epoch := time.Now().Unix()
	paneName := fmt.Sprintf("sub-%d-%s", epoch, shortID)
	openPane(paneName, "tail -n +1 -F "+logPath)
}

// CloseOld is a no-op. Zellij's CLI does not expose a list-panes-by-name
// command, so age-based cleanup is not yet implemented. Panes in "Bud Sessions"
// can be closed manually or via a future implementation.
func (m *Manager) CloseOld(_ time.Duration) int { return 0 }

// StartCleanupLoop is a no-op matching the terminal.Manager interface.
// Zellij pane cleanup is not yet implemented.
func (m *Manager) StartCleanupLoop(_, _ time.Duration) {}

func openPane(paneName, command string) {
	if err := ensureTab(); err != nil {
		log.Printf("[zellij] cannot ensure tab %q: %v", tabName, err)
		return
	}
	if err := exec.Command(zellijBin(),
		"--session", zellijSession,
		"run", "--name", paneName, "--",
		"sh", "-c", command,
	).Run(); err != nil {
		log.Printf("[zellij] failed to open pane %q: %v", paneName, err)
	}
}

func ensureTab() error {
	return exec.Command(zellijBin(),
		"--session", zellijSession,
		"action", "go-to-tab-name", tabName, "--create",
	).Run()
}
