package disgo

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/x1xo/discord-transcript-go/transcript"
)

// guildChannel builds a real disgo guild channel the way a REST response would,
// so Adapter.Channel is exercised on a concrete type rather than a stub.
func guildChannel(t *testing.T, guild snowflake.ID, id snowflake.ID, name string) discord.Channel {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"id": id.String(), "type": 0, "guild_id": guild.String(), "name": name,
		"position": 0, "permission_overwrites": []any{},
	})
	if err != nil {
		t.Fatalf("marshal channel: %v", err)
	}
	var channel discord.UnmarshalChannel
	if err := json.Unmarshal(body, &channel); err != nil {
		t.Fatalf("unmarshal channel: %v", err)
	}
	if channel.Channel == nil {
		t.Fatal("channel did not unmarshal")
	}
	return channel.Channel
}

// restMessage is what GET /channels/{id}/messages actually returns: no guild_id
// and no member, even though both are the only way to reach a nickname or a role
// colour. Everything until now keyed off those two fields, which is why a
// transcript built from REST history had no colours at all.
func restMessage() discord.Message {
	msg := plainMessage()
	msg.GuildID = nil
	msg.Member = nil
	return msg
}

func populatedCaches() cache.Caches {
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
		ID: roleID, GuildID: guildID, Name: "Moderator", Color: 0x57F287, Position: 3,
	})
	return caches
}

func TestChannelGuildIDLetsTheCacheAnswer(t *testing.T) {
	msg := restMessage()
	msg.Content = "hi <@&333333333333333333>"
	adapter := New(WithCaches(populatedCaches()))

	channel := adapter.Channel(guildChannel(t, guildID, chanID, "general"))
	if channel.GuildID != guildID.String() {
		t.Fatalf("Adapter.Channel should carry the guild, got %q", channel.GuildID)
	}

	tr := adapter.Transcript(channel, []discord.Message{msg})
	out, err := tr.HTML(transcript.WithMedia(transcript.URLMedia()))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	for _, want := range []string{
		`author="Pit"`,
		`<span class="dt-author" style="color:#57f287">Pit</span>`,
		`<discord-mention type="role" style="--dt-mention-role-color: #57f287">Moderator</discord-mention>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in:\n%s", want, html)
		}
	}
}

func TestGuildIDOptionCoversAChannelThatDoesNotCarryOne(t *testing.T) {
	msg := restMessage()
	html := render(t, New(WithCaches(populatedCaches()), WithGuildID(guildID)), msg)

	if !strings.Contains(html, `style="color:#57f287"`) {
		t.Errorf("WithGuildID should be enough to reach the cache:\n%s", html)
	}
}

func TestChannelTakesTheGuildHeaderFromTheCache(t *testing.T) {
	caches := cache.New(cache.WithCaches(cache.FlagChannels, cache.FlagGuilds))
	caches.AddChannel(guildChannel(t, guildID, chanID, "ticket-0001").(discord.GuildChannel))
	caches.AddGuild(discord.Guild{ID: guildID, Name: "Test Guild", Icon: ptr("abc123")})

	got := New(WithCaches(caches)).Channel(interactionChannel(t, chanID, "ticket-0001"))
	if got.Guild != "Test Guild" {
		t.Errorf("guild name = %q", got.Guild)
	}
	if !strings.Contains(got.GuildIcon, "/icons/"+guildID.String()+"/") || !strings.HasSuffix(got.GuildIcon, ".png") {
		t.Errorf("guild icon = %q", got.GuildIcon)
	}

	// An uncached channel has no guild of its own, so the option is the only
	// thing that can name it — and the header still comes out.
	got = New(WithCaches(caches), WithGuildID(guildID)).
		Channel(interactionChannel(t, snowflake.ID(555555555555555555), "unknown"))
	if got.Guild != "Test Guild" || got.GuildIcon == "" {
		t.Errorf("WithGuildID should still name the server, got %q / %q", got.Guild, got.GuildIcon)
	}

	// No caches: no guild to name, and no panic.
	if got := New().Channel(interactionChannel(t, chanID, "ticket-0001")); got.Guild != "" || got.GuildIcon != "" {
		t.Errorf("expected no guild data, got %q / %q", got.Guild, got.GuildIcon)
	}
}

func TestRolesOverrideColoursTheAuthorName(t *testing.T) {
	// A member is present (an offline export carries one) but there is no cache
	// and no live guild. The override map is the only source of role data, and it
	// used to be consulted for role mentions only — never for the author colour.
	msg := plainMessage()
	msg.GuildID = nil
	msg.Member = &discord.Member{
		User:    msg.Author,
		GuildID: guildID,
		RoleIDs: []snowflake.ID{roleID, otherRoleID},
	}
	roles := RolesFrom([]discord.Role{
		{ID: roleID, Name: "Moderator", Color: 0x57F287, Position: 3},
		{ID: otherRoleID, Name: "Regular", Color: 0xEB459E, Position: 1},
	})
	msg.Content = "hi <@&333333333333333333>"

	html := render(t, New(WithRoles(roles)), msg)

	if !strings.Contains(html, `<span class="dt-author" style="color:#57f287">`) {
		t.Errorf("the highest coloured role should colour the name:\n%s", html)
	}
	if !strings.Contains(html, `<discord-mention type="role" style="--dt-mention-role-color: #57f287">Moderator</discord-mention>`) {
		t.Errorf("role mention should resolve from the override map:\n%s", html)
	}
}

func TestActionRowsFromComponents(t *testing.T) {
	msg := plainMessage()
	msg.Content = "pick one"
	msg.Components = []discord.LayoutComponent{
		discord.NewActionRow(
			discord.NewPrimaryButton("Confirm", "confirm").WithEmoji(discord.ComponentEmoji{Name: "✅"}),
			discord.NewLinkButton("Docs", "https://example.com/docs"),
			discord.NewDangerButton("Delete", "delete").WithDisabled(true),
			discord.ButtonComponent{
				Style: discord.ButtonStyleSecondary,
				Emoji: &discord.ComponentEmoji{ID: 999999999999999999, Name: "party"},
			},
		),
		// A row whose only item is a select menu has no element in the
		// stylesheet, so it must vanish instead of leaving an empty gap.
		discord.NewActionRow(discord.NewStringSelectMenu("pick", "Pick one")),
	}

	html := render(t, New(), msg)

	for _, want := range []string{
		`<discord-action-row data-dt-r>`,
		`<discord-button data-dt-r type="primary">✅Confirm</discord-button>`,
		`<a class="dt-button-link" href="https://example.com/docs" target="_blank" rel="noopener noreferrer"><discord-button data-dt-r type="link">Docs</discord-button></a>`,
		`<discord-button data-dt-r type="destructive" disabled>Delete</discord-button>`,
		`<discord-button data-dt-r><img class="dt-button-emoji" src="https://cdn.discordapp.com/emojis/999999999999999999.png?size=64" alt=":party:"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in:\n%s", want, html)
		}
	}
	if strings.Contains(html, "Pick one") {
		t.Errorf("a select-menu row has no renderer and should be dropped:\n%s", html)
	}
}

