package transcript

import (
	"strconv"
	"strings"
	"time"
)

// ParseContent parses Discord message content into renderable nodes.
//
// It understands the parts of Discord's markdown that carry meaning in a
// transcript: inline formatting (bold, italic, underline, strikethrough,
// spoilers, inline code), fenced code blocks, block quotes, lists, headings,
// subtext, links, mentions, custom emoji and timestamps.
//
// It is deliberately forgiving. Discord's own parser is quirky about edge cases;
// this one prefers keeping the author's text visible over matching every quirk,
// because a transcript is a record of what was said.
func ParseContent(content string) []Node {
	return ParseContentWith(content, Resolvers{})
}

// ParseContentWith parses content, using res to turn mention IDs into names.
func ParseContentWith(content string, res Resolvers) []Node {
	if content == "" {
		return nil
	}
	p := &parser{res: res}
	return p.parseBlocks(splitLines(content))
}

type parser struct {
	res Resolvers
}

// splitLines splits on \n, tolerating both \r\n and \r.
func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}

// ------------------------------------------------------------------ blocks

func (p *parser) parseBlocks(lines []string) []Node {
	var out []Node
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, "```"):
			node, next := p.parseCodeBlock(lines, i)
			out = append(out, node)
			i = next
		case strings.HasPrefix(trimmed, ">>>"):
			// A multiline quote swallows the rest of the message.
			body := strings.TrimLeft(strings.TrimPrefix(trimmed, ">>>"), " \t")
			rest := []string{}
			if body != "" {
				rest = append(rest, body)
			}
			rest = append(rest, lines[i+1:]...)
			out = append(out, Quote(p.parseInlineLines(rest)...))
			return out
		case strings.HasPrefix(trimmed, ">"):
			node, next := p.parseQuote(lines, i)
			out = append(out, node)
			i = next
		case isOrderedListLine(trimmed) || isUnorderedListLine(trimmed):
			node, next := p.parseList(lines, i)
			out = append(out, node)
			i = next
		case headingLevel(trimmed) > 0:
			level := headingLevel(trimmed)
			text := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			out = append(out, Node{Kind: NodeHeading, Level: level, Children: p.parseInline(text)})
		case strings.HasPrefix(trimmed, "-# "):
			text := strings.TrimSpace(strings.TrimPrefix(trimmed, "-# "))
			out = append(out, Node{Kind: NodeSubtext, Children: p.parseInline(text)})
		case trimmed == "":
			// A blank line separates paragraphs, so it contributes the empty line
			// Discord shows — except in front of a block, which already carries its
			// own margin and would otherwise gain a stray blank line above it.
			if i < len(lines)-1 && !isBlockStart(lines[i+1]) {
				out = append(out, Node{Kind: NodeLineBreak})
			}
		default:
			out = append(out, p.parseInline(line)...)
			// Every line ends with a hard line break unless it is the last one or
			// the next line opens a block (which breaks visually on its own).
			// Blank lines therefore contribute the empty line Discord shows.
			if i < len(lines)-1 && !isBlockStart(lines[i+1]) {
				out = append(out, Node{Kind: NodeLineBreak})
			}
		}
	}
	return trimBreaks(out)
}

// parseCodeBlock consumes a fenced block starting at lines[start].
func (p *parser) parseCodeBlock(lines []string, start int) (Node, int) {
	opening := strings.TrimSpace(lines[start])
	info := strings.TrimSpace(strings.TrimPrefix(opening, "```"))
	var body []string
	i := start + 1
	closed := false
	for ; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
			closed = true
			break
		}
		body = append(body, lines[i])
	}
	next := i
	if !closed {
		// Unterminated fence: treat everything to the end as code, which is what
		// Discord shows while the message is being typed.
		next = len(lines) - 1
	}
	return Node{Kind: NodeCodeBlock, Language: info, Text: strings.Join(body, "\n")}, next
}

// parseQuote consumes consecutive "> " lines starting at lines[start].
func (p *parser) parseQuote(lines []string, start int) (Node, int) {
	var body []string
	i := start
	for ; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, ">") {
			break
		}
		body = append(body, strings.TrimPrefix(strings.TrimPrefix(trimmed, ">"), " "))
	}
	return Quote(p.parseInlineLines(body)...), i - 1
}

