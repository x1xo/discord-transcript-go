package transcript

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// HTML renders the complete, self-contained transcript document.
//
// The result is one file: the pinned stylesheet and script (linked from a CDN,
// referenced locally, or inlined), the author profile map, and the conversation
// itself. Media is resolved through Options.Media, which by default downloads it
// and inlines it as base64, so the document keeps working after Discord's signed
// CDN URLs expire.
func (t *Transcript) HTML(opts ...Option) ([]byte, error) {
	var buf bytes.Buffer
	if err := t.WriteTo(&buf, opts...); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteTo writes the document to w.
func (t *Transcript) WriteTo(w io.Writer, opts ...Option) error {
	o := Apply(opts...)
	if err := checkContractVersion(); err != nil {
		o.warn(err)
	}

	fragment, profiles, order, err := renderFragment(context.Background(), t, &o)
	if err != nil {
		return err
	}
	document, err := buildDocument(t, &o, fragment, profiles, order)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, document)
	return err
}

// WriteFile writes the document to a file, creating parent directories.
func (t *Transcript) WriteFile(path string, opts ...Option) error {
	var buf bytes.Buffer
	if err := t.WriteTo(&buf, opts...); err != nil {
		return err
	}
	return osWriteFile(path, buf.Bytes())
}

// profileConfig is the shape the enhancement script expects in
// window.$discordMessage.profiles.
type profileConfig struct {
	Author      string `json:"author"`
	Avatar      string `json:"avatar,omitempty"`
	RoleColor   string `json:"roleColor,omitempty"`
	Bot         bool   `json:"bot,omitempty"`
	Verified    bool   `json:"verified,omitempty"`
	Server      bool   `json:"server,omitempty"`
	OfficialApp bool   `json:"officialApp,omitempty"`
	OP          bool   `json:"op,omitempty"`
}

type discordMessageConfig struct {
	Profiles map[string]profileConfig `json:"profiles"`
}

type scriptConfig struct {
	Locale string `json:"locale,omitempty"`
}

