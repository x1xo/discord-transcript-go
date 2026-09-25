package transcript

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// renderer walks the model and writes the HTML fragment.
//
// Profiles are collected on the way so the document can emit one config block
// for all authors instead of repeating avatars on every message.
type renderer struct {
	ctx      context.Context
	o        *Options
	b        strings.Builder
	profiles map[string]Author
	order    []string
}

// renderFragment renders the <discord-messages> element and the profile map that
// belongs with it.
func renderFragment(ctx context.Context, t *Transcript, o *Options) (string, map[string]Author, []string, error) {
	r := &renderer{ctx: ctx, o: o, profiles: make(map[string]Author)}
	r.writeMessages(t)
	return r.b.String(), r.profiles, r.order, nil
}

func (r *renderer) writeMessages(t *Transcript) {
	r.b.WriteString("<discord-messages")
	if t.Channel.Name != "" {
		r.b.WriteString(` channel-name="` + escapeText(t.Channel.Name) + `"`)
		if t.Channel.Type != "" {
			r.b.WriteString(` channel-type="` + escapeText(string(t.Channel.Type)) + `"`)
		}
	}
	if r.o.Theme == ThemeLight {
		r.b.WriteString(" light-theme")
	}
	r.b.WriteString(">\n")
	for _, m := range t.Messages {
		r.writeMessage(m)
	}
	r.b.WriteString("</discord-messages>")
}

func (r *renderer) writeMessage(m Message) {
	if m.System != nil {
		r.writeSystemMessage(m)
		return
	}

	profile := r.profileKey(m.Author)
	r.b.WriteString("\t<discord-message")
	r.b.WriteString(` profile="` + escapeText(profile) + `"`)
	// author is repeated so a script-less view still attributes the message.
	if m.Author.Name != "" {
		r.b.WriteString(` author="` + escapeText(m.Author.Name) + `"`)
	}
	if !m.Timestamp.IsZero() {
		r.b.WriteString(` timestamp="` + m.Timestamp.UTC().Format(time.RFC3339) + `"`)
	}
	if m.Edited {
		r.b.WriteString(" edited")
	}
	if m.Highlight {
		r.b.WriteString(" highlight")
	}
	if m.Ephemeral {
		r.b.WriteString(" ephemeral")
	}
	r.b.WriteString(">\n")

	if m.Reply != nil {
		r.writeReply(*m.Reply)
	}
	r.writeNodes(m.Content, 2)

	for _, e := range m.Embeds {
		r.writeEmbed(e, 2)
	}
	if len(m.Attachments) > 0 {
		r.writeAttachments(m.Attachments, 2)
	}
	if len(m.Reactions) > 0 {
		r.writeReactions(m.Reactions, 2)
	}
	if m.Thread != nil {
		r.writeThread(*m.Thread, 2)
	}

	r.newlineIfNeeded()
	r.b.WriteString("\t</discord-message>\n")
}

func (r *renderer) writeSystemMessage(m Message) {
	r.b.WriteString("\t<discord-system-message")
	r.b.WriteString(` type="` + escapeText(string(m.System.Type)) + `"`)
	if !m.Timestamp.IsZero() {
		r.b.WriteString(` timestamp="` + m.Timestamp.UTC().Format(time.RFC3339) + `"`)
	}
	r.b.WriteString(">")
	r.writeNodes(inlineOnly(m.System.Content), 0)
	r.b.WriteString("</discord-system-message>\n")
}

func (r *renderer) writeReply(reply Reply) {
	r.b.WriteString("\t\t<discord-reply")
	if !reply.Author.isZero() {
		r.b.WriteString(` profile="` + escapeText(r.profileKey(reply.Author)) + `"`)
		if reply.Author.Name != "" {
			r.b.WriteString(` author="` + escapeText(reply.Author.Name) + `"`)
		}
	}
	if reply.Mentions {
		r.b.WriteString(" mentions")
	}
	if reply.Deleted {
		r.b.WriteString(" deleted></discord-reply>\n")
		return
	}
	r.b.WriteString(">")
	r.writeNodes(inlineOnly(reply.Content), 0)
	r.b.WriteString("</discord-reply>\n")
}

