package transcript

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return parsed
}

// sampleTranscript exercises every element the renderer can emit.
func sampleTranscript(t *testing.T) *Transcript {
	t.Helper()
	res := Resolvers{
		Users:    Profiles{"222222222222222222": {Key: "222222222222222222", Name: "ravik"}},
		Channels: Channels{"333333333333333333": "rules"},
		Roles:    Roles{"444444444444444444": {Name: "Moderators", Color: "#57f287"}},
	}
	parse := func(s string) []Node { return ParseContentWith(s, res) }

	piton := Author{Key: "111111111111111111", Name: "piton", AvatarURL: "https://cdn.discordapp.com/avatars/111/abc.png", RoleColor: "#57f287"}
	miona := Author{Key: "555555555555555555", Name: "miona", AvatarURL: "https://cdn.discordapp.com/avatars/555/def.png", RoleColor: "#eb459e", Bot: true, Verified: true}

	return &Transcript{
		Channel: Channel{Name: "general", Type: ChannelText, Guild: "Test Guild"},
		Messages: []Message{
			{
				Author:    piton,
				Timestamp: mustTime(t, "2024-03-15T14:28:00Z"),
				Content: parse("Hey! **bold**, *italic*, __underline__, ~~strike~~, ||spoiler||, `code`.\n" +
					"Welcome <@222222222222222222>, read <#333333333333333333>, ping <@&444444444444444444>, hi @everyone.\n" +
					"Party <:party:999999999999999999> and a timestamp <t:1710508800:R> and a link https://example.com/x."),
			},
			{
				Author:    miona,
				Timestamp: mustTime(t, "2024-03-15T14:29:30Z"),
				Edited:    true,
				Highlight: true,
				Reply: &Reply{
					Author:   piton,
					Mentions: true,
					Content:  parse("Hey! **bold** and the rest"),
				},
				Content: parse("Thanks! Here is a quote and a list:\n> quoted line\n- one\n- two\n1. first\n2. second\n" +
					"```go\nfmt.Println(\"hi\")\n```\n-# small print"),
				Reactions: []Reaction{
					{Emoji: "🎉", Count: 3},
					{Emoji: "https://cdn.discordapp.com/emojis/999999999999999999.png", Name: ":party:", Count: 12, Reacted: true},
					{Emoji: "🍰", Count: 1},
				},
				Attachments: []Attachment{
					{Kind: MediaImage, URL: "https://cdn.discordapp.com/attachments/1/2/shot.png", Alt: "screenshot", Width: 400, Height: 300},
					{Kind: MediaImage, URL: "https://cdn.discordapp.com/attachments/1/2/spoiler.png", Alt: "hidden", Spoiler: true},
					{Kind: MediaFile, URL: "https://cdn.discordapp.com/attachments/1/2/report.pdf", Name: "report.pdf", SizeBytes: 1536},
				},
			},
			{
				Timestamp: mustTime(t, "2024-03-15T14:31:00Z"),
				System:    &SystemMessage{Type: SystemJoin, Content: parse("piton joined the server.")},
			},
			{
				Author:    miona,
				Timestamp: mustTime(t, "2024-03-15T14:32:00Z"),
				Embeds: []Embed{{
					Color:       "#5865f2",
					URL:         "https://example.com/embed",
					Title:       "Embed title",
					Provider:    "Example",
					Description: parse("A description with `inline code` and a <@222222222222222222> mention."),
					Author:      &EmbedAuthor{Name: "miona", URL: "https://example.com", Icon: &Media{URL: "https://cdn.discordapp.com/avatars/555/def.png", Kind: MediaImage}},
					Fields: []EmbedField{
						{Name: "Inline one", Value: parse("Value A"), Inline: true},
						{Name: "Inline two", Value: parse("Value B"), Inline: true},
						{Name: "Full width", Value: parse("Not inline")},
					},
					Footer:    &EmbedFooter{Text: "Footer text", Icon: &Media{URL: "https://cdn.discordapp.com/avatars/555/def.png", Kind: MediaImage}, Timestamp: mustTime(t, "2024-03-15T14:32:00Z")},
					Thumbnail: &Media{URL: "https://cdn.discordapp.com/attachments/1/2/thumb.png", Kind: MediaImage},
					Image:     &Media{URL: "https://cdn.discordapp.com/attachments/1/2/full.png", Kind: MediaImage},
				}},
			},
			{
				Author:    piton,
				Timestamp: mustTime(t, "2024-03-15T14:33:00Z"),
				Ephemeral: true,
				Content:   parse("<script>alert('xss')</script> and a [bad](javascript:alert(1)) link"),
			},
		},
	}
}

func TestDocumentGolden(t *testing.T) {
	tr := sampleTranscript(t)
	got, err := tr.HTML(WithMedia(URLMedia()), WithGenerator("discord-transcript-go test"))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	golden := filepath.Join("testdata", "document.golden.html")
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run: go test ./transcript -update): %v", err)
	}
	if string(want) != string(got) {
		t.Errorf("rendered document differs from %s\n--- got ---\n%s", golden, got)
	}
}

