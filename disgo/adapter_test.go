package disgo

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/x1xo/discord-transcript-go/transcript"
)

func ptr[T any](v T) *T { return &v }

var (
	userID  = snowflake.ID(111111111111111111)
	otherID = snowflake.ID(222222222222222222)
	guildID = snowflake.ID(900000000000000001)
	roleID  = snowflake.ID(333333333333333333)
	chanID  = snowflake.ID(444444444444444444)
)

func baseTime() time.Time {
	t, _ := time.Parse(time.RFC3339, "2024-03-15T14:28:00Z")
	return t
}

func plainMessage() discord.Message {
	return discord.Message{
		ID:        snowflake.ID(1000),
		GuildID:   &guildID,
		ChannelID: chanID,
		Author:    discord.User{ID: userID, Username: "piton", GlobalName: ptr("Piton")},
		Content:   "hello **world**",
		CreatedAt: baseTime(),
	}
}

func render(t *testing.T, adapter *Adapter, messages ...discord.Message) string {
	t.Helper()
	tr := adapter.Transcript(transcript.Channel{Name: "general", Type: transcript.ChannelText}, messages)
	out, err := tr.HTML(transcript.WithMedia(transcript.URLMedia()))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	return string(out)
}

func TestBasicMessage(t *testing.T) {
	html := render(t, New(), plainMessage())

	for _, want := range []string{
		`<discord-messages channel-name="general" channel-type="text">`,
		`<discord-message profile="111111111111111111" author="Piton" timestamp="2024-03-15T14:28:00Z">`,
		"hello <discord-bold>world</discord-bold>",
		`"author":"Piton"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in:\n%s", want, html)
		}
	}
}

func TestMemberNickAndRoleColourFromCache(t *testing.T) {
	// disgo's caches are no-ops unless the relevant flags are enabled, so a bot
	// must ask for members/roles for nicknames and role colours to resolve.
	caches := cache.New(cache.WithCaches(
		cache.FlagMembers, cache.FlagRoles, cache.FlagChannels, cache.FlagGuilds,
	))
	caches.MemberCache().Put(guildID, userID, discord.Member{
		User:    discord.User{ID: userID, Username: "piton", GlobalName: ptr("Piton")},
		Nick:    ptr("Pit"),
		GuildID: guildID,
		RoleIDs: []snowflake.ID{roleID},
	})
	caches.RoleCache().Put(guildID, roleID, discord.Role{
		ID:       roleID,
		GuildID:  guildID,
		Name:     "Moderator",
		Color:    0x57F287,
		Position: 3,
	})

	msg := plainMessage()
	msg.Content = "hi <@222222222222222222> and <@&333333333333333333>"
	html := render(t, New(WithCaches(caches)), msg)

	if !strings.Contains(html, `author="Pit"`) {
		t.Errorf("member nick should be used, got:\n%s", html)
	}
	if !strings.Contains(html, `"roleColor":"#57f287"`) {
		t.Errorf("role colour from the cache should reach the profile map:\n%s", html)
	}
	if !strings.Contains(html, `<discord-mention type="role" style="--dt-mention-role-color: #57f287">Moderator</discord-mention>`) {
		t.Errorf("role mention should resolve its name and colour:\n%s", html)
	}
}

func TestOverridesWinOverMessageData(t *testing.T) {
	profiles := transcript.Profiles{
		userID.String(): {Key: userID.String(), Name: "renamed", RoleColor: "#eb459e"},
	}
	html := render(t, New(WithUsers(profiles)), plainMessage())

	if !strings.Contains(html, `author="renamed"`) {
		t.Errorf("override name should win:\n%s", html)
	}
	if !strings.Contains(html, `"roleColor":"#eb459e"`) {
		t.Errorf("override colour should win:\n%s", html)
	}
}

func TestUnresolvedMentionKeepsItsID(t *testing.T) {
	msg := plainMessage()
	msg.Content = "hi <@222222222222222222>"
	html := render(t, New(), msg)

	if !strings.Contains(html, `<discord-mention type="user">222222222222222222</discord-mention>`) {
		t.Errorf("an unresolved mention should keep its ID rather than vanish:\n%s", html)
	}
}

func TestChannelMentionFromMessagePayload(t *testing.T) {
	msg := plainMessage()
	msg.Content = "see <#444444444444444444>"
	msg.MentionChannels = []discord.MentionChannel{{ID: chanID, Name: "rules"}}
	html := render(t, New(), msg)

	if !strings.Contains(html, `<discord-mention type="channel">rules</discord-mention>`) {
		t.Errorf("channel mentions resolve from the message payload:\n%s", html)
	}
}

func TestReplyAndEditedAndEphemeralAndHighlight(t *testing.T) {
	referenced := plainMessage()
	referenced.ID = snowflake.ID(999)
	referenced.Content = "the original text"

	msg := plainMessage()
	msg.ID = snowflake.ID(1001)
	msg.Content = "<@111111111111111111> thanks!"
	msg.EditedTimestamp = ptr(baseTime().Add(time.Minute))
	msg.Flags = discord.MessageFlagEphemeral
	msg.ReferencedMessage = &referenced

	html := render(t, New(WithSelfUserID(userID)), msg)

	for _, want := range []string{
		` edited`,
		` ephemeral`,
		` highlight`,
		`<discord-reply profile="111111111111111111" author="Piton" mentions>the original text</discord-reply>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in:\n%s", want, html)
		}
	}
}