// parseList consumes a run of list items starting at lines[start].
func (p *parser) parseList(lines []string, start int) (Node, int) {
	ordered := isOrderedListLine(strings.TrimSpace(lines[start]))
	start1 := 0
	node := Node{Kind: NodeList, Ordered: ordered}
	if ordered {
		start1, _ = orderedListNumber(strings.TrimSpace(lines[start]))
		node.Start = start1
	}

	i := start
	var items []Node
	for ; i < len(lines); i++ {
		raw := lines[i]
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			break
		}
		isItem := isUnorderedListLine(trimmed) || isOrderedListLine(trimmed)
		// A change of list kind starts a new list rather than continuing this one.
		if isItem && (isOrderedListLine(trimmed) != ordered) {
			break
		}
		if isItem {
			text := listItemText(trimmed)
			items = append(items, Node{Kind: NodeListItem, Children: p.parseInline(text)})
			continue
		}
		// An indented continuation line belongs to the previous item.
		if len(items) > 0 && (strings.HasPrefix(raw, "  ") || strings.HasPrefix(raw, "\t")) {
			last := &items[len(items)-1]
			last.Children = append(last.Children, Node{Kind: NodeLineBreak})
			last.Children = append(last.Children, p.parseInline(trimmed)...)
			continue
		}
		break
	}
	node.Children = items
	return node, i - 1
}

// parseInlineLines parses each line and joins them with hard line breaks.
func (p *parser) parseInlineLines(lines []string) []Node {
	var out []Node
	for i, line := range lines {
		if i > 0 {
			out = append(out, Node{Kind: NodeLineBreak})
		}
		out = append(out, p.parseInline(line)...)
	}
	return out
}

func trimBreaks(nodes []Node) []Node {
	for len(nodes) > 0 && nodes[0].Kind == NodeLineBreak {
		nodes = nodes[1:]
	}
	for len(nodes) > 0 && nodes[len(nodes)-1].Kind == NodeLineBreak {
		nodes = nodes[:len(nodes)-1]
	}
	return nodes
}

// isBlockStart reports whether a line opens a block-level construct, which
// already breaks the visual flow without a hard line break before it.
func isBlockStart(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	return strings.HasPrefix(t, "```") || strings.HasPrefix(t, ">") ||
		strings.HasPrefix(t, "-# ") || headingLevel(t) > 0 ||
		isUnorderedListLine(t) || isOrderedListLine(t)
}

func isUnorderedListLine(s string) bool {
	return strings.HasPrefix(s, "- ") || strings.HasPrefix(s, "* ") || strings.HasPrefix(s, "+ ")
}

func isOrderedListLine(s string) bool {
	_, ok := orderedListNumber(s)
	return ok
}

// orderedListNumber parses the leading "12. " of an ordered list item.
func orderedListNumber(s string) (int, bool) {
	dot := strings.Index(s, ".")
	if dot <= 0 || dot > 3 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:dot])
	if err != nil || n <= 0 {
		return 0, false
	}
	if dot+1 >= len(s) || s[dot+1] != ' ' {
		return 0, false
	}
	return n, true
}

func listItemText(s string) string {
	for _, prefix := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(s, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(s, prefix))
		}
	}
	if dot := strings.Index(s, ". "); dot > 0 {
		return strings.TrimSpace(s[dot+2:])
	}
	return strings.TrimSpace(s)
}

func headingLevel(s string) int {
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n == 0 || n > 3 || n >= len(s) || s[n] != ' ' {
		return 0
	}
	return n
}

// ------------------------------------------------------------------ inline