func TestDocumentStructure(t *testing.T) {
	tr := sampleTranscript(t)
	doc, err := tr.HTML(WithMedia(URLMedia()))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(doc)

	for _, want := range []string{
		"<!doctype html>",
		`<discord-messages channel-name="general" channel-type="text">`,
		`<discord-message profile="111111111111111111" author="piton" timestamp="2024-03-15T14:28:00Z">`,
		"<discord-bold>bold</discord-bold>",
		"<discord-italic>italic</discord-italic>",
		"<discord-underlined>underline</discord-underlined>",
		"<discord-strikethrough>strike</discord-strikethrough>",
		"<discord-spoiler>spoiler</discord-spoiler>",
		"<discord-code>code</discord-code>",
		`<discord-mention type="user">ravik</discord-mention>`,
		`<discord-mention type="channel">rules</discord-mention>`,
		`<discord-mention type="role" style="--dt-mention-role-color: #57f287">Moderators</discord-mention>`,
		`<discord-mention type="everyone">everyone</discord-mention>`,
		`<discord-custom-emoji name=":party:" url="https://cdn.discordapp.com/emojis/999999999999999999.png"></discord-custom-emoji>`,
		`<discord-time timestamp="2024-03-15T13:20:00Z" format="R">`,
		`<discord-link href="https://example.com/x"`,
		`<discord-reply profile="111111111111111111" author="piton" mentions>`,
		"<discord-quote>quoted line</discord-quote>",
		"<discord-unordered-list><discord-list-item>one</discord-list-item>",
		`<discord-ordered-list start="1"><discord-list-item>first</discord-list-item>`,
		`<span class="dt-code-lang">go</span><discord-code>fmt.Println(&#34;hi&#34;)</discord-code>`,
		"<discord-subscript>small print</discord-subscript>",
		`<discord-system-message type="join" timestamp="2024-03-15T14:31:00Z">`,
		`<discord-reaction emoji="🎉" count="3"></discord-reaction>`,
		`<discord-reaction emoji="https://cdn.discordapp.com/emojis/999999999999999999.png" name=":party:" count="12" reacted>`,
		`<discord-embed color="#5865f2">`,
		`<a class="dt-embed-title" href="https://example.com/embed"`, // the embed has a url
		`<discord-embed-description>`,
		`<discord-embed-field field-title="Inline one" inline>`,
		`<div class="dt-embed-image">`,
		`<div class="dt-embed-thumbnail">`,
		`<discord-embed-footer>`,
		`<discord-image-attachment>`,
		`<discord-image-attachment spoiler>`,
		`<discord-file-attachment name="report.pdf" bytes="1.5" bytes-unit="KB" type="PDF">`,
		`<span class="dt-file-size">1.5 KB</span>`,
		`window.$discordMessage = {"profiles":{`,
		`"author":"piton"`,
		`"roleColor":"#57f287"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("document is missing %s", want)
		}
	}
}

func TestDocumentEscapesUntrustedContent(t *testing.T) {
	tr := &Transcript{
		Channel: Channel{Name: `"><script>alert(1)</script>`, Type: ChannelText},
		Messages: []Message{{
			Author:    Author{Key: `"><img src=x onerror=alert(1)>`, Name: `"><img src=x onerror=alert(1)>`},
			Timestamp: mustTime(t, "2024-03-15T14:28:00Z"),
			Content:   ParseContent(`<script>alert('xss')</script> <img src=x onerror=alert(1)>`),
			Attachments: []Attachment{{
				Kind: MediaImage,
				URL:  `https://example.com/x.png" onerror="alert(1)`,
				Alt:  `"><script>alert(1)</script>`,
			}},
		}},
	}
	doc, err := tr.HTML(WithMedia(URLMedia()))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(doc)

	for _, bad := range []string{
		"<script>alert('xss')</script>",
		`<img src=x onerror=alert(1)>`,
		`onerror="alert(1)">`,
		`"><script>alert(1)</script>`,
	} {
		if strings.Contains(html, bad) {
			t.Errorf("unescaped content reached the document: %s", bad)
		}
	}
	for _, want := range []string{"&lt;script&gt;", "&gt;", "&#34;"} {
		if !strings.Contains(html, want) {
			t.Errorf("expected escaped output containing %s", want)
		}
	}
}