// The production path is a JSON payload, not a struct a Go caller built, so the
// concrete component types that come out of disgo's unmarshaller matter.
func TestActionRowsFromAJSONPayload(t *testing.T) {
	body := `{
		"id": "1000",
		"channel_id": "444444444444444444",
		"guild_id": "900000000000000001",
		"timestamp": "2024-03-15T14:28:00Z",
		"author": {"id": "111111111111111111", "username": "piton", "global_name": "Piton"},
		"content": "pick one",
		"components": [
			{"type": 1, "components": [
				{"type": 2, "style": 1, "label": "Confirm", "custom_id": "confirm"},
				{"type": 2, "style": 5, "label": "Docs", "url": "https://example.com/docs"},
				{"type": 2, "style": 4, "label": "Delete", "custom_id": "delete", "disabled": true}
			]}
		]
	}`
	var msg discord.Message
	if err := json.Unmarshal([]byte(body), &msg); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}

	html := render(t, New(), msg)
	for _, want := range []string{
		`<discord-button data-dt-r type="primary">Confirm</discord-button>`,
		`<a class="dt-button-link" href="https://example.com/docs" target="_blank" rel="noopener noreferrer"><discord-button data-dt-r type="link">Docs</discord-button></a>`,
		`<discord-button data-dt-r type="destructive" disabled>Delete</discord-button>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in:\n%s", want, html)
		}
	}
}

// interactionChannel is the channel an interaction carries: partial, and it
// does not implement discord.GuildChannel, so it names no guild.
func interactionChannel(t *testing.T, id snowflake.ID, name string) discord.Channel {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"id": id.String(), "type": 0, "name": name, "permissions": "0",
	})
	if err != nil {
		t.Fatalf("marshal channel: %v", err)
	}
	var channel discord.InteractionChannel
	if err := json.Unmarshal(body, &channel); err != nil {
		t.Fatalf("unmarshal interaction channel: %v", err)
	}
	if channel.MessageChannel == nil {
		t.Fatal("interaction channel did not unmarshal")
	}
	return channel
}

func TestInteractionChannelTakesItsGuildFromTheCache(t *testing.T) {
	caches := cache.New(cache.WithCaches(cache.FlagChannels, cache.FlagGuilds))
	caches.AddChannel(guildChannel(t, guildID, chanID, "ticket-0001").(discord.GuildChannel))

	got := New(WithCaches(caches)).Channel(interactionChannel(t, chanID, "ticket-0001"))
	if got.GuildID != guildID.String() {
		t.Errorf("the cached channel should supply the guild, got %q", got.GuildID)
	}
	if got.Name != "ticket-0001" {
		t.Errorf("name = %q", got.Name)
	}
}

func TestChannelWithoutCachesDoesNotPanic(t *testing.T) {
	// A guild channel used to reach into a nil cache the moment it was seen.
	got := New().Channel(guildChannel(t, guildID, chanID, "general"))
	if got.GuildID != guildID.String() {
		t.Errorf("a full guild channel names its own guild, got %q", got.GuildID)
	}
	// A partial one with nothing to ask has to degrade, not crash.
	if partial := New().Channel(interactionChannel(t, chanID, "ticket-0001")); partial.GuildID != "" {
		t.Errorf("expected no guild, got %q", partial.GuildID)
	}
}

// The whole ticket path: an interaction channel, messages fetched over REST
// (no guild_id, no member), and a cache that holds roles but not the author.
func TestTicketTranscriptResolvesGuildRolesAndColour(t *testing.T) {
	caches := cache.New(cache.WithCaches(
		cache.FlagMembers, cache.FlagRoles, cache.FlagChannels, cache.FlagGuilds,
	))
	caches.AddChannel(guildChannel(t, guildID, chanID, "ticket-0001").(discord.GuildChannel))
	caches.RoleCache().Put(guildID, roleID, discord.Role{
		ID: roleID, GuildID: guildID, Name: "Moderator", Color: 0x57F287, Position: 3,
	})

	var asked []snowflake.ID
	adapter := New(
		WithCaches(caches),
		WithMemberFetcher(func(guild, user snowflake.ID) (discord.Member, bool) {
			asked = append(asked, user)
			if user != userID {
				return discord.Member{}, false
			}
			return discord.Member{
				User:    discord.User{ID: userID, Username: "piton", GlobalName: ptr("Piton")},
				Nick:    ptr("Pit"),
				GuildID: guild,
				RoleIDs: []snowflake.ID{roleID},
			}, true
		}),
	)

	first := restMessage()
	first.Content = "ping <@&333333333333333333> and <@222222222222222222>"
	second := restMessage()
	second.ID = snowflake.ID(1002)
	second.Content = "again"

	channel := adapter.Channel(interactionChannel(t, chanID, "ticket-0001"))
	tr := adapter.Transcript(channel, []discord.Message{first, second})
	out, err := tr.HTML(transcript.WithMedia(transcript.URLMedia()))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	for _, want := range []string{
		`channel-name="ticket-0001"`,
		// The role is cached, so its name and colour resolve without any fetch.
		`<discord-mention type="role" style="--dt-mention-role-color: #57f287">Moderator</discord-mention>`,
		// The author is not cached; the fetcher supplies the nickname and the
		// role colour the cache alone could not.
		`author="Pit"`,
		`<span class="dt-author" style="color:#57f287">Pit</span>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in:\n%s", want, html)
		}
	}
	// One lookup for the author, none for the user they mentioned, and only one
	// even though two messages share the author.
	if len(asked) != 1 || asked[0] != userID {
		t.Errorf("expected exactly one author lookup, got %v", asked)
	}
}

func TestUnsafeButtonURLIsNotALink(t *testing.T) {
	msg := plainMessage()
	msg.Components = []discord.LayoutComponent{
		discord.NewActionRow(discord.NewLinkButton("click", "javascript:alert(1)")),
	}
	html := render(t, New(), msg)

	if strings.Contains(html, "<a ") || strings.Contains(html, "javascript:") {
		t.Errorf("a javascript: button URL must not become an anchor:\n%s", html)
	}
	if !strings.Contains(html, `<discord-button data-dt-r type="link">click</discord-button>`) {
		t.Errorf("the button itself should still draw:\n%s", html)
	}
}