func (r *renderer) writeEmbed(e Embed, depth int) {
	pad := strings.Repeat("\t", depth)
	r.b.WriteString(pad + "<discord-embed")
	if e.Color != "" {
		r.b.WriteString(` color="` + escapeText(e.Color) + `"`)
	}
	r.b.WriteString(">\n")

	if e.Provider != "" {
		r.b.WriteString(pad + "\t" + `<span class="dt-embed-provider">` + escapeText(e.Provider) + "</span>\n")
	}
	if e.Author != nil {
		r.b.WriteString(pad + "\t" + `<span class="dt-embed-author">`)
		if e.Author.Icon != nil {
			r.writeMediaImg(*e.Author.Icon, "", "dt-embed-author-icon")
		}
		name := escapeText(e.Author.Name)
		if href := sanitizeURL(e.Author.URL); href != "" {
			r.b.WriteString(`<a href="` + escapeText(href) + `" target="_blank" rel="noopener noreferrer">` + name + "</a>")
		} else {
			r.b.WriteString(name)
		}
		r.b.WriteString("</span>\n")
	}
	if e.Title != "" {
		title := escapeText(e.Title)
		if href := sanitizeURL(e.URL); href != "" {
			r.b.WriteString(pad + "\t" + `<a class="dt-embed-title" href="` + escapeText(href) + `" target="_blank" rel="noopener noreferrer">` + title + "</a>\n")
		} else {
			r.b.WriteString(pad + "\t" + `<span class="dt-embed-title">` + title + "</span>\n")
		}
	}
	if len(e.Description) > 0 {
		r.b.WriteString(pad + "\t<discord-embed-description>")
		r.writeNodes(inlineOnly(e.Description), 0)
		r.b.WriteString("</discord-embed-description>\n")
	}
	if len(e.Fields) > 0 {
		r.b.WriteString(pad + "\t<discord-embed-fields>\n")
		for _, f := range e.Fields {
			r.b.WriteString(pad + "\t\t<discord-embed-field")
			if f.Name != "" {
				r.b.WriteString(` field-title="` + escapeText(f.Name) + `"`)
			}
			if f.Inline {
				r.b.WriteString(" inline")
			}
			r.b.WriteString(">")
			r.writeNodes(inlineOnly(f.Value), 0)
			r.b.WriteString("</discord-embed-field>\n")
		}
		r.b.WriteString(pad + "\t</discord-embed-fields>\n")
	}
	if e.Image != nil {
		r.b.WriteString(pad + "\t" + `<div class="dt-embed-image">`)
		r.writeMediaImg(*e.Image, "", "")
		r.b.WriteString("</div>\n")
	}
	if e.Video != nil {
		r.b.WriteString(pad + "\t" + `<div class="dt-embed-image">`)
		r.writeVideo(*e.Video, "")
		r.b.WriteString("</div>\n")
	}
	if e.Thumbnail != nil {
		r.b.WriteString(pad + "\t" + `<div class="dt-embed-thumbnail">`)
		r.writeMediaImg(*e.Thumbnail, "", "")
		r.b.WriteString("</div>\n")
	}
	if e.Footer != nil {
		r.b.WriteString(pad + "\t<discord-embed-footer>")
		if e.Footer.Icon != nil {
			r.b.WriteString(`<span class="dt-embed-footer-icon">`)
			r.writeMediaImg(*e.Footer.Icon, "", "")
			r.b.WriteString("</span>")
		}
		if e.Footer.Text != "" {
			r.b.WriteString(escapeText(e.Footer.Text))
		}
		if !e.Footer.Timestamp.IsZero() {
			r.b.WriteString(`<span class="dt-embed-footer-sep"></span>`)
			r.b.WriteString(escapeText(e.Footer.Timestamp.Format("2006-01-02 15:04")))
		}
		r.b.WriteString("</discord-embed-footer>\n")
	}
	r.b.WriteString(pad + "</discord-embed>\n")
}

func (r *renderer) writeAttachments(list []Attachment, depth int) {
	pad := strings.Repeat("\t", depth)
	r.b.WriteString(pad + "<discord-attachments>\n")
	for _, a := range list {
		switch a.Kind {
		case MediaVideo:
			r.b.WriteString(pad + "\t<discord-video-attachment")
			if a.Spoiler {
				r.b.WriteString(" spoiler")
			}
			r.b.WriteString(">")
			r.writeVideo(Media{URL: a.URL, Alt: a.Alt, Kind: MediaVideo}, "")
			r.b.WriteString("</discord-video-attachment>\n")
		case MediaAudio:
			src := r.resolve(MediaRef{URL: a.URL, Kind: MediaAudio, Filename: a.Name})
			r.b.WriteString(pad + "\t<discord-audio-attachment>")
			if src != "" {
				r.b.WriteString(`<audio controls preload="metadata" src="` + escapeText(src) + `"></audio>`)
			}
			r.b.WriteString("</discord-audio-attachment>\n")
		case MediaFile:
			r.writeFileAttachment(a, depth+1)
		default:
			r.b.WriteString(pad + "\t<discord-image-attachment")
			if a.Spoiler {
				r.b.WriteString(" spoiler")
			}
			r.b.WriteString(">")
			r.writeImage(Media{URL: a.URL, Alt: a.Alt, Width: a.Width, Height: a.Height, Kind: MediaImage}, "")
			r.b.WriteString("</discord-image-attachment>\n")
		}
	}
	r.b.WriteString(pad + "</discord-attachments>\n")
}

