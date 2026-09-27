package transcript

import (
	"context"
	"strings"
	"time"
	"unicode/utf16"
)

// The renderer emits the complete markup the stylesheet targets, which is why a
// transcript renders fully with the stylesheet alone.
//
// Output is compact on purpose: no indentation, no newlines and no comments, so
// the file is as small as it can be while staying readable to a browser. Every
// element that carries a structure the enhancement script would otherwise build
// is marked data-dt-r, so the script (when included) leaves it alone
// instead of rebuilding it.
type renderer struct {
	ctx context.Context
	o   *Options
	b   strings.Builder
	// mediaMemo remembers what an inline blob became at a given edge, so a store
	// that hands back the same data URI for every message is only re-encoded once
	// per render.
	mediaMemo map[string]string
}

func renderFragment(ctx context.Context, t *Transcript, o *Options) (string, error) {
	r := &renderer{ctx: ctx, o: o}
	r.writeMessages(t)
	fragment := r.b.String()
	if o.ShortTags {
		fragment = shortenTags(fragment)
	}
	return fragment, nil
}

func (r *renderer) writeMessages(t *Transcript) {
	channel := effectiveChannel(t, r.o)
	groups := groupMessages(t.Messages, DefaultGroupWindow)
	r.b.WriteString("<discord-messages")
	r.attr("channel-name", channel.Name)
	if channel.Name != "" {
		r.attr("channel-type", string(channel.Type))
	}
	r.b.WriteString(">")
	r.writeGuildHeader(channel)
	for i, m := range t.Messages {
		r.writeMessage(m, groups[i])
	}
	r.b.WriteString("</discord-messages>")
}

// writeGuildHeader draws the server above the conversation: its icon, its name,
// and the channel under it. It is skipped entirely when there is no guild to
// show, in which case the stylesheet still names the channel from
// channel-name — that is why the channel is named in two places.
func (r *renderer) writeGuildHeader(c Channel) {
	if c.Guild == "" && c.GuildIcon == "" {
		return
	}
	r.b.WriteString(`<discord-guild-header data-dt-r>`)
	switch {
	case c.GuildIcon != "":
		r.b.WriteString(`<span class="dt-guild-icon">`)
		r.writeIcon(MediaRef{URL: c.GuildIcon, Kind: MediaImage, Filename: "guild-icon"}, "avatar")
		r.b.WriteString(`</span>`)
	case c.Guild != "":
		r.b.WriteString(`<span class="dt-guild-icon dt-guild-icon--initials" style="background-color:` +
			initialsColor(c.Guild) + `">` + escapeText(initial(c.Guild)) + `</span>`)
	}
	if c.Guild != "" || c.Name != "" {
		r.b.WriteString(`<span class="dt-guild-meta">`)
		if c.Guild != "" {
			r.b.WriteString(`<span class="dt-guild-name">` + escapeText(c.Guild) + `</span>`)
		}
		if c.Name != "" {
			r.b.WriteString(`<span class="dt-guild-channel">` +
				escapeText(channelPrefix(c.Type)+c.Name) + `</span>`)
		}
		r.b.WriteString(`</span>`)
	}
	r.b.WriteString(`</discord-guild-header>`)
}

// channelPrefix is what Discord puts in front of a channel name, and what the
// stylesheet's own channel-name fallback uses.
func channelPrefix(t ChannelType) string {
	switch t {
	case ChannelText, ChannelVoice, ChannelThread, ChannelForum, ChannelLocked:
		return "#"
	default:
		return ""
	}
}

