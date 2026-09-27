package transcript

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// The media pool: a conversation that shows the same avatar in twenty messages
// should not carry twenty copies of it.
//
// It runs on the finished document rather than while rendering, because the
// decision needs the whole document — an image is only worth hoisting once it
// is known to repeat. The renderer marks the images whose bytes are inline and
// whose role is decorative; the pass below hoists every blob that appears more
// than once into one <style> block and points each occurrence at it.
//
// The markup stays self-contained: the rules live in the document, so nothing
// depends on a stylesheet release, and no script has to run. An image that
// appears once is left exactly as it was, as an <img> with a data URI.

// pooledTagRE matches the images the renderer marked as poolable. It is safe to
// match this narrowly: the renderer writes these tags itself, every attribute is
// double-quoted, and escapeText has already turned any " or > in the values into
// entities.
var pooledTagRE = regexp.MustCompile(`<img src="([^"]*)" alt="([^"]*)"[^>]* data-dt-media="([a-z-]+)">`)

// pooledBaseRule lays out a hoisted element. Each role is already inside a sized
// box in the stylesheet (an avatar, an embed footer icon, a thumbnail), so the
// element just fills it.
const pooledBaseRule = `.dt-media{display:block;background-position:center;background-size:cover;background-repeat:no-repeat}`

// pooledRoleStyle is the box a hoisted element takes. The embed author row is a
// flex row with no size of its own, so that one carries its own.
var pooledRoleStyle = map[string]string{
	"avatar":      "width:100%;height:100%",
	"author-icon": "width:24px;height:24px;border-radius:50%",
	"footer-icon": "width:100%;height:100%",
	"thumbnail":   "width:100%;height:100%",
}

// applyMediaPool rewrites a finished document so each repeated inline image is
// stored once. Documents with nothing to hoist come back untouched.
func applyMediaPool(doc string) string {
	matches := pooledTagRE.FindAllStringSubmatchIndex(doc, -1)
	if len(matches) == 0 {
		return doc
	}

	// Count how often each blob is used before deciding anything.
	counts := make(map[string]int, len(matches))
	for _, m := range matches {
		counts[doc[m[2]:m[3]]]++
	}

	var rules bytes.Buffer
	var out strings.Builder
	out.Grow(len(doc))
	ids := make(map[string]int)
	last := 0
	for _, m := range matches {
		src := doc[m[2]:m[3]]
		alt := doc[m[4]:m[5]]
		role := doc[m[6]:m[7]]

		out.WriteString(doc[last:m[0]])
		last = m[1]

		if counts[src] < 2 {
			// Used once: the marker comes off and the image stays an image.
			fmt.Fprintf(&out, `<img src="%s" alt="%s" loading="lazy" decoding="async">`, src, alt)
			continue
		}
		id, ok := ids[src]
		if !ok {
			id = len(ids) + 1
			ids[src] = id
			fmt.Fprintf(&rules, ".dt-media-%d{background-image:url(%q)}", id, src)
		}
		fmt.Fprintf(&out, `<span class="dt-media dt-media-%d" role="img" aria-label="%s" style="%s"></span>`,
			id, alt, pooledRoleStyle[role])
	}
	out.WriteString(doc[last:])
	if len(ids) == 0 {
		return out.String()
	}

	style := "<style data-dt-media-pool>" + pooledBaseRule + rules.String() + "</style>"
	return strings.Replace(out.String(), "</head>", style+"</head>", 1)
}
