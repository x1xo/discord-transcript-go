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
		Channel: Channel{Name: "general", Type: ChannelText, Guild: "Test Guild", GuildIcon: "https://cdn.discordapp.com/icons/900/abc.png"},
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
				Content: parse("# Release notes\n## Highlights\nThanks! Here is a quote and a list:\n> quoted line\n- one\n- two\n1. first\n2. second\n" +
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
				ActionRows: []ActionRow{{
					Buttons: []Button{
						{Label: "Confirm", Style: ButtonPrimary, Emoji: "✅"},
						{Label: "Docs", Style: ButtonLink, URL: "https://example.com/docs"},
						{Label: "Delete", Style: ButtonDanger, Disabled: true},
						{EmojiURL: "https://cdn.discordapp.com/emojis/999999999999999999.png", EmojiName: ":party:"},
					},
				}},
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
	// A fixed stylesheet keeps the golden about markup: the real pins are covered
	// by TestAssetsConfiguration and TestPinsMatchLocalUIBuild, so a version bump
	// no longer churns this file.
	got, err := tr.HTML(
		WithMedia(URLMedia()),
		WithGenerator("discord-transcript-go test"),
		WithCSS("https://cdn.example.test/discord-transcript.min.css", "sha384-test"),
	)
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

	// The document must carry the complete static structure, because the
	// stylesheet alone has to render it.
	for _, want := range []string{
		"<!doctype html>",
		`<link rel="stylesheet" href="` + DefaultCSSURL + `" integrity="` + DefaultCSSIntegrity + `" crossorigin="anonymous">`,
		`<discord-messages channel-name="general" channel-type="text">`,
		`<discord-message profile="111111111111111111" author="piton" timestamp="2024-03-15T14:28:00Z" data-dt-r>`,
		`<div class="dt-msg">`,
		`<span class="dt-avatar"><img src="https://cdn.discordapp.com/avatars/111/abc.png?size=64"`,
		`<div class="dt-content">`,
		`<div class="dt-header">`,
		`<span class="dt-author" style="color:#57f287">piton</span>`,
		`<time class="dt-timestamp" datetime="2024-03-15T14:28:00Z"`,
		`<div class="dt-body">`,
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
		`<discord-custom-emoji name=":party:"><img src="https://cdn.discordapp.com/emojis/999999999999999999.png?size=64"`,
		`<discord-time timestamp="2024-03-15T13:20:00Z" format="R">`,
		`<discord-link href="https://example.com/x"`,
		`<span class="dt-badges"><span class="dt-badge dt-badge--verified">APP</span></span>`,
		`data-dt-continuation`,
		`data-dt-short-time="2:32PM"`,
		`<discord-reply mentions data-dt-r><span class="dt-reply-avatar">`,
		`<span class="dt-reply-author">@piton</span>`,
		"<discord-quote>quoted line</discord-quote>",
		"<discord-unordered-list><discord-list-item>one</discord-list-item>",
		`<discord-ordered-list start="1"><discord-list-item>first</discord-list-item>`,
		`<span class="dt-code-lang">go</span><discord-code>fmt.Println(&#34;hi&#34;)</discord-code>`,
		"<discord-subscript>small print</discord-subscript>",
		`<discord-header level="1">Release notes</discord-header>`,
		`<discord-header level="2">Highlights</discord-header>`,
		`<discord-system-message type="join" timestamp="2024-03-15T14:31:00Z">`,
		`<discord-reaction data-dt-r><span class="dt-reaction-emoji">🎉</span><span class="dt-reaction-count">3</span>`,
		`<discord-reaction data-dt-r reacted><img class="dt-reaction-emoji" src="https://cdn.discordapp.com/emojis/999999999999999999.png?size=64" alt=":party:"`,
		`<discord-reaction data-dt-r><span class="dt-reaction-emoji">🍰</span>`,
		`<discord-embed color="#5865f2" style="--dt-embed-color:#5865f2" data-dt-r>`,
		`<a class="dt-embed-title" href="https://example.com/embed"`,
		`<discord-embed-description>`,
		`<discord-embed-field field-title="Inline one" inline>`,
		`<div class="dt-embed-image">`,
		`<div class="dt-embed-thumbnail">`,
		`<discord-embed-footer>`,
		`<discord-image-attachment data-dt-r>`,
		`<discord-image-attachment spoiler data-dt-r>`,
		`<discord-file-attachment data-dt-r name="report.pdf" bytes="1.5" bytes-unit="KB" type="PDF">`,
		`<span class="dt-file-size">1.5 KB</span>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("document is missing %s", want)
		}
	}

	// Minimal by construction: no comments, no indentation, no script.
	if strings.Contains(html, "<!--") {
		t.Errorf("generated document must not contain comments")
	}
	if strings.Contains(html, "\n\t") || strings.Contains(html, "\t<") {
		t.Errorf("generated document must not be indented")
	}
	if strings.Contains(html, "<script") {
		t.Errorf("the enhancement script is off by default:\n%s", html)
	}
	if strings.Contains(html, "dt-page__meta") {
		t.Errorf("the metadata line is off by default")
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

func TestAssetsConfiguration(t *testing.T) {
	tr := sampleTranscript(t)

	// Defaults: the pinned stylesheet with its SRI hash, and no script.
	def, err := tr.HTML(WithMedia(URLMedia()))
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	if !strings.Contains(string(def), DefaultCSSURL) || !strings.Contains(string(def), DefaultCSSIntegrity) {
		t.Errorf("defaults should reference the pinned stylesheet and its hash")
	}
	if !strings.Contains(string(def), `crossorigin="anonymous"`) {
		t.Errorf("SRI on a cross-origin stylesheet requires crossorigin")
	}

	// A caller-supplied CDN, with their own hash.
	custom, err := tr.HTML(
		WithMedia(URLMedia()),
		WithCSS("https://cdn.example.com/discord-transcript.min.css", "sha384-abc123"),
	)
	if err != nil {
		t.Fatalf("custom css: %v", err)
	}
	if !strings.Contains(string(custom), `href="https://cdn.example.com/discord-transcript.min.css" integrity="sha384-abc123"`) {
		t.Errorf("custom stylesheet URL and hash should be used verbatim:\n%s", custom)
	}
	if strings.Contains(string(custom), "jsdelivr") {
		t.Errorf("an explicit stylesheet should replace the default")
	}

	// Opting into the enhancement script, which defaults to the pinned release.
	withScript, err := tr.HTML(WithMedia(URLMedia()), WithScript())
	if err != nil {
		t.Fatalf("script: %v", err)
	}
	if !strings.Contains(string(withScript), `<script defer src="`+DefaultScriptURL+`" integrity="`+DefaultScriptIntegrity+`" crossorigin="anonymous"></script>`) {
		t.Errorf("WithScript should emit a deferred, verified script tag:\n%s", withScript)
	}
	// The script is the pinned one, so a rebuild against a new UI release is the
	// only thing that moves it.
	if !strings.Contains(DefaultScriptURL, ContractVersion) {
		t.Errorf("the pinned script URL %q should name contract %s", DefaultScriptURL, ContractVersion)
	}

	// The same, through the whole-assets helper.
	same, err := tr.HTML(WithMedia(URLMedia()), WithAssets(AssetsWithScript()))
	if err != nil {
		t.Fatalf("assets with script: %v", err)
	}
	if string(same) != string(withScript) {
		t.Errorf("WithScript and AssetsWithScript should agree on the script tag")
	}

	// A script from somewhere else, with the caller's own hash.
	customScript, err := tr.HTML(
		WithMedia(URLMedia()),
		WithScriptURL("https://cdn.example.com/discord-transcript.min.js", "sha384-abc123"),
	)
	if err != nil {
		t.Fatalf("custom script: %v", err)
	}
	if !strings.Contains(string(customScript), `src="https://cdn.example.com/discord-transcript.min.js" integrity="sha384-abc123"`) {
		t.Errorf("WithScriptURL should use the URL and hash verbatim:\n%s", customScript)
	}
	if strings.Contains(string(customScript), DefaultScriptURL) {
		t.Errorf("an explicit script URL should replace the default")
	}

	// The script survives the short-tag vocabulary, in either order.
	for _, opts := range [][]Option{
		{WithScript(), WithShortTags()},
		{WithShortTags(), WithScript()},
	} {
		short, err := tr.HTML(append([]Option{WithMedia(URLMedia())}, opts...)...)
		if err != nil {
			t.Fatalf("short tags with script: %v", err)
		}
		if !strings.Contains(string(short), DefaultScriptURL) || !strings.Contains(string(short), DefaultShortCSSURL) {
			t.Errorf("WithScript and WithShortTags should compose in either order:\n%s", short)
		}
	}

	// A caller who assembled the whole Assets keeps their own CrossOrigin, so the
	// pinned pair never re-opens a CORS request they turned off.
	kept, err := tr.HTML(
		WithMedia(URLMedia()),
		WithAssets(Assets{CSSURL: "https://cdn.example.com/x.css", CSSIntegrity: "sha384-abc"}),
		WithScript(),
	)
	if err != nil {
		t.Fatalf("custom assets with script: %v", err)
	}
	if strings.Contains(string(kept), "crossorigin") {
		t.Errorf("WithScript should not override an explicit Assets.CrossOrigin:\n%s", kept)
	}
	if !strings.Contains(string(kept), DefaultScriptURL) {
		t.Errorf("but it should still add the pinned script")
	}

	// Opting back out.
	off, err := tr.HTML(WithMedia(URLMedia()), WithScript(), WithoutScript())
	if err != nil {
		t.Fatalf("without script: %v", err)
	}
	if strings.Contains(string(off), "<script") {
		t.Errorf("WithoutScript should omit the script tag")
	}

	// Omitting the stylesheet entirely.
	none, err := tr.HTML(WithMedia(URLMedia()), WithoutStylesheet())
	if err != nil {
		t.Fatalf("no css: %v", err)
	}
	if strings.Contains(string(none), "<link") {
		t.Errorf("WithoutStylesheet should omit the link")
	}

	// Light theme rides on the root element.
	light, err := tr.HTML(WithMedia(URLMedia()), WithTheme(ThemeLight))
	if err != nil {
		t.Fatalf("light: %v", err)
	}
	if !strings.Contains(string(light), `<html lang="en" data-theme="light">`) {
		t.Errorf("light theme should be set on the root element")
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

func TestGroupingMetadata(t *testing.T) {
	tr := sampleTranscript(t)
	doc, err := tr.HTML(WithMedia(URLMedia()))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(doc)

	// Every user message carries the stable key the continuation rule uses.
	if strings.Count(html, ` profile="`) < 4 {
		t.Errorf("every user message should carry a profile key")
	}
	// The second miona message follows within the window, so the author row is
	// omitted and the gutter timestamp is available for hover.
	if !strings.Contains(html, `data-dt-continuation data-dt-short-time="2:32PM"`) {
		t.Errorf("expected a continuation row with a short timestamp:\n%s", html)
	}
	// Continuation rows must not repeat the header.
	if strings.Count(html, `class="dt-header"`) != 3 {
		t.Errorf("continuation rows should omit the header, got %d headers", strings.Count(html, `class="dt-header"`))
	}
	if !strings.Contains(html, `data-dt-group-start`) {
		t.Errorf("expected group-start markers between authors")
	}
}

