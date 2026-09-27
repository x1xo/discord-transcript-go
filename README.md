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
tr.WriteFile("transcript.html") // the pinned jsDelivr stylesheet, SRI checked
```

The enhancement script is a second, optional file. `transcript.WithScript()`
adds it with the pinned URL and hash, which the generated pins refresh whenever
this module is rebuilt against a new `discord-transcript-ui` release:

```go
tr.WriteFile("transcript.html", transcript.WithScript())
```

Point either file anywhere — your own host, a mirror, another version — by
passing the URL and hash yourself:

```go
transcript.WithCSS("https://cdn.example.com/discord-transcript.min.css", "sha384-…")
transcript.WithScriptURL("https://cdn.example.com/discord-transcript.min.js", "sha384-…")
transcript.WithAssets(transcript.Assets{ /* full control, including CrossOrigin */ })
transcript.WithoutStylesheet() // inject your own <link> yourself
transcript.WithoutScript()     // the default: no script tag at all
```

The relevant constants, if you want to build your own tags or a CSP:

| Constant | Value |
| --- | --- |
| `transcript.ContractVersion` | `1.1.2` — the markup contract this module emits |
| `transcript.DefaultCSSURL` | jsDelivr URL of the pinned stylesheet |
| `transcript.DefaultCSSIntegrity` | its `sha384-…` hash |
| `transcript.DefaultShortCSSURL` | the same stylesheet with compact tag names |
| `transcript.DefaultShortCSSIntegrity` | its `sha384-…` hash |
| `transcript.DefaultScriptURL` | jsDelivr URL of the optional script |
| `transcript.DefaultScriptIntegrity` | its `sha384-…` hash |

`AssetsWithScript()` returns the stylesheet and script together when you want the
interactive extras as one `Assets` value; `WithScript()` is the same script on its
own, which composes with `WithCSS` and `WithShortTags` in any order.

The pinned defaults point at a specific `discord-transcript-ui` release. If that
release is not on npm yet (or you would rather not depend on jsDelivr), pass your
own URL and hash with `WithCSS` — the markup contract is unchanged between minor
releases.

This module versions on its own; `CHANGELOG.md` records what each release emits,
and `transcript.Version` is what a download reports in its `User-Agent`.

## Compact tag names (opt-in)

The default markup uses readable names (`<discord-message>`, `<discord-mention>`).
`transcript.WithShortTags()` switches to a compact vocabulary
and links the matching stylesheet automatically:

```go
tr.WriteFile("small.html", transcript.WithShortTags())
// <dms channel-name="general"><dm profile="…" data-dt-r><div class="dt-msg">…
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

## Examples

`examples/` holds four committed transcripts — the default, one with the
optional script, one with the compact tag names, and one whose embeds carry
styled text (`transcript-embeds.html`) — which `scripts/verify-browser.mjs`
renders in a real browser. Regenerate them after a renderer change:

```bash
make example      # or: go test ./transcript -run TestGenerateExamples -update
```

They use inline media, so they are deterministic and need no network; the
download path, proxies included, is covered by `media_test.go`.

`examples/third-party-markup.html` and `examples/third-party-markup-enhanced.html`
are hand-written: the same transcript twice, in the shape a skyra-style parser
emits rather than the shape this renderer emits — a bare multi-line description,
media and the footer marked with `slot` attributes, and Discord's decimal
`color` attribute. The first loads the stylesheet only, the second adds the
script. They exist so embed layout is exercised against markup the renderer did
not generate; open either in a browser, or point the check at it:

```bash
node scripts/verify-browser.mjs examples/transcript.html --css ../dist/discord-transcript.min.css
```

## Media: why base64, and when not to

Discord serves attachments from **signed URLs that expire within hours**, so a
transcript that keeps them renders broken images the next day. The default store
downloads each URL once and rewrites it to a `data:` URI:

| Store | Result | Use when |
| --- | --- | --- |
| `transcript.InlineMedia()` *(default)* | base64 `data:` URIs inside the HTML | Archiving. One file, works forever, no network. Costs ~33% size overhead; repeated images are stored once and right-sized (see below). |
| `transcript.DirMedia(dir, base)` | files in a directory plus relative paths | Serving transcripts from your own host, or when size matters. |
| `transcript.URLMedia()` | the original URLs | Quick previews; anything already re-hosted somewhere durable. |
| your own `transcript.MediaStore` | whatever you want | Re-hosting to S3/R2, signing, caching, deduplication. |

