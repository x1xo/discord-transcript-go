// Package transcript renders Discord conversations as self-contained HTML
// transcripts using the discord-transcript-ui web components.
//
// The package is provider-neutral: it has no dependency on any Discord library.
// Producers convert their own message types into the small model in model.go and
// hand the result to Render, or use the ready-made adapter in the sibling
// package github.com/x1xo/discord-transcript-go/disgo.
//
// Typical use:
//
//	tr := &transcript.Transcript{
//		Channel:  transcript.Channel{Name: "general", Type: transcript.ChannelText},
//		Messages: messages,
//	}
//	if err := tr.WriteFile("transcript.html", transcript.WithMedia(transcript.InlineMedia())); err != nil {
//		log.Fatal(err)
//	}
package transcript

import "time"

// Channel is the conversation a transcript belongs to.
type Channel struct {
	// Name is rendered in the transcript header. Leave empty to omit the header.
	Name string
	// Type chooses the icon/prefix on that header.
	Type ChannelType
	// Guild is optional context for the page metadata.
	Guild string
	// ID is the channel snowflake, when known.
	ID string
}

// ChannelType mirrors the channel-type attribute of <discord-messages>.
type ChannelType string

// Supported channel types.
const (
	ChannelText   ChannelType = "text"
	ChannelVoice  ChannelType = "voice"
	ChannelThread ChannelType = "thread"
	ChannelForum  ChannelType = "forum"
	ChannelLocked ChannelType = "locked"
)

// Author is the identity drawn in the message gutter and header.
//
// Key is what ties messages to the profile map in the generated document: it
// must be stable for a given person (a user ID works well), and is what the
// stylesheet uses to detect continuation rows.
type Author struct {
	Key       string
	Name      string
	AvatarURL string
	RoleColor string // "#rrggbb"; empty means the default colour
	Bot       bool
	Verified  bool
	Server    bool
	Official  bool
	OP        bool
}

// isZero reports whether the author carries no usable identity.
func (a Author) isZero() bool { return a.Key == "" && a.Name == "" }

// Transcript is a whole conversation ready to be rendered.
type Transcript struct {
	Channel  Channel
	Messages []Message

	// Title overrides the <title> tag. Defaults to the channel name.
	Title string
	// GeneratedAt is stamped into the page metadata.
	GeneratedAt time.Time
	// Generator names the producer, shown in the page metadata.
	Generator string
}

// Message is one rendered row.
type Message struct {
	Author    Author
	Timestamp time.Time
	// Edited adds the "(edited)" marker.
	Edited bool
	// Highlight tints the row and adds an accent bar (mentions of the viewer).
	Highlight bool
	// Ephemeral tints the row like Discord's "only you can see this" messages.
	Ephemeral bool

	// Content is the parsed message body.
	Content []Node

	// Reply is drawn above the author row, as Discord does.
	Reply *Reply
	// System replaces the author row for Discord's non-user messages.
	System *SystemMessage

	Embeds      []Embed
	Attachments []Attachment
	Reactions   []Reaction
	Thread      *Thread
}

// IsSystem reports whether the row renders as a system message.
func (m Message) IsSystem() bool { return m.System != nil }

// Reply is the "replying to" preview.
type Reply struct {
	Author   Author
	Content  []Node
	Mentions bool
	Deleted  bool
}

// SystemType is the type attribute of <discord-system-message>.
type SystemType string

// Supported system message types.
const (
	SystemJoin       SystemType = "join"
	SystemLeave      SystemType = "leave"
	SystemCall       SystemType = "call"
	SystemMissedCall SystemType = "missed-call"
	SystemBoost      SystemType = "boost"
	SystemEdit       SystemType = "edit"
	SystemThread     SystemType = "thread"
	SystemPin        SystemType = "pin"
	SystemAlert      SystemType = "alert"
	SystemError      SystemType = "error"
	SystemUpgrade    SystemType = "upgrade"
)

// SystemMessage is a non-user message row.
type SystemMessage struct {
	Type    SystemType
	Content []Node
}

// Embed mirrors the subset of Discord's embed that the stylesheet renders.
type Embed struct {
	Color       string // "#rrggbb"; empty uses the stylesheet default
	URL         string
	Title       string
	Provider    string
	Description []Node
	Author      *EmbedAuthor
	Fields      []EmbedField
	Footer      *EmbedFooter
	Thumbnail   *Media
	Image       *Media
	Video       *Media
}

// EmbedAuthor is the small author row at the top of an embed.
type EmbedAuthor struct {
	Name string
	URL  string
	Icon *Media
}

// EmbedField is one field inside an embed's field grid.
type EmbedField struct {
	Name   string
	Value  []Node
	Inline bool
}

// EmbedFooter is the footer bar of an embed.
type EmbedFooter struct {
	Text      string
	Icon      *Media
	Timestamp time.Time
}

// MediaKind tells the renderer which element to emit.
type MediaKind string

// Supported media kinds.
const (
	MediaImage  MediaKind = "image"
	MediaVideo  MediaKind = "video"
	MediaAudio  MediaKind = "audio"
	MediaFile   MediaKind = "file"
	MediaEmoji  MediaKind = "emoji"
	MediaAvatar MediaKind = "avatar"
)

// Media is a reference to an image, video or audio resource.
//
// URL is resolved through the configured MediaStore while rendering, so it may
// end up as a data URI, a relative path or the original remote URL.
type Media struct {
	URL    string
	Alt    string
	Width  int
	Height int
	Kind   MediaKind
}

// Attachment is a file, image, video or audio clip posted with a message.
type Attachment struct {
	Kind MediaKind
	URL  string
	Name string
	// Size overrides the rendered size text; when empty it is derived from
	// SizeBytes ("1.2 MB").
	Size            string
	SizeBytes       int64
	Alt             string
	Width           int
	Height          int
	Spoiler         bool
	DurationSeconds float64
}

// Reaction is one reaction pill.
type Reaction struct {
	// Emoji is a unicode character, or an image URL for a custom emoji.
	Emoji string
	// Name is the ":name:" form, used as alt text for custom emoji.
	Name string
	// Count defaults to 1 when zero.
	Count int
	// Reacted highlights the pill.
	Reacted bool
}

// Thread is the thread preview attached to a message.
type Thread struct {
	Name    string
	CTA     string
	Message []Message
}