func TestSystemMessageMapping(t *testing.T) {
	msg := plainMessage()
	msg.Type = discord.MessageTypeUserJoin
	msg.Content = "piton joined the server."
	html := render(t, New(), msg)

	if !strings.Contains(html, `<discord-system-message type="join" timestamp="2024-03-15T14:28:00Z">piton joined the server.</discord-system-message>`) {
		t.Errorf("join message should map to a system row:\n%s", html)
	}
	if strings.Contains(html, "<discord-message ") {
		t.Errorf("system messages should not emit a user row:\n%s", html)
	}
}

func TestEmbedsAttachmentsAndReactions(t *testing.T) {
	msg := plainMessage()
	msg.Content = ""
	msg.Embeds = []discord.Embed{{
		Title:       "Embed title",
		Description: "Some **description**",
		URL:         "https://example.com",
		Color:       0x5865F2,
		Author:      &discord.EmbedAuthor{Name: "miona", URL: "https://example.com/m"},
		Fields:      []discord.EmbedField{{Name: "Field", Value: "Value", Inline: ptr(true)}},
		Footer:      &discord.EmbedFooter{Text: "Footer"},
		Image:       &discord.EmbedResource{URL: "https://cdn.discordapp.com/attachments/1/2/img.png", Width: 400, Height: 300},
		Thumbnail:   &discord.EmbedResource{URL: "https://cdn.discordapp.com/attachments/1/2/thumb.png"},
	}}
	msg.Attachments = []discord.Attachment{
		{Filename: "shot.png", URL: "https://cdn.discordapp.com/attachments/1/2/shot.png", ContentType: ptr("image/png"), Size: 2048, Width: ptr(640), Height: ptr(480)},
		{Filename: "clip.mp4", URL: "https://cdn.discordapp.com/attachments/1/2/clip.mp4", ContentType: ptr("video/mp4"), Size: 4096},
		{Filename: "voice.ogg", URL: "https://cdn.discordapp.com/attachments/1/2/voice.ogg", ContentType: ptr("audio/ogg"), Size: 1024},
		{Filename: "spoiler.png", URL: "https://cdn.discordapp.com/attachments/1/2/spoiler.png", ContentType: ptr("image/png"), Size: 512, Flags: discord.AttachmentFlagIsSpoiler},
		{Filename: "report.pdf", URL: "https://cdn.discordapp.com/attachments/1/2/report.pdf", ContentType: ptr("application/pdf"), Size: 1536},
	}
	msg.Reactions = []discord.MessageReaction{
		{Emoji: discord.Emoji{Name: "🎉"}, Count: 3, Me: true},
		{Emoji: discord.Emoji{Name: "party", ID: snowflake.ID(999999999999999999)}, Count: 12},
	}
	msg.StickerItems = []discord.MessageSticker{{Name: "wave"}}

	html := render(t, New(), msg)

	for _, want := range []string{
		`<discord-embed color="#5865f2">`,
		`<span class="dt-embed-author">`,
		`<a class="dt-embed-title" href="https://example.com"`,
		`<discord-embed-description>Some <discord-bold>description</discord-bold></discord-embed-description>`,
		`<discord-embed-field field-title="Field" inline>Value</discord-embed-field>`,
		`<discord-embed-footer>`,
		`<div class="dt-embed-image">`,
		`<div class="dt-embed-thumbnail">`,
		`<discord-image-attachment><img src="https://cdn.discordapp.com/attachments/1/2/shot.png"`,
		`<discord-video-attachment><video controls`,
		`<discord-audio-attachment><audio controls`,
		`<discord-image-attachment spoiler><img src="https://cdn.discordapp.com/attachments/1/2/spoiler.png"`,
		`<discord-file-attachment name="report.pdf" bytes="1.5" bytes-unit="KB" type="PDF">`,
		`<discord-reaction emoji="🎉" count="3" reacted></discord-reaction>`,
		`<discord-reaction emoji="https://cdn.discordapp.com/emojis/999999999999999999.png" name=":party:" count="12">`,
		`[sticker: wave]`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in:\n%s", want, html)
		}
	}
}