func (r *renderer) writeMessage(m Message, g grouping) {
	if m.System != nil {
		r.writeSystemMessage(m)
		return
	}

	key := m.Author.Key
	if key == "" {
		key = m.Author.Name
	}

	r.b.WriteString("<discord-message")
	r.attr("profile", key)
	r.attr("author", m.Author.Name)
	r.attr("timestamp", r.iso(m.Timestamp))
	if m.Edited {
		r.flag("edited")
	}
	if m.Highlight {
		r.flag("highlight")
	}
	if m.Ephemeral {
		r.flag("ephemeral")
	}
	if g.continuation {
		r.flag("data-dt-continuation")
		r.attr("data-dt-short-time", r.shortStamp(m.Timestamp))
	} else if g.groupStart {
		r.flag("data-dt-group-start")
	}
	r.flag("data-dt-r")
	r.b.WriteString(">")

	r.b.WriteString(`<div class="dt-msg`)
	if m.Reply != nil {
		r.b.WriteString(" dt-msg--has-reply")
	}
	r.b.WriteString(`">`)
	if m.Reply != nil {
		r.writeReply(*m.Reply)
	}
	r.writeAvatar(m.Author, g)
	r.b.WriteString(`<div class="dt-content">`)
	if !g.continuation {
		r.writeHeader(m)
	}
	r.b.WriteString(`<div class="dt-body">`)
	r.writeNodes(m.Content)
	if m.Edited {
		r.b.WriteString(`<span class="dt-edited">(edited)</span>`)
	}
	for _, e := range m.Embeds {
		r.writeEmbed(e)
	}
	if len(m.Attachments) > 0 {
		r.writeAttachments(m.Attachments)
	}
	r.writeActionRows(m.ActionRows)
	if len(m.Reactions) > 0 {
		r.writeReactions(m.Reactions)
	}
	if m.Thread != nil {
		r.writeThread(*m.Thread)
	}
	r.b.WriteString(`</div></div></div></discord-message>`)
}

func (r *renderer) writeSystemMessage(m Message) {
	r.b.WriteString("<discord-system-message")
	r.attr("type", string(m.System.Type))
	r.attr("timestamp", r.iso(m.Timestamp))
	r.b.WriteString(">")
	r.writeNodes(inlineOnly(m.System.Content))
	r.b.WriteString("</discord-system-message>")
}

// writeHeader emits the author row: name, badges and timestamp.
func (r *renderer) writeHeader(m Message) {
	r.b.WriteString(`<div class="dt-header"><span class="dt-author"`)
	if m.Author.RoleColor != "" {
		r.b.WriteString(` style="color:` + escapeText(m.Author.RoleColor) + `"`)
	}
	r.b.WriteString(">" + escapeText(m.Author.Name) + "</span>")

	badges := ""
	switch {
	case m.Author.Bot && m.Author.Verified:
		badges = `<span class="dt-badge dt-badge--verified">APP</span>`
	case m.Author.Bot:
		badges = `<span class="dt-badge">APP</span>`
	case m.Author.Server:
		badges = `<span class="dt-badge dt-badge--server">SERVER</span>`
	case m.Author.Official:
		badges = `<span class="dt-badge dt-badge--official">OFFICIAL</span>`
	}
	if m.Author.OP {
		badges += `<span class="dt-badge">OP</span>`
	}
	if badges != "" {
		r.b.WriteString(`<span class="dt-badges">` + badges + `</span>`)
	}

	if !m.Timestamp.IsZero() {
		r.b.WriteString(`<time class="dt-timestamp" datetime="` + m.Timestamp.UTC().Format(time.RFC3339) +
			`" title="` + escapeText(r.fullStamp(m.Timestamp)) + `">` +
			escapeText(r.headerStamp(m.Timestamp)) + `</time>`)
	}
	r.b.WriteString(`</div>`)
}

// writeAvatar emits the gutter avatar. Continuation rows keep the column but
// not the image, because the stylesheet hides it anyway.
func (r *renderer) writeAvatar(a Author, g grouping) {
	if g.continuation && a.AvatarURL != "" {
		r.b.WriteString(`<span class="dt-avatar"></span>`)
		return
	}
	if a.AvatarURL == "" {
		r.b.WriteString(`<span class="dt-avatar dt-avatar--initials" style="background-color:` +
			initialsColor(a.Name) + `">` + escapeText(initial(a.Name)) + `</span>`)
		return
	}
	r.b.WriteString(`<span class="dt-avatar">`)
	r.writeIcon(MediaRef{URL: a.AvatarURL, Kind: MediaAvatar, Filename: a.Key}, "avatar")
	r.b.WriteString(`</span>`)
}

func (r *renderer) writeReply(reply Reply) {
	r.b.WriteString("<discord-reply")
	if reply.Mentions {
		r.flag("mentions")
	}
	if reply.Deleted {
		r.flag("deleted")
		r.b.WriteString("></discord-reply>")
		return
	}
	r.flag("data-dt-r")
	r.b.WriteString(">")
	if reply.Author.AvatarURL != "" {
		r.b.WriteString(`<span class="dt-reply-avatar">`)
		r.writeIcon(MediaRef{URL: reply.Author.AvatarURL, Kind: MediaAvatar, Filename: reply.Author.Key}, "avatar")
		r.b.WriteString(`</span>`)
	}
	name := reply.Author.Name
	if reply.Mentions {
		name = "@" + name
	}
	r.b.WriteString(`<span class="dt-reply-author">` + escapeText(name) + `</span>`)
	r.writeNodes(inlineOnly(reply.Content))
	r.b.WriteString("</discord-reply>")
}