func TestShortTags(t *testing.T) {
	tr := sampleTranscript(t)

	doc, err := tr.HTML(WithMedia(URLMedia()), WithShortTags())
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(doc)

	// No long tag name may survive, or the element would be unstyled.
	for _, bad := range []string{"<discord-", "</discord-"} {
		if strings.Contains(html, bad) {
			t.Errorf("short-tag output still contains %s:\n%s", bad, html)
		}
	}
	for _, want := range []string{"<dms ", "<dm ", "<dme ", "<dsp>", "<de ", "<drp ", "<dc>", "<dh ", "<def "} {
		if !strings.Contains(html, want) {
			t.Errorf("short-tag output is missing %s", want)
		}
	}
	// The document must point at the matching stylesheet, or nothing renders.
	if !strings.Contains(html, DefaultShortCSSURL) || !strings.Contains(html, DefaultShortCSSIntegrity) {
		t.Errorf("short tags should switch the default stylesheet to the short one")
	}
}

func TestShortTagsRespectAnExplicitStylesheet(t *testing.T) {
	tr := sampleTranscript(t)
	doc, err := tr.HTML(
		WithMedia(URLMedia()),
		WithShortTags(),
		WithCSS("https://cdn.example.com/short.css", "sha384-abc"),
	)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(doc)
	if !strings.Contains(html, "https://cdn.example.com/short.css") {
		t.Errorf("an explicit stylesheet must win over the short default:\n%s", html)
	}
	if strings.Contains(html, DefaultShortCSSURL) {
		t.Errorf("the pinned default should have been replaced")
	}
	if !strings.Contains(html, "<dm ") {
		t.Errorf("the markup should still use short tags")
	}
}

