# discord-transcript-go

Turn Discord messages into a **single minimal HTML transcript** that renders in a
browser with nothing but a stylesheet, using
[`disgo`](https://github.com/disgoorg/disgo) for the Discord data.

Two things make that possible:

* **The renderer emits the full markup the stylesheet targets**, so avatars, author
  rows, badges, timestamps, replies, embeds, attachments and reactions render
  with **no JavaScript at all**. The optional enhancement script adds
  click-to-reveal spoilers, copy buttons and viewer-local timestamps — it
  rebuilds nothing, because the markup is already complete.
* **The output is minimal on purpose**: no comments, no indentation, no metadata,
  no config block. Everything in the file either renders or tells the browser how
  to render it.

The renderer also knows nothing about disgo. `transcript/` owns the model, the
markdown parser, the HTML renderer and the document writer and imports no Discord
library; `disgo/` is a thin adapter. Swapping libraries later costs one adapter.

## Install

```bash
go get github.com/x1xo/discord-transcript-go
```

Requires Go 1.24 or newer (disgo's floor).

## Quick start

```go
package main

import (
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

	// One file: a CDN stylesheet link plus base64 media.
	return tr.WriteFile("transcript.html")
}
```

`disgo/example_test.go` is a compiling example that needs no network.

## Stylesheet: defaults and overrides

The stylesheet comes from a CDN and is pinned with Subresource Integrity. The
defaults target the current `discord-transcript-ui` release:

```go
tr.WriteFile("transcript.html") // https://cdn.jsdelivr.net/npm/discord-transcript-ui@1.0.1/…, SRI checked
```

Point it anywhere — your own host, a mirror, another version — by passing the URL
and hash yourself:

```go
transcript.WithCSS("https://cdn.example.com/discord-transcript.min.css", "sha384-…")
transcript.WithScript("https://cdn.example.com/discord-transcript.min.js", "sha384-…")
transcript.WithAssets(transcript.Assets{ /* full control, including CrossOrigin */ })
transcript.WithoutStylesheet() // inject your own <link> yourself
transcript.WithoutScript()     // the default: no script tag at all
```

The relevant constants, if you want to build your own tags or a CSP:

| Constant | Value |
| --- | --- |
| `transcript.ContractVersion` | `1.0.1` — the markup contract this module emits |
| `transcript.DefaultCSSURL` | jsDelivr URL of the pinned stylesheet |
| `transcript.DefaultCSSIntegrity` | its `sha384-…` hash |
| `transcript.DefaultShortCSSURL` | the same stylesheet with compact tag names |
| `transcript.DefaultShortCSSIntegrity` | its `sha384-…` hash |
| `transcript.DefaultScriptURL` | jsDelivr URL of the optional script |
| `transcript.DefaultScriptIntegrity` | its `sha384-…` hash |

`AssetsWithScript()` returns the stylesheet and script together when you want the
interactive extras.

The pinned defaults point at a specific `discord-transcript-ui` release. If that
release is not on npm yet (or you would rather not depend on jsDelivr), pass your
own URL and hash with `WithCSS` — the markup contract is unchanged between minor
releases.

## Compact tag names (opt-in)

The default markup uses readable names (`<discord-message>`, `<discord-mention>`).
`-short-tags` (or `transcript.WithShortTags()`) switches to a compact vocabulary
and links the matching stylesheet automatically:

```go
tr.WriteFile("small.html", transcript.WithShortTags())
// <dms channel-name="general"><dm profile="…" data-dt-ready><div class="dt-msg">…
// … <dme type="user">piton</dme> <dsp>spoiler</dsp> <dc>code</dc>
```

The mapping is a straight rename of 40 element names, so nothing is lost and the
short stylesheet is the same size as the default one. Measured on a text-heavy
transcript it cuts **~21% of the raw markup** and **~5% of the gzipped markup** —
repeated long names compress well, so the win is real only for uncompressed files
on disk. `transcript.ShortTags()` returns the mapping if you want to emit short
markup yourself.

An explicit `WithCSS` always wins over the short default, so a self-hosted or
different-version stylesheet still works. This needs `discord-transcript-ui`
1.1.0 or newer, which is where the short stylesheet is published.

## Command line

```bash
# Default: one minimal file, stylesheet from the CDN, media inlined
discord-transcript -in messages.json -out transcript.html

# Your own CDN, or a different version
discord-transcript -in messages.json -css https://cdn.example.com/d.min.css \
  -css-integrity sha384-… -out transcript.html

# Add the optional enhancement script
discord-transcript -in messages.json -script -out interactive.html

# Keep the original (expiring) CDN URLs, or write media next to the HTML
discord-transcript -in messages.json -media url -out transcript.html
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

Other flags: `-theme dark|light`, `-media inline|dir|url`, `-max-media-bytes`,
`-profiles overrides.json`, `-self <user-id>`, `-timezone`, `-locale`,
`-no-replies`, `-header`, `-meta`, `-quiet`. Run `discord-transcript -h` for all of them.

## Headers

Three kinds, all working:

1. **Markdown headings** — `#`, `##` and `###` become `<discord-header level="1|2|3">`
   and are styled as Discord's heading sizes. This is on by default; the example
   transcript contains both an `h1` and an `h2`.
2. **Channel header** — `<discord-messages channel-name="general" channel-type="text">`
   renders a header with the `#`/channel-type prefix straight from the stylesheet,
   so it needs no JavaScript. It comes from the `channel` object in the JSON input
   or from `-channel`/`-channel-type`.
3. **Page header** — opt-in with `-header` (or `transcript.WithPageHeader()`), which
   adds `<h1 class="dt-page__title">` above the conversation; pair it with `-meta`
   for the message count and date range. It is off by default because the channel
   header above already shows the name, and minimal output is the point.

## Media: why base64, and when not to

Discord serves attachments from **signed URLs that expire within hours**, so a
transcript that keeps them renders broken images the next day. The default store
downloads each URL once and rewrites it to a `data:` URI:

| Store | Result | Use when |
| --- | --- | --- |
| `transcript.InlineMedia()` *(default)* | base64 `data:` URIs inside the HTML | Archiving. One file, works forever, no network. Costs ~33% size overhead and is not shared between transcripts. |
| `transcript.DirMedia(dir, base)` | files in a directory plus relative paths | Serving transcripts from your own host, or when size matters. |
| `transcript.URLMedia()` | the original URLs | Quick previews; anything already re-hosted somewhere durable. |
| your own `transcript.MediaStore` | whatever you want | Re-hosting to S3/R2, signing, caching, deduplication. |

Custom downloaders work too: `transcript.InlineMediaWith(fetcher)` accepts any
`transcript.Fetcher`. Downloads are capped by `-max-media-bytes` (8 MiB default)
and a 30-second timeout, and **a failed download is never fatal** — the renderer
keeps the original URL and reports it through `transcript.WithWarn`.

Only group-start messages embed their avatar, so a repeated author costs its
avatar bytes once. Authors with no usable avatar URL get a coloured initial
instead of a network request.

## What renders

Discord markdown: `**bold**`, `*italic*`, `__underline__`, `~~strikethrough~~`,
`||spoilers||`, `` `inline code` ``, fenced code blocks with a language label,
`> quotes` and `>>>` quotes, `-`/`1.` lists, `#`/`##`/`###` headings, `-#` subtext,
bare URLs, `<https://…>`, user/role/channel mentions, `@everyone`/`@here`, custom
emoji (including animated) and `<t:…>` timestamps in every format flag.

Messages: author identity (nickname, guild avatar, role colour), timestamps,
`(edited)`, replies, embeds (provider, author, title, description, fields, footer,
image, video, thumbnail), attachments (image, video, audio, file, spoilers),
reactions, system messages (`MessageType` → join/leave/call/boost/edit/pin/thread),
and continuation grouping computed at render time.

Not yet: threads, buttons and select menus, stickers (kept as `[sticker: name]` so
nothing is lost), polls, forwarded snapshots and slash-command rows. The model and
renderer understand threads; the adapter does not map them yet.

## Identity resolution

Nicknames and role colours come from, in order:

1. explicit overrides (`disgo.WithUsers`, `disgo.WithRoles`, `-profiles`);
2. the data carried by the message itself (`member`, `mentions`, `mention_channels`);
3. disgo's caches (`disgo.WithCaches(client.Caches)`).

**Gotcha:** disgo's caches are no-ops unless they were created with the right flags:

```go
caches := cache.New(cache.WithCaches(
	cache.FlagMembers, cache.FlagRoles, cache.FlagChannels, cache.FlagGuilds,
))
```

Without them you get usernames and default avatars, but no nicknames and no role
colours. Offline exports should prefer overrides, which need no cache.

Mentions that cannot be resolved keep their raw ID rather than vanishing, because
a transcript is a record of what was said.

## Architecture

```
transcript/            no Discord dependency
  model.go             the IR: Transcript, Message, Author, Embed, Attachment, …
  markdown.go          Discord markdown -> node tree
  render.go            node tree -> the complete static markup the stylesheet targets
  grouping.go          continuation rows (same author, close in time)
  document.go          the minimal document: doctype, charset, viewport, stylesheet, body
  assets.go            CDN URL and SRI defaults, and the override surface
  media.go             MediaStore: inline (base64) / dir / url, plus custom Fetcher
  identity.go          Resolver interfaces + map-backed implementations
disgo/                 the adapter: []discord.Message -> IR, cache-backed resolver
cmd/discord-transcript JSON in, minimal HTML out
```

## Where this module lives

It is consumed as a submodule of
[`transcripts`](https://github.com/x1xo/transcripts), which owns the stylesheet:

```bash
git clone --recurse-submodules https://github.com/x1xo/transcripts.git
```

The module path matches this repository's root, so `go get
github.com/x1xo/discord-transcript-go` works directly, with or without the
submodule checked out.

## Generated files

Two files in `transcript/` are written by the UI repository's build and must not
be edited by hand:

| File | Source |
| --- | --- |
| `transcript/pins.go` | `dist/manifest.json` — the contract version, CDN URLs and SRI hashes |
| `transcript/tags_gen.go` | `build/short-tags.mjs` — the compact tag mapping |

Run `npm run build` in `discord-transcript-ui` after changing it: that refreshes
both files here (and fails the UI's own `build:check` if they are stale).
`TestPinsMatchLocalUIBuild` catches a hand edit or a rebuild the sync missed, and
skips when the UI repository is not checked out beside this module. The generated
files carry a `DO NOT EDIT` header, and `TestGeneratedFilesAreMarkedGenerated`
keeps it there.

## Development

```bash
go test ./...                # parser, renderer, grouping, media, adapter, escaping
make verify                  # tests + real-browser check of both examples (needs Chrome)
node scripts/verify-browser.mjs examples/transcript.html
```

`scripts/verify-browser.mjs` loads a generated transcript in headless Chrome and
asserts the stylesheet alone renders it: every message has its layout, avatar slot,
author row, role colour and timestamp; inlined images decode; no media URI leaks
into visible text; the verified-bot tag appears; nothing overflows; and the console
stays clean. When the script is included, it also asserts the script does not
duplicate the existing markup. It catches the class of bug string tests cannot.

## Licence

MIT. The markup contract and stylesheet belong to
[discord-transcript-ui](https://github.com/x1xo/transcripts); `disgo` is MIT and
Copyright its authors.
