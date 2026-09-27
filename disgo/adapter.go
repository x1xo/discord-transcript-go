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

// Options configures an Adapter. The zero value is usable: messages render with
// whatever identity they carry, and Transcript resolves mentions against the
// whole export, so a mention only keeps its raw ID when nothing in the export
// knows the name.
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
	// GuildID is the guild the messages belong to. It is only needed when the
	// messages do not carry their own guild: a message fetched over REST has
	// neither guild_id nor member, so without this the adapter cannot ask the
	// cache for a nickname, a guild avatar or a role colour. Adapter.Channel
	// fills it in from any guild channel, so this is for callers who build the
	// channel themselves.
	GuildID snowflake.ID
	// SelfUserID marks messages that mention this user as highlighted.
	SelfUserID snowflake.ID
	// SkipReplies renders messages without their "replying to" preview.
	SkipReplies bool
	// MemberFetcher supplies a guild member the caches do not hold. disgo fills
	// the member cache from GUILD_CREATE and from member, voice and chunk
	// events, so in a large guild most authors are simply absent — and without
	// a member there is no nickname, no guild avatar and no role colour. Point
	// it at Rest.GetMember and each unknown author is looked up once.
	MemberFetcher MemberFetcher
}

// MemberFetcher looks a guild member up outside the caches. Returning false
// leaves the author with the identity the message itself carries.
//
// It is called at most once per distinct author per transcript, and only for
// message authors — never for a mention, which would be unbounded.
type MemberFetcher func(guildID, userID snowflake.ID) (discord.Member, bool)

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

// WithGuildID names the guild the messages belong to, for messages that do not
// carry it themselves (anything fetched over REST). Adapter.Channel already
// fills this in from a guild channel, so this is the escape hatch for a channel
// object that is not one.
func WithGuildID(id snowflake.ID) Option { return func(o *Options) { o.GuildID = id } }

// WithSelfUserID marks messages mentioning this user as highlighted.
func WithSelfUserID(id snowflake.ID) Option { return func(o *Options) { o.SelfUserID = id } }

// WithMemberFetcher installs a fallback for authors the caches do not hold, so
// a transcript from a large guild still draws nicknames and role colours.
//
//	adapter := disgo.New(
//		disgo.WithCaches(client.Caches),
//		disgo.WithGuildID(guildID),
//		disgo.WithMemberFetcher(func(guildID, userID snowflake.ID) (discord.Member, bool) {
//			member, err := client.Rest.GetMember(guildID, userID)
//			if err != nil || member == nil {
//				return discord.Member{}, false
//			}
//			return *member, true
//		}),
//	)
func WithMemberFetcher(fetch MemberFetcher) Option {
	return func(o *Options) { o.MemberFetcher = fetch }
}

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
	// One pass to learn every identity the export carries, then a pass per
	// message, so a mention resolves even when only some other message knows the
	// name. See transcriptIndex.
	idx := newTranscriptIndex(channel, messages)
	// A message fetched over REST carries no guild, so the channel (or the
	// explicit option) is the only thing that can name it. Without a guild the
	// cache cannot be asked for members or roles at all.
	if idx.guildID == 0 {
		if id, err := snowflake.Parse(channel.GuildID); err == nil {
			idx.guildID = id
		}
	}
	if idx.guildID == 0 {
		idx.guildID = a.opts.GuildID
	}
	for _, m := range messages {
		tr.Messages = append(tr.Messages, a.message(m, idx))
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
	// A guild channel names its guild directly.
	if guild, ok := ch.(discord.GuildChannel); ok {
		out.GuildID = guildIDString(guild.GuildID())
	}
	// An interaction hands over a partial channel: it satisfies discord.Channel
	// but not discord.GuildChannel, so it names no guild — and an interaction is
	// exactly how a ticket transcript is usually triggered. The cache holds the
	// whole channel, so that is where the guild, and a missing name, come from.
	if a.opts.Caches != nil {
		if cached, ok := a.opts.Caches.Channel(ch.ID()); ok {
			if out.GuildID == "" {
				out.GuildID = guildIDString(cached.GuildID())
			}
			if out.Name == "" {
				out.Name = cached.Name()
			}
		}
	}
	return out
}

// guildIDString renders a guild snowflake, or "" when there is none, so a
// missing guild never travels as the string "0".
func guildIDString(id snowflake.ID) string {
	if id == 0 {
		return ""
	}
	return id.String()
}