func TestShortTagsDoNotTouchMessageText(t *testing.T) {
	// A message that literally mentions a tag must stay escaped text, not become
	// markup or a short code.
	tr := &Transcript{
		Channel: Channel{Name: "general", Type: ChannelText},
		Messages: []Message{{
			Author:  Author{Key: "1", Name: "piton"},
			Content: ParseContent("use <discord-embed> and <discord-spoiler> here"),
		}},
	}
	doc, err := tr.HTML(WithMedia(URLMedia()), WithShortTags())
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(doc)
	if !strings.Contains(html, "&lt;discord-embed&gt;") || !strings.Contains(html, "&lt;discord-spoiler&gt;") {
		t.Errorf("escaped tag names in message text must be left alone:\n%s", html)
	}
	if strings.Contains(html, "&lt;dsp&gt;") {
		t.Errorf("escaped text must not be shortened")
	}
}

func TestShortenTagsCoversEveryName(t *testing.T) {
	for name, short := range ShortTags() {
		long := "discord-" + name
		got := shortenTags("<" + long + " a=\"1\">x</" + long + ">")
		want := "<" + short + " a=\"1\">x</" + short + ">"
		if got != want {
			t.Errorf("shortenTags(%s) = %q, want %q", long, got, want)
		}
	}
}

