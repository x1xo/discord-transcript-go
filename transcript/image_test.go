package transcript

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// noisyPNG builds a deterministically noisy image, which is what makes a PNG
// big enough for downscaling to be worth anything.
func noisyPNG(t *testing.T, size int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	seed := uint32(12345)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			seed = seed*1664525 + 1013904223
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(seed >> 24), G: uint8(seed >> 16), B: uint8(seed >> 8), A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func decodePNG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if format != "png" {
		t.Fatalf("expected a png, got %s", format)
	}
	return img
}

func TestShrinkImageDownscalesAndKeepsTheSmallerBytes(t *testing.T) {
	source := noisyPNG(t, 128)

	shrunk, contentType := shrinkImage(source, "image/png", 64)
	if contentType != "image/png" {
		t.Fatalf("content type should survive a re-encode, got %q", contentType)
	}
	if len(shrunk) >= len(source) {
		t.Fatalf("downscaling should be smaller: %d -> %d bytes", len(source), len(shrunk))
	}
	if bounds := decodePNG(t, shrunk).Bounds(); bounds.Dx() != 64 || bounds.Dy() != 64 {
		t.Errorf("expected 64x64, got %v", bounds)
	}

	// A zero edge means "keep the original", which is how an attachment asks to
	// be left alone.
	if got, _ := shrinkImage(source, "image/png", 0); !bytes.Equal(got, source) {
		t.Errorf("edge 0 should pass the bytes through untouched")
	}

	// Already small enough: never re-encode, never grow.
	small := noisyPNG(t, 32)
	if got, _ := shrinkImage(small, "image/png", 64); !bytes.Equal(got, small) {
		t.Errorf("an image within the edge should keep its original bytes")
	}

	// Formats without a decoder in the standard library stay as they are.
	webp := append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), source...)
	if got, ct := shrinkImage(webp, "image/webp", 64); !bytes.Equal(got, webp) || ct != "image/webp" {
		t.Errorf("an undecodable format should pass through untouched")
	}
	// So do vector and animated ones: re-encoding would flatten them.
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="512" height="512"></svg>`)
	if got, _ := shrinkImage(svg, "image/svg+xml", 64); !bytes.Equal(got, svg) {
		t.Errorf("svg should pass through untouched")
	}
}

func TestShrinkDataURIRightSizesInlineMedia(t *testing.T) {
	source := noisyPNG(t, 128)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(source)

	shrunk := shrinkDataURI(uri, 64)
	if shrunk == uri {
		t.Fatalf("an oversized inline image should be right-sized")
	}
	if !strings.HasPrefix(shrunk, "data:image/png;base64,") {
		t.Fatalf("the data URI shape should survive, got %.40s", shrunk)
	}
	// A second pass must be a no-op rather than a re-encode loop.
	if again := shrinkDataURI(shrunk, 64); again != shrunk {
		t.Errorf("shrinking is not idempotent")
	}
	// And a URI that needs nothing keeps its bytes exactly.
	if got := shrinkDataURI(uri, 0); got != uri {
		t.Errorf("edge 0 should pass the URI through untouched")
	}
}

func TestDiscordSizedURLOnlyTouchesFixedSizeImages(t *testing.T) {
	tests := []struct {
		name string
		in   string
		edge int
		want string
	}{
		{"avatar", "https://cdn.discordapp.com/avatars/1/abc.png?size=1024", 64,
			"https://cdn.discordapp.com/avatars/1/abc.png?size=64"},
		{"avatar without a query", "https://cdn.discordapp.com/avatars/1/abc.png", 160,
			"https://cdn.discordapp.com/avatars/1/abc.png?size=256"},
		{"animated avatar keeps its format", "https://cdn.discordapp.com/avatars/1/abc.gif", 64,
			"https://cdn.discordapp.com/avatars/1/abc.gif?size=64"},
		{"guild icon", "https://cdn.discordapp.com/icons/1/abc.webp", 64,
			"https://cdn.discordapp.com/icons/1/abc.webp?size=64"},
		{"emoji", "https://cdn.discordapp.com/emojis/999.png", 64,
			"https://cdn.discordapp.com/emojis/999.png?size=64"},
		{"media host", "https://media.discordapp.net/avatars/1/abc.png", 64,
			"https://media.discordapp.net/avatars/1/abc.png?size=64"},
		// Attachments are content: the parameter is not documented there and a
		// wrong one could change the image.
		{"attachment", "https://cdn.discordapp.com/attachments/1/2/shot.png", 64,
			"https://cdn.discordapp.com/attachments/1/2/shot.png"},
		{"other host", "https://example.com/avatar.png", 64, "https://example.com/avatar.png"},
		// A size the producer already asked for: kept when it is smaller than we
		// need, replaced when it is bigger.
		{"smaller explicit size", "https://cdn.discordapp.com/avatars/1/abc.png?size=16", 64,
			"https://cdn.discordapp.com/avatars/1/abc.png?size=16"},
		{"no edge", "https://cdn.discordapp.com/avatars/1/abc.png", 0,
			"https://cdn.discordapp.com/avatars/1/abc.png"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := discordSizedURL(tc.in, tc.edge); got != tc.want {
				t.Errorf("discordSizedURL(%q, %d)\n got: %s\nwant: %s", tc.in, tc.edge, got, tc.want)
			}
		})
	}
}

func TestDiscordSizeRoundsUpToAPowerOfTwo(t *testing.T) {
	for edge, want := range map[int]int{1: 16, 16: 16, 17: 32, 64: 64, 65: 128, 160: 256, 4096: 4096, 9000: 4096} {
		if got := discordSize(edge); got != want {
			t.Errorf("discordSize(%d) = %d, want %d", edge, got, want)
		}
	}
}