func TestMediaStoreReceivesEveryMediaURL(t *testing.T) {
	var seen []string
	store := transcript.MediaStoreFunc(func(_ context.Context, ref transcript.MediaRef) (string, error) {
		seen = append(seen, ref.URL)
		return "data:image/png;base64,AAAA", nil
	})

	msg := plainMessage()
	msg.Author.Avatar = ptr("avatars/hash")
	msg.Attachments = []discord.Attachment{{
		Filename: "shot.png", URL: "https://cdn.discordapp.com/attachments/1/2/shot.png",
		ContentType: ptr("image/png"), Size: 10,
	}}
	msg.Embeds = []discord.Embed{{
		Title:     "t",
		Thumbnail: &discord.EmbedResource{URL: "https://cdn.discordapp.com/attachments/1/2/thumb.png"},
	}}

	tr := New().Transcript(transcript.Channel{Name: "general", Type: transcript.ChannelText}, []discord.Message{msg})
	out, err := tr.HTML(transcript.WithMedia(store))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	if !strings.Contains(string(out), "data:image/png;base64,AAAA") {
		t.Errorf("custom media store output should be used:\n%s", out)
	}
	// Avatar, attachment and embed thumbnail all go through the same store.
	if len(seen) < 3 {
		t.Errorf("expected avatar, attachment and thumbnail to be stored, got %v", seen)
	}
}

func TestChannelTypeMapping(t *testing.T) {
	adapter := New()
	cases := map[discord.ChannelType]transcript.ChannelType{
		discord.ChannelTypeGuildText:         transcript.ChannelText,
		discord.ChannelTypeGuildVoice:        transcript.ChannelVoice,
		discord.ChannelTypeGuildPublicThread: transcript.ChannelThread,
		discord.ChannelTypeGuildForum:        transcript.ChannelForum,
	}
	for in, want := range cases {
		if got := channelType(in); got != want {
			t.Errorf("channelType(%v) = %q, want %q", in, got, want)
		}
	}
	_ = adapter
}

func TestWithoutReplies(t *testing.T) {
	referenced := plainMessage()
	referenced.Content = "original"
	msg := plainMessage()
	msg.ReferencedMessage = &referenced

	html := render(t, New(WithoutReplies()), msg)
	if strings.Contains(html, "<discord-reply") {
		t.Errorf("WithoutReplies should drop the preview:\n%s", html)
	}
}