func (r *renderer) writeFileAttachment(a Attachment, depth int) {
	pad := strings.Repeat("\t", depth)
	size := a.Size
	if size == "" && a.SizeBytes > 0 {
		size = HumanSize(int(a.SizeBytes))
	}
	kind := strings.ToUpper(fileKind(a.Name))
	r.b.WriteString(pad + "<discord-file-attachment")
	r.b.WriteString(` name="` + escapeText(a.Name) + `"`)
	if size != "" {
		number, unit := splitSize(size)
		r.b.WriteString(` bytes="` + escapeText(number) + `"`)
		if unit != "" {
			r.b.WriteString(` bytes-unit="` + escapeText(unit) + `"`)
		}
	}
	if kind != "" {
		r.b.WriteString(` type="` + escapeText(kind) + `"`)
	}
	r.b.WriteString(">\n")
	if href := sanitizeURL(a.URL); href != "" {
		r.b.WriteString(pad + "\t" + `<a href="` + escapeText(href) + `" target="_blank" rel="noopener noreferrer">` + "\n")
		r.b.WriteString(pad + "\t\t" + `<span class="dt-file-icon">` + escapeText(kind) + "</span>\n")
		r.b.WriteString(pad + "\t\t" + `<span class="dt-file-meta">` + "\n")
		r.b.WriteString(pad + "\t\t\t" + `<span class="dt-file-name">` + escapeText(a.Name) + "</span>\n")
		if size != "" {
			r.b.WriteString(pad + "\t\t\t" + `<span class="dt-file-size">` + escapeText(size) + "</span>\n")
		}
		r.b.WriteString(pad + "\t\t</span>\n")
		r.b.WriteString(pad + "\t</a>\n")
	}
	r.b.WriteString(pad + "</discord-file-attachment>\n")
}

func (r *renderer) writeReactions(list []Reaction, depth int) {
	pad := strings.Repeat("\t", depth)
	r.b.WriteString(pad + "<discord-reactions>\n")
	for _, reaction := range list {
		emoji := reaction.Emoji
		if isRemote(emoji) {
			emoji = r.resolve(MediaRef{URL: emoji, Kind: MediaEmoji, Filename: reaction.Name})
		}
		r.b.WriteString(pad + "\t<discord-reaction")
		r.b.WriteString(` emoji="` + escapeText(emoji) + `"`)
		if reaction.Name != "" {
			r.b.WriteString(` name="` + escapeText(reaction.Name) + `"`)
		}
		count := reaction.Count
		if count < 1 {
			count = 1
		}
		r.b.WriteString(` count="` + itoa(count) + `"`)
		if reaction.Reacted {
			r.b.WriteString(" reacted")
		}
		r.b.WriteString("></discord-reaction>\n")
	}
	r.b.WriteString(pad + "</discord-reactions>\n")
}

func (r *renderer) writeThread(t Thread, depth int) {
	pad := strings.Repeat("\t", depth)
	cta := t.CTA
	if cta == "" {
		cta = "See thread"
	}
	r.b.WriteString(pad + "<discord-thread")
	if t.Name != "" {
		r.b.WriteString(` name="` + escapeText(t.Name) + `"`)
	}
	r.b.WriteString(` cta="` + escapeText(cta) + `">` + "\n")
	for _, m := range t.Message {
		r.b.WriteString(pad + "\t<discord-thread-message")
		if m.Author.Name != "" {
			r.b.WriteString(` author="` + escapeText(m.Author.Name) + `"`)
		}
		if !m.Timestamp.IsZero() {
			r.b.WriteString(` relative-timestamp="` + escapeText(relativeLabel(m.Timestamp)) + `"`)
		}
		r.b.WriteString(">")
		r.writeNodes(inlineOnly(m.Content), 0)
		r.b.WriteString("</discord-thread-message>\n")
	}
	r.b.WriteString(pad + "</discord-thread>\n")
}

// ------------------------------------------------------------------ content

func (r *renderer) writeNodes(nodes []Node, depth int) {
	for _, n := range nodes {
		r.writeNode(n, depth)
	}
}

