package transcript

import (
	"bytes"
	"context"
	"io"
	"strings"
	"time"
)

// HTML renders the complete transcript document.
//
// The result is one file that renders in a browser with nothing but the
// stylesheet: the markup is already complete, and media is resolved through
// Options.Media, which by default inlines it as base64 so the document keeps
// working after Discord's signed CDN URLs expire.
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
	fragment, err := renderFragment(context.Background(), t, &o)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, buildDocument(t, &o, fragment))
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

// buildDocument assembles the final file.
//
// It contains no comments and no whitespace between tags: everything in it
// either renders or tells the browser how to render, so the file is as small as
// a usable document can be. Offsets where a comment would usually go (asset
// hashes, mirrors, provenance) live in the code and in the repository instead.
func buildDocument(t *Transcript, o *Options, fragment string) string {
	var b strings.Builder

	b.WriteString(`<!doctype html><html lang="en"`)
	if o.Theme == ThemeLight {
		b.WriteString(` data-theme="light"`)
	}
	b.WriteString(`><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">`)

	if title := documentTitle(t, o); title != "" {
		b.WriteString("<title>" + escapeText(title) + "</title>")
	}

	if o.Assets.CSSURL != "" {
		b.WriteString(`<link rel="stylesheet" href="` + escapeText(o.Assets.CSSURL) + `"`)
		if o.Assets.CSSIntegrity != "" {
			b.WriteString(` integrity="` + escapeText(o.Assets.CSSIntegrity) + `"`)
		}
		if o.Assets.CrossOrigin {
			b.WriteString(` crossorigin="anonymous"`)
		}
		b.WriteString(">")
	}

	if o.Assets.ScriptURL != "" {
		// The script is a progressive extra: the markup is already complete, so
		// deferring it never delays the first paint.
		b.WriteString(`<script defer src="` + escapeText(o.Assets.ScriptURL) + `"`)
		if o.Assets.ScriptIntegrity != "" {
			b.WriteString(` integrity="` + escapeText(o.Assets.ScriptIntegrity) + `"`)
		}
		if o.Assets.CrossOrigin {
			b.WriteString(` crossorigin="anonymous"`)
		}
		b.WriteString("></script>")
	}

	b.WriteString(`</head><body class="dt-page"><div class="dt-page__inner">`)
	if o.ShowHeader {
		if title := documentTitle(t, o); title != "" {
			b.WriteString(`<h1 class="dt-page__title">` + escapeText(title) + `</h1>`)
		}
	}
	if o.ShowMeta {
		if meta := metaLine(t, o); meta != "" {
			b.WriteString(`<p class="dt-page__meta">` + escapeText(meta) + `</p>`)
		}
	}
	b.WriteString(fragment)
	b.WriteString(`</div></body></html>`)
	return b.String()
}

// documentTitle picks the <title>, or returns "" to omit it entirely.
func documentTitle(t *Transcript, o *Options) string {
	switch {
	case t.Title != "":
		return t.Title
	case o.Title != "":
		return o.Title
	case effectiveChannel(t, o).Name != "":
		return "#" + effectiveChannel(t, o).Name
	default:
		return ""
	}
}

// effectiveChannel prefers the transcript's own channel and falls back to the
// one supplied through Options.
func effectiveChannel(t *Transcript, o *Options) Channel {
	if t.Channel.Name != "" || t.Channel.Type != "" {
		return t.Channel
	}
	return o.Channel
}

// metaLine summarises the conversation. Only rendered with WithMeta, because it
// is content the reader did not ask for.
func metaLine(t *Transcript, o *Options) string {
	var parts []string
	count := 0
	var first, last time.Time
	for _, m := range t.Messages {
		if m.System != nil {
			continue
		}
		count++
		if m.Timestamp.IsZero() {
			continue
		}
		if first.IsZero() || m.Timestamp.Before(first) {
			first = m.Timestamp
		}
		if last.IsZero() || m.Timestamp.After(last) {
			last = m.Timestamp
		}
	}
	if count > 0 {
		if count == 1 {
			parts = append(parts, "1 message")
		} else {
			parts = append(parts, itoa(count)+" messages")
		}
	}
	if !first.IsZero() {
		loc := o.TimeZone
		if loc == nil {
			loc = time.UTC
		}
		if first.Equal(last) {
			parts = append(parts, first.In(loc).Format("2 January 2006"))
		} else {
			parts = append(parts, first.In(loc).Format("2 January 2006")+" – "+last.In(loc).Format("2 January 2006"))
		}
	}
	if channel := effectiveChannel(t, o); channel.Guild != "" {
		parts = append(parts, channel.Guild)
	}
	if o.Generator != "" {
		parts = append(parts, o.Generator)
	}
	return strings.Join(parts, " · ")
}