Custom downloaders work too: `transcript.InlineMediaWith(fetcher)` accepts any
`transcript.Fetcher`. Downloads are capped by `transcript.WithMaxMediaBytes`
(8 MiB default) and a 30-second timeout, and **a failed download is never
fatal** — the renderer keeps the original URL and reports it through
`transcript.WithWarn`.

Only group-start messages embed their avatar, so a repeated author costs its
avatar bytes once. Authors with no usable avatar URL get a coloured initial
instead of a network request.

### Making the file smaller

Avatars and icons are what make an inlined transcript big: Discord serves a
128–1024px image for something drawn at 20–32 CSS pixels, and the same avatar
would otherwise be copied into the file once per message. Both are handled for
you, and neither needs a script or a newer stylesheet:

* **Right-sizing.** Every media request carries the size it is drawn at
  (`MediaRef.TargetEdge`, twice the CSS size): the URL handed to your store
  already names it (`…/avatars/1/abc.png?size=64`), and anything that comes back
  inline still too large is downscaled in Go. It never grows a file: an image
  that is already small enough, animated, vector, or in a format the standard
  library cannot decode keeps its original bytes.
* **One copy.** A blob that appears more than once — an avatar in twenty
  messages, the same icon in an embed — is stored once in a
  `<style data-dt-media-pool>` block and every use becomes
  `<span class="dt-media dt-media-1">`. Rules and references live in the
  document, so the file still renders with no script.

```go
tr.WriteFile("ticket.html")                                      // both, on by default
tr.WriteFile("ticket.html", transcript.WithoutMediaPool())        // an <img> per use
tr.WriteFile("ticket.html", transcript.WithoutMediaDownscale())   // original bytes
```

### Linking instead of inlining

If the transcript is served over HTTP rather than archived as a single file, the
small images do not need to be in the file at all. `WithCDNMedia()` keeps
avatars, emoji, thumbnails and embed images pointing at the URL they came from —
for Discord those are hash-based and only change when the image does — and
downloads only a message's attachments, whose links expire within hours:

```go
tr.WriteFile("ticket.html", transcript.WithCDNMedia()) // attachments only
tr.WriteFile("ticket.html", transcript.WithLinkedMedia(transcript.UseAvatar, transcript.UseThumbnail))
tr.WriteFile("ticket.html", transcript.WithoutLinkedMedia()) // the default again
```

The avatar URLs still carry the size the renderer draws at (`?size=64`), so the
browser fetches small images, and nothing is downloaded while exporting. The
trade-off is the reason the inline default exists: the document now needs the
network, and an embed thumbnail that happens to point at a *signed*
`cdn.discordapp.com/attachments` link — rather than an `/external/` proxy link or
the producer's own host — will be a broken image once that link lapses.

| Call | Avatars, emoji | Thumbnails, embed images | Attachments |
| --- | --- | --- | --- |
| *(default)* | inlined | inlined | inlined |
| `WithCDNMedia()` | linked | linked | inlined |
| `WithLinkedMedia(...)` | your choice | your choice | your choice |

Measured on a four-message ticket with two avatars, an embed, an attachment and a
repeated icon (synthetic 128px avatars, the default store):

| | on disk | gzipped |
| --- | --- | --- |
| before | 328.6 KB | 247.3 KB |
| pooled only | 183.3 KB | 136.9 KB |
| right-sized only | 135.7 KB | 81.6 KB |
| both (default) | **101.6 KB** | **75.0 KB** |

gzip does not rescue repetition on its own: a base64 avatar is larger than gzip's
32 KB window, so the second copy only partly matches. One-off images — an
attachment, an embed image — stay real `<img>` elements with their own bytes, so
`alt` text and printing keep working.

Both halves live in the renderer, so they apply to every store — your own, and
any proxy behind its fetcher — with no code on your side: the URL already asks
for the size, and the bytes are corrected on the way out. What a fetcher does
with that URL is up to it, as always.

