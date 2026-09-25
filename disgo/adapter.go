// Package disgo adapts github.com/disgoorg/disgo messages into the transcript
// model, so a disgo bot can export a channel to a self-contained HTML file.
//
// The adapter is a thin translation layer: the heavy lifting (markdown, HTML,
// media) lives in github.com/x1xo/discord-transcript-go/transcript, which does
// not depend on disgo. Swapping Discord libraries therefore means writing
// another adapter, not touching the renderer.
package disgo

import (
	"strings"

	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/x1xo/discord-transcript-go/transcript"
)

// Options configures an Adapter. The zero value is usable: messages then render
// with whatever identity they carry, and mentions keep their raw IDs.
type Options struct {
	// Caches supplies nicknames, guild avatars and role colours. Usually
	// bot.Client.Caches. Optional, and only useful for the entity kinds the bot
	// actually caches: disgo's caches are no-ops unless they were created with
	// cache.FlagMembers / cache.FlagRoles (and friends). For offline exports,
	// prefer WithUsers/WithRoles, which need no cache at all.
	Caches cache.Caches
	// Users overrides author identity by user ID. Useful for offline exports of
	// raw API JSON, where no cache exists.
	Users transcript.Profiles
	// Channels overrides channel-name resolution by channel ID.
	Channels transcript.Channels
	// Roles overrides role name and colour by role ID.
	Roles transcript.Roles
	// SelfUserID marks messages that mention this user as highlighted.
	SelfUserID snowflake.ID
	// SkipReplies renders messages without their "replying to" preview.
	SkipReplies bool
}

// Option mutates Options.
type Option func(*Options)

// WithCaches resolves identity through disgo's caches.
func WithCaches(c cache.Caches) Option { return func(o *Options) { o.Caches = c } }

// WithUsers supplies author identities directly.
func WithUsers(users transcript.Profiles) Option { return func(o *Options) { o.Users = users } }

// WithChannels supplies channel names directly.
func WithChannels(channels transcript.Channels) Option {
	return func(o *Options) { o.Channels = channels }
}

// WithRoles supplies role names and colours directly.
func WithRoles(roles transcript.Roles) Option { return func(o *Options) { o.Roles = roles } }

// WithSelfUserID marks messages mentioning this user as highlighted.
func WithSelfUserID(id snowflake.ID) Option { return func(o *Options) { o.SelfUserID = id } }

// WithoutReplies drops the reply previews.
func WithoutReplies() Option { return func(o *Options) { o.SkipReplies = true } }

// Adapter converts disgo messages into the transcript model.
type Adapter struct {
	opts Options
}

// New returns an adapter. Options are optional.
func New(opts ...Option) *Adapter {
	o := Options{}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return &Adapter{opts: o}
}

// Options returns the adapter's configuration.
func (a *Adapter) Options() Options { return a.opts }

// Transcript converts a channel and its messages into a renderable transcript.
func (a *Adapter) Transcript(channel transcript.Channel, messages []discord.Message) *transcript.Transcript {
	tr := &transcript.Transcript{
		Channel:  channel,
		Messages: make([]transcript.Message, 0, len(messages)),
	}
	for _, m := range messages {
		tr.Messages = append(tr.Messages, a.Message(m))
	}
	return tr
}

// Channel derives the transcript channel header from a disgo channel.
func (a *Adapter) Channel(ch discord.Channel) transcript.Channel {
	out := transcript.Channel{
		ID:   ch.ID().String(),
		Type: channelType(ch.Type()),
	}
	if named, ok := ch.(interface{ Name() string }); ok {
		out.Name = named.Name()
	}
	if guild, ok := ch.(discord.GuildChannel); ok {
		if cached, ok := a.opts.Caches.Channel(ch.ID()); ok {
			if named, ok := cached.(interface{ Name() string }); ok && out.Name == "" {
				out.Name = named.Name()
			}
		}
		_ = guild
	}
	return out
}

