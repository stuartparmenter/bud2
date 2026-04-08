// Package paths provides well-known filesystem path helpers for bud.
package paths

import (
	"os"
	"path/filepath"
)

// LogDir returns the bud log directory: ~/Library/Logs/bud/
func LogDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Logs", "bud")
}
