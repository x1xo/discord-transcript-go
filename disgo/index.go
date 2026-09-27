package disgo

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/x1xo/discord-transcript-go/transcript"
)

// identitySet is the identity data one message — or a whole export — carries.
type identitySet struct {
	users    map[snowflake.ID]discord.User
	members  map[snowflake.ID]discord.Member
	channels map[snowflake.ID]string
}

func newIdentitySet(messages int) *identitySet {
	return &identitySet{
		users:    make(map[snowflake.ID]discord.User, messages*2),
		members:  make(map[snowflake.ID]discord.Member, messages/4+1),
		channels: make(map[snowflake.ID]string, messages/4+1),
	}
}

// absorb takes everything a message says about identities. Mention channels only
// carry a name when the API bothered to include one, which is also the only
// source of channel names in an export with no cache.
func (s *identitySet) absorb(m discord.Message) {
	if m.Author.ID != 0 {
		s.users[m.Author.ID] = m.Author
	}
	if m.Member != nil {
		s.members[m.Author.ID] = *m.Member
	}
	for _, u := range m.Mentions {
		s.users[u.ID] = u
	}
	for _, c := range m.MentionChannels {
		if c.Name != "" {
			s.channels[c.ID] = c.Name
		}
	}
}

// transcriptIndex is every identity the export itself knows about.
//
// Mentions resolve per message first, because a message is the best source for
// its own. What that cannot cover is everything else: a mention of someone who
// only appears in an earlier message, a channel named by a different message, and
// the common case where a mention lives inside an embed — Discord does not put
// those in the message's mentions array at all. Building the index once from the
// whole export is what makes "ticket closed by <@123>" render a name.
//
// Roles are the exception, and are not here on purpose: Discord never sends role
// names with a message, only IDs. Pass WithRoles (disgo.RolesFrom converts a
// guild's roles) or a cache for `<@&role>` mentions and role colours.
type transcriptIndex struct {
	set     *identitySet
	guildID snowflake.ID
}

func newTranscriptIndex(channel transcript.Channel, messages []discord.Message) *transcriptIndex {
	idx := &transcriptIndex{set: newIdentitySet(len(messages))}
	for _, m := range messages {
		idx.set.absorb(m)
		if m.ReferencedMessage != nil {
			idx.set.absorb(*m.ReferencedMessage)
		}
		if idx.guildID == 0 && m.GuildID != nil {
			idx.guildID = *m.GuildID
		}
		if m.Member != nil && idx.guildID == 0 {
			idx.guildID = m.Member.GuildID
		}
	}
	idx.nameOwnChannel(channel, messages)
	return idx
}

// nameOwnChannel maps the channel the transcript belongs to, so a transcript can
// mention itself by name without anyone handing the ID over separately.
func (idx *transcriptIndex) nameOwnChannel(channel transcript.Channel, messages []discord.Message) {
	if channel.Name == "" {
		return
	}
	counts := make(map[snowflake.ID]int, 4)
	for _, m := range messages {
		counts[m.ChannelID]++
	}
	best, bestCount := snowflake.ID(0), 0
	for id, count := range counts {
		if count > bestCount {
			best, bestCount = id, count
		}
	}
	if best != 0 {
		idx.set.channels[best] = channel.Name
	}
}

// user finds a user, preferring a guild member so a nickname wins.
func (idx *transcriptIndex) user(id snowflake.ID) (discord.User, discord.Member, bool) {
	if member, ok := idx.set.members[id]; ok {
		return member.User, member, true
	}
	if user, ok := idx.set.users[id]; ok {
		return user, discord.Member{}, true
	}
	return discord.User{}, discord.Member{}, false
}

// RolesFrom converts a guild's roles into the override map WithRoles takes, so
// an offline export can resolve role mentions and draw role colours without a
// cache. It is a convenience: WithRoles accepts the map directly.
func RolesFrom(roles []discord.Role) transcript.Roles {
	out := make(transcript.Roles, len(roles))
	for _, role := range roles {
		out[role.ID.String()] = transcript.RoleInfo{Name: role.Name, Color: roleColor(role)}
	}
	return out
}
