// Command syncassets refreshes the copies of the discord-transcript-ui build
// artifacts that this module embeds.
//
// Run it through go generate from the repository root:
//
//	go generate ./...
//
// The JavaScript repository is expected next to this module (../). Override the
// location with DISCORD_TRANSCRIPT_UI_DIR when it lives elsewhere:
//
//	DISCORD_TRANSCRIPT_UI_DIR=/path/to/transcripts go generate ./...
//
// Committing the copied files keeps this module buildable on its own, without
// the JavaScript repository present.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// file pairs a source relative to the UI repository with a destination relative
// to the module root.
type file struct {
	source string
	dest   string
	// optional files are reported but not treated as an error when missing.
	optional bool
}

var files = []file{
	{source: "dist/manifest.json", dest: "transcript/assets/manifest.json"},
	{source: "dist/recovery-comment.txt", dest: "transcript/assets/recovery-comment.txt"},
	{source: "assets/cdn-loader.html", dest: "transcript/assets/cdn-loader.html"},
	{source: "dist/discord-transcript.min.css", dest: "transcript/assets/discord-transcript.min.css"},
	{source: "dist/discord-transcript.min.js", dest: "transcript/assets/discord-transcript.min.js"},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "syncassets:", err)
		os.Exit(1)
	}
}

func run() error {
	moduleRoot, err := findModuleRoot()
	if err != nil {
		return err
	}
	uiDir := os.Getenv("DISCORD_TRANSCRIPT_UI_DIR")
	if uiDir == "" {
		uiDir = filepath.Clean(filepath.Join(moduleRoot, ".."))
	}

	copied, unchanged := 0, 0
	for _, f := range files {
		src := filepath.Join(uiDir, f.source)
		dst := filepath.Join(moduleRoot, f.dest)

		data, err := os.ReadFile(src)
		if err != nil {
			if f.optional && os.IsNotExist(err) {
				fmt.Printf("skip  %s (not present)\n", f.source)
				continue
			}
			return fmt.Errorf("read %s: %w (set DISCORD_TRANSCRIPT_UI_DIR if the UI repo is elsewhere)", src, err)
		}
		if existing, err := os.ReadFile(dst); err == nil && hash(existing) == hash(data) {
			unchanged++
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
		fmt.Printf("sync  %-42s -> %s (%d bytes)\n", f.source, f.dest, len(data))
		copied++
	}
	fmt.Printf("done: %d copied, %d already current (source %s)\n", copied, unchanged, uiDir)
	return nil
}

// findModuleRoot walks up from the working directory until it finds go.mod.
func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod found above %s", dir)
		}
		dir = parent
	}
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
