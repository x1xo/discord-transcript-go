package transcript

import (
	"strings"
	"testing"
	"time"
)

// dump renders a node tree to a compact, readable string for assertions.
func dump(nodes []Node) string {
	var b strings.Builder
	for _, n := range nodes {
		dumpNode(&b, n)
	}
	return b.String()
}

func dumpNode(b *strings.Builder, n Node) {
	switch n.Kind {
	case NodeText:
		b.WriteString("text(" + n.Text + ")")
		return
	case NodeLineBreak:
		b.WriteString("br")
		return
	case NodeCode:
		b.WriteString("code(" + n.Text + ")")
		return
	case NodeCodeBlock:
		b.WriteString("codeblock(" + n.Language + ":" + n.Text + ")")
		return
	case NodeLink:
		b.WriteString("link(" + n.URL + ")")
	case NodeMention:
		b.WriteString("mention(" + string(n.MentionType) + ":" + n.Text)
		if n.RoleColor != "" {
			b.WriteString(":" + n.RoleColor)
		}
		b.WriteString(")")
		return
	case NodeEmoji:
		b.WriteString("emoji(" + n.Text + ":" + n.EmojiURL + ")")
		return
	case NodeTimestamp:
		b.WriteString("ts(" + itoa64(n.Timestamp.Unix()) + ":" + string(n.Format) + ")")
		return
	case NodeHeading:
		b.WriteString("h" + itoa(n.Level))
	case NodeList:
		if n.Ordered {
			b.WriteString("ol(" + itoa(n.Start) + ")")
		} else {
			b.WriteString("ul")
		}
	case NodeListItem:
		b.WriteString("li")
	default:
		b.WriteString(kindName(n.Kind))
	}
	b.WriteString("[")
	for _, child := range n.Children {
		dumpNode(b, child)
	}
	b.WriteString("]")
}

func kindName(k NodeKind) string {
	switch k {
	case NodeBold:
		return "bold"
	case NodeItalic:
		return "italic"
	case NodeUnderline:
		return "underline"
	case NodeStrikethrough:
		return "strike"
	case NodeSpoiler:
		return "spoiler"
	case NodeQuote:
		return "quote"
	case NodeSubtext:
		return "subtext"
	}
	return "?"
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestParseInline(t *testing.T) {
	res := Resolvers{
		Users:    Profiles{"111111111111111111": {Key: "1", Name: "piton"}},
		Channels: Channels{"222222222222222222": "general"},
		Roles:    Roles{"333333333333333333": {Name: "Moderator", Color: "#57f287"}},
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello world", "text(hello world)"},
		{"bold", "a **b** c", "text(a )bold[text(b)]text( c)"},
		{"italic star", "*i*", "italic[text(i)]"},
		{"italic underscore", "_i_", "italic[text(i)]"},
		{"bold italic", "***both***", "bold[italic[text(both)]]"},
		{"underline", "__u__", "underline[text(u)]"},
		{"strike", "~~s~~", "strike[text(s)]"},
		{"spoiler", "||s||", "spoiler[text(s)]"},
		{"nested", "**bold *and italic* end**", "bold[text(bold )italic[text(and italic)]text( end)]"},
		{"inline code is literal", "`**not bold**`", "code(**not bold**)"},
		{"escape asterisk", `\*literal\*`, "text(*literal*)"},
		{"escape backtick", "\\`x\\`", "text(`x`)"},
		{"snake case survives", "some_variable_name", "text(some_variable_name)"},
		{"underscore mid word", "foo_bar_baz", "text(foo_bar_baz)"},
		{"user mention", "<@111111111111111111>", "mention(user:piton)"},
		{"user mention with bang", "<@!111111111111111111>", "mention(user:piton)"},
		{"unknown user mention", "<@999999999999999999>", "mention(user:999999999999999999)"},
		{"role mention", "<@&333333333333333333>", "mention(role:Moderator:#57f287)"},
		{"channel mention", "<#222222222222222222>", "mention(channel:general)"},
		{"everyone", "hi @everyone", "text(hi )mention(everyone:everyone)"},
		{"here", "@here", "mention(here:here)"},
		{"custom emoji", "<:party:444444444444444444>", "emoji(:party::https://cdn.discordapp.com/emojis/444444444444444444.png)"},
		{"animated emoji", "<a:spin:444444444444444444>", "emoji(:spin::https://cdn.discordapp.com/emojis/444444444444444444.gif)"},
		{"timestamp default", "<t:1700000000>", "ts(1700000000:f)"},
		{"timestamp relative", "<t:1700000000:R>", "ts(1700000000:R)"},
		{"suppressed link", "<https://example.com/a>", "link(https://example.com/a)[text(https://example.com/a)]"},
		{"bare link", "see https://example.com/x now", "text(see )link(https://example.com/x)[text(https://example.com/x)]text( now)"},
		{"bare link trailing dot", "https://example.com/x.", "link(https://example.com/x)[text(https://example.com/x)]text(.)"},
		// Discord has no markdown links: the syntax stays literal, but the bare
		// URL inside is still auto-linked.
		{"markdown link syntax is literal, url still linked", "[x](https://example.com)",
			"text([x]()link(https://example.com)[text(https://example.com)]text())"},
		{"unmatched delimiter", "**open", "text(**open)"},
		{"empty delimiter", "****", "text(****)"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := dump(ParseContentWith(tc.in, res))
			if got != tc.want {
				t.Errorf("ParseContentWith(%q)\n got: %s\nwant: %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseBlocks(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"code block", "```go\nfmt.Println()\n```", "codeblock(go:fmt.Println())"},
		{"code block no language", "```\nx\n```", "codeblock(:x)"},
		{"unterminated code block", "```go\nx", "codeblock(go:x)"},
		{"quote", "> one\n> two", "quote[text(one)brtext(two)]"},
		{"multiline quote", ">>> one\ntwo", "quote[text(one)brtext(two)]"},
		{"unordered list", "- a\n- b", "ul[li[text(a)]li[text(b)]]"},
		{"ordered list", "3. c\n4. d", "ol(3)[li[text(c)]li[text(d)]]"},
		{"heading one", "# Title", "h1[text(Title)]"},
		{"heading three", "### Small", "h3[text(Small)]"},
		{"not a heading", "#nospace", "text(#nospace)"},
		{"subtext", "-# small print", "subtext[text(small print)]"},
		{"line breaks", "a\nb", "text(a)brtext(b)"},
		{"blank line", "a\n\nb", "text(a)brbrtext(b)"},
		{"leading and trailing blank lines", "\n\na\n\n", "text(a)"},
		{"block then inline", "```\ncode\n```\nafter", "codeblock(:code)text(after)"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := dump(ParseContent(tc.in))
			if got != tc.want {
				t.Errorf("ParseContent(%q)\n got: %s\nwant: %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseTerminatesOnHostileInput(t *testing.T) {
	// These would loop forever if a construct could match with zero width.
	inputs := []string{
		"<", "<<>>", "<@", "<@>", "<:>", "<a:>", "<t:>", "<t:x>", "``", "`", "**", "***",
		"__", "~~", "||", "\\", "@", "@e", "http://", "https://.", "<:", "<a::", ">>>", ">",
		"- ", "1. ", "#", "##", "###", "####", "\n\n\n", strings.Repeat("*", 64),
	}
	for _, in := range inputs {
		done := make(chan string, 1)
		go func(s string) { done <- dump(ParseContent(s)) }(in)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("ParseContent(%q) did not terminate", in)
		}
	}
}
