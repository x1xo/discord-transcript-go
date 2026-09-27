# Changelog

Versions here belong to the Go module and move on their own. The markup contract
the renderer emits is `transcript.ContractVersion`, generated from the
`discord-transcript-ui` build, and it is listed per release below.

## 1.6.0

Contract: `discord-transcript-ui` 1.1.4 (unchanged).

* **The channel an interaction carries names no guild, so nothing resolved.**
  `Adapter.Channel` read the guild off the channel object, but
  `event.Channel()` — how a ticket transcript is usually triggered — is a
  `discord.InteractionChannel`: partial, satisfying `discord.Channel` but not
  `discord.GuildChannel`, with no `guild_id` in the payload. Since every cache
  lookup is keyed on the guild, such a transcript had no role names, no role
  colours and no nicknames even with a warm cache. The adapter now falls back to
  the channel the cache holds, which does carry the guild, and takes a missing
  channel name from the same place. `Adapter.Channel` also no longer reaches into
  a nil cache — it used to panic for any guild channel when no caches were
  configured.
* **`WithMemberFetcher` fills in authors the caches do not hold.** disgo caches
  roles for every guild, but members only from `GUILD_CREATE`, from member and
  voice events, and from explicit chunking. In a large guild most message authors
  therefore have no cached member, and a member is the only source of a nickname,
  a guild avatar and a role colour. A fetcher — point it at `Rest.GetMember` —
  is asked at most once per distinct author per transcript, and never for a
  mention, whose count is unbounded. `Adapter.Message` honours it too.

## 1.5.0

Contract: `discord-transcript-ui` **1.1.4** (was 1.1.3).

* **Action rows and buttons are rendered.** `discord.Message.Components` was
  ignored, so every button a bot posted vanished from the transcript. The model
  gains `ActionRow` and `Button`, and `Message.ActionRows` reaches
  `<discord-action-row>` / `<discord-button>`: all five styles, the label, a
  unicode or custom emoji, and the disabled state. A button with a URL is wrapped
  in an anchor, so it is clickable with no script. A row whose only item is
  something the stylesheet has no element for — a select menu, a components-v2
  container — is dropped rather than drawn as an empty gap.
* **Guild identity now resolves for messages fetched over REST.** Discord's HTTP
  message object carries neither `guild_id` nor `member` (both are gateway-event
  fields), and every cache lookup was keyed on the guild, so
  `rest.GetMessages` history produced transcripts with no nicknames, no guild
  avatars and no role colours. `Adapter.Channel` now carries the guild from a
  guild channel in `Channel.GuildID`, and `WithGuildID` covers a channel built by
  hand, so the cache can answer.
* **`WithRoles` colours author names.** `disgo.RolesFrom` / `WithRoles` fed role
  *mentions* only: the author-colour lookup never consulted the override map, so
  the "role colours come from the same map" promise of 1.4.0 did not hold without
  a cache. Both paths now share one lookup, and `RoleInfo.Position` keeps the
  hierarchy so a member with several coloured roles is drawn in the highest one.
* Unresolved role mentions still keep their raw ID, and a button URL that fails
  sanitising renders as an inert button rather than a `javascript:` link.

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