// parseInline turns one line of content into nodes.
func (p *parser) parseInline(s string) []Node {
	var out []Node
	var text strings.Builder

	flush := func() {
		if text.Len() > 0 {
			out = append(out, Text(text.String()))
			text.Reset()
		}
	}

	for i := 0; i < len(s); {
		// Backslash escapes the next character.
		if s[i] == '\\' && i+1 < len(s) && isEscapable(s[i+1]) {
			text.WriteByte(s[i+1])
			i += 2
			continue
		}

		// Constructs that start with '<'.
		if s[i] == '<' {
			if node, width, ok := p.parseAngle(s[i:]); ok {
				flush()
				out = append(out, node)
				i += width
				continue
			}
		}

		// Bare URLs.
		if node, width, ok := parseBareURL(s[i:]); ok {
			flush()
			out = append(out, node)
			i += width
			continue
		}

		// Inline code wins over formatting inside it.
		if s[i] == '`' {
			if end := indexUnescaped(s, i+1, "`"); end > i+1 {
				flush()
				out = append(out, Code(s[i+1:end]))
				i = end + 1
				continue
			}
		}

		// Formatting delimiters, longest first.
		if node, width, ok := p.parseDelimiter(s, i); ok {
			flush()
			out = append(out, node)
			i += width
			continue
		}

		// @everyone / @here are literal text in the API payload.
		if s[i] == '@' {
			if node, width, ok := parseEveryoneHere(s[i:]); ok {
				flush()
				out = append(out, node)
				i += width
				continue
			}
		}

		text.WriteByte(s[i])
		i++
	}
	flush()
	return out
}

// delimiters maps an opening token to the node kind it produces.
var delimiters = []struct {
	token string
	kind  NodeKind
}{
	{"***", NodeBold}, // bold+italic is special-cased below
	{"**", NodeBold},
	{"__", NodeUnderline},
	{"~~", NodeStrikethrough},
	{"||", NodeSpoiler},
	{"*", NodeItalic},
	{"_", NodeItalic},
}

func (p *parser) parseDelimiter(s string, i int) (Node, int, bool) {
	for _, d := range delimiters {
		if !strings.HasPrefix(s[i:], d.token) {
			continue
		}
		end := indexUnescaped(s, i+len(d.token), d.token)
		if end <= i+len(d.token) {
			continue
		}
		// Underscores only italicise at word boundaries, so snake_case survives.
		if d.token == "_" && !underscoreBoundary(s, i, end) {
			continue
		}
		inner := s[i+len(d.token) : end]
		children := p.parseInline(inner)
		width := end + len(d.token) - i

		if d.token == "***" {
			return Node{Kind: NodeBold, Children: []Node{{Kind: NodeItalic, Children: children}}}, width, true
		}
		return Node{Kind: d.kind, Children: children}, width, true
	}
	return Node{}, 0, false
}

// underscoreBoundary reports whether an underscore pair sits on word edges.
func underscoreBoundary(s string, open, close int) bool {
	if open > 0 && isWordByte(s[open-1]) {
		return false
	}
	if close+1 < len(s) && isWordByte(s[close+1]) {
		return false
	}
	return true
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isEscapable(b byte) bool {
	switch b {
	case '*', '_', '~', '|', '`', '\\', '>', '#', '-', ':', '<', '@', '.':
		return true
	}
	return false
}

// indexUnescaped finds the next unescaped occurrence of needle at or after from.
func indexUnescaped(s string, from int, needle string) int {
	for i := from; i+len(needle) <= len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if strings.HasPrefix(s[i:], needle) {
			return i
		}
	}
	return -1
}

// parseAngle handles every construct written as <...>.
func (p *parser) parseAngle(s string) (Node, int, bool) {
	end := strings.IndexByte(s, '>')
	if end < 0 {
		return Node{}, 0, false
	}
	inner := s[1:end]
	width := end + 1

	switch {
	case strings.HasPrefix(inner, "@&") && isSnowflake(inner[2:]):
		name, color := p.res.roleInfo(inner[2:])
		return Node{Kind: NodeMention, MentionType: MentionRole, Text: name, RoleColor: color}, width, true
	case strings.HasPrefix(inner, "@!") && isSnowflake(inner[2:]):
		return p.userMention(inner[2:]), width, true
	case strings.HasPrefix(inner, "@") && isSnowflake(inner[1:]):
		return p.userMention(inner[1:]), width, true
	case strings.HasPrefix(inner, "#") && isSnowflake(inner[1:]):
		return Node{Kind: NodeMention, MentionType: MentionChannel, Text: p.res.channelName(inner[1:])}, width, true
	case strings.HasPrefix(inner, "a:") || strings.HasPrefix(inner, ":"):
		if node, ok := parseCustomEmoji(inner); ok {
			return node, width, true
		}
	case strings.HasPrefix(inner, "t:") || strings.HasPrefix(inner, "T:"):
		if node, ok := parseTimestamp(inner); ok {
			return node, width, true
		}
	case strings.HasPrefix(inner, "http://"), strings.HasPrefix(inner, "https://"):
		// <url> suppresses the embed; the text is still a link.
		return Node{Kind: NodeLink, URL: inner, Children: []Node{Text(inner)}}, width, true
	}
	return Node{}, 0, false
}

