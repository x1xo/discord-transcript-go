# Changelog

Versions here belong to the Go module and move on their own. The markup contract
the renderer emits is `transcript.ContractVersion`, generated from the
`discord-transcript-ui` build, and it is listed per release below.

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
