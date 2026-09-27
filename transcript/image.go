package transcript

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/url"
	"strconv"
	"strings"
)

// Avatars and icons arrive far larger than they are drawn: Discord serves up to
// 1024px for something that renders at 20–32 CSS pixels, and a single 128px
// avatar can weigh more than the rest of the document.
//
// The edges below are the longest side worth keeping, in device pixels — twice
// the CSS size, so the result stays crisp on a retina display. Every icon class
// shares one edge on purpose: the same avatar then resolves to the same bytes
// wherever it appears (message, reply, embed author, embed footer), which is
// what lets the media pool collapse the copies into one blob.
const (
	edgeIcon      = 64  // avatars, embed author/footer icons, custom emoji (drawn 20–32px)
	edgeThumbnail = 160 // embed thumbnails (drawn 80px)
)

// discordSizedURL asks Discord's CDN for a smaller copy of a fixed-size image,
// so the bytes never travel in the first place.
//
// Only the endpoints that document a `size` parameter are touched. Attachments
// are left alone: a wrong parameter there would either be ignored or change the
// image, and the download is already capped by MaxMediaBytes.
func discordSizedURL(raw string, edge int) string {
	if edge <= 0 || !isRemote(raw) {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	switch strings.ToLower(u.Hostname()) {
	case "cdn.discordapp.com", "media.discordapp.net":
	default:
		return raw
	}
	switch {
	case strings.HasPrefix(u.Path, "/avatars/"),
		strings.HasPrefix(u.Path, "/icons/"),
		strings.HasPrefix(u.Path, "/banners/"),
		strings.HasPrefix(u.Path, "/emojis/"),
		strings.HasPrefix(u.Path, "/embed/avatars/"):
	default:
		return raw
	}
	// A size the producer already asked for is only replaced when it is larger
	// than what the renderer needs: a thumbnail that was already requested small
	// stays small, and one requested at 1024 stops at the size we draw it.
	query := u.Query()
	if existing, err := strconv.Atoi(query.Get("size")); err == nil && existing <= edge {
		return raw
	}
	query.Set("size", strconv.Itoa(discordSize(edge)))
	u.RawQuery = query.Encode()
	return u.String()
}

// discordSize rounds an edge up to the power of two Discord accepts (16…4096).
// It returns the request unchanged when asked for something out of range.
func discordSize(edge int) int {
	size := 16
	for size < edge && size < 4096 {
		size *= 2
	}
	return size
}

// ShrinkImage downscales an in-memory image so its longest side is at most
// edge, re-encoding it in the format it arrived in, and returns the content type
// that goes with the bytes it produced.
//
// The built-in stores call this for the MediaRef.TargetEdge they are given. A
// custom MediaStore can call it too, and should, or avatars will keep arriving
// at ten times the size they are drawn at.
//
// It is deliberately conservative: the original bytes are returned whenever the
// result would not be smaller. An image that is already small, animated (GIF),
// vector (SVG), or in a format the standard library cannot decode (WebP, AVIF)
// is passed through untouched rather than re-encoded into something larger or
// flatter. That also means a caller can enable this without ever making a
// document bigger.
func ShrinkImage(data []byte, contentType string, edge int) ([]byte, string) {
	if edge <= 0 || len(data) == 0 {
		return data, contentType
	}
	switch contentType {
	case "image/png", "image/jpeg":
	default:
		return data, contentType
	}
	src, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		// Undecodable, or a format no decoder was linked in for (GIF is on
		// purpose: re-encoding it would drop every frame but the first).
		return data, contentType
	}
	bounds := src.Bounds()
	if bounds.Dx() <= edge && bounds.Dy() <= edge {
		return data, contentType
	}

	out := downscale(src, edge)
	var buf bytes.Buffer
	if format == "jpeg" {
		if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 82}); err != nil {
			return data, contentType
		}
		if buf.Len() >= len(data) {
			return data, contentType
		}
		return buf.Bytes(), "image/jpeg"
	}
	if err := png.Encode(&buf, out); err != nil {
		return data, contentType
	}
	if buf.Len() >= len(data) {
		return data, contentType
	}
	return buf.Bytes(), "image/png"
}

// shrinkDataURI applies shrinkImage to an already inlined image, which is how a
// producer that hands over data URIs still benefits from right-sizing.
func shrinkDataURI(uri string, edge int) string {
	if edge <= 0 {
		return uri
	}
	meta, payload, ok := strings.Cut(uri, ",")
	if !ok || !strings.Contains(meta, ";base64") {
		return uri
	}
	contentType := strings.TrimPrefix(meta, "data:")
	contentType, _, _ = strings.Cut(contentType, ";")
	contentType = strings.TrimSpace(contentType)
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return uri
	}
	shrunk, newType := ShrinkImage(data, contentType, edge)
	if newType == contentType && len(shrunk) == len(data) {
		return uri
	}
	return "data:" + newType + ";base64," + base64.StdEncoding.EncodeToString(shrunk)
}

// downscale averages the source pixels that each destination pixel covers. For
// a shrinking box filter that is close to what a good resampler does, and it
// needs no dependency: golang.org/x/image is not an option here.
func downscale(src image.Image, edge int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := float64(edge) / float64(w)
	if h > w {
		scale = float64(edge) / float64(h)
	}
	dw := int(float64(w)*scale + 0.5)
	dh := int(float64(h)*scale + 0.5)
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}

	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		y0 := b.Min.Y + y*h/dh
		y1 := b.Min.Y + (y+1)*h/dh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < dw; x++ {
			x0 := b.Min.X + x*w/dw
			x1 := b.Min.X + (x+1)*w/dw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, ca := src.At(sx, sy).RGBA() // premultiplied
					r += uint64(cr)
					g += uint64(cg)
					bl += uint64(cb)
					a += uint64(ca)
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			r, g, bl, a = r/n, g/n, bl/n, a/n
			if a == 0 {
				dst.SetNRGBA(x, y, color.NRGBA{})
				continue
			}
			// Undo the premultiplication the averages above are in.
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8((r * 0xffff / a) >> 8),
				G: uint8((g * 0xffff / a) >> 8),
				B: uint8((bl * 0xffff / a) >> 8),
				A: uint8(a >> 8),
			})
		}
	}
	return dst
}
