package disgo

import (
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/x1xo/discord-transcript-go/transcript"
)

// Mention resolution is a two-pass affair: the identity a mention needs is often
// carried by a *different* message than the one that mentions it, and never by an
// embed, which Discord leaves out of the mentions array entirely. These cases all
// failed before the export-wide index.
func TestMentionsResolveFromTheWholeExport(t *testing.T) {
	const (
		me     = snowflake.ID(100000000000000001)
		other  = snowflake.ID(100000000000000002)
		role   = snowflake.ID(400000000000000001)
		room   = snowflake.ID(300000000000000002)
		other2 = snowflake.ID(300000000000000003)
	)
	roomID := room
	at := time.Date(2024, 3, 15, 14, 28, 0, 0, time.UTC)
	nick := "piton (AFK)"

	messages := []discord.Message{
		{
			ID: 1, ChannelID: room, GuildID: &roomID,
			Author:    discord.User{ID: other, Username: "piton"},
			Content:   "hello <@" + me.String() + ">",
			CreatedAt: at,
			Mentions:  []discord.User{{ID: me, Username: "miona"}},
			// A member row carries the nickname the cover page should show.
			Member: &discord.Member{User: discord.User{ID: other, Username: "piton"}, Nick: &nick},
		},
		{
			// Nothing here says who `other` is: the index has to remember.
			ID: 2, ChannelID: room, GuildID: &roomID,
			Author:    discord.User{ID: me, Username: "miona"},
			Content:   "thanks <@" + other.String() + "> in <#" + room.String() + ">",
			CreatedAt: at.Add(time.Minute),
			// Another channel is named here, and only here.
			MentionChannels: []discord.MentionChannel{{ID: other2, Name: "archive"}},
		},
		{
			// The mentions live inside an embed, which the mentions array misses.
			ID: 3, ChannelID: room, GuildID: &roomID,
			Author:    discord.User{ID: me, Username: "miona"},
			CreatedAt: at.Add(2 * time.Minute),
			Embeds: []discord.Embed{{
				Title:       "Closed by <@" + other.String() + ">",
				Description: "Ticket has been closed by <@" + other.String() + ">",
				Fields: []discord.EmbedField{
					{Name: "Owner", Value: "<@" + other.String() + ">"},
					{Name: "Room", Value: "<#" + other2.String() + ">"},
					{Name: "Role", Value: "<@&" + role.String() + ">"},
				},
				Color: 0x5865f2,
			}},
		},
	}

	tr := New(WithRoles(RolesFrom([]discord.Role{{ID: role, Name: "Moderators", Color: 0x57f287}}))).
		Transcript(transcript.Channel{Name: "ticket-332", Type: transcript.ChannelText}, messages)
	page, err := tr.HTML()
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(page)

	for _, want := range []string{
		// The author only message 1 knows about, mentioned in message 2.
		`<discord-mention type="user">piton (AFK)</discord-mention>`,
		// The same user, mentioned inside an embed title, description and field.
		`Ticket has been closed by <discord-mention type="user">piton (AFK)</discord-mention>`,
		`<discord-embed-field field-title="Owner"><discord-mention type="user">piton (AFK)</discord-mention></discord-embed-field>`,
		// A channel named only by another message, and one named by the caller.
		`<discord-mention type="channel">archive</discord-mention>`,
		`<discord-mention type="channel">ticket-332</discord-mention>`,
		// A role, which only reaches a name through the caller's override.
		`<discord-mention type="role" style="--dt-mention-role-color: #57f287">Moderators</discord-mention>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s", want)
		}
	}

	// An ID nothing knows about stays visible rather than vanishing.
	lonely := New().Transcript(transcript.Channel{Name: "general", Type: transcript.ChannelText}, []discord.Message{{
		ID: 1, ChannelID: room,
		Author:    discord.User{ID: me, Username: "miona"},
		Content:   "ping <@999999999999999999> and <@&999999999999999998>",
		CreatedAt: at,
	}})
	page, err = lonely.HTML()
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	for _, want := range []string{
		`<discord-mention type="user">999999999999999999</discord-mention>`,
		`<discord-mention type="role">999999999999999998</discord-mention>`,
	} {
		if !strings.Contains(string(page), want) {
			t.Errorf("an unresolved mention should keep its ID, missing %s", want)
		}
	}
}

func TestMentionInsideAnEmbedHighlightsTheMessage(t *testing.T) {
	// Highlighting scans the mentions array and the text; an embed is text too,
	// and Discord does not fill the array from it.
	const me = snowflake.ID(100000000000000001)
	message := discord.Message{
		ID: 1, ChannelID: 300000000000000002,
		Author:    discord.User{ID: 100000000000000002, Username: "piton"},
		CreatedAt: time.Date(2024, 3, 15, 14, 28, 0, 0, time.UTC),
		Embeds: []discord.Embed{{
			Description: "Ticket has been closed by <@" + me.String() + ">",
			Color:       0x5865f2,
		}},
	}

	adapter := New(WithSelfUserID(me))
	converted := adapter.Message(message)
	if !converted.Highlight {
		t.Errorf("a mention inside an embed should highlight the message")
	}

	// The same embed without the mention is not highlighted.
	message.Embeds[0].Description = "Ticket has been closed by an operator"
	if New(WithSelfUserID(me)).Message(message).Highlight {
		t.Errorf("an embed without the mention should not highlight")
	}
}

func TestMemberIdentityWinsOverTheBareUser(t *testing.T) {
	// The export's own member row gives the nickname and the guild avatar; a
	// cache would only be consulted if neither the message nor the export had it.
	const (
		who  = snowflake.ID(100000000000000002)
		room = snowflake.ID(300000000000000002)
	)
	at := time.Date(2024, 3, 15, 14, 28, 0, 0, time.UTC)
	nick := "piton"
	avatar := "a1b2c3"

	tr := New().Transcript(transcript.Channel{Name: "general", Type: transcript.ChannelText}, []discord.Message{
		{
			ID: 1, ChannelID: room,
			Author:    discord.User{ID: who, Username: "piton_global"},
			CreatedAt: at,
			Member:    &discord.Member{User: discord.User{ID: who, Username: "piton_global"}, Nick: &nick, Avatar: &avatar},
		},
	})
	if tr.Messages[0].Author.Name != "piton" {
		t.Errorf("a nickname should win, got %q", tr.Messages[0].Author.Name)
	}
	if !strings.Contains(tr.Messages[0].Author.AvatarURL, "guilds") {
		t.Errorf("a guild avatar should win, got %q", tr.Messages[0].Author.AvatarURL)
	}
}

func TestRolesFromConvertsGuildRoles(t *testing.T) {
	roles := RolesFrom([]discord.Role{
		{ID: 1, Name: "Moderators", Color: 0x57f287},
		{ID: 2, Name: "No colour"},
	})
	if got := roles["1"]; got.Name != "Moderators" || got.Color != "#57f287" {
		t.Errorf("role 1: %+v", got)
	}
	if got := roles["2"]; got.Name != "No colour" || got.Color != "" {
		t.Errorf("a role without a colour should have none: %+v", got)
	}
}
