package transcript

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
)

// MediaRef describes one piece of media to be resolved.
type MediaRef struct {
	// URL is where the media currently lives.
	URL string
	// Kind is what it is; the store may use it to choose an extension.
	Kind MediaKind
	// Filename is a suggested name (attachments carry one).
	Filename string
	// ContentType is a hint, when the producer already knows it.
	ContentType string
	// Alt is the alt text the producer has for the media, when any.
	Alt string
}

// MediaStore turns a media reference into the URL that ends up in the document.
//
// Returning an error is always non-fatal: the renderer falls back to the
// original URL and reports the problem through Options.Warn. That keeps an
// export working when one CDN object has already expired.
type MediaStore interface {
	Store(ctx context.Context, ref MediaRef) (string, error)
}

// MediaStoreFunc adapts a function to MediaStore.
type MediaStoreFunc func(ctx context.Context, ref MediaRef) (string, error)

// Store implements MediaStore.
func (f MediaStoreFunc) Store(ctx context.Context, ref MediaRef) (string, error) {
	if f == nil {
		return ref.URL, nil
	}
	return f(ctx, ref)
}

// Fetcher downloads media. The built-in stores use HTTPFetcher, and any custom
// downloader can be injected by implementing this interface.
type Fetcher interface {
	// Fetch returns the bytes and the detected content type.
	Fetch(ctx context.Context, ref MediaRef, maxBytes int64) ([]byte, string, error)
}

// HTTPFetcher is the default downloader.
type HTTPFetcher struct {
	// Client defaults to http.DefaultClient.
	Client *http.Client
	// UserAgent is sent with every request.
	UserAgent string
}

// Fetch implements Fetcher.
func (f HTTPFetcher) Fetch(ctx context.Context, ref MediaRef, maxBytes int64) ([]byte, string, error) {
	if ref.URL == "" {
		return nil, "", errors.New("empty media URL")
	}
	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref.URL, nil)
	if err != nil {
		return nil, "", err
	}
	if f.UserAgent != "" {
		req.Header.Set("User-Agent", f.UserAgent)
	} else {
		req.Header.Set("User-Agent", "discord-transcript-go/"+Version)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("fetch %s: %s", ref.URL, resp.Status)
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxMediaBytes
	}
	limited := io.LimitReader(resp.Body, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > maxBytes {
		return nil, "", fmt.Errorf("fetch %s: larger than %d bytes", ref.URL, maxBytes)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(data)
	}
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		contentType = strings.TrimSpace(contentType[:i])
	}
	return data, contentType, nil
}

// URLMedia keeps the original CDN URLs. Use it for quick previews, or when the
// media has already been re-hosted somewhere durable.
func URLMedia() MediaStore { return MediaStoreFunc(nil) }

// InlineMedia downloads media and rewrites it to a base64 data URI, producing a
// document that keeps working when the original CDN object is gone. This is the
// default, and it is why a finished transcript is a single self-contained file.
//
// The cost is size: base64 adds ~33% and inline images are not shared between
// transcripts. Pass DirMedia or a custom store when that matters more.
func InlineMedia() *InlineMediaStore {
	return &InlineMediaStore{
		Fetcher: HTTPFetcher{},
		cache:   make(map[string]string),
	}
}

// InlineMediaWith returns an inline store that uses a custom downloader.
func InlineMediaWith(fetcher Fetcher) *InlineMediaStore {
	return &InlineMediaStore{Fetcher: fetcher, cache: make(map[string]string)}
}

// InlineMediaStore downloads media and returns data URIs. It is safe for
// concurrent use and downloads each URL at most once.
type InlineMediaStore struct {
	// Fetcher downloads the bytes. Defaults to HTTPFetcher.
	Fetcher Fetcher
	// MaxBytes caps a single download; zero uses DefaultMaxMediaBytes.
	MaxBytes int64

	mu    sync.Mutex
	cache map[string]string
}