// Message converts one disgo message. It never fails: anything it cannot
// resolve degrades to the raw value rather than losing the message.
func (a *Adapter) Message(m discord.Message) transcript.Message {
	res := a.resolversFor(m)

	out := transcript.Message{
		Timestamp: m.CreatedAt,
		Edited:    m.EditedTimestamp != nil,
		Ephemeral: m.Flags.Has(discord.MessageFlagEphemeral),
		Highlight: a.mentionsSelf(m),
	}

	if kind, ok := systemType(m.Type); ok {
		out.System = &transcript.SystemMessage{
			Type:    kind,
			Content: transcript.ParseContentWith(m.Content, res.bundle()),
		}
	} else {
		out.Author = a.authorFor(m, res)
		out.Content = transcript.ParseContentWith(m.Content, res.bundle())
	}

	if m.ReferencedMessage != nil && !a.opts.SkipReplies {
		ref := m.ReferencedMessage
		out.Reply = &transcript.Reply{
			Author:   a.authorFor(*ref, res),
			Content:  transcript.ParseContentWith(ref.Content, res.bundle()),
			Mentions: replyMentions(m, *ref),
			Deleted:  false,
		}
	}

	for _, e := range m.Embeds {
		out.Embeds = append(out.Embeds, a.embed(e, res))
	}
	for _, att := range m.Attachments {
		out.Attachments = append(out.Attachments, a.attachment(att))
	}
	for _, r := range m.Reactions {
		out.Reactions = append(out.Reactions, a.reaction(r))
	}
	if len(m.StickerItems) > 0 {
		// Stickers are out of scope for the current renderer. They are kept as
		// text so the transcript does not silently lose the fact one was posted.
		names := make([]string, 0, len(m.StickerItems))
		for _, s := range m.StickerItems {
			names = append(names, s.Name)
		}
		out.Content = append(out.Content, transcript.Text("\n[sticker: "+strings.Join(names, ", ")+"]"))
	}
	return out
}

// ---------------------------------------------------------------- mapping

func (a *Adapter) authorFor(m discord.Message, res *messageResolvers) transcript.Author {
	author := authorFromUser(m.Author)
	// The member may come from the payload or, more often for history fetches,
	// from disgo's cache; either way it carries the nickname and role colour.
	member, hasMember := res.memberFor(m.Author.ID)
	if m.Member != nil {
		member, hasMember = *m.Member, true
	}
	if hasMember {
		if color := res.roleColorFor(member); color != "" {
			author.RoleColor = color
		}
		if name := member.EffectiveName(); name != "" {
			author.Name = name
		}
		if avatar := member.EffectiveAvatarURL(); avatar != "" {
			author.AvatarURL = avatar
		}
	}
	// A user-supplied override always wins, so offline exports can correct names.
	if override, ok := res.overrideUser(author.Key); ok {
		merged := override
		if merged.Key == "" {
			merged.Key = author.Key
		}
		if merged.Name == "" {
			merged.Name = author.Name
		}
		if merged.AvatarURL == "" {
			merged.AvatarURL = author.AvatarURL
		}
		if merged.RoleColor == "" {
			merged.RoleColor = author.RoleColor
		}
		return merged
	}
	return author
}

func authorFromUser(u discord.User) transcript.Author {
	author := transcript.Author{
		Key:  u.ID.String(),
		Name: u.EffectiveName(),
		Bot:  u.Bot,
	}
	if author.Name == "" {
		author.Name = u.Username
	}
	if u.PublicFlags&discord.UserFlagVerifiedBot != 0 {
		author.Verified = true
	}
	if avatar := u.EffectiveAvatarURL(); avatar != "" {
		author.AvatarURL = avatar
	}
	return author
}

func (a *Adapter) embed(e discord.Embed, res *messageResolvers) transcript.Embed {
	out := transcript.Embed{
		Color:       transcript.HexColor(e.Color),
		URL:         e.URL,
		Title:       e.Title,
		Description: transcript.ParseContentWith(e.Description, res.bundle()),
	}
	if e.Provider != nil {
		out.Provider = e.Provider.Name
	}
	if e.Author != nil {
		out.Author = &transcript.EmbedAuthor{
			Name: e.Author.Name,
			URL:  e.Author.URL,
		}
		if e.Author.IconURL != "" {
			out.Author.Icon = &transcript.Media{URL: e.Author.IconURL, Kind: transcript.MediaImage}
		}
	}
	for _, f := range e.Fields {
		out.Fields = append(out.Fields, transcript.EmbedField{
			Name:   f.Name,
			Value:  transcript.ParseContentWith(f.Value, res.bundle()),
			Inline: f.Inline != nil && *f.Inline,
		})
	}
	if e.Footer != nil {
		out.Footer = &transcript.EmbedFooter{Text: e.Footer.Text}
		if e.Footer.IconURL != "" {
			out.Footer.Icon = &transcript.Media{URL: e.Footer.IconURL, Kind: transcript.MediaImage}
		}
		if e.Timestamp != nil {
			out.Footer.Timestamp = *e.Timestamp
		}
	}
	if e.Image != nil {
		out.Image = mediaFromResource(*e.Image, transcript.MediaImage)
	}
	if e.Video != nil {
		out.Video = mediaFromResource(*e.Video, transcript.MediaVideo)
	}
	if e.Thumbnail != nil {
		out.Thumbnail = mediaFromResource(*e.Thumbnail, transcript.MediaImage)
	}
	return out
}