func (r *renderer) writeEmbed(e Embed) {
	r.b.WriteString(`<discord-embed`)
	r.attr("color", e.Color)
	if e.Color != "" {
		// The stylesheet reads the custom property; the attribute alone is inert,
		// and the enhancement script skips rows already marked ready.
		r.b.WriteString(` style="--dt-embed-color:` + escapeText(e.Color) + `"`)
	}
	r.flag("data-dt-r")
	r.b.WriteString(">")

	if e.Provider != "" {
		r.b.WriteString(`<span class="dt-embed-provider">` + escapeText(e.Provider) + `</span>`)
	}
	if e.Author != nil {
		r.b.WriteString(`<span class="dt-embed-author">`)
		if e.Author.Icon != nil {
			r.writeIcon(MediaRef{URL: e.Author.Icon.URL, Kind: e.Author.Icon.Kind, Alt: e.Author.Icon.Alt}, "author-icon")
		}
		name := escapeText(e.Author.Name)
		if href := sanitizeURL(e.Author.URL); href != "" {
			r.b.WriteString(`<a href="` + escapeText(href) + `" target="_blank" rel="noopener noreferrer">` + name + `</a>`)
		} else {
			r.b.WriteString(name)
		}
		r.b.WriteString(`</span>`)
	}
	if e.Title != "" {
		title := escapeText(e.Title)
		if href := sanitizeURL(e.URL); href != "" {
			r.b.WriteString(`<a class="dt-embed-title" href="` + escapeText(href) + `" target="_blank" rel="noopener noreferrer">` + title + `</a>`)
		} else {
			r.b.WriteString(`<span class="dt-embed-title">` + title + `</span>`)
		}
	}
	if len(e.Description) > 0 {
		r.b.WriteString(`<discord-embed-description>`)
		// An embed description is a document, not a single line: headings, lists,
		// quotes and fenced blocks are what the author wrote there, and flattening
		// them dropped the structure (and, for a code block, the text with it).
		r.writeNodes(e.Description)
		r.b.WriteString(`</discord-embed-description>`)
	}
	if len(e.Fields) > 0 {
		r.b.WriteString(`<discord-embed-fields>`)
		for _, f := range e.Fields {
			r.b.WriteString(`<discord-embed-field`)
			r.attr("field-title", f.Name)
			if f.Inline {
				r.flag("inline")
			}
			r.b.WriteString(">")
			r.writeNodes(f.Value)
			r.b.WriteString(`</discord-embed-field>`)
		}
		r.b.WriteString(`</discord-embed-fields>`)
	}
	if e.Image != nil {
		r.b.WriteString(`<div class="dt-embed-image">`)
		r.writeMediaImg(*e.Image, "")
		r.b.WriteString(`</div>`)
	}
	if e.Video != nil {
		r.b.WriteString(`<div class="dt-embed-image">`)
		r.writeVideo(*e.Video)
		r.b.WriteString(`</div>`)
	}
	if e.Thumbnail != nil {
		r.b.WriteString(`<div class="dt-embed-thumbnail">`)
		r.writeIcon(MediaRef{URL: e.Thumbnail.URL, Kind: e.Thumbnail.Kind, Alt: e.Thumbnail.Alt}, "thumbnail")
		r.b.WriteString(`</div>`)
	}
	if e.Footer != nil {
		r.b.WriteString(`<discord-embed-footer>`)
		if e.Footer.Icon != nil {
			r.b.WriteString(`<span class="dt-embed-footer-icon">`)
			r.writeIcon(MediaRef{URL: e.Footer.Icon.URL, Kind: e.Footer.Icon.Kind, Alt: e.Footer.Icon.Alt}, "footer-icon")
			r.b.WriteString(`</span>`)
		}
		if e.Footer.Text != "" {
			r.b.WriteString(escapeText(e.Footer.Text))
		}
		if !e.Footer.Timestamp.IsZero() {
			r.b.WriteString(`<span class="dt-embed-footer-sep"></span>` +
				escapeText(r.inlineStamp(e.Footer.Timestamp, 'f')))
		}
		r.b.WriteString(`</discord-embed-footer>`)
	}
	r.b.WriteString(`</discord-embed>`)
}