func TestTextNodesKeepTheirLineBreaks(t *testing.T) {
	// A producer that hands over a text node with newlines, instead of parsing the
	// content into nodes, must still get the line breaks a reader expects: without
	// this the lines collapse when nothing styles them.
	tr := &Transcript{
		Channel: Channel{Name: "general", Type: ChannelText},
		Messages: []Message{{
			Author: Author{Key: "1", Name: "piton"},
			Embeds: []Embed{{Description: []Node{Text("first line\nsecond line\r\nthird")}}},
		}},
	}
	doc, err := tr.HTML(WithMedia(URLMedia()))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	if !strings.Contains(string(doc), "first line<br>second line<br>third") {
		t.Errorf("newlines in a text node should become hard breaks:\n%s", doc)
	}
}

func TestPageHeaderIsOptIn(t *testing.T) {
	tr := sampleTranscript(t)

	plain, err := tr.HTML(WithMedia(URLMedia()))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	if strings.Contains(string(plain), "dt-page__title") {
		t.Errorf("the page header should be off by default")
	}

	withHeader, err := tr.HTML(WithMedia(URLMedia()), WithPageHeader())
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	if !strings.Contains(string(withHeader), `<h1 class="dt-page__title">#general</h1>`) {
		t.Errorf("WithPageHeader should render the title as an h1:\n%s", withHeader)
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
	// Unwrapping a code block used to take its body with it, because a code block
	// keeps its text in Text and has no children to fall back on.
	if !strings.Contains(string(doc), "not a block here") {
		t.Errorf("a system message must not lose the text of a code block:\n%s", doc)
	}
}

func TestEmbedDescriptionKeepsItsBlocks(t *testing.T) {
	// An embed description is a document, not one line of preview text: headings,
	// lists, quotes, code blocks and subtext have to reach the markup, in the
	// description and in field values alike.
	tr := &Transcript{
		Channel: Channel{Name: "general", Type: ChannelText},
		Messages: []Message{{
			Author: Author{Key: "1", Name: "ticket bot"},
			Embeds: []Embed{{
				Description: ParseContent("# Ticket\n\nopen since yesterday\n\n## Steps\n- one\n- two\n1. first\n> quoted\n```go\nfmt.Println()\n```\n-# from the bot"),
				Fields: []EmbedField{
					{Name: "Status", Value: ParseContent("**Open**"), Inline: true},
					{Name: "Log", Value: ParseContent("```\npanic: nil map\n```")},
				},
			}},
		}},
	}

	doc, err := tr.HTML(WithMedia(URLMedia()))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	for _, want := range []string{
		`<discord-embed-description><discord-header level="1">Ticket</discord-header>`,
		`<discord-header level="2">Steps</discord-header>`,
		`<discord-unordered-list><discord-list-item>one</discord-list-item><discord-list-item>two</discord-list-item></discord-unordered-list>`,
		`<discord-ordered-list start="1"><discord-list-item>first</discord-list-item></discord-ordered-list>`,
		`<discord-quote>quoted</discord-quote>`,
		`<discord-pre><span class="dt-code-lang">go</span><discord-code>fmt.Println()</discord-code></discord-pre>`,
		`<discord-subscript>from the bot</discord-subscript>`,
		`<discord-embed-field field-title="Status" inline><discord-bold>Open</discord-bold></discord-embed-field>`,
		`<discord-embed-field field-title="Log"><discord-pre><discord-code>panic: nil map</discord-code></discord-pre></discord-embed-field>`,
	} {
		if !strings.Contains(string(doc), want) {
			t.Errorf("missing %s in the rendered embed:\n%s", want, doc)
		}
	}
}

// renderChannel renders one message with the given channel header, so the guild
// header can be inspected on its own.
func renderChannel(t *testing.T, channel Channel) string {
	t.Helper()
	tr := &Transcript{
		Channel: channel,
		Messages: []Message{{
			Author:    Author{Key: "1", Name: "piton"},
			Timestamp: mustTime(t, "2024-03-15T14:28:00Z"),
			Content:   []Node{Text("hello")},
		}},
	}
	doc, err := tr.HTML(WithMedia(URLMedia()))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	return string(doc)
}

func TestGuildHeader(t *testing.T) {
	withIcon := renderChannel(t, Channel{
		Name: "general", Type: ChannelText,
		Guild: "Test Guild", GuildIcon: "https://cdn.discordapp.com/icons/900/abc.png",
	})
	for _, want := range []string{
		`<discord-guild-header data-dt-r>`,
		`<span class="dt-guild-icon"><img src="https://cdn.discordapp.com/icons/900/abc.png?size=64"`,
		`<span class="dt-guild-name">Test Guild</span>`,
		`<span class="dt-guild-channel">#general</span>`,
	} {
		if !strings.Contains(withIcon, want) {
			t.Errorf("missing %s in:\n%s", want, withIcon)
		}
	}
	// The channel name stays on the container too: that is the stylesheet's own
	// fallback header, for documents with no guild to show.
	if !strings.Contains(withIcon, `channel-name="general"`) {
		t.Errorf("the channel name should stay on discord-messages:\n%s", withIcon)
	}

	// A guild with no icon keeps the header's shape with a coloured initial.
	initials := renderChannel(t, Channel{Name: "general", Type: ChannelText, Guild: "Test Guild"})
	if !strings.Contains(initials, `class="dt-guild-icon dt-guild-icon--initials"`) {
		t.Errorf("a guild without an icon should fall back to an initial:\n%s", initials)
	}

	// A channel with no type is named without a prefix.
	noType := renderChannel(t, Channel{Name: "general", Guild: "Test Guild"})
	if !strings.Contains(noType, `<span class="dt-guild-channel">general</span>`) {
		t.Errorf("a typeless channel should have no prefix:\n%s", noType)
	}

	// No guild at all: no header, so the channel-name fallback is the only thing
	// naming the channel.
	plain := renderChannel(t, Channel{Name: "general", Type: ChannelText})
	if strings.Contains(plain, "discord-guild-header") {
		t.Errorf("a channel with no guild should not emit a header:\n%s", plain)
	}
}

func TestGuildIconIsPooledLikeAnAvatar(t *testing.T) {
	// Under the inline default the icon becomes a data URI, and a repeated one is
	// hoisted like any avatar. It must still fill its box, because the pooled
	// element carries the size of the role it was written with.
	tr := &Transcript{
		Channel: Channel{Name: "general", Type: ChannelText, Guild: "Test Guild", GuildIcon: examplePNG},
		Messages: []Message{
			{Author: Author{Key: "1", Name: "piton", AvatarURL: examplePNG}, Timestamp: mustTime(t, "2024-03-15T14:28:00Z"), Content: []Node{Text("one")}},
			{Author: Author{Key: "1", Name: "piton", AvatarURL: examplePNG}, Timestamp: mustTime(t, "2024-03-15T14:29:00Z"), Content: []Node{Text("two")}},
		},
	}
	doc, err := tr.HTML(WithMedia(InlineMedia()))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	if html := string(doc); !strings.Contains(html, `<span class="dt-guild-icon"><span class="dt-media dt-media-`) {
		t.Errorf("a repeated guild icon should be pooled inside its box:\n%s", html)
	}
}
