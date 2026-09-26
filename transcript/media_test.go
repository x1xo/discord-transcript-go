package transcript

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestURLTemplateRewriter(t *testing.T) {
	const raw = "https://cdn.discordapp.com/attachments/1/2/shot final.png"

	t.Run("query parameter", func(t *testing.T) {
		rewrite := URLTemplateRewriter("https://proxy.example/fetch?url={urlenc}&size=large")
		got, err := url.Parse(rewrite(raw))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got.Host != "proxy.example" || got.Path != "/fetch" {
			t.Errorf("template address changed: %s", got)
		}
		if decoded := got.Query().Get("url"); decoded != raw {
			t.Errorf("url parameter = %q, want %q", decoded, raw)
		}
		if got.Query().Get("size") != "large" {
			t.Errorf("other parameters should survive: %s", got.RawQuery)
		}
	})

	t.Run("path segment", func(t *testing.T) {
		rewrite := URLTemplateRewriter("https://proxy.example/media/{urlenc}")
		wire := rewrite(raw)
		// The wire form must keep the source URL inside a single path segment, so
		// its slashes and colon are encoded and a space is %20, not +.
		if !strings.HasPrefix(wire, "https://proxy.example/media/https%3A%2F%2Fcdn.discordapp.com%2F") {
			t.Errorf("wire form should carry the encoded URL: %s", wire)
		}
		if strings.Contains(wire, "+") {
			t.Errorf("a space must encode as %%20, not +: %s", wire)
		}
		parsed, err := url.Parse(wire)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		// A proxy decoding the path must recover the source URL exactly.
		if parsed.Host != "proxy.example" || parsed.Path != "/media/"+raw {
			t.Errorf("decoded path = %q, want %q", parsed.Path, "/media/"+raw)
		}
	})

	t.Run("verbatim", func(t *testing.T) {
		rewrite := URLTemplateRewriter("https://proxy.example/{url}")
		if got := rewrite(raw); got != "https://proxy.example/"+raw {
			t.Errorf("verbatim substitution = %q", got)
		}
	})

	t.Run("no placeholder", func(t *testing.T) {
		rewrite := URLTemplateRewriter("https://proxy.example/fixed")
		if got := rewrite(raw); got != "https://proxy.example/fixed" {
			t.Errorf("a template without a placeholder should pass through, got %q", got)
		}
	})
}

// TestHTTPFetcherDownloadsThroughAProxy is the pattern the hooks exist for: every
// download goes to the caller's own endpoint, which receives the source URL as a
// parameter, and the source server is never contacted.
func TestHTTPFetcherDownloadsThroughAProxy(t *testing.T) {
	image := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3, 4}

	var originHits int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&originHits, 1)
		http.Error(w, "the transcript must not talk to the origin", http.StatusInternalServerError)
	}))
	defer origin.Close()

	var sawAuth string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		source := r.URL.Query().Get("url")
		if source == "" {
			http.Error(w, "missing url parameter", http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(source, origin.URL) {
			http.Error(w, "wrong source", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(image)
	}))
	defer proxy.Close()

	fetcher := HTTPFetcher{
		Client:     proxy.Client(),
		RewriteURL: URLTemplateRewriter(proxy.URL + "/fetch?url={urlenc}"),
		Header:     func(req *http.Request) { req.Header.Set("Authorization", "Bearer secret") },
	}
	store := InlineMediaWith(fetcher)

	got, err := store.Store(context.Background(), MediaRef{
		URL:  origin.URL + "/attachments/1/2/shot.png",
		Kind: MediaImage,
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if hits := atomic.LoadInt32(&originHits); hits != 0 {
		t.Errorf("the origin server was contacted %d times; the proxy should have handled it", hits)
	}
	if sawAuth != "Bearer secret" {
		t.Errorf("the Header hook should run, saw Authorization %q", sawAuth)
	}

	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("store returned %q, want a %s data URI", trim(got), prefix)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(got, prefix))
	if err != nil {
		t.Fatalf("decode data URI: %v", err)
	}
	if string(decoded) != string(image) {
		t.Errorf("bytes from the proxy did not survive: got %v, want %v", decoded, image)
	}
}

func TestHTTPFetcherKeepsTheOriginalURLInErrors(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusForbidden)
	}))
	defer proxy.Close()

	original := "https://cdn.discordapp.com/attachments/1/2/gone.png"
	fetcher := HTTPFetcher{
		Client:     proxy.Client(),
		RewriteURL: URLTemplateRewriter(proxy.URL + "?url={urlenc}"),
	}
	if _, _, err := fetcher.Fetch(context.Background(), MediaRef{URL: original}, 1<<20); err == nil {
		t.Fatal("expected an error for a 403 response")
	} else if !strings.Contains(err.Error(), original) {
		t.Errorf("error should name the original URL, got %v", err)
	}
}

func TestHTTPFetcherWithoutRewriterUsesTheSourceURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/gif")
		_, _ = w.Write([]byte("GIF89a"))
	}))
	defer server.Close()

	fetcher := HTTPFetcher{Client: server.Client()}
	data, contentType, err := fetcher.Fetch(context.Background(), MediaRef{URL: server.URL + "/a.gif"}, 1<<20)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if contentType != "image/gif" || string(data) != "GIF89a" {
		t.Errorf("unexpected result: %q %q", contentType, data)
	}
}

func TestFetchFuncAdapter(t *testing.T) {
	calls := 0
	fetcher := FetchFunc(func(_ context.Context, ref MediaRef, maxBytes int64) ([]byte, string, error) {
		calls++
		if maxBytes <= 0 {
			t.Errorf("the store should pass a byte cap, got %d", maxBytes)
		}
		return []byte("from a function for " + ref.URL), "text/plain", nil
	})

	store := InlineMediaWith(fetcher)
	got, err := store.Store(context.Background(), MediaRef{URL: "https://example.test/a.txt", Kind: MediaFile})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if calls != 1 {
		t.Errorf("fetcher called %d times, want 1", calls)
	}
	const prefix = "data:text/plain;base64,"
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("store returned %q, want a %s data URI", trim(got), prefix)
	}
}

// A store can also rewrite the URL without downloading anything, which is how a
// transcript keeps its size while still serving media the reader can reach.
func TestMediaStoreFuncCanServeThroughAProxy(t *testing.T) {
	rewrite := URLTemplateRewriter("https://proxy.example/fetch?url={urlenc}")
	var store MediaStore = MediaStoreFunc(func(_ context.Context, ref MediaRef) (string, error) {
		return rewrite(ref.URL), nil
	})

	got, err := store.Store(context.Background(), MediaRef{URL: "https://cdn.discordapp.com/attachments/1/2/a.png"})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if !strings.HasPrefix(got, "https://proxy.example/fetch?url=https%3A%2F%2Fcdn.discordapp.com%2F") {
		t.Errorf("store should return the proxy URL, got %q", got)
	}
}

func trim(s string) string {
	if len(s) > 60 {
		return s[:60] + "…"
	}
	return s
}