One thing to keep in mind before wiring this into a pipeline: pooled media is a
background image rather than an `<img>`. Nothing in the stylesheet or the script
needs to know that, but a strict Content-Security-Policy has to allow the inline
`<style>` (`style-src 'unsafe-inline'`, or a nonce) and `img-src data:`.
`WithoutMediaPool()` gives you plain `<img>` elements back.

### Routing downloads through your own proxy

The downloader is all hooks, so a proxy endpoint does not mean reimplementing the
fetch:

```go
proxy := transcript.HTTPFetcher{
	// Your endpoint receives the source URL. URLTemplateRewriter also supports the
	// path form, "https://proxy.example/{urlenc}".
	RewriteURL: transcript.URLTemplateRewriter("https://proxy.example/fetch?url={urlenc}"),
	Header:     func(req *http.Request) { req.Header.Set("Authorization", "Bearer "+token) },
	Client:     &http.Client{Timeout: 20 * time.Second},
}

tr.WriteFile("transcript.html", transcript.WithMedia(transcript.InlineMediaWith(proxy)))
```

The original URL still names the media in errors and warnings, and still serves as
the fallback when the proxy fails.

To keep transcripts small while making the media reachable, serve through the
proxy rather than downloading it:

```go
rewrite := transcript.URLTemplateRewriter("https://proxy.example/fetch?url={urlenc}")
store := transcript.MediaStoreFunc(func(_ context.Context, ref transcript.MediaRef) (string, error) {
	return rewrite(ref.URL), nil
})
```

That writes `<img src="https://proxy.example/fetch?url=…">` and downloads nothing.

For anything more exotic — signed requests, a cache lookup, a command-line
helper — implement `transcript.Fetcher`, use the one-method
`transcript.FetchFunc`, or wrap an `http.RoundTripper` in `HTTPFetcher.Client` and
let the transport rewrite URLs, retry or use mutual TLS.

| Hook | Use it for |
| --- | --- |
| `HTTPFetcher.RewriteURL` | your proxy endpoint, a CDN shim, a mirror |
| `HTTPFetcher.Header` | proxy credentials, referer, signatures |
| `HTTPFetcher.Client` | timeouts, redirects, a custom `RoundTripper`, Go's own proxy settings |
| `Fetcher` / `FetchFunc` | anything the three above cannot express |
| `MediaStore` / `MediaStoreFunc` | choosing what the document references at all |

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

An embed description and every field value are parsed as markdown documents, not
as one line of text: headings, lists, quotes, fenced blocks and subtext inside an
embed reach the markup as the elements they are. `examples/transcript-embeds.html`
is a ticket bot's reply and shows all of it. Reply previews, system messages and
thread previews stay single-line instead, because those slots render a snippet
rather than a document — they keep the text of a code block, but not its frame.

Not yet: threads, buttons and select menus, stickers (kept as `[sticker: name]` so
nothing is lost), polls, forwarded snapshots and slash-command rows. The model and
renderer understand threads; the adapter does not map them yet.

## Identity resolution

Nicknames and role colours come from, in order:

1. explicit overrides (`disgo.WithUsers`, `disgo.WithRoles`, `disgo.WithChannels`);
2. the data carried by the message itself (`member`, `mentions`, `mention_channels`);
3. the same data seen anywhere else in the export — `Transcript` reads every
   message once before converting, so a mention of someone who only posted
   earlier still resolves;
4. disgo's caches (`disgo.WithCaches(client.Caches)`).

The export wins over a live cache, so a transcript reads the way it did when it
was exported.

Step 3 is what makes mentions work in practice, because the API does not help:
mentions inside an **embed** never reach the message's `mentions` array, and a
channel is only named by the messages that happen to mention it. A mention of the
channel the transcript itself covers resolves from `Channel.Name`. Anything still
unknown keeps its raw ID rather than vanishing, because a transcript is a record
of what was said.

**Role mentions are the exception.** Discord sends role IDs (`mention_roles`)
and never their names, so `<@&123>` can only resolve from an override or a cache.
`disgo.RolesFrom` turns a guild's roles into the map `WithRoles` takes:

```go
adapter := disgo.New(
	disgo.WithUsers(userMap),
	disgo.WithRoles(disgo.RolesFrom(guild.Roles)), // role names and colours
)
```

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
