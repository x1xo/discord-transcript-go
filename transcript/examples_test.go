package transcript

import (
	"path/filepath"
	"testing"
	"time"
)

// examplePNG is a 1x1 PNG used for every avatar, attachment and embed image in
// the examples. Inline media keeps the examples deterministic and offline: the
// real download path — proxies included — is covered by media_test.go.
const examplePNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="

// TestGenerateExamples rewrites examples/*.html. It is skipped unless -update is
// passed, which is also what regenerates the golden file:
//
//	go test ./transcript -run TestGenerateExamples -update
//
// The examples are what scripts/verify-browser.mjs renders in a real browser, so
// they are committed alongside the source.
func TestGenerateExamples(t *testing.T) {
	if !*update {
		t.Skip("pass -update to rewrite examples/")
	}

	variants := []struct {
		file    string
		options []Option
	}{
		{"transcript.html", nil},
		{"transcript-interactive.html", []Option{WithAssets(AssetsWithScript())}},
		{"transcript-short.html", []Option{WithShortTags()}},
	}

	for _, variant := range variants {
		options := append([]Option{WithMedia(URLMedia())}, variant.options...)
		path := filepath.Join("..", "examples", variant.file)
		if err := exampleTranscript(t).WriteFile(path, options...); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("wrote %s", path)
	}
}

// exampleTranscript is the conversation every example renders: one of each
// supported component, including the markdown the parser handles.
func exampleTranscript(t *testing.T) *Transcript {
	t.Helper()
	res := Resolvers{
		Users:    Profiles{"100000000000000002": {Key: "100000000000000002", Name: "ravik"}},
		Channels: Channels{"300000000000000002": "rules"},
		Roles:    Roles{"400000000000000001": {Name: "Moderators", Color: "#57f287"}},
	}
	parse := func(content string) []Node { return ParseContentWith(content, res) }
	at := func(hour, minute int) time.Time {
		return time.Date(2024, 3, 15, hour, minute, 0, 0, time.UTC)
	}

	piton := Author{Key: "100000000000000001", Name: "piton", AvatarURL: examplePNG, RoleColor: "#57f287"}
	miona := Author{
		Key: "100000000000000002", Name: "miona", AvatarURL: examplePNG,
		RoleColor: "#eb459e", Bot: true, Verified: true,
	}

	return &Transcript{
		Channel: Channel{Name: "general", Type: ChannelText, Guild: "Test Server"},
		Title:   "general — 2024-03-15",
		Messages: []Message{
			{
				Author:    piton,
				Timestamp: at(14, 28),
				Content: parse("Hey! **bold**, *italic*, __underline__, ~~strike~~, ||spoiler|| and `inline code`.\n" +
					"Welcome <@100000000000000002>, please read <#300000000000000002>, ping <@&400000000000000001>, hi @everyone.\n" +
					"An inline timestamp <t:1710512880:R> and a link https://example.com/docs."),
			},
			{
				Author:    miona,
				Timestamp: at(14, 29),
				Edited:    true,
				Highlight: true,
				Reply: &Reply{
					Author:   piton,
					Mentions: true,
					Content:  parse("Hey! **bold**, *italic*, __underline__, ~~strike~~, ||spoiler|| and `inline code`."),
				},
				Content: parse("# Release notes\n## Highlights\nThanks! Here is a quote and a list:\n" +
					"> quoted line\n- one\n- two\n1. first\n2. second\n-# small print\n" +
					"```go\nfmt.Println(\"hi\")\n```"),
				Attachments: []Attachment{
					{Kind: MediaImage, URL: examplePNG, Name: "screenshot.png", Alt: "a screenshot", Width: 400, Height: 300},
					{Kind: MediaFile, URL: "https://example.com/report.pdf", Name: "report.pdf", SizeBytes: 48128},
				},
				Reactions: []Reaction{
					{Emoji: "🎉", Count: 3, Reacted: true},
					{Emoji: "🍰", Count: 12},
				},
			},
			{
				Timestamp: at(14, 31),
				System:    &SystemMessage{Type: SystemJoin, Content: parse("piton joined the server.")},
			},
			{
				Author:    miona,
				Timestamp: at(14, 32),
				Embeds: []Embed{{
					Color:       "#5865f2",
					URL:         "https://example.com/embed",
					Title:       "Embeds keep their content as real markup",
					Provider:    "Example",
					Description: parse("The description keeps **markdown** and a <@100000000000000002> mention."),
					Author:      &EmbedAuthor{Name: "miona", URL: "https://example.com/miona", Icon: &Media{URL: examplePNG, Kind: MediaImage}},
					Fields: []EmbedField{
						{Name: "Stylesheet", Value: parse("one file"), Inline: true},
						{Name: "Script", Value: parse("optional"), Inline: true},
						{Name: "Media", Value: parse("base64 inline by default")},
					},
					Footer: &EmbedFooter{
						Text:      "Footer text",
						Icon:      &Media{URL: examplePNG, Kind: MediaImage},
						Timestamp: at(14, 32),
					},
					Thumbnail: &Media{URL: examplePNG, Kind: MediaImage},
					Image:     &Media{URL: examplePNG, Kind: MediaImage},
				}},
			},
		},
	}
}