func (p *parser) userMention(id string) Node {
	return Node{Kind: NodeMention, MentionType: MentionUser, Text: p.res.userName(id)}
}

// parseCustomEmoji handles both <:name:id> and <a:name:id>.
func parseCustomEmoji(inner string) (Node, bool) {
	animated := false
	body := inner
	if strings.HasPrefix(body, "a:") {
		animated = true
		body = body[2:]
	} else {
		body = strings.TrimPrefix(body, ":")
	}
	colon := strings.LastIndexByte(body, ':')
	if colon <= 0 {
		return Node{}, false
	}
	name, id := body[:colon], body[colon+1:]
	if !isSnowflake(id) {
		return Node{}, false
	}
	ext := ".png"
	if animated {
		ext = ".gif"
	}
	return Node{
		Kind:     NodeEmoji,
		Text:     ":" + name + ":",
		EmojiURL: "https://cdn.discordapp.com/emojis/" + id + ext,
	}, true
}

// parseTimestamp handles <t:1700000000> and <t:1700000000:R>.
func parseTimestamp(inner string) (Node, bool) {
	parts := strings.Split(inner, ":")
	if len(parts) < 2 {
		return Node{}, false
	}
	seconds, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return Node{}, false
	}
	format := byte('f')
	if len(parts) > 2 && len(parts[2]) > 0 {
		format = parts[2][0]
	}
	return Node{Kind: NodeTimestamp, Timestamp: time.Unix(seconds, 0).UTC(), Format: format}, true
}

// parseBareURL linkifies a URL that is not wrapped in <>.
func parseBareURL(s string) (Node, int, bool) {
	var rest string
	switch {
	case strings.HasPrefix(s, "https://"):
		rest = s
	case strings.HasPrefix(s, "http://"):
		rest = s
	default:
		return Node{}, 0, false
	}
	end := len(rest)
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case ' ', '\n', '\t', '<', '>', '"':
			end = i
			i = len(rest)
		}
	}
	url := rest[:end]
	// Trim trailing punctuation that belongs to the sentence, not the URL.
	for len(url) > 0 {
		last := url[len(url)-1]
		if strings.ContainsRune(".,;:!?)]}'\"", rune(last)) {
			// Keep a closing paren when the URL itself opened one.
			if last == ')' && strings.Count(url, "(") > strings.Count(url, ")") {
				break
			}
			url = url[:len(url)-1]
			continue
		}
		break
	}
	if url == "" || !strings.Contains(strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://"), ".") {
		return Node{}, 0, false
	}
	return Node{Kind: NodeLink, URL: url, Children: []Node{Text(url)}}, len(url), true
}

// parseEveryoneHere recognises literal @everyone and @here.
func parseEveryoneHere(s string) (Node, int, bool) {
	if strings.HasPrefix(s, "@everyone") {
		return Node{Kind: NodeMention, MentionType: MentionEveryone, Text: "everyone"}, len("@everyone"), true
	}
	if strings.HasPrefix(s, "@here") {
		return Node{Kind: NodeMention, MentionType: MentionHere, Text: "here"}, len("@here"), true
	}
	return Node{}, 0, false
}

// isSnowflake reports whether s looks like a Discord ID.
func isSnowflake(s string) bool {
	if len(s) < 15 || len(s) > 21 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
