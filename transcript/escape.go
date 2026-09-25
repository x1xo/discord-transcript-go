package transcript

import (
	"html"
	"net/url"
	"strings"
)

// escapeText escapes a string for use as HTML text content. It also escapes
// quotes, so the same function is safe inside a double-quoted attribute.
func escapeText(s string) string { return html.EscapeString(s) }

// sanitizeURL returns a URL safe to place in an href, or "" when the scheme is
// not allowed. Discord content is attacker-controlled, so this is a hard gate:
// javascript:, vbscript:, file: and data: links never reach the output.
func sanitizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	// Relative references are fine and useful for local asset folders.
	if strings.HasPrefix(lower, "//") || strings.HasPrefix(lower, "/") ||
		strings.HasPrefix(lower, "./") || strings.HasPrefix(lower, "../") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto", "tel":
		return raw
	default:
		return ""
	}
}

// sanitizeMediaURL is the looser variant used for src attributes: it also allows
// inline data images produced by the media store, but never data:text/html or
// any other scheme.
func sanitizeMediaURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "data:image/") ||
		strings.HasPrefix(lower, "data:video/") ||
		strings.HasPrefix(lower, "data:audio/") {
		return raw
	}
	return sanitizeURL(raw)
}

// HumanSize formats a byte count the way Discord does: 1536 -> "1.5 KB".
func HumanSize(bytes int) string {
	const unit = 1024
	if bytes < unit {
		return itoa(bytes) + " B"
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(bytes)
	for i, suffix := range units {
		value /= unit
		if value < unit || i == len(units)-1 {
			// One decimal place, dropping a trailing ".0".
			rounded := float64(int(value*10+0.5)) / 10
			if rounded == float64(int(rounded)) {
				return itoa(int(rounded)) + " " + suffix
			}
			return formatFloat1(rounded) + " " + suffix
		}
	}
	return itoa(bytes) + " B"
}

// HexColor normalises 0xRRGGBB (Discord's embed and role colour format) into
// "#rrggbb". It returns "" for "no colour".
func HexColor(value int) string {
	if value <= 0 || value > 0xFFFFFF {
		return ""
	}
	const digits = "0123456789abcdef"
	out := make([]byte, 7)
	out[0] = '#'
	for i := 5; i >= 0; i-- {
		out[i+1] = digits[value&0xF]
		value >>= 4
	}
	return string(out)
}

// itoa and formatFloat1 avoid pulling strconv into every call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func formatFloat1(v float64) string {
	whole := int(v)
	frac := int((v-float64(whole))*10 + 0.5)
	if frac >= 10 {
		whole++
		frac = 0
	}
	return itoa(whole) + "." + itoa(frac)
}
