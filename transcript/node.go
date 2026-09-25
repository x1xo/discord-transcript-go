package transcript

import "time"

// NodeKind discriminates the parsed content tree.
type NodeKind uint8

// Node kinds produced by the markdown parser.
const (
	// NodeText is literal text. Its value is in Text.
	NodeText NodeKind = iota
	// NodeLineBreak is a hard line break inside a paragraph.
	NodeLineBreak
	// NodeBold, NodeItalic, NodeUnderline, NodeStrikethrough and NodeSpoiler are
	// inline containers.
	NodeBold
	NodeItalic
	NodeUnderline
	NodeStrikethrough
	NodeSpoiler
	// NodeCode is an inline code span.
	NodeCode
	// NodeCodeBlock is a fenced code block; Language holds the info string.
	NodeCodeBlock
	// NodeQuote is a block quote.
	NodeQuote
	// NodeList is a list container; Ordered and Start describe it.
	NodeList
	// NodeListItem is one list item.
	NodeListItem
	// NodeHeading is a markdown heading; Level is 1..3.
	NodeHeading
	// NodeSubtext is Discord's "-# small print" line.
	NodeSubtext
	// NodeLink is a hyperlink; URL holds the target.
	NodeLink
	// NodeMention is a user, role or channel mention.
	NodeMention
	// NodeEmoji is a custom emoji; EmojiURL holds its image.
	NodeEmoji
	// NodeTimestamp is a Discord <t:...> timestamp.
	NodeTimestamp
)

// MentionType is the mention kind rendered into the type attribute.
type MentionType string

// Supported mention types.
const (
	MentionUser     MentionType = "user"
	MentionRole     MentionType = "role"
	MentionChannel  MentionType = "channel"
	MentionVoice    MentionType = "voice"
	MentionThread   MentionType = "thread"
	MentionForum    MentionType = "forum"
	MentionLocked   MentionType = "locked"
	MentionEveryone MentionType = "everyone"
	MentionHere     MentionType = "here"
)

// Node is one piece of parsed message content.
//
// A single struct with a Kind discriminator keeps the model trivially
// serialisable and cheap to walk; only the fields relevant to Kind are set.
type Node struct {
	Kind NodeKind

	// Text holds literal text (NodeText), the link label (NodeLink), the
	// mention's display text without the sigil (NodeMention), the fallback text
	// for a timestamp (NodeTimestamp) and the emoji name (NodeEmoji).
	Text string

	// Children holds the nested nodes of container kinds, the items of a list,
	// and the value of an embed field.
	Children []Node

	// URL is the link target for NodeLink.
	URL string

	// MentionType and RoleColor describe a NodeMention.
	MentionType MentionType
	RoleColor   string

	// EmojiURL is the image for a NodeEmoji.
	EmojiURL string

	// Timestamp and Format describe a NodeTimestamp (Format is one of
	// Discord's t T d D f F R s S flags).
	Timestamp time.Time
	Format    byte

	// Language is the info string of a NodeCodeBlock.
	Language string

	// Level is the heading level of a NodeHeading.
	Level int

	// Ordered and Start describe a NodeList.
	Ordered bool
	Start   int
}

// Children helpers keep the model readable at call sites.

// Text returns a literal text node.
func Text(s string) Node { return Node{Kind: NodeText, Text: s} }

// Bold wraps nodes in <discord-bold>.
func Bold(children ...Node) Node { return Node{Kind: NodeBold, Children: children} }

// Italic wraps nodes in <discord-italic>.
func Italic(children ...Node) Node { return Node{Kind: NodeItalic, Children: children} }

// Underline wraps nodes in <discord-underlined>.
func Underline(children ...Node) Node { return Node{Kind: NodeUnderline, Children: children} }

// Strikethrough wraps nodes in <discord-strikethrough>.
func Strikethrough(children ...Node) Node {
	return Node{Kind: NodeStrikethrough, Children: children}
}

// Spoiler wraps nodes in <discord-spoiler>.
func Spoiler(children ...Node) Node { return Node{Kind: NodeSpoiler, Children: children} }

// Code returns an inline code span.
func Code(s string) Node { return Node{Kind: NodeCode, Text: s} }

// Quote wraps nodes in <discord-quote>.
func Quote(children ...Node) Node { return Node{Kind: NodeQuote, Children: children} }

// Link returns a hyperlink node.
func Link(url string, children ...Node) Node {
	return Node{Kind: NodeLink, URL: url, Children: children}
}

// PlainText flattens a node tree to its text, for alt text and fallbacks.
func PlainText(nodes []Node) string {
	var b []byte
	for _, n := range nodes {
		b = appendPlain(b, n)
	}
	return string(b)
}

func appendPlain(b []byte, n Node) []byte {
	switch n.Kind {
	case NodeText, NodeCode:
		b = append(b, n.Text...)
	case NodeMention:
		b = append(b, n.Text...)
	case NodeEmoji:
		if n.Text != "" {
			b = append(b, n.Text...)
		}
	case NodeTimestamp:
		b = append(b, n.Text...)
	case NodeLineBreak:
		b = append(b, '\n')
	default:
		for _, child := range n.Children {
			b = appendPlain(b, child)
		}
	}
	return b
}