func (r *renderer) writeAttachments(list []Attachment) {
	r.b.WriteString(`<discord-attachments>`)
	for _, a := range list {
		switch a.Kind {
		case MediaVideo:
			r.b.WriteString(`<discord-video-attachment`)
			if a.Spoiler {
				r.flag("spoiler")
			}
			r.flag("data-dt-r")
			r.b.WriteString(">")
			r.writeVideo(Media{URL: a.URL, Alt: a.Alt, Kind: MediaVideo})
			r.b.WriteString(`</discord-video-attachment>`)
		case MediaAudio:
			src := r.resolve(MediaRef{URL: a.URL, Kind: MediaAudio, Use: UseAttachment, Filename: a.Name})
			if src == "" {
				continue
			}
			r.b.WriteString(`<discord-audio-attachment data-dt-r><audio controls preload="metadata" src="` +
				escapeText(src) + `"></audio></discord-audio-attachment>`)
		case MediaFile:
			r.writeFileAttachment(a)
		default:
			r.b.WriteString(`<discord-image-attachment`)
			if a.Spoiler {
				r.flag("spoiler")
			}
			r.flag("data-dt-r")
			r.b.WriteString(">")
			r.writeImage(a)
			r.b.WriteString(`</discord-image-attachment>`)
		}
	}
	r.b.WriteString(`</discord-attachments>`)
}

func (r *renderer) writeFileAttachment(a Attachment) {
	size := a.Size
	if size == "" && a.SizeBytes > 0 {
		size = HumanSize(int(a.SizeBytes))
	}
	kind := strings.ToUpper(fileKind(a.Name))

	r.b.WriteString(`<discord-file-attachment data-dt-r`)
	r.attr("name", a.Name)
	if size != "" {
		number, unit := splitSize(size)
		r.attr("bytes", number)
		r.attr("bytes-unit", unit)
	}
	r.attr("type", kind)
	r.b.WriteString(">")
	if href := sanitizeURL(a.URL); href != "" {
		r.b.WriteString(`<a href="` + escapeText(href) + `" target="_blank" rel="noopener noreferrer">`)
		r.b.WriteString(`<span class="dt-file-icon">` + escapeText(kind) + `</span>`)
		r.b.WriteString(`<span class="dt-file-meta"><span class="dt-file-name">` + escapeText(a.Name) + `</span>`)
		if size != "" {
			r.b.WriteString(`<span class="dt-file-size">` + escapeText(size) + `</span>`)
		}
		r.b.WriteString(`</span></a>`)
	}
	r.b.WriteString(`</discord-file-attachment>`)
}

// writeActionRows draws the interactive components under a message. A button
// carries everything the stylesheet needs — its style, its label and its emoji —
// so the row reads correctly with no script at all.
//
// A link button is wrapped in an anchor, because a light-DOM custom element
// cannot navigate on its own; every other button is deliberately inert, since a
// bot's custom_id means nothing to a reader.
func (r *renderer) writeActionRows(rows []ActionRow) {
	for _, row := range rows {
		opened := false
		for _, button := range row.Buttons {
			if button.IsEmpty() {
				continue
			}
			if !opened {
				r.b.WriteString(`<discord-action-row data-dt-r>`)
				opened = true
			}
			r.writeButton(button)
		}
		if opened {
			r.b.WriteString(`</discord-action-row>`)
		}
	}
}

func (r *renderer) writeButton(b Button) {
	href := ""
	if b.IsLink() {
		href = sanitizeURL(b.URL)
	}
	if href != "" {
		r.b.WriteString(`<a class="dt-button-link" href="` + escapeText(href) +
			`" target="_blank" rel="noopener noreferrer">`)
	}
	r.b.WriteString(`<discord-button data-dt-r`)
	// Secondary is what the stylesheet draws by default, so it stays implicit.
	if b.Style != "" && b.Style != ButtonSecondary {
		r.attr("type", string(b.Style))
	}
	if b.Disabled {
		r.flag("disabled")
	}
	r.b.WriteString(">")
	if b.EmojiURL != "" {
		src := r.resolve(MediaRef{
			URL: b.EmojiURL, Kind: MediaEmoji, Use: UseEmoji,
			Filename: b.EmojiName, TargetEdge: r.targetEdge("emoji"),
		})
		if src != "" {
			alt := b.EmojiName
			if alt == "" {
				alt = "emoji"
			}
			r.b.WriteString(`<img class="dt-button-emoji" src="` + escapeText(src) +
				`" alt="` + escapeText(alt) + `" loading="lazy" decoding="async">`)
		}
	} else if b.Emoji != "" {
		// A unicode emoji is just text; the flex gap spaces it from the label.
		r.b.WriteString(escapeText(b.Emoji))
	}
	r.b.WriteString(escapeText(b.Label) + `</discord-button>`)
	if href != "" {
		r.b.WriteString(`</a>`)
	}
}