// Message converts one disgo message. It never fails: anything it cannot resolve
// degrades to the raw value rather than losing the message.
//
// Identities come from this message alone. Transcript is the entry point that
// resolves mentions against the whole export, which is usually what a caller
// wants; this one exists for mapping a message in isolation. It still honours
// WithMemberFetcher, because a lone message has exactly the same problem with an
// uncached author.
func (a *Adapter) Message(m discord.Message) transcript.Message {
	return a.message(m, newTranscriptIndex(transcript.Channel{}, []discord.Message{m}))
}

func (a *Adapter) message(m discord.Message, shared *transcriptIndex) transcript.Message {
	res := a.resolversFor(m, shared)

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
	out.ActionRows = a.actionRows(m.Components)
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
	member, hasMember := res.authorMemberFor(m.Author.ID)
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
	author.AvatarURL = avatarURL(u)
	return author
}

// avatarURL returns the user's avatar, falling back to Discord's generated
// default. Both are empty for a partial payload with no discriminator, in which
// case the renderer draws a coloured initial instead.
func avatarURL(u discord.User) string {
	if url := u.EffectiveAvatarURL(); url != "" {
		return url
	}
	return u.DefaultAvatarURL()
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

// actionRows maps a message's interactive components onto the model. Only
// action rows of buttons are drawn: a select menu, or a components-v2 container,
// has no element in the stylesheet, and drawing one as something else would
// misrepresent what a reader could have pressed. A row that holds nothing else
// is therefore dropped rather than left as an empty gap.
func (a *Adapter) actionRows(components []discord.LayoutComponent) []transcript.ActionRow {
	var rows []transcript.ActionRow
	for _, component := range components {
		items := actionRowItems(component)
		if len(items) == 0 {
			continue
		}
		row := transcript.ActionRow{Buttons: make([]transcript.Button, 0, len(items))}
		for _, item := range items {
			button, ok := asButton(item)
			if !ok {
				continue
			}
			row.Buttons = append(row.Buttons, a.button(button))
		}
		if len(row.Buttons) > 0 {
			rows = append(rows, row)
		}
	}
	return rows
}

// actionRowItems unwraps an action row. Both a value and a pointer are accepted
// because disgo stores values when it unmarshals and callers may hand over
// either when they build a message by hand.
func actionRowItems(component discord.LayoutComponent) []discord.InteractiveComponent {
	switch row := component.(type) {
	case discord.ActionRowComponent:
		return row.Components
	case *discord.ActionRowComponent:
		return row.Components
	}
	return nil
}

func asButton(item discord.InteractiveComponent) (discord.ButtonComponent, bool) {
	switch button := item.(type) {
	case discord.ButtonComponent:
		return button, true
	case *discord.ButtonComponent:
		return *button, true
	}
	return discord.ButtonComponent{}, false
}

func (a *Adapter) button(b discord.ButtonComponent) transcript.Button {
	out := transcript.Button{
		Label:    b.Label,
		Style:    buttonStyle(b.Style),
		URL:      b.URL,
		Disabled: b.Disabled,
	}
	if b.Emoji == nil {
		return out
	}
	if b.Emoji.ID != 0 {
		out.EmojiURL = discord.Emoji{ID: b.Emoji.ID, Name: b.Emoji.Name, Animated: b.Emoji.Animated}.URL()
		out.EmojiName = ":" + b.Emoji.Name + ":"
	} else {
		out.Emoji = b.Emoji.Name
	}
	return out
}

func buttonStyle(style discord.ButtonStyle) transcript.ButtonStyle {
	switch style {
	case discord.ButtonStylePrimary:
		return transcript.ButtonPrimary
	case discord.ButtonStyleSuccess:
		return transcript.ButtonSuccess
	case discord.ButtonStyleDanger:
		return transcript.ButtonDanger
	case discord.ButtonStyleLink:
		return transcript.ButtonLink
	default:
		// Secondary, and the premium/SKU style the stylesheet has no colour for.
		return transcript.ButtonSecondary
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
	// the mention syntax in the text — the message's own text and, because Discord
	// does not parse embeds for the mentions array, every embed's text as well.
	id := a.opts.SelfUserID.String()
	for _, text := range mentionTexts(m) {
		if strings.Contains(text, "<@"+id+">") || strings.Contains(text, "<@!"+id+">") {
			return true
		}
	}
	return false
}

// mentionTexts is every string in a message that can carry a mention.
func mentionTexts(m discord.Message) []string {
	out := []string{m.Content}
	for _, e := range m.Embeds {
		out = append(out, e.Title, e.Description)
		if e.Author != nil {
			out = append(out, e.Author.Name)
		}
		if e.Footer != nil {
			out = append(out, e.Footer.Text)
		}
		for _, f := range e.Fields {
			out = append(out, f.Name, f.Value)
		}
	}
	return out
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
