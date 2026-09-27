# Changelog

Versions here belong to the Go module and move on their own. The markup contract
the renderer emits is `transcript.ContractVersion`, generated from the
`discord-transcript-ui` build, and it is listed per release below.

## 1.4.1

Contract: `discord-transcript-ui` **1.1.3** (was 1.1.2).

* Emits the shorter ready marker, `data-dt-r`, four bytes less on every message,
  reply, embed, attachment and reaction. The 1.1.3 stylesheet reads the old
  `data-dt-ready` name too, so transcripts archived from earlier releases keep
  rendering — but this renderer needs the 1.1.3 stylesheet, because 1.1.2 does not
  know the short name and would draw its attr-only fallbacks twice.
* Nothing else changed in the module: the grouped-line spacing fix in this
  release is in the stylesheet (`[data-dt-continuation]` avatars are now zero
  height), which is why the contract moved.

## 1.4.0

Contract: `discord-transcript-ui` 1.1.2 (unchanged).

* **`transcript.WithCDNMedia()` links instead of inlining.** Avatars, emoji,
  thumbnails and embed images keep the URL they came from, and only a message's
  attachments are downloaded and written as base64. For Discord those links are
  hash-based and long-lived, so a transcript served over HTTP stays small and
  downloads nothing while exporting. `WithLinkedMedia(uses...)` chooses per use,
  `WithoutLinkedMedia()` goes back to inlining everything, and `MediaRef.Use`
  tells a custom store which is which. The trade-off is the one the inline
  default exists for: the document needs the network, and a thumbnail that points
  at a *signed* attachment link rots when that link lapses.
* **Mentions resolve against the whole export.** `Transcript` reads every message
  once to build an identity index — authors, members, `mentions`,
  `mention_channels` — and parses each message with it, so a mention of someone
  who only posted earlier still renders a name, a channel named by a different
  message resolves, and **mentions inside embeds** work at all: Discord never puts
  those in the message's `mentions` array. A mention of the channel the transcript
  covers resolves from `Channel.Name`.
* `disgo.RolesFrom` converts a guild's roles into the `WithRoles` map, which is
  the only way a `<@&role>` mention can render a name — Discord sends role IDs and
  never their names. Role colours for author names come from the same map.
* A message that mentions the viewer inside an **embed** is now highlighted, not
  only one that mentions them in its content.
* Identity lookup order is documented and now puts the export ahead of a live
  cache, so a transcript reads the way it did when it was exported: overrides,
  the message itself, the export, caches. Unresolved mentions keep their raw ID.

## 1.3.0

Contract: `discord-transcript-ui` 1.1.2 (unchanged).

* **Text inside an embed is parsed again.** A description or a field value is a
  document, not one line of text: the renderer flattened both, so headings,
  lists, quotes and fenced blocks inside an embed arrived as bare words — and a
  fenced block lost its body with its wrapper, because a code block carries its
  text in `Text` rather than in children. Both now keep their block markup, and a
  code block in a one-line slot (a reply preview, a system message, a thread
  preview) keeps its text without the frame.
* A blank line in front of a heading, list, quote or fenced block no longer adds
  a stray empty line: the block's own margin is the separation.
* **`transcript.WithScript()` takes no arguments and adds the pinned enhancement
  script**, URL and SRI hash, both read from the generated pins — so a UI release
  moves them without touching call sites. Pointing at your own copy moved to
  `transcript.WithScriptURL(url, integrity)`, which is the one API break in this
  release. `WithScript()` composes with `WithCSS` and `WithShortTags` in any
  order, `WithoutScript()` still opts back out, and the script stays off by
  default.
* **Transcripts are about three times smaller.** Media is right-sized to the size
  it is drawn at: the URL handed to your store already names it (`?size=64` for a
  Discord avatar, untouched for an attachment), and anything a store returns
  inline still too large is downscaled in Go, keeping the original bytes whenever
  the result would not be smaller. An inline blob used more than once is stored
  once in a `<style data-dt-media-pool>` block, with
  `<span class="dt-media dt-media-1">` references in its place. Both happen in
  the renderer, so a custom `MediaStore` — and any proxy behind its fetcher —
  gets them without extra code. `WithoutMediaPool()` and
  `WithoutMediaDownscale()` restore the old output.
* New example `examples/transcript-embeds.html` (a ticket bot's reply) renders a
  description with every block-level construct in it; the browser check asserts
  the headings, list markers, quote bar and code-block body survive, that pooled
  blobs are stored once each, and that a pooled avatar fills its box.

Measured on a four-message ticket with two avatars, an embed, an attachment and a
repeated icon (synthetic 128px avatars, the default store):

| | on disk | gzipped |
| --- | --- | --- |
| before | 328.6 KB | 247.3 KB |
| pooled only | 183.3 KB | 136.9 KB |
| right-sized only | 135.7 KB | 81.6 KB |
| both (default) | **101.6 KB** | **75.0 KB** |

## 1.2.1

Contract: `discord-transcript-ui` 1.1.2.

* Embeds lay out the way Discord does: the accent colour reaches the border, the
  thumbnail is pinned to the corner whatever order the parser emits children in,
  a bare description keeps its line breaks, and the footer gets its own
  full-width row below everything.

## 1.2.0

Contract: `discord-transcript-ui` 1.1.1.

* Discord's timestamp formats, including the compact `11:49PM` and the
  `Yesterday at …` prefix.
* The media fetcher is hookable — `HTTPFetcher{Client, UserAgent, RewriteURL,
  Header}`, `URLTemplateRewriter` and `FetchFunc` — so a custom proxy or a signed
  request does not mean reimplementing the download.
* The CLI was removed: the library is a package, and `examples/` plus
  `scripts/verify-browser.mjs` cover what a command used to.

## 1.1.0

Contract: `discord-transcript-ui` 1.1.0.

* First release as a package: disgo messages to `discord-transcript-ui` HTML.
* Provider-neutral core — the transcript model has no disgo dependency and the
  adapter maps into it — with minimal output and a configurable stylesheet CDN.
* The pins and the short tag map are generated from the UI build rather than
  copied by hand.
* Opt-in compact tag names, an opt-in page header, and small-print styling.