func (r *renderer) writeReactions(list []Reaction) {
	r.b.WriteString(`<discord-reactions>`)
	for _, reaction := range list {
		r.b.WriteString(`<discord-reaction data-dt-r`)
		if reaction.Reacted {
			r.flag("reacted")
		}
		r.b.WriteString(">")
		emoji := reaction.Emoji
		switch {
		case emoji == "":
			// nothing to draw
		case isRemote(emoji) || strings.HasPrefix(emoji, "data:"):
			src := emoji
			if isRemote(emoji) {
				src = r.resolve(MediaRef{
					URL: emoji, Kind: MediaEmoji, Use: UseEmoji,
					Filename: reaction.Name, TargetEdge: r.targetEdge("emoji"),
				})
			}
			alt := reaction.Name
			if alt == "" {
				alt = "emoji"
			}
			r.b.WriteString(`<img class="dt-reaction-emoji" src="` + escapeText(src) + `" alt="` + escapeText(alt) + `" loading="lazy" decoding="async">`)
		default:
			r.b.WriteString(`<span class="dt-reaction-emoji">` + escapeText(emoji) + `</span>`)
		}
		if reaction.Count > 1 {
			r.b.WriteString(`<span class="dt-reaction-count">` + itoa(reaction.Count) + `</span>`)
		}
		r.b.WriteString(`</discord-reaction>`)
	}
	r.b.WriteString(`</discord-reactions>`)
}

func (r *renderer) writeThread(t Thread) {
	cta := t.CTA
	if cta == "" {
		cta = "See thread"
	}
	r.b.WriteString(`<discord-thread`)
	r.attr("name", t.Name)
	r.attr("cta", cta)
	r.b.WriteString(">")
	for _, m := range t.Message {
		r.b.WriteString(`<discord-thread-message`)
		r.attr("author", m.Author.Name)
		r.attr("relative-timestamp", relativeLabel(m.Timestamp, time.Now()))
		r.b.WriteString(">")
		r.writeNodes(inlineOnly(m.Content))
		r.b.WriteString(`</discord-thread-message>`)
	}
	r.b.WriteString(`</discord-thread>`)
}

// ---------------------------------------------------------------- content

func (r *renderer) writeNodes(nodes []Node) {
	for _, n := range nodes {
		r.writeNode(n)
	}
}