func (r *renderer) writeNode(n Node, depth int) {
	switch n.Kind {
	case NodeText:
		r.b.WriteString(escapeText(n.Text))
	case NodeLineBreak:
		r.b.WriteString("<br />")
	case NodeBold:
		r.wrap("discord-bold", n.Children, depth)
	case NodeItalic:
		r.wrap("discord-italic", n.Children, depth)
	case NodeUnderline:
		r.wrap("discord-underlined", n.Children, depth)
	case NodeStrikethrough:
		r.wrap("discord-strikethrough", n.Children, depth)
	case NodeSpoiler:
		r.wrap("discord-spoiler", n.Children, depth)
	case NodeSubtext:
		r.wrap("discord-subscript", n.Children, depth)
	case NodeQuote:
		r.wrap("discord-quote", n.Children, depth)
	case NodeCode:
		r.b.WriteString("<discord-code>" + escapeText(n.Text) + "</discord-code>")
	case NodeCodeBlock:
		r.b.WriteString("<discord-pre>")
		if n.Language != "" {
			r.b.WriteString(`<span class="dt-code-lang">` + escapeText(n.Language) + "</span>")
		}
		r.b.WriteString("<discord-code>" + escapeText(n.Text) + "</discord-code></discord-pre>")
	case NodeHeading:
		r.b.WriteString(`<discord-header level="` + itoa(n.Level) + `">`)
		r.writeNodes(n.Children, depth)
		r.b.WriteString("</discord-header>")
	case NodeList:
		if n.Ordered {
			start := n.Start
			if start < 1 {
				start = 1
			}
			r.b.WriteString(`<discord-ordered-list start="` + itoa(start) + `">`)
			r.writeNodes(n.Children, depth)
			r.b.WriteString("</discord-ordered-list>")
		} else {
			r.b.WriteString("<discord-unordered-list>")
			r.writeNodes(n.Children, depth)
			r.b.WriteString("</discord-unordered-list>")
		}
	case NodeListItem:
		r.b.WriteString("<discord-list-item>")
		r.writeNodes(n.Children, depth)
		r.b.WriteString("</discord-list-item>")
	case NodeLink:
		href := sanitizeURL(n.URL)
		if href == "" {
			r.writeNodes(n.Children, depth)
			return
		}
		r.b.WriteString(`<discord-link href="` + escapeText(href) + `" target="_blank" rel="noopener noreferrer">`)
		r.writeNodes(n.Children, depth)
		r.b.WriteString("</discord-link>")
	case NodeMention:
		r.writeMention(n)
	case NodeEmoji:
		url := r.resolve(MediaRef{URL: n.EmojiURL, Kind: MediaEmoji, Filename: n.Text})
		r.b.WriteString(`<discord-custom-emoji name="` + escapeText(n.Text) + `"`)
		if url != "" {
			r.b.WriteString(` url="` + escapeText(url) + `"`)
		}
		r.b.WriteString("></discord-custom-emoji>")
	case NodeTimestamp:
		fallback := fallbackTimestamp(n.Timestamp, n.Format, r.o.TimeZone)
		r.b.WriteString(`<discord-time timestamp="` + n.Timestamp.UTC().Format(time.RFC3339) + `"`)
		if n.Format != 0 {
			r.b.WriteString(` format="` + escapeText(string(n.Format)) + `"`)
		}
		r.b.WriteString(">" + escapeText(fallback) + "</discord-time>")
	}
}

// newlineIfNeeded terminates an inline run before a block-level closing tag, so
// the generated HTML stays readable instead of gluing a tab to the last word.
func (r *renderer) newlineIfNeeded() {
	s := r.b.String()
	if s == "" || strings.HasSuffix(s, "\n") {
		return
	}
	r.b.WriteString("\n")
}

func (r *renderer) wrap(tag string, children []Node, depth int) {
	r.b.WriteString("<" + tag + ">")
	r.writeNodes(children, depth)
	r.b.WriteString("</" + tag + ">")
}

func (r *renderer) writeMention(n Node) {
	mentionType := n.MentionType
	if mentionType == "" {
		mentionType = MentionUser
	}
	r.b.WriteString(`<discord-mention type="` + escapeText(string(mentionType)) + `"`)
	if mentionType == MentionRole && n.RoleColor != "" {
		r.b.WriteString(` style="--dt-mention-role-color: ` + escapeText(n.RoleColor) + `"`)
	}
	r.b.WriteString(">" + escapeText(n.Text) + "</discord-mention>")
}

// ------------------------------------------------------------------- media

