package disgo

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/x1xo/discord-transcript-go/transcript"
)

// messageResolvers answers identity questions while parsing one message.
//
// Lookup order is deliberate: explicit overrides first (so offline exports can
// correct anything), then the data carried by the message itself (which works
// with no cache and no network), then the same data seen anywhere else in the
// export, and only then disgo's caches. The export wins over a live cache, so a
// transcript reads the way it did when it was exported.
type messageResolvers struct {
	base    *Adapter
	guildID snowflake.ID
	local   *identitySet
	shared  *transcriptIndex
	roles   map[snowflake.ID]discord.Role
}

// resolversFor collects everything the message already knows about identities,
// including the message it replies to, and pairs it with the export-wide index.
func (a *Adapter) resolversFor(m discord.Message, shared *transcriptIndex) *messageResolvers {
	r := &messageResolvers{
		base:   a,
		shared: shared,
		local:  newIdentitySet(2),
		roles:  make(map[snowflake.ID]discord.Role),
	}
	if m.GuildID != nil {
		r.guildID = *m.GuildID
	}
	if m.Member != nil && r.guildID == 0 {
		r.guildID = m.Member.GuildID
	}
	if r.guildID == 0 && shared != nil {
		r.guildID = shared.guildID
	}
	r.local.absorb(m)
	if m.ReferencedMessage != nil {
		r.local.absorb(*m.ReferencedMessage)
	}
	return r
}

// memberFor finds a guild member for an author: the message's own data, then the
// export, then disgo's cache.
func (r *messageResolvers) memberFor(id snowflake.ID) (discord.Member, bool) {
	if member, ok := r.local.members[id]; ok {
		return member, true
	}
	if r.shared != nil {
		if member, ok := r.shared.set.members[id]; ok {
			return member, true
		}
	}
	return r.cacheMember(id)
}

// cacheMember finds a guild member in disgo's cache, the last resort.
func (r *messageResolvers) cacheMember(id snowflake.ID) (discord.Member, bool) {
	if r.base.opts.Caches != nil && r.guildID != 0 {
		if member, ok := r.base.opts.Caches.Member(r.guildID, id); ok {
			return member, true
		}
	}
	return discord.Member{}, false
}

// authorFromMember is the identity a mentioned member renders with: nickname,
// guild avatar and the colour of their highest coloured role.
func (r *messageResolvers) authorFromMember(member discord.Member) transcript.Author {
	author := authorFromUser(member.User)
	author.Name = member.EffectiveName()
	if avatar := member.EffectiveAvatarURL(); avatar != "" {
		author.AvatarURL = avatar
	}
	if color := r.roleColorFor(member); color != "" {
		author.RoleColor = color
	}
	return author
}

// User implements transcript.UserResolver.
func (r *messageResolvers) User(id string) (transcript.Author, bool) {
	snowflakeID, ok := parseID(id)
	if !ok {
		return transcript.Author{}, false
	}
	if override, ok := r.overrideUser(id); ok {
		return override, true
	}
	if member, ok := r.local.members[snowflakeID]; ok {
		return r.authorFromMember(member), true
	}
	if user, ok := r.local.users[snowflakeID]; ok {
		return authorFromUser(user), true
	}
	if r.shared != nil {
		if user, member, ok := r.shared.user(snowflakeID); ok {
			if member.User.ID != 0 {
				return r.authorFromMember(member), true
			}
			return authorFromUser(user), true
		}
	}
	if member, ok := r.cacheMember(snowflakeID); ok {
		return r.authorFromMember(member), true
	}
	return transcript.Author{}, false
}

// Channel implements transcript.ChannelResolver.
func (r *messageResolvers) Channel(id string) (string, bool) {
	snowflakeID, ok := parseID(id)
	if !ok {
		return "", false
	}
	if r.base.opts.Channels != nil {
		if name, ok := r.base.opts.Channels.Channel(id); ok {
			return name, true
		}
	}
	if name, ok := r.local.channels[snowflakeID]; ok {
		return name, true
	}
	if r.shared != nil {
		if name, ok := r.shared.set.channels[snowflakeID]; ok {
			return name, true
		}
	}
	if r.base.opts.Caches != nil {
		if ch, ok := r.base.opts.Caches.Channel(snowflakeID); ok {
			if named, ok := ch.(interface{ Name() string }); ok && named.Name() != "" {
				return named.Name(), true
			}
		}
	}
	return "", false
}

// Role implements transcript.RoleResolver.
func (r *messageResolvers) Role(id string) (string, string, bool) {
	snowflakeID, ok := parseID(id)
	if !ok {
		return "", "", false
	}
	if r.base.opts.Roles != nil {
		if name, color, ok := r.base.opts.Roles.Role(id); ok {
			return name, color, true
		}
	}
	if role, ok := r.lookupRole(snowflakeID); ok {
		return role.Name, roleColor(role), true
	}
	return "", "", false
}

// roleColorFor returns the colour of a member's highest coloured role, which is
// the colour Discord draws their name in.
func (r *messageResolvers) roleColorFor(member discord.Member) string {
	best := ""
	bestPosition := -1
	for _, roleID := range member.RoleIDs {
		role, ok := r.lookupRole(roleID)
		if !ok {
			continue
		}
		if roleColor(role) == "" {
			continue
		}
		if role.Position > bestPosition {
			bestPosition = role.Position
			best = roleColor(role)
		}
	}
	return best
}

func (r *messageResolvers) lookupRole(id snowflake.ID) (discord.Role, bool) {
	if role, ok := r.roles[id]; ok {
		return role, true
	}
	if r.base.opts.Caches == nil || r.guildID == 0 {
		return discord.Role{}, false
	}
	role, ok := r.base.opts.Caches.Role(r.guildID, id)
	if ok {
		r.roles[id] = role
	}
	return role, ok
}

// overrideUser returns a caller-supplied identity, which always wins.
func (r *messageResolvers) overrideUser(key string) (transcript.Author, bool) {
	if r.base.opts.Users == nil {
		return transcript.Author{}, false
	}
	author, ok := r.base.opts.Users.User(key)
	if !ok {
		return transcript.Author{}, false
	}
	if author.Key == "" {
		author.Key = key
	}
	return author, true
}

// roleColor returns a role's colour as "#rrggbb", preferring the newer
// primary-colour field and returning "" when the role has no colour.
func roleColor(role discord.Role) string {
	if color := transcript.HexColor(role.RoleColors.PrimaryColor); color != "" {
		return color
	}
	return transcript.HexColor(role.Color)
}

func parseID(value string) (snowflake.ID, bool) {
	id, err := snowflake.Parse(value)
	if err != nil {
		return 0, false
	}
	return id, true
}

// bundle adapts the per-message resolvers to the bundle the parser expects.
func (r *messageResolvers) bundle() transcript.Resolvers {
	return transcript.Resolvers{Users: r, Channels: r, Roles: r}
}