func (r *renderer) writeNode(n Node) {
	switch n.Kind {
	case NodeText:
		r.writeText(n.Text)
	case NodeLineBreak:
		r.b.WriteString("<br>")
	case NodeBold:
		r.wrap("discord-bold", n.Children)
	case NodeItalic:
		r.wrap("discord-italic", n.Children)
	case NodeUnderline:
		r.wrap("discord-underlined", n.Children)
	case NodeStrikethrough:
		r.wrap("discord-strikethrough", n.Children)
	case NodeSpoiler:
		// No data-dt-r: the script adds click-to-reveal, the stylesheet
		// already hides it.
		r.wrap("discord-spoiler", n.Children)
	case NodeSubtext:
		r.wrap("discord-subscript", n.Children)
	case NodeQuote:
		r.wrap("discord-quote", n.Children)
	case NodeCode:
		r.b.WriteString("<discord-code>" + escapeText(n.Text) + "</discord-code>")
	case NodeCodeBlock:
		r.b.WriteString("<discord-pre>")
		if n.Language != "" {
			r.b.WriteString(`<span class="dt-code-lang">` + escapeText(n.Language) + `</span>`)
		}
		r.b.WriteString("<discord-code>" + escapeText(n.Text) + "</discord-code></discord-pre>")
	case NodeHeading:
		r.b.WriteString(`<discord-header`)
		r.attr("level", itoa(n.Level))
		r.b.WriteString(">")
		r.writeNodes(n.Children)
		r.b.WriteString(`</discord-header>`)
	case NodeList:
		if n.Ordered {
			start := n.Start
			if start < 1 {
				start = 1
			}
			r.b.WriteString(`<discord-ordered-list`)
			r.attr("start", itoa(start))
			r.b.WriteString(">")
			r.writeNodes(n.Children)
			r.b.WriteString(`</discord-ordered-list>`)
		} else {
			r.b.WriteString(`<discord-unordered-list>`)
			r.writeNodes(n.Children)
			r.b.WriteString(`</discord-unordered-list>`)
		}
	case NodeListItem:
		r.b.WriteString(`<discord-list-item>`)
		r.writeNodes(n.Children)
		r.b.WriteString(`</discord-list-item>`)
	case NodeLink:
		href := sanitizeURL(n.URL)
		if href == "" {
			r.writeNodes(n.Children)
			return
		}
		r.b.WriteString(`<discord-link href="` + escapeText(href) + `" target="_blank" rel="noopener noreferrer">`)
		r.writeNodes(n.Children)
		r.b.WriteString(`</discord-link>`)
	case NodeMention:
		r.writeMention(n)
	case NodeEmoji:
		// The image is emitted directly, so the emoji renders with the
		// stylesheet alone and the script has nothing to build.
		src := r.resolve(MediaRef{
			URL: n.EmojiURL, Kind: MediaEmoji, Use: UseEmoji,
			Filename: n.Text, TargetEdge: r.targetEdge("emoji"),
		})
		r.b.WriteString(`<discord-custom-emoji`)
		r.attr("name", n.Text)
		r.b.WriteString(">")
		if src != "" {
			r.b.WriteString(`<img src="` + escapeText(src) + `" alt="` + escapeText(n.Text) + `" loading="lazy" decoding="async">`)
		}
		r.b.WriteString(`</discord-custom-emoji>`)
	case NodeTimestamp:
		// No data-dt-r: with the script the text becomes the reader's local
		// format, without it the absolute text below stands on its own.
		r.b.WriteString(`<discord-time`)
		r.attr("timestamp", r.iso(n.Timestamp))
		if n.Format != 0 {
			r.attr("format", string(n.Format))
		}
		r.b.WriteString(">" + escapeText(r.inlineStamp(n.Timestamp, n.Format)) + `</discord-time>`)
	}
}

// writeText emits literal text, converting newlines into hard breaks. Producers
// that hand over a text node with newlines rather than parsing it into nodes
// still get the line breaks a reader expects; code blocks keep theirs verbatim
// because they travel as their own node kind.
func (r *renderer) writeText(text string) {
	if !strings.ContainsAny(text, "\r\n") {
		r.b.WriteString(escapeText(text))
		return
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if i > 0 {
			r.b.WriteString("<br>")
		}
		r.b.WriteString(escapeText(strings.TrimSuffix(line, "\r")))
	}
}

func (r *renderer) wrap(tag string, children []Node) {
	r.b.WriteString("<" + tag + ">")
	r.writeNodes(children)
	r.b.WriteString("</" + tag + ">")
}

func (r *renderer) writeMention(n Node) {
	mentionType := n.MentionType
	if mentionType == "" {
		mentionType = MentionUser
	}
	r.b.WriteString("<discord-mention")
	r.attr("type", string(mentionType))
	if mentionType == MentionRole && n.RoleColor != "" {
		r.b.WriteString(` style="--dt-mention-role-color: ` + escapeText(n.RoleColor) + `"`)
	}
	r.b.WriteString(">" + escapeText(n.Text) + "</discord-mention>")
}

// ------------------------------------------------------------------- media

func (r *renderer) writeMediaImg(m Media, class string) {
	src := r.resolve(MediaRef{URL: m.URL, Kind: m.Kind, Alt: m.Alt, Use: UseEmbedImage})
	if src == "" {
		return
	}
	r.b.WriteString(`<img src="` + escapeText(src) + `"`)
	alt := m.Alt
	if alt == "" {
		alt = ""
	}
	r.b.WriteString(` alt="` + escapeText(alt) + `"`)
	if class != "" {
		r.b.WriteString(` class="` + class + `"`)
	}
	r.b.WriteString(` loading="lazy" decoding="async">`)
}

// writeIcon writes a decorative image — an avatar, an embed author or footer
// icon, a thumbnail — at the size it is drawn, and marks it so the media pool
// can hoist it when the same bytes appear again elsewhere in the document.
//
// The marker is only worth carrying for inline media: a shared URL is already
// fetched once by the browser.
func (r *renderer) writeIcon(ref MediaRef, role string) {
	ref.TargetEdge = r.targetEdge(role)
	ref.Use = useForRole(role)
	src := r.resolve(ref)
	if src == "" {
		return
	}
	r.b.WriteString(`<img src="` + escapeText(src) + `" alt="` + escapeText(ref.Alt) + `" loading="lazy" decoding="async"`)
	if r.o.MediaPool && strings.HasPrefix(src, "data:image/") {
		r.attr("data-dt-media", role)
	}
	r.b.WriteString(">")
}