func TestJavascriptURLIsDropped(t *testing.T) {
	nodes := ParseContent("[click](javascript:alert(1)) and <javascript:alert(2)>")
	tr := &Transcript{
		Channel:  Channel{Name: "general", Type: ChannelText},
		Messages: []Message{{Author: Author{Key: "1", Name: "a"}, Content: nodes}},
	}
	doc, err := tr.HTML(WithMedia(URLMedia()))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	// The literal text is kept (Discord has no markdown links), but it must never
	// become a live URL in an attribute.
	if strings.Contains(string(doc), `href="javascript:`) || strings.Contains(string(doc), `src="javascript:`) {
		t.Errorf("javascript: URL became a live attribute:\n%s", doc)
	}
	if !strings.Contains(string(doc), "[click](javascript:alert(1))") {
		t.Errorf("literal link syntax should still be shown as text")
	}
	if strings.Contains(string(doc), "<javascript:") {
		t.Errorf("angle-bracket URL should be escaped")
	}
}

func TestAssetModes(t *testing.T) {
	tr := sampleTranscript(t)

	cdn, err := tr.HTML(WithMedia(URLMedia()), WithAssets(AssetsCDN))
	if err != nil {
		t.Fatalf("cdn: %v", err)
	}
	if !strings.Contains(string(cdn), "cdn.jsdelivr.net/npm/") || !strings.Contains(string(cdn), "integrity=") {
		t.Errorf("CDN mode should link jsDelivr with SRI")
	}
	if strings.Contains(string(cdn), "%%CSS_SOURCES%%") || strings.Contains(string(cdn), "%%JS_SOURCES%%") {
		t.Errorf("CDN loader placeholders were not substituted")
	}
	if !strings.Contains(string(cdn), `"u":"https://cdn.jsdelivr.net/npm/`) {
		t.Errorf("CDN loader should list the npm mirrors first")
	}

	local, err := tr.HTML(WithMedia(URLMedia()), WithAssets(AssetsLocal), WithAssetsBase("assets/"))
	if err != nil {
		t.Fatalf("local: %v", err)
	}
	if !strings.Contains(string(local), `href="assets/discord-transcript.min.css"`) ||
		!strings.Contains(string(local), `src="assets/discord-transcript.min.js"`) {
		t.Errorf("local mode should reference relative paths")
	}

	inline, err := tr.HTML(WithMedia(URLMedia()), WithAssets(AssetsInline))
	if err != nil {
		t.Fatalf("inline: %v", err)
	}
	if !strings.Contains(string(inline), "<style>") || !strings.Contains(string(inline), "--dt-bg-primary") {
		t.Errorf("inline mode should embed the stylesheet")
	}
	// The recovery comment still documents the CDN URLs on purpose; what must
	// not happen is the document *loading* anything from a CDN.
	if strings.Contains(string(inline), `<link rel="stylesheet" href="https://`) ||
		strings.Contains(string(inline), `<script src="https://`) {
		t.Errorf("inline mode should not load anything from a CDN")
	}
}

// failingStore simulates an expired CDN object.
type failingStore struct{ calls int }

func (f *failingStore) Store(_ context.Context, ref MediaRef) (string, error) {
	f.calls++
	return "", errors.New("410 Gone")
}

func TestMediaFailureKeepsOriginalURLAndWarns(t *testing.T) {
	store := &failingStore{}
	var warnings []string

	tr := &Transcript{
		Channel: Channel{Name: "general", Type: ChannelText},
		Messages: []Message{{
			Author:  Author{Key: "1", Name: "piton", AvatarURL: "https://example.com/avatar.png"},
			Content: ParseContent("hi"),
			Attachments: []Attachment{{
				Kind: MediaImage,
				URL:  "https://example.com/expired.png",
			}},
		}},
	}
	doc, err := tr.HTML(
		WithMedia(store),
		WithWarn(func(err error) { warnings = append(warnings, err.Error()) }),
	)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	if !strings.Contains(string(doc), "https://example.com/expired.png") {
		t.Errorf("failed media should fall back to the original URL")
	}
	if len(warnings) == 0 {
		t.Errorf("a failed media download should be reported")
	}
	if store.calls == 0 {
		t.Errorf("media store was never consulted")
	}
}

func TestContinuationMetadataIsEmitted(t *testing.T) {
	// The script groups continuation rows from profile+timestamp, so both must
	// always be present.
	tr := sampleTranscript(t)
	doc, err := tr.HTML(WithMedia(URLMedia()))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	if strings.Count(string(doc), " profile=\"") < 3 {
		t.Errorf("every user message should carry a profile key")
	}
	if !strings.Contains(string(doc), `"profiles":{"111111111111111111"`) {
		t.Errorf("profile map should be keyed by the stable author key")
	}
}

func TestSystemMessageContentIsInline(t *testing.T) {
	tr := &Transcript{
		Channel: Channel{Name: "general", Type: ChannelText},
		Messages: []Message{{
			System: &SystemMessage{Type: SystemJoin, Content: ParseContent("```\nnot a block here\n```")},
		}},
	}
	doc, err := tr.HTML(WithMedia(URLMedia()))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	if strings.Contains(string(doc), "<discord-pre>") {
		t.Errorf("system messages should not contain block elements")
	}
}
