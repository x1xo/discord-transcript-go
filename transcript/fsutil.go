package transcript

import (
	"os"
	"path/filepath"
)

// osWriteFile writes a file, creating parent directories as needed.
func osWriteFile(name string, data []byte) error {
	if dir := filepath.Dir(name); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(name, data, 0o644)
}

func joinPath(dir, name string) string { return filepath.Join(dir, name) }
