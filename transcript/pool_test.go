package transcript

import (
	"context"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
	"time"
)

// inlineFixture is a tiny PNG, the same bytes every time, so a document built
// from it is deterministic.
func inlineFixture(t *testing.T) (uri string, raw []byte) {
	t.Helper()
	raw = noisyPNG(t, 8)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw), raw
}

// pooledTranscript repeats one avatar across three messages and reuses it as an
// embed icon, which is exactly the shape a real conversation has.
func pooledTranscript(t *testing.T, avatar string) *Transcript {
	t.Helper()
	at := func(minute int) time.Time { return time.Date(2024, 3, 15, 14, minute, 0, 0, time.UTC) }
	author := Author{Key: "1", Name: "piton", AvatarURL: avatar}
	return &Transcript{
		Channel: Channel{Name: "general", Type: ChannelText},
		Messages: []Message{
			{Author: author, Timestamp: at(28), Content: ParseContent("one")},
			{Author: author, Timestamp: at(29), Content: ParseContent("two")},
			{Author: author, Timestamp: at(30), Embeds: []Embed{{
				Description: ParseContent("three"),
				Author:      &EmbedAuthor{Name: "piton", Icon: &Media{URL: avatar, Kind: MediaImage}},
				Footer:      &EmbedFooter{Text: "footer", Icon: &Media{URL: avatar, Kind: MediaImage}},
				Thumbnail:   &Media{URL: avatar, Kind: MediaImage},
			}}},
		},
	}
}

func TestMediaPoolStoresEachBlobOnce(t *testing.T) {
	avatar, raw := inlineFixture(t)
	tr := pooledTranscript(t, avatar)

	doc, err := tr.HTML(WithMedia(URLMedia())) // the URI is already inline
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	page := string(doc)

	// One rule, one blob.
	rules := regexp.MustCompile(`\.dt-media-\d+\{background-image:url\("[^"]*"\)\}`).FindAllString(page, -1)
	if len(rules) != 1 {
		t.Fatalf("expected one pooled rule, got %d:\n%s", len(rules), page)
	}
	if n := strings.Count(page, base64.StdEncoding.EncodeToString(raw)); n != 1 {
		t.Errorf("the blob should be stored once, found %d copies", n)
	}
	// Every use points at it, and the marker never leaks into the output. The
	// three messages share an author, so two of them are continuations with no
	// avatar: the four uses are one message avatar, the embed author icon, the
	// footer icon and the thumbnail.
	uses := regexp.MustCompile(`<span class="dt-media dt-media-1" role="img" aria-label="" style="[^"]*"></span>`).FindAllString(page, -1)
	if len(uses) != 4 {
		t.Errorf("expected 4 pooled uses, got %d:\n%s", len(uses), page)
	}
	if strings.Contains(page, ` data-dt-media="`) {
		t.Errorf("the pool marker should not reach the output")
	}
	if !strings.Contains(page, "<style data-dt-media-pool>") {
		t.Errorf("the pool rules belong in one style block")
	}
	// The embed author row has no size of its own, so its element carries one.
	if !strings.Contains(page, `style="width:24px;height:24px;border-radius:50%"`) {
		t.Errorf("the embed author icon should carry its own box:\n%s", page)
	}
}

func TestMediaPoolLeavesASingleUseAlone(t *testing.T) {
	avatar, _ := inlineFixture(t)
	tr := &Transcript{
		Channel: Channel{Name: "general", Type: ChannelText},
		Messages: []Message{{
			Author:    Author{Key: "1", Name: "piton", AvatarURL: avatar},
			Timestamp: time.Date(2024, 3, 15, 14, 28, 0, 0, time.UTC),
			Content:   ParseContent("only once"),
		}},
	}

	page := render(t, tr, WithMedia(URLMedia()))
	if strings.Contains(page, "<style data-dt-media-pool>") {
		t.Errorf("nothing repeats, so nothing should be hoisted:\n%s", page)
	}
	if strings.Contains(page, ` data-dt-media="`) {
		t.Errorf("the marker should be stripped when an image is not pooled")
	}
	if !strings.Contains(page, `<img src="`+avatar+`" alt="" loading="lazy" decoding="async">`) {
		t.Errorf("a one-off image stays an img with its own data URI:\n%s", page)
	}
}

func TestWithoutMediaPoolKeepsEveryCopy(t *testing.T) {
	avatar, _ := inlineFixture(t)
	tr := pooledTranscript(t, avatar)

	page := render(t, tr, WithMedia(URLMedia()), WithoutMediaPool())
	if strings.Contains(page, "<style data-dt-media-pool>") || strings.Contains(page, "dt-media") {
		t.Errorf("WithoutMediaPool should leave the markup free of pool rules")
	}
	if n := strings.Count(page, avatar); n < 4 {
		t.Errorf("every use should keep its own data URI, found %d", n)
	}
}

func TestInlineMediaStoreRightSizesAnInlineBlob(t *testing.T) {
	// A producer that hands over data URIs gets them right-sized too, which is
	// how the committed examples shrink.
	source := noisyPNG(t, 128)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(source)

	store := InlineMedia()
	out, err := store.Store(context.Background(), MediaRef{URL: uri, Kind: MediaAvatar, TargetEdge: 64})
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if len(out) >= len(uri) {
		t.Errorf("an oversized inline avatar should shrink: %d -> %d", len(uri), len(out))
	}

	// The same request twice is cached, and a different edge is a different
	// entry, not a downgrade of the first.
	again, _ := store.Store(context.Background(), MediaRef{URL: uri, Kind: MediaAvatar, TargetEdge: 64})
	if again != out {
		t.Errorf("the store should cache per request")
	}
	capped, _ := store.Store(context.Background(), MediaRef{URL: uri, Kind: MediaAvatar, TargetEdge: 0})
	if capped != uri {
		t.Errorf("an uncapped request should get the original bytes")
	}
}

// render is a shorthand for the assertions above.
func render(t *testing.T, tr *Transcript, opts ...Option) string {
	t.Helper()
	doc, err := tr.HTML(opts...)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	return string(doc)
}
