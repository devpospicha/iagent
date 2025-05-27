package agentidentity

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/devpospicha/ishared/utils"
	"github.com/google/uuid"
)

// LoadOrCreateAgentID returns a stable UUID stored on disk.
// It creates a new one on first run and saves it to disk.
func LoadOrCreateAgentID() (string, error) {
	path := getAgentIDPath()

	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		utils.Debug("Loaded agent ID from disk: %s", string(data))
		return string(data), nil
	}

	id := uuid.NewString()
	utils.Debug("Generated new agent ID: %s", id)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(id), 0600); err != nil {
		return "", err
	}

	return id, nil
}

// getAgentIDPath returns the path to the agent ID file based on the operating system.
// It uses the APPDATA environment variable for Windows and XDG_STATE_HOME for Linux.
// If these variables are not set, it falls back to a default path in the user's home directory.
func getAgentIDPath() string {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("APPDATA") // e.g. C:\Users\you\AppData\Roaming
		if base == "" {
			base = "C:\\gosight"
		}
		return filepath.Join(base, "gosight", "agent_id")
	default:
		base := os.Getenv("XDG_STATE_HOME")
		if base == "" {
			base = filepath.Join(os.Getenv("HOME"), ".local", "state")
		}
		return filepath.Join(base, "gosight", "agent_id")
	}
}