func (r *renderer) writeMediaImg(m Media, class, wrapClass string) {
	src := r.resolve(MediaRef{URL: m.URL, Kind: m.Kind, Alt: m.Alt})
	if src == "" {
		return
	}
	if wrapClass != "" {
		r.b.WriteString(`<span class="` + wrapClass + `">`)
	}
	r.b.WriteString(`<img src="` + escapeText(src) + `"`)
	if m.Alt != "" {
		r.b.WriteString(` alt="` + escapeText(m.Alt) + `"`)
	} else {
		r.b.WriteString(` alt=""`)
	}
	if class != "" {
		r.b.WriteString(` class="` + class + `"`)
	}
	r.b.WriteString(` loading="lazy" decoding="async" />`)
	if wrapClass != "" {
		r.b.WriteString("</span>")
	}
}

// writeImage writes an attachment image, honouring per-image width limits in a
// responsive way.
func (r *renderer) writeImage(m Media, class string) {
	src := r.resolve(MediaRef{URL: m.URL, Kind: m.Kind, Filename: m.Alt})
	if src == "" {
		return
	}
	r.b.WriteString(`<img src="` + escapeText(src) + `"`)
	alt := m.Alt
	if alt == "" {
		alt = "attachment"
	}
	r.b.WriteString(` alt="` + escapeText(alt) + `"`)
	if class != "" {
		r.b.WriteString(` class="` + class + `"`)
	}
	if m.Width > 0 {
		r.b.WriteString(` style="max-width:min(100%,` + itoa(m.Width) + `px)"`)
	}
	r.b.WriteString(` loading="lazy" decoding="async" />`)
}

func (r *renderer) writeVideo(m Media, class string) {
	src := r.resolve(MediaRef{URL: m.URL, Kind: MediaVideo, Filename: m.Alt})
	if src == "" {
		return
	}
	r.b.WriteString(`<video controls preload="metadata" playsinline src="` + escapeText(src) + `"`)
	if class != "" {
		r.b.WriteString(` class="` + class + `"`)
	}
	r.b.WriteString("></video>")
}

// resolve runs a media reference through the configured store, falling back to
// the original URL so a failed download never loses the content.
func (r *renderer) resolve(ref MediaRef) string {
	if ref.URL == "" {
		return ""
	}
	if r.o.Media == nil {
		return ref.URL
	}
	out, err := r.o.Media.Store(r.ctx, ref)
	if err != nil {
		r.o.warn(fmt.Errorf("media %s: %w", ref.URL, err))
		return ref.URL
	}
	if out == "" {
		return ref.URL
	}
	return out
}

// profileKey registers an author in the profile map and returns its key.
func (r *renderer) profileKey(a Author) string {
	key := a.Key
	if key == "" {
		key = a.Name
	}
	if key == "" {
		return ""
	}
	if _, seen := r.profiles[key]; !seen {
		stored := a
		stored.Key = key
		if a.AvatarURL != "" {
			stored.AvatarURL = r.resolve(MediaRef{URL: a.AvatarURL, Kind: MediaAvatar, Filename: key})
		}
		r.profiles[key] = stored
		r.order = append(r.order, key)
	}
	return key
}

// ------------------------------------------------------------------ helpers

// inlineOnly drops block-level nodes, for places that only accept phrasing
// content (a system message body, an embed description).
func inlineOnly(nodes []Node) []Node {
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		switch n.Kind {
		case NodeQuote, NodeList, NodeCodeBlock, NodeHeading:
			out = append(out, n.Children...)
		default:
			out = append(out, n)
		}
	}
	return out
}

// fallbackTimestamp renders the text a script-less viewer sees. The enhancement
// script replaces it with the reader's own locale and time zone.
func fallbackTimestamp(t time.Time, format byte, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	t = t.In(loc)
	switch format {
	case 't':
		return t.Format("15:04")
	case 'T':
		return t.Format("15:04:05")
	case 'd':
		return t.Format("2006-01-02")
	case 'D':
		return t.Format("January 2, 2006")
	case 'F':
		return t.Format("Monday, January 2, 2006 15:04")
	case 'R', 's':
		return t.Format("2006-01-02 15:04")
	case 'S':
		return t.Format("2006-01-02 15:04:05")
	default:
		return t.Format("January 2, 2006 15:04")
	}
}

// relativeLabel produces the short "2m ago" style label used by thread previews.
func relativeLabel(t time.Time) string {
	delta := time.Since(t)
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

// fileKind guesses the short label shown in a file attachment's icon.
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

// splitSize splits "1.2 MB" into "1.2" and "MB".
func splitSize(size string) (string, string) {
	if i := strings.IndexByte(size, ' '); i > 0 {
		return size[:i], size[i+1:]
	}
	return size, ""
}
