# discord-transcript-go

Turn Discord messages into a **single self-contained HTML transcript**, using
[`discord-transcript-ui`](https://github.com/x1xo/transcripts) for the look and
[`disgo`](https://github.com/disgoorg/disgo) as the Discord library.

Two design decisions do most of the work:

* **The renderer knows nothing about disgo.** `transcript/` owns the model, the
  Discord markdown parser, the HTML renderer and the document writer, and imports
  no Discord library at all. `disgo/` is a thin adapter. Swapping to discordgo or
  raw JSON later costs one adapter, not a rewrite.
* **A transcript is one file.** Media is downloaded and inlined as base64 by
  default, so the HTML keeps rendering after Discord's signed CDN URLs expire —
  which is the failure mode that quietly kills archived transcripts.

## Install

```bash
go get github.com/x1xo/discord-transcript-go
```

Requires Go 1.24 or newer (disgo's floor).

## Quick start

```go
package main

import (
	"log"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/snowflake/v2"

	"github.com/x1xo/discord-transcript-go/disgo"
	"github.com/x1xo/discord-transcript-go/transcript"
)

func exportChannel(client *bot.Client, channelID snowflake.ID) error {
	// disgo's REST client pages by snowflake; 0 means "unset" for the cursors.
	messages, err := client.Rest.GetMessages(channelID, 0, 0, 0, 100)
	if err != nil {
		return err
	}
	reverse(messages) // oldest first, which is how a transcript reads

	channel, ok := client.Caches.Channel(channelID)
	if !ok {
		return err
	}

	adapter := disgo.New(disgo.WithCaches(client.Caches))
	tr := adapter.Transcript(adapter.Channel(channel), messages)
	tr.Title = "general — 2024-03-15"

	// One file: stylesheet reference, profile map, conversation, base64 media.
	return tr.WriteFile("transcript.html", transcript.WithMedia(transcript.InlineMedia()))
}

func reverse(messages []discord.Message) { /* swap the slice in place */ }
```

`transcript.WithMedia` is optional — base64 inlining is already the default. See
`disgo/example_test.go` for a compiling example that needs no network.

## Command line

```bash
# From a JSON export (raw Discord API payloads work as-is)
discord-transcript -in messages.json -out transcript.html

# From stdin, with channel metadata and author overrides
cat export.json | discord-transcript -in - -out - > transcript.html

# Keep the original (expiring) CDN URLs instead of inlining
discord-transcript -in messages.json -media url -out transcript.html

# Ship HTML plus an assets folder instead of one big file
discord-transcript -in messages.json -media dir -media-dir assets -out out/
```

Two input shapes are accepted. A bare array is a list of messages:

```json
[{ "id": "…", "timestamp": "2024-03-15T14:28:00Z", "author": { "id": "…", "username": "piton" }, "content": "hello" }]
```

A richer object adds channel metadata and author overrides, which is what an
offline export needs when no cache is available:

```json
{
	"channel": { "name": "general", "type": "text", "guild": "Test Server" },
	"profiles": {
		"100000000000000001": { "author": "piton", "avatar": "https://…", "roleColor": "#57f287" },
		"100000000000000002": { "author": "miona", "bot": true, "verified": true }
	},
	"messages": [ … ]
}
```

Useful flags: `-assets cdn|local|inline`, `-theme dark|light`, `-media inline|dir|url`,
`-max-media-bytes`, `-profiles overrides.json`, `-self <user-id>`, `-timezone`,
`-locale`, `-no-replies`, `-no-meta`. Run `discord-transcript -h` for all of them.

## Media: why base64, and when not to

Discord serves attachments from **signed URLs that expire within hours**. A
transcript that keeps those URLs renders broken images the next day. Avatars and
emoji URLs are stable, but they still require the reader to have network access to
Discord's CDN.

The default `InlineMedia` store downloads each URL once and rewrites it to a
`data:` URI:

| Store | Result | Use when |
| --- | --- | --- |
| `transcript.InlineMedia()` *(default)* | base64 `data:` URIs inside the HTML | Archiving. One file, works forever, no network. Costs ~33% size overhead and is not shared between transcripts. |
| `transcript.DirMedia(dir, base)` | files in a directory plus relative paths | Serving transcripts from your own host, or when size matters. |
| `transcript.URLMedia()` | the original URLs | Quick previews; anything already re-hosted somewhere durable. |
| your own `transcript.MediaStore` | whatever you want | Re-hosting to S3/R2, signing, caching, deduplication. |

Custom downloaders are supported too: `transcript.InlineMediaWith(fetcher)` takes
any `transcript.Fetcher`, so you can plug in your own HTTP client, proxy or cache.
Downloads are bounded by `-max-media-bytes` (8 MiB by default) and a 30-second
timeout; **a failed download is never fatal** — the renderer keeps the original URL
and reports the problem through `transcript.WithWarn`.

## What renders

Discord markdown: `**bold**`, `*italic*`, `__underline__`, `~~strikethrough~~`,
`||spoilers||`, `` `inline code` ``, fenced code blocks with a language label,
`> quotes` and `>>>` quotes, `-`/`1.` lists, `#`/`##`/`###` headings, `-#` subtext,
bare URLs, `<https://…>`, user/role/channel mentions, `@everyone`/`@here`, custom
emoji (including animated) and `<t:…>` timestamps in every format flag.

Messages: author identity (nickname, guild avatar, role colour), timestamps,
`(edited)`, replies, embeds (provider, author, title, description, fields, footer,
image, video, thumbnail), attachments (image, video, audio, file, spoilers), reactions,
system messages (`MessageType` → join/leave/call/boost/edit/pin/thread).

Not yet: threads, buttons and select menus, stickers (kept as text so nothing is
lost), polls, forwarded message snapshots and slash-command rows. The model and
renderer already understand threads; the adapter does not map them yet.

## Identity resolution

Nicknames and role colours come from wherever they are available, in this order:

1. explicit overrides (`disgo.WithUsers`, `disgo.WithRoles`, `-profiles`);
2. the data carried by the message itself (`member`, `mentions`, `mention_channels`);
3. disgo's caches (`disgo.WithCaches(client.Caches)`).

**Gotcha:** disgo's caches are no-ops unless they were created with the right flags.
`cache.New()` on its own returns nothing:

```go
caches := cache.New(cache.WithCaches(
	cache.FlagMembers, cache.FlagRoles, cache.FlagChannels, cache.FlagGuilds,
))
```

Without them you get usernames and default avatars, but no nicknames and no role
colours. Offline exports should prefer overrides, which need no cache.

Mentions that cannot be resolved keep their raw ID (`<discord-mention>222…</…>`)
rather than vanishing, because a transcript is a record of what was said.

## Architecture

```
transcript/            no Discord dependency
  model.go             the IR: Transcript, Message, Author, Embed, Attachment, …
  markdown.go          Discord markdown -> node tree
  render.go            node tree -> discord-transcript-ui HTML fragments
  document.go          the complete HTML document (head, config, recovery comment)
  media.go             MediaStore: inline (base64) / dir / url, plus custom Fetcher
  identity.go          Resolver interfaces + map-backed implementations
  assets.go            embedded copies of the JS library's build artifacts
disgo/                 the adapter: []discord.Message -> IR, cache-backed resolver
cmd/discord-transcript JSON in, self-contained HTML out
```

The generated document links the pinned stylesheet and script (CDN with a
multi-mirror fallback chain and Subresource Integrity, or local paths, or inlined)
and stamps in a recovery comment with the exact URLs, sha256 hashes and mirrors.

### Keeping the embedded assets in sync

`transcript/assets/` holds copies of the JavaScript library's build output so this
module builds on its own. After rebuilding that library:

```bash
go generate ./...            # copies ../dist and ../assets
# or, if the UI repo lives elsewhere:
DISCORD_TRANSCRIPT_UI_DIR=/path/to/transcripts go generate ./...
```

The module warns (through `WithWarn`) if the embedded assets' version does not
match the markup contract version it targets.

## Development

```bash
go test ./...                                  # parser, renderer, media, adapter
make verify                                    # tests + real-browser check (needs Chrome)
node scripts/verify-browser.mjs examples/transcript.html
go run ./cmd/discord-transcript -in testdata/export.json -out examples/transcript.html
```

`scripts/verify-browser.mjs` loads a generated transcript in headless Chrome and
asserts the enhancement script upgraded every message, inlined images decoded, no
media URI leaked into visible text, the verified-bot tag rendered, spoilers toggle,
nothing overflows and the console stayed clean. Run it after changing the renderer:
it catches the class of bug that unit tests on strings cannot.

## Licence

MIT. The markup contract, stylesheet and enhancement script belong to
[discord-transcript-ui](https://github.com/x1xo/transcripts); `disgo` is MIT and
Copyright its authors.