// useForRole maps a decorative role onto the media use it represents. An embed
// author or footer icon is a user avatar wearing a different hat, and the CDN
// treats it the same way.
func useForRole(role string) MediaUse {
	if role == "thumbnail" {
		return UseThumbnail
	}
	return UseAvatar
}

// targetEdge is the longest edge an image with this role is worth keeping, or 0
// when the caller asked for the original bytes.
func (r *renderer) targetEdge(role string) int {
	if !r.o.MediaDownscale {
		return 0
	}
	if role == "thumbnail" {
		return edgeThumbnail
	}
	return edgeIcon
}

func (r *renderer) writeImage(a Attachment) {
	src := r.resolve(MediaRef{URL: a.URL, Kind: MediaImage, Use: UseAttachment, Filename: a.Name})
	if src == "" {
		return
	}
	alt := a.Alt
	if alt == "" {
		alt = a.Name
	}
	r.b.WriteString(`<img src="` + escapeText(src) + `" alt="` + escapeText(alt) + `"`)
	if a.Width > 0 {
		r.b.WriteString(` style="max-width:min(100%,` + itoa(a.Width) + `px)"`)
	}
	r.b.WriteString(` loading="lazy" decoding="async">`)
}

func (r *renderer) writeVideo(m Media) {
	src := r.resolve(MediaRef{URL: m.URL, Kind: MediaVideo, Use: UseAttachment, Filename: m.Alt})
	if src == "" {
		return
	}
	r.b.WriteString(`<video controls preload="metadata" playsinline src="` + escapeText(src) + `"></video>`)
}

// resolve runs a media reference through the configured store, falling back to
// the original URL so a failed download never loses the content.
func (r *renderer) resolve(ref MediaRef) string {
	if ref.URL == "" {
		return ""
	}
	// Ask the origin for the size we need before anyone fetches it. The store —
	// or the proxy sitting behind the store's fetcher — then downloads the small
	// copy instead of the 1024px original, and a store that ignores the query is
	// corrected below.
	ref.URL = discordSizedURL(ref.URL, ref.TargetEdge)
	if r.o.LinkedMedia[ref.Use] {
		// Nothing is downloaded: the document points at wherever it already lives.
		return ref.URL
	}
	if r.o.Media == nil {
		return ref.URL
	}
	out, err := r.o.Media.Store(r.ctx, ref)
	if err != nil {
		r.o.warn(err)
		return ref.URL
	}
	if out == "" {
		return ref.URL
	}
	return r.rightSize(out, ref.TargetEdge)
}

// rightSize downscales an image that came back inline, whichever store produced
// it. A custom MediaStore therefore gets right-sized media without having to
// know that MediaRef.TargetEdge exists — the pipeline does it on the way out.
func (r *renderer) rightSize(uri string, edge int) string {
	if edge <= 0 || !strings.HasPrefix(uri, "data:image/") {
		// A file path or a remote URL carries no bytes to re-encode here.
		return uri
	}
	key := uri + "|" + itoa(edge)
	if r.mediaMemo == nil {
		r.mediaMemo = make(map[string]string, 8)
	}
	if cached, ok := r.mediaMemo[key]; ok {
		return cached
	}
	out := shrinkDataURI(uri, edge)
	r.mediaMemo[key] = out
	return out
}

// ------------------------------------------------------------------ attrs

func (r *renderer) attr(name, value string) {
	if value == "" {
		return
	}
	r.b.WriteString(" " + name + `="` + escapeText(value) + `"`)
}

func (r *renderer) flag(name string) { r.b.WriteString(" " + name) }

// ------------------------------------------------------------------ stamps

func (r *renderer) loc() *time.Location {
	if r.o.TimeZone == nil {
		return time.UTC
	}
	return r.o.TimeZone
}

func (r *renderer) iso(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// headerStamp is the text shown next to the author name, in the shape Discord
// uses: the time alone for today, "Yesterday at <time>" for yesterday, and the
// short date and time before that.
//
// It is computed when the transcript is written, so a document generated today
// says "11:49PM" for a message from today even when it is read years later. That
// is inherent to the format; the enhancement script recomputes it for the
// markup it builds itself, but not for rows this renderer marks data-dt-r.
func (r *renderer) headerStamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	local := t.In(r.loc())
	now := time.Now().In(r.loc())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, r.loc())
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, r.loc())
	switch {
	case day.Equal(today):
		return local.Format(compactTimeLayout)
	case day.Equal(today.AddDate(0, 0, -1)):
		return "Yesterday at " + local.Format(compactTimeLayout)
	default:
		return local.Format(shortDateTimeLayout)
	}
}