func buildDocument(t *Transcript, o *Options, fragment string, profiles map[string]Author, order []string) (string, error) {
	var b strings.Builder

	title := t.Title
	if title == "" {
		title = o.Title
	}
	if title == "" && t.Channel.Name != "" {
		title = "#" + t.Channel.Name
	}
	if title == "" {
		title = "Discord transcript"
	}

	b.WriteString("<!doctype html>\n")
	if o.Theme == ThemeLight {
		b.WriteString("<html lang=\"en\" data-theme=\"light\">\n")
	} else {
		b.WriteString("<html lang=\"en\">\n")
	}
	b.WriteString("<head>\n")
	b.WriteString("\t<meta charset=\"utf-8\" />\n")
	b.WriteString("\t<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\" />\n")
	b.WriteString("\t<meta name=\"color-scheme\" content=\"dark light\" />\n")
	if o.Generator != "" {
		b.WriteString("\t<meta name=\"generator\" content=\"" + escapeText(o.Generator) + "\" />\n")
	}
	b.WriteString("\t<title>" + escapeText(title) + "</title>\n")

	if comment := strings.TrimSpace(recoveryComment); comment != "" {
		b.WriteString("\n")
		b.WriteString(indentBlock(comment, 1))
		b.WriteString("\n")
	}

	// Author metadata, shared by every message.
	config := discordMessageConfig{Profiles: make(map[string]profileConfig, len(profiles))}
	for _, key := range order {
		a := profiles[key]
		config.Profiles[key] = profileConfig{
			Author:      a.Name,
			Avatar:      a.AvatarURL,
			RoleColor:   a.RoleColor,
			Bot:         a.Bot,
			Verified:    a.Verified,
			Server:      a.Server,
			OfficialApp: a.Official,
			OP:          a.OP,
		}
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal profile map: %w", err)
	}
	scriptJSON, err := json.Marshal(scriptConfig{Locale: o.Locale})
	if err != nil {
		return "", fmt.Errorf("marshal script config: %w", err)
	}

	if o.Assets != AssetsInline {
		b.WriteString("\t<script>\n")
		b.WriteString("\t\twindow.$discordMessage = " + string(configJSON) + ";\n")
		b.WriteString("\t\twindow.discordTranscript = " + string(scriptJSON) + ";\n")
		b.WriteString("\t</script>\n")
	}

	switch o.Assets {
	case AssetsInline:
		b.WriteString("\t<style>\n")
		b.WriteString(minifiedCSS)
		if !strings.HasSuffix(minifiedCSS, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\t</style>\n")
	case AssetsLocal:
		b.WriteString("\t<link rel=\"stylesheet\" href=\"" + escapeText(o.AssetsBase+"discord-transcript.min.css") + "\" />\n")
		b.WriteString("\t<script src=\"" + escapeText(o.AssetsBase+"discord-transcript.min.js") + "\" defer></script>\n")
	default:
		// The bootstrap creates the stylesheet and script tags itself, so the
		// config above is already in place by the time it runs.
		loader, err := cdnLoader(o)
		if err != nil {
			return "", err
		}
		b.WriteString(indentBlock(loader, 1))
		b.WriteString("\n")
	}

	b.WriteString("</head>\n")
	b.WriteString("<body class=\"dt-page\">\n")
	b.WriteString("\t<div class=\"dt-page__inner\">\n")
	if o.ShowMeta {
		if meta := metaLine(t, o); meta != "" {
			b.WriteString("\t\t<p class=\"dt-page__meta\">" + escapeText(meta) + "</p>\n")
		}
	}
	b.WriteString(indentBlock(fragment, 2))
	b.WriteString("\n\t</div>\n")
	if o.Assets == AssetsInline {
		b.WriteString("\t<script>\n")
		b.WriteString(minifiedJS)
		if !strings.HasSuffix(minifiedJS, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\t</script>\n")
	}
	b.WriteString("</body>\n</html>\n")
	return b.String(), nil
}

// cdnOrder is the mirror preference: npm-backed mirrors first (they serve
// byte-identical files with CORS, so they can carry SRI), then independent
// copies, then anything else the manifest knows about.
var cdnOrder = []string{
	"jsdelivr", "unpkg", "esm.run", "esm.sh",
	"github-pages", "statically", "raw-githack", "self-hosted",
}

// sriCapable lists the mirrors that are known to send CORS headers, which SRI
// requires for a cross-origin resource.
var sriCapable = map[string]bool{"jsdelivr": true, "unpkg": true, "esm.run": true, "esm.sh": true}

// cdnLoader fills the multi-mirror bootstrap with the URLs from the embedded
// manifest.
func cdnLoader(o *Options) (string, error) {
	manifest, err := EmbeddedManifest()
	if err != nil {
		return "", err
	}
	css, err := mirrorSources(manifest, "css")
	if err != nil {
		return "", err
	}
	js, err := mirrorSources(manifest, "js")
	if err != nil {
		return "", err
	}
	cssJSON, err := json.Marshal(css)
	if err != nil {
		return "", err
	}
	jsJSON, err := json.Marshal(js)
	if err != nil {
		return "", err
	}
	out := strings.Replace(cdnLoaderTemplate, "%%CSS_SOURCES%%", string(cssJSON), 1)
	out = strings.Replace(out, "%%JS_SOURCES%%", string(jsJSON), 1)
	// The loader's own explanatory comment mentions "%%…%%", so only the real
	// tokens count as a failure.
	if strings.Contains(out, "%%CSS_SOURCES%%") || strings.Contains(out, "%%JS_SOURCES%%") {
		return "", fmt.Errorf("cdn loader template still has placeholders after substitution")
	}
	return out, nil
}

type mirrorSource struct {
	URL       string `json:"u"`
	Integrity string `json:"s,omitempty"`
}

func mirrorSources(manifest Manifest, extension string) ([]mirrorSource, error) {
	urls := manifest.URLs[extension]
	if len(urls) == 0 {
		return nil, fmt.Errorf("embedded manifest has no %s mirrors", extension)
	}
	integrity := manifest.Integrity[extension]

	seen := make(map[string]bool, len(urls))
	var ordered []string
	for _, id := range cdnOrder {
		if _, ok := urls[id]; ok {
			ordered = append(ordered, id)
			seen[id] = true
		}
	}
	rest := make([]string, 0, len(urls))
	for id := range urls {
		if !seen[id] {
			rest = append(rest, id)
		}
	}
	sort.Strings(rest)
	ordered = append(ordered, rest...)

	out := make([]mirrorSource, 0, len(ordered))
	for _, id := range ordered {
		src := mirrorSource{URL: urls[id]}
		if sriCapable[id] {
			src.Integrity = integrity
		}
		out = append(out, src)
	}
	return out, nil
}

// metaLine summarises the conversation for the page header.
func metaLine(t *Transcript, o *Options) string {
	parts := []string{}
	count := 0
	var first, last time.Time
	for _, m := range t.Messages {
		if m.System != nil {
			continue
		}
		count++
		if !m.Timestamp.IsZero() {
			if first.IsZero() || m.Timestamp.Before(first) {
				first = m.Timestamp
			}
			if last.IsZero() || m.Timestamp.After(last) {
				last = m.Timestamp
			}
		}
	}
	if count == 1 {
		parts = append(parts, "1 message")
	} else if count > 0 {
		parts = append(parts, itoa(count)+" messages")
	}
	if !first.IsZero() {
		if first.Equal(last) {
			parts = append(parts, first.UTC().Format("2 January 2006"))
		} else {
			parts = append(parts, first.UTC().Format("2 January 2006")+" – "+last.UTC().Format("2 January 2006"))
		}
	}
	if t.Channel.Guild != "" {
		parts = append(parts, t.Channel.Guild)
	}
	if o.Generator != "" {
		parts = append(parts, o.Generator)
	}
	return strings.Join(parts, " · ")
}

// indentBlock prefixes every non-empty line with tabs.
func indentBlock(s string, tabs int) string {
	pad := strings.Repeat("\t", tabs)
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = pad + line
	}
	return strings.Join(lines, "\n")
}
