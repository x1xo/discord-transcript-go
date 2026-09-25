package transcript

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// The embedded copies of the discord-transcript-ui build artifacts. Refresh them
// with `go generate ./...` after rebuilding the JavaScript library; the sync
// tool copies from ../dist and ../assets so the two stay in step.
//
//go:generate go run ../internal/syncassets

//go:embed assets/manifest.json
var manifestJSON []byte

//go:embed assets/recovery-comment.txt
var recoveryComment string

//go:embed assets/cdn-loader.html
var cdnLoaderTemplate string

//go:embed assets/discord-transcript.min.css
var minifiedCSS string

//go:embed assets/discord-transcript.min.js
var minifiedJS string

// Artifact describes one built file of the JavaScript library.
type Artifact struct {
	Bytes  int    `json:"bytes"`
	Gzip   int    `json:"gzip"`
	Brotli int    `json:"brotli"`
	SHA256 string `json:"sha256"`
	SRI    string `json:"sri"`
}

// Manifest mirrors dist/manifest.json: the pinned versions, hashes and the full
// list of mirrors for each asset.
type Manifest struct {
	Name      string                       `json:"name"`
	Version   string                       `json:"version"`
	License   string                       `json:"license"`
	Artifacts map[string]Artifact          `json:"artifacts"`
	URLs      map[string]map[string]string `json:"urls"`
	Integrity map[string]string            `json:"integrity"`
	Recovery  string                       `json:"recovery"`
}

// EmbeddedManifest parses the manifest that was compiled into this binary.
func EmbeddedManifest() (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(manifestJSON, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse embedded manifest: %w", err)
	}
	return m, nil
}

// RecoveryComment returns the asset recovery comment compiled into this binary.
func RecoveryComment() string { return recoveryComment }

// checkContractVersion reports whether the embedded assets match the contract
// version this module claims to emit. A mismatch is a warning, not an error: the
// markup contract is stable within a major version.
func checkContractVersion() error {
	m, err := EmbeddedManifest()
	if err != nil {
		return err
	}
	if m.Version != ContractVersion {
		return fmt.Errorf("embedded discord-transcript-ui assets are %s but this module targets %s; run go generate ./...",
			m.Version, ContractVersion)
	}
	return nil
}