// shortStamp is the gutter time shown on hover for a continuation row.
func (r *renderer) shortStamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(r.loc()).Format(compactTimeLayout)
}

func (r *renderer) fullStamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(r.loc()).Format("Mon, 02 Jan 2006 15:04:05 -0700")
}

// The two timestamp shapes Discord uses. Message headers drop the space before
// AM/PM ("11:49PM"); every other stamp keeps it ("3/14/26, 11:57 AM").
const (
	compactTimeLayout   = "3:04PM"
	shortTimeLayout     = "3:04 PM"
	shortTimeSecsLayout = "3:04:05 PM"
	shortDateLayout     = "1/2/06"
	longDateLayout      = "January 2, 2006"
	shortDateTimeLayout = "1/2/06, 3:04 PM"
	shortDateSecsLayout = "1/2/06, 3:04:05 PM"
	fullDateTimeLayout  = "Monday, January 2, 2006 at 3:04 PM"
)

// inlineStamp is the text inside a <discord-time>, used until (or instead of)
// the enhancement script rewriting it in the reader's locale. These are the
// formats Discord's client shows for each of the <t:...> flags.
func (r *renderer) inlineStamp(t time.Time, format byte) string {
	if t.IsZero() {
		return ""
	}
	local := t.In(r.loc())
	switch format {
	case 't':
		return local.Format(shortTimeLayout)
	case 'T':
		return local.Format(shortTimeSecsLayout)
	case 'd':
		return local.Format(shortDateLayout)
	case 'D':
		return local.Format(longDateLayout)
	case 'F':
		return local.Format(fullDateTimeLayout)
	case 's':
		return local.Format(shortDateTimeLayout)
	case 'S':
		return local.Format(shortDateSecsLayout)
	default:
		// Includes 'f' and 'R': both fall back to the absolute short date and
		// time, which is what a script-less reader sees.
		return local.Format(shortDateTimeLayout)
	}
}

// ------------------------------------------------------------------ helpers

var initialsPalette = []string{
	"#5865f2", "#3ba55c", "#faa81a", "#ed4245",
	"#eb459e", "#00a8fc", "#9b59b6", "#1abc9c",
}

// initialsColor picks the placeholder colour for an author without an avatar.
// It matches the enhancement script so both paths look identical.
func initialsColor(name string) string {
	if name == "" {
		name = "?"
	}
	sum := 0
	for _, unit := range utf16.Encode([]rune(name)) {
		sum = (sum + int(unit)) % 997
	}
	return initialsPalette[sum%len(initialsPalette)]
}

func initial(name string) string {
	for _, r := range name {
		return string(r)
	}
	return "?"
}

// inlineOnly unwraps block-level nodes for the places that render one line of
// preview text rather than a document: a system message, a reply's quoted
// message and a thread preview. Embeds are not among them — a description or a
// field value keeps its headings, lists, quotes and code blocks.
func inlineOnly(nodes []Node) []Node {
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		switch n.Kind {
		case NodeQuote, NodeList, NodeHeading:
			out = append(out, n.Children...)
		case NodeCodeBlock:
			// Unwrapping would take the text with the wrapper, because a code
			// block carries its body in Text, not in Children. Keep the body as
			// plain text so a preview never drops what was said.
			if n.Text != "" {
				out = append(out, Text(n.Text))
			}
		default:
			out = append(out, n)
		}
	}
	return out
}

func relativeLabel(t time.Time, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	delta := now.Sub(t)
	switch {
	case delta < time.Minute:
		return "just now"
	case delta < time.Hour:
		return itoa(int(delta.Minutes())) + "m ago"
	case delta < 24*time.Hour:
		return itoa(int(delta.Hours())) + "h ago"
	default:
		return itoa(int(delta.Hours()/24)) + "d ago"
	}
}

func fileKind(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 && i < len(name)-1 {
		ext := name[i+1:]
		if len(ext) <= 4 {
			return ext
		}
		return ext[:4]
	}
	return "file"
}

func splitSize(size string) (string, string) {
	if i := strings.IndexByte(size, ' '); i > 0 {
		return size[:i], size[i+1:]
	}
	return size, ""
}
