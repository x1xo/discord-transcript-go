package transcript

import (
	"context"
	"encoding/base64"
	"regexp"
	"slices"
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

// urlStore records what the renderer asked for and hands back a fixed blob, the
// way a caller's own store would.
type urlStore struct {
	blob string
	seen []MediaRef
}

func (s *urlStore) Store(_ context.Context, ref MediaRef) (string, error) {
	s.seen = append(s.seen, ref)
	return s.blob, nil
}

func TestTheSizeWantedReachesTheStoreAndTheBytesComeBackRightSized(t *testing.T) {
	// A custom store gets both halves of right-sizing for free: the size we draw
	// at is already in the URL it is handed, so a proxy behind it fetches the
	// small copy, and whatever it returns inline is downscaled on the way out.
	// The store never has to know that TargetEdge exists.
	source := noisyPNG(t, 128)
	blob := "data:image/png;base64," + base64.StdEncoding.EncodeToString(source)
	store := &urlStore{blob: blob}

	at := time.Date(2024, 3, 15, 14, 28, 0, 0, time.UTC)
	tr := &Transcript{
		Channel: Channel{Name: "general", Type: ChannelText},
		Messages: []Message{{
			Author:    Author{Key: "1", Name: "piton", AvatarURL: "https://cdn.discordapp.com/avatars/1/abc.png"},
			Timestamp: at,
			Embeds: []Embed{{
				Description: ParseContent("hi"),
				Thumbnail:   &Media{URL: "https://cdn.discordapp.com/attachments/1/2/thumb.png"},
				Image:       &Media{URL: "https://cdn.discordapp.com/attachments/1/2/full.png"},
			}},
		}},
	}

	page := render(t, tr, WithMedia(store), WithoutMediaPool())

	// Fixed-size Discord images are asked for at the size they are drawn at; an
	// attachment is content, and is left exactly as the producer wrote it.
	for _, want := range []string{
		"https://cdn.discordapp.com/avatars/1/abc.png?size=64",
		"https://cdn.discordapp.com/attachments/1/2/thumb.png",
		"https://cdn.discordapp.com/attachments/1/2/full.png",
	} {
		if !slices.Contains(refURLs(store.seen), want) {
			t.Errorf("the store was never handed %s, saw %v", want, refURLs(store.seen))
		}
	}
	for _, ref := range store.seen {
		if strings.HasPrefix(ref.URL, "https://cdn.discordapp.com/avatars/") && !strings.Contains(ref.URL, "size=") {
			t.Errorf("the avatar reached the store unsized: %s", ref.URL)
		}
	}

	// The oversized blob the store handed back is downscaled before it reaches the
	// page: the avatar is drawn at 32 CSS pixels, so it arrives at 64.
	avatarSrc := between(page, `<span class="dt-avatar"><img src="`, `"`)
	if avatarSrc == blob {
		t.Fatalf("an oversized inline blob from a custom store should be downscaled")
	}
	bounds := decodePNG(t, decodeDataURI(t, avatarSrc)).Bounds()
	if bounds.Dx() != 64 || bounds.Dy() != 64 {
		t.Errorf("the avatar should be right-sized to 64x64, got %v", bounds)
	}
	// The embed image is content: it keeps every byte it arrived with.
	if imageSrc := between(page, `<div class="dt-embed-image"><img src="`, `"`); imageSrc != blob {
		t.Errorf("content images should keep their original bytes")
	}
}

// between returns the text between two markers, or "" when either is missing.
func between(text, after, before string) string {
	_, rest, ok := strings.Cut(text, after)
	if !ok {
		return ""
	}
	value, _, _ := strings.Cut(rest, before)
	return value
}

func decodeDataURI(t *testing.T, uri string) []byte {
	t.Helper()
	_, payload, ok := strings.Cut(uri, ",")
	if !ok {
		t.Fatalf("not a data URI: %.40s", uri)
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("decode data URI: %v", err)
	}
	return raw
}

func refURLs(refs []MediaRef) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.URL)
	}
	return out
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