func mediaFromResource(r discord.EmbedResource, kind transcript.MediaKind) *transcript.Media {
	if r.URL == "" {
		return nil
	}
	return &transcript.Media{URL: r.URL, Kind: kind, Width: r.Width, Height: r.Height}
}

func (a *Adapter) attachment(att discord.Attachment) transcript.Attachment {
	out := transcript.Attachment{
		Kind:      attachmentKind(att.ContentType),
		URL:       att.URL,
		Name:      att.Filename,
		SizeBytes: int64(att.Size),
		Spoiler:   att.Flags&discord.AttachmentFlagIsSpoiler != 0,
	}
	if att.Description != nil {
		out.Alt = *att.Description
	}
	if att.Width != nil {
		out.Width = *att.Width
	}
	if att.Height != nil {
		out.Height = *att.Height
	}
	if att.DurationSecs != nil {
		out.DurationSeconds = *att.DurationSecs
	}
	return out
}

func attachmentKind(contentType *string) transcript.MediaKind {
	if contentType == nil {
		return transcript.MediaFile
	}
	switch {
	case strings.HasPrefix(*contentType, "image/"):
		return transcript.MediaImage
	case strings.HasPrefix(*contentType, "video/"):
		return transcript.MediaVideo
	case strings.HasPrefix(*contentType, "audio/"):
		return transcript.MediaAudio
	default:
		return transcript.MediaFile
	}
}

func (a *Adapter) reaction(r discord.MessageReaction) transcript.Reaction {
	out := transcript.Reaction{Count: r.Count, Reacted: r.Me}
	if r.Emoji.ID != 0 {
		out.Emoji = r.Emoji.URL()
		out.Name = ":" + r.Emoji.Name + ":"
	} else {
		out.Emoji = r.Emoji.Name
	}
	return out
}

func (a *Adapter) mentionsSelf(m discord.Message) bool {
	if a.opts.SelfUserID == 0 {
		return false
	}
	if m.MentionEveryone {
		return true
	}
	for _, u := range m.Mentions {
		if u.ID == a.opts.SelfUserID {
			return true
		}
	}
	// Raw or partial payloads sometimes omit the mentions array, so fall back to
	// the mention syntax in the content.
	id := a.opts.SelfUserID.String()
	return strings.Contains(m.Content, "<@"+id+">") || strings.Contains(m.Content, "<@!"+id+">")
}

// replyMentions reports whether the reply content opens with a mention of the
// replied-to author, which is how Discord shows "@someone" in the preview.
func replyMentions(m discord.Message, ref discord.Message) bool {
	id := ref.Author.ID.String()
	return strings.HasPrefix(m.Content, "<@"+id+">") || strings.HasPrefix(m.Content, "<@!"+id+">")
}

// systemType maps Discord's message types onto the system rows the stylesheet
// renders. ok is false for ordinary messages.
func systemType(t discord.MessageType) (transcript.SystemType, bool) {
	switch t {
	case discord.MessageTypeRecipientAdd, discord.MessageTypeUserJoin:
		return transcript.SystemJoin, true
	case discord.MessageTypeRecipientRemove:
		return transcript.SystemLeave, true
	case discord.MessageTypeCall:
		return transcript.SystemCall, true
	case discord.MessageTypeChannelNameChange, discord.MessageTypeChannelIconChange:
		return transcript.SystemEdit, true
	case discord.MessageTypeChannelPinnedMessage:
		return transcript.SystemPin, true
	case discord.MessageTypeGuildBoost,
		discord.MessageTypeGuildBoostTier1,
		discord.MessageTypeGuildBoostTier2,
		discord.MessageTypeGuildBoostTier3:
		return transcript.SystemBoost, true
	case discord.MessageTypeThreadCreated:
		return transcript.SystemThread, true
	default:
		return "", false
	}
}

func channelType(t discord.ChannelType) transcript.ChannelType {
	switch t {
	case discord.ChannelTypeGuildVoice, discord.ChannelTypeGuildStageVoice:
		return transcript.ChannelVoice
	case discord.ChannelTypeGuildPublicThread,
		discord.ChannelTypeGuildPrivateThread,
		discord.ChannelTypeGuildNewsThread:
		return transcript.ChannelThread
	case discord.ChannelTypeGuildForum, discord.ChannelTypeGuildMedia:
		return transcript.ChannelForum
	default:
		return transcript.ChannelText
	}
}
