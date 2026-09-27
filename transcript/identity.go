package transcript

// Identity lookups used while parsing message content and adapting messages.
//
// All three are optional. When a resolver is missing, the parser degrades
// gracefully: an unresolved mention keeps its ID as text rather than vanishing,
// which matters because a transcript is a record of what was said.

// UserResolver resolves a user ID to an author identity (name, avatar, colours).
type UserResolver interface {
	User(id string) (Author, bool)
}

// ChannelResolver resolves a channel ID to its display name.
type ChannelResolver interface {
	Channel(id string) (string, bool)
}

// RoleResolver resolves a role ID to its name and colour.
type RoleResolver interface {
	Role(id string) (name string, color string, ok bool)
}

// Resolvers bundles the optional lookups handed to the markdown parser.
type Resolvers struct {
	Users    UserResolver
	Channels ChannelResolver
	Roles    RoleResolver
}

// Profiles is a map-backed UserResolver, useful for offline exports and tests.
type Profiles map[string]Author

// User implements UserResolver.
func (p Profiles) User(id string) (Author, bool) {
	a, ok := p[id]
	return a, ok
}

// Channels maps channel IDs to display names.
type Channels map[string]string

// Channel implements ChannelResolver.
func (c Channels) Channel(id string) (string, bool) {
	name, ok := c[id]
	return name, ok
}

// RoleInfo is a role's display name and colour.
type RoleInfo struct {
	Name  string
	Color string // "#rrggbb"
	// Position is the role's place in the guild hierarchy, higher meaning more
	// important. It only matters when a member has several coloured roles:
	// Discord draws the name in the highest one, and that is the role this
	// position picks. Leave it zero and the first coloured role wins.
	Position int
}

// Roles maps role IDs to their display information.
type Roles map[string]RoleInfo

// Role implements RoleResolver.
func (r Roles) Role(id string) (string, string, bool) {
	info, ok := r[id]
	return info.Name, info.Color, ok
}

// userName resolves a user mention, falling back to the raw ID.
func (r Resolvers) userName(id string) string {
	if r.Users != nil {
		if author, ok := r.Users.User(id); ok && author.Name != "" {
			return author.Name
		}
	}
	return id
}

// channelName resolves a channel mention, falling back to the raw ID.
func (r Resolvers) channelName(id string) string {
	if r.Channels != nil {
		if name, ok := r.Channels.Channel(id); ok && name != "" {
			return name
		}
	}
	return id
}

// roleInfo resolves a role mention, falling back to the raw ID.
func (r Resolvers) roleInfo(id string) (string, string) {
	if r.Roles != nil {
		if name, color, ok := r.Roles.Role(id); ok {
			if name == "" {
				name = id
			}
			return name, color
		}
	}
	return id, ""
}