// Store implements MediaStore.
func (s *InlineMediaStore) Store(ctx context.Context, ref MediaRef) (string, error) {
	if ref.URL == "" {
		return "", nil
	}
	// Already inline or local: nothing to do.
	if strings.HasPrefix(ref.URL, "data:") || !isRemote(ref.URL) {
		return ref.URL, nil
	}

	s.mu.Lock()
	if cached, ok := s.cache[ref.URL]; ok {
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	fetcher := s.Fetcher
	if fetcher == nil {
		fetcher = HTTPFetcher{}
	}
	ctx, cancel := contextWithTimeout(ctx)
	defer cancel()

	max := s.MaxBytes
	if max <= 0 {
		max = DefaultMaxMediaBytes
	}
	data, contentType, err := fetcher.Fetch(ctx, ref, max)
	if err != nil {
		return ref.URL, err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	dataURI := "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)

	s.mu.Lock()
	s.cache[ref.URL] = dataURI
	s.mu.Unlock()
	return dataURI, nil
}

// DirMedia writes media into a directory and returns relative paths, for
// transcripts shipped as a folder (HTML plus an assets directory).
func DirMedia(dir, base string) *DirMediaStore {
	if base == "" {
		base = "assets/"
	}
	return &DirMediaStore{Dir: dir, Base: base, Fetcher: HTTPFetcher{}, seen: make(map[string]string)}
}

// DirMediaStore implements MediaStore by saving files to disk.
type DirMediaStore struct {
	// Dir is the directory files are written to.
	Dir string
	// Base is the URL prefix used in the document.
	Base string
	// Fetcher downloads the bytes. Defaults to HTTPFetcher.
	Fetcher Fetcher
	// MaxBytes caps a single download; zero uses DefaultMaxMediaBytes.
	MaxBytes int64
	// FS is the filesystem used to write files; defaults to the OS filesystem
	// via writeFile.
	WriteFile func(name string, data []byte) error

	mu   sync.Mutex
	seen map[string]string
}

// Store implements MediaStore.
func (s *DirMediaStore) Store(ctx context.Context, ref MediaRef) (string, error) {
	if ref.URL == "" || !isRemote(ref.URL) {
		return ref.URL, nil
	}
	s.mu.Lock()
	if cached, ok := s.seen[ref.URL]; ok {
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	fetcher := s.Fetcher
	if fetcher == nil {
		fetcher = HTTPFetcher{}
	}
	ctx, cancel := contextWithTimeout(ctx)
	defer cancel()

	max := s.MaxBytes
	if max <= 0 {
		max = DefaultMaxMediaBytes
	}
	data, contentType, err := fetcher.Fetch(ctx, ref, max)
	if err != nil {
		return ref.URL, err
	}

	name := mediaFilename(ref, contentType)
	write := s.WriteFile
	if write == nil {
		write = func(name string, data []byte) error { return osWriteFile(name, data) }
	}
	if err := write(joinPath(s.Dir, name), data); err != nil {
		return ref.URL, err
	}
	href := s.Base + name

	s.mu.Lock()
	s.seen[ref.URL] = href
	s.mu.Unlock()
	return href, nil
}

// mediaFilename picks a stable, filesystem-safe name for a piece of media.
func mediaFilename(ref MediaRef, contentType string) string {
	if ref.Filename != "" {
		return sanitizeFilename(ref.Filename)
	}
	if u, err := url.Parse(ref.URL); err == nil {
		if base := path.Base(u.Path); base != "" && base != "." && base != "/" {
			return sanitizeFilename(base)
		}
	}
	ext := ""
	if exts, err := mime.ExtensionsByType(contentType); err == nil && len(exts) > 0 {
		ext = exts[0]
	}
	return sanitizeFilename(string(ref.Kind)) + "-" + shortHash(ref.URL) + ext
}

func sanitizeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._")
	if out == "" {
		return "media"
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

// shortHash is an FNV-1a hex digest, used to make generated filenames unique.
func shortHash(s string) string {
	const (
		offset = 14695981039346656037
		prime  = 1099511628211
	)
	var h uint64 = offset
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime
	}
	const digits = "0123456789abcdef"
	var buf [12]byte
	for i := 11; i >= 0; i-- {
		buf[i] = digits[h&0xF]
		h >>= 4
	}
	return string(buf[:])
}

func isRemote(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}
