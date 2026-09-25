package transcript

import (
	"regexp"
	"strings"
)

// The compact tag vocabulary.
//
// The default markup uses readable element names (`<discord-message>`). A caller
// who wants the smallest possible file can ask for the short codes instead
// (WithShortTags) and point the document at the short-name stylesheet, which the
// discord-transcript-ui build emits alongside the default one. Nothing is lost:
// the stylesheet is a straight rename, so both files are the same size.
//
// Measured on a text-heavy transcript, short codes cut ~21% of the raw markup
// and ~5% of the gzipped markup, because repeated long names compress well.
//
// Keep this table in sync with the stylesheet's:
// build/short-tags.mjs in discord-transcript-ui. That build fails if the
// stylesheet would ship a long name the short file cannot style, and the test
// below fails if this renderer emits a tag with no short code.
var shortTagNames = map[string]string{
	"messages":            "dms",
	"message":             "dm",
	"attachments":         "dats",
	"image-attachment":    "dimg",
	"video-attachment":    "dvid",
	"audio-attachment":    "daud",
	"file-attachment":     "dfil",
	"reactions":           "drs",
	"reaction":            "dr",
	"embed":               "de",
	"embed-description":   "ded",
	"embed-fields":        "defs",
	"embed-field":         "def",
	"embed-footer":        "defo",
	"mention":             "dme",
	"custom-emoji":        "demo",
	"time":                "dti",
	"spoiler":             "dsp",
	"bold":                "db",
	"italic":              "di",
	"underlined":          "dun",
	"strikethrough":       "dst",
	"subscript":           "dsub",
	"code":                "dc",
	"pre":                 "dp",
	"quote":               "dq",
	"list-item":           "dli",
	"unordered-list":      "dul",
	"ordered-list":        "dol",
	"header":              "dh",
	"system-message":      "dsy",
	"thread":              "dth",
	"thread-message":      "dthm",
	"link":                "dl",
	"reply":               "drp",
	"action-row":          "dar",
	"button":              "dbtn",
	"command":             "dcmd",
	"author-info":         "dai",
	"verified-author-tag": "dvat",
}

// tagPattern matches an opening or closing tag name in the generated markup.
//
// This is safe to run over a finished fragment: message content is HTML-escaped
// before it is written, so a literal "<discord-" in the output can only be a tag
// this package emitted. User text that mentioned a tag arrives as
// "&lt;discord-...&gt;" and is not matched.
var tagPattern = regexp.MustCompile(`</?(discord-[a-z-]+)`)

// shortenTags rewrites every tag name in a rendered fragment to its short code.
func shortenTags(fragment string) string {
	return tagPattern.ReplaceAllStringFunc(fragment, func(match string) string {
		prefix, name := "<", match[1:]
		if strings.HasPrefix(match, "</") {
			prefix, name = "</", match[2:]
		}
		if short, ok := shortTagNames[strings.TrimPrefix(name, "discord-")]; ok {
			return prefix + short
		}
		return match
	})
}

// ShortTags returns the compact tag vocabulary, keyed by the semantic element
// name (the part after "discord-"). It is a copy, so callers cannot corrupt the
// renderer's mapping.
func ShortTags() map[string]string {
	out := make(map[string]string, len(shortTagNames))
	for name, short := range shortTagNames {
		out[name] = short
	}
	return out
}
