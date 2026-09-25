// Command discord-transcript turns a JSON export of Discord messages into a
// single self-contained HTML transcript.
//
// Two input shapes are accepted:
//
//	[
//	  { "id": "…", "content": "hello", "timestamp": "2024-03-15T14:28:00Z", "author": {…} }
//	]
//
//	{
//	  "channel": { "name": "general", "type": "text", "guild": "My Server" },
//	  "profiles": { "111111111111111111": { "author": "piton", "roleColor": "#57f287" } },
//	  "messages": [ … ]
//	}
//
// The second form carries channel metadata and author overrides, which is what an
// offline export needs when no cache is available. Messages are decoded with
// disgo's types, so raw Discord API payloads work as-is.
//
// Examples:
//
//	discord-transcript -in messages.json -out transcript.html
//	cat messages.json | discord-transcript -in - -out - > transcript.html
//	discord-transcript -in messages.json -channel general -media dir -media-dir assets -out out/
//	discord-transcript -in messages.json -css https://cdn.example.com/discord-transcript.min.css -out t.html
//	discord-transcript -in messages.json -script -out interactive.html
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/x1xo/discord-transcript-go/disgo"
	"github.com/x1xo/discord-transcript-go/transcript"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "discord-transcript:", err)
		os.Exit(1)
	}
}

// export is the richer input shape.
type export struct {
	Channel  channelInfo                `json:"channel"`
	Profiles map[string]profileOverride `json:"profiles"`
	Messages []discord.Message          `json:"messages"`
}

type channelInfo struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Guild string `json:"guild"`
	ID    string `json:"id"`
}

type profileOverride struct {
	Author      string `json:"author"`
	Avatar      string `json:"avatar"`
	RoleColor   string `json:"roleColor"`
	Bot         bool   `json:"bot"`
	Verified    bool   `json:"verified"`
	Server      bool   `json:"server"`
	OfficialApp bool   `json:"officialApp"`
	OP          bool   `json:"op"`
}

func run() error {
	var (
		inPath       = flag.String("in", "-", `input JSON file, or "-" for stdin`)
		outPath      = flag.String("out", "transcript.html", `output HTML file, or "-" for stdout`)
		title        = flag.String("title", "", "document title (defaults to the channel name)")
		channelName  = flag.String("channel", "", "channel name, when the input does not carry one")
		channelType  = flag.String("channel-type", "text", "text|voice|thread|forum|locked")
		guild        = flag.String("guild", "", "guild name, shown in the page metadata")
		theme        = flag.String("theme", "dark", "dark|light")
		cssURL       = flag.String("css", transcript.DefaultCSSURL, "stylesheet URL (empty to omit the stylesheet)")
		cssSRI       = flag.String("css-integrity", transcript.DefaultCSSIntegrity, "stylesheet SRI hash (empty to omit)")
		withScript   = flag.Bool("script", false, "include the optional enhancement script")
		scriptURL    = flag.String("script-url", transcript.DefaultScriptURL, "enhancement script URL")
		scriptSRI    = flag.String("script-integrity", transcript.DefaultScriptIntegrity, "enhancement script SRI hash")
		media        = flag.String("media", "inline", "inline|dir|url")
		mediaDir     = flag.String("media-dir", "assets", "directory for -media dir")
		mediaBase    = flag.String("media-base", "assets/", "URL prefix for -media dir")
		maxMedia     = flag.Int64("max-media-bytes", transcript.DefaultMaxMediaBytes, "per-file download cap")
		locale       = flag.String("locale", "en-US", "locale for fallback timestamp text")
		timeZone     = flag.String("timezone", "UTC", "IANA zone for fallback timestamp text")
		self         = flag.String("self", "", "self user ID; messages mentioning it are highlighted")
		profilesPath = flag.String("profiles", "", "JSON file with author overrides")
		noReplies    = flag.Bool("no-replies", false, "drop reply previews")
		header       = flag.Bool("header", false, "add a page-level <h1> with the title")
		meta         = flag.Bool("meta", false, "add a metadata line (message count, date range)")
		quiet        = flag.Bool("quiet", false, "suppress the summary on stderr")
	)
	flag.Parse()

	raw, err := readInput(*inPath)
	if err != nil {
		return err
	}
	messages, channel, overrides, err := decode(raw)
	if err != nil {
		return err
	}

	if *channelName != "" {
		channel.Name = *channelName
	}
	if *guild != "" {
		channel.Guild = *guild
	}
	if *channelType != "" {
		channel.Type = transcript.ChannelType(*channelType)
	}
	if channel.Type == "" {
		channel.Type = transcript.ChannelText
	}
	switch channel.Type {
	case transcript.ChannelText, transcript.ChannelVoice, transcript.ChannelThread,
		transcript.ChannelForum, transcript.ChannelLocked:
	default:
		return fmt.Errorf("invalid -channel-type %q", channel.Type)
	}

	if *profilesPath != "" {
		extra, err := readProfiles(*profilesPath)
		if err != nil {
			return err
		}
		overrides = mergeProfiles(overrides, extra)
	}

	opts := []disgo.Option{disgo.WithUsers(overrides)}
	if *self != "" {
		id, err := snowflake.Parse(*self)
		if err != nil {
			return fmt.Errorf("invalid -self %q: %w", *self, err)
		}
		opts = append(opts, disgo.WithSelfUserID(id))
	}
	if *noReplies {
		opts = append(opts, disgo.WithoutReplies())
	}
	adapter := disgo.New(opts...)

	tr := adapter.Transcript(channel, messages)
	tr.Title = *title
	tr.GeneratedAt = time.Now()
	tr.Generator = "discord-transcript " + transcript.Version

	loc, err := time.LoadLocation(*timeZone)
	if err != nil {
		return fmt.Errorf("invalid -timezone %q: %w", *timeZone, err)
	}

	assets := transcript.Assets{
		CSSURL:       *cssURL,
		CSSIntegrity: *cssSRI,
		CrossOrigin:  true,
	}
	if *withScript {
		assets.ScriptURL = *scriptURL
		assets.ScriptIntegrity = *scriptSRI
	}

	renderOpts := []transcript.Option{
		transcript.WithTheme(transcript.Theme(*theme)),
		transcript.WithAssets(assets),
		transcript.WithLocale(*locale),
		transcript.WithTimeZone(loc),
		transcript.WithMaxMediaBytes(*maxMedia),
		transcript.WithWarn(func(err error) {
			if !*quiet {
				fmt.Fprintln(os.Stderr, "warning:", err)
			}
		}),
	}
	switch *media {
	case "inline":
		renderOpts = append(renderOpts, transcript.WithMedia(transcript.InlineMedia()))
	case "dir":
		renderOpts = append(renderOpts, transcript.WithMedia(transcript.DirMedia(*mediaDir, *mediaBase)))
	case "url":
		renderOpts = append(renderOpts, transcript.WithMedia(transcript.URLMedia()))
	default:
		return fmt.Errorf("invalid -media %q (want inline, dir or url)", *media)
	}
	if *header {
		renderOpts = append(renderOpts, transcript.WithPageHeader())
	}
	if *meta {
		renderOpts = append(renderOpts, transcript.WithMeta())
	}

	out, err := tr.HTML(renderOpts...)
	if err != nil {
		return err
	}

	if *outPath == "-" {
		if _, err := os.Stdout.Write(out); err != nil {
			return err
		}
	} else if err := writeFile(*outPath, out); err != nil {
		return err
	}

	if !*quiet {
		fmt.Fprintf(os.Stderr, "%d messages, %d bytes → %s\n", len(tr.Messages), len(out), *outPath)
	}
	return nil
}

func readInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	return data, nil
}

// decode accepts either the rich export shape or a bare array of messages.
func decode(raw []byte) ([]discord.Message, transcript.Channel, transcript.Profiles, error) {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		var messages []discord.Message
		if err := json.Unmarshal(raw, &messages); err != nil {
			return nil, transcript.Channel{}, nil, fmt.Errorf("decode message array: %w", err)
		}
		return messages, transcript.Channel{}, nil, nil
	}

	var exp export
	if err := json.Unmarshal(raw, &exp); err != nil {
		return nil, transcript.Channel{}, nil, fmt.Errorf("decode export: %w", err)
	}
	channel := transcript.Channel{
		Name:  exp.Channel.Name,
		Type:  transcript.ChannelType(exp.Channel.Type),
		Guild: exp.Channel.Guild,
		ID:    exp.Channel.ID,
	}
	return exp.Messages, channel, profilesFromOverrides(exp.Profiles), nil
}

func readProfiles(path string) (transcript.Profiles, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profiles: %w", err)
	}
	var overrides map[string]profileOverride
	if err := json.Unmarshal(data, &overrides); err != nil {
		return nil, fmt.Errorf("decode profiles: %w", err)
	}
	return profilesFromOverrides(overrides), nil
}

func profilesFromOverrides(overrides map[string]profileOverride) transcript.Profiles {
	if len(overrides) == 0 {
		return nil
	}
	out := make(transcript.Profiles, len(overrides))
	for id, o := range overrides {
		out[id] = transcript.Author{
			Key:       id,
			Name:      o.Author,
			AvatarURL: o.Avatar,
			RoleColor: o.RoleColor,
			Bot:       o.Bot,
			Verified:  o.Verified,
			Server:    o.Server,
			Official:  o.OfficialApp,
			OP:        o.OP,
		}
	}
	return out
}

func mergeProfiles(base, extra transcript.Profiles) transcript.Profiles {
	if base == nil {
		return extra
	}
	for id, author := range extra {
		base[id] = author
	}
	return base
}

func writeFile(path string, data []byte) error {
	if dir := strings.TrimSuffix(path, "/"); strings.HasSuffix(path, "/") || dir == "" {
		path = dir + "/transcript.html"
	}
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func dirOf(path string) string {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[:i]
	}
	return "."
}
