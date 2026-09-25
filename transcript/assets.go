package transcript

// ContractVersion is the discord-transcript-ui release whose markup contract
// this module emits.
const ContractVersion = "1.1.0"

// Pinned defaults. The stylesheet is all a transcript needs to render: this
// module emits the full markup the stylesheet targets, so avatars, author
// headers, badges, timestamps, replies, embeds, attachments and reactions all
// work without JavaScript.
const (
	// DefaultCSSURL is the jsDelivr URL of the pinned stylesheet.
	DefaultCSSURL = "https://cdn.jsdelivr.net/npm/discord-transcript-ui@" + ContractVersion + "/dist/discord-transcript.min.css"
	// DefaultCSSIntegrity is its Subresource Integrity hash.
	DefaultCSSIntegrity = "sha384-ZwplFA3/xu47EZ0oSGTMwyi8siRbfdDRiGANX4HbKlPdbwlEjnDZjeTZz4rw9jyx"

	// DefaultShortCSSURL is the same stylesheet rewritten to the compact tag
	// vocabulary, for documents rendered with WithShortTags.
	DefaultShortCSSURL = "https://cdn.jsdelivr.net/npm/discord-transcript-ui@" + ContractVersion + "/dist/discord-transcript.short.min.css"
	// DefaultShortCSSIntegrity is its Subresource Integrity hash.
	DefaultShortCSSIntegrity = "sha384-o+TMmUarWmSsA4gonFBZ8qDLhrZEJGJkAMyTwsFheJxYEgkd9XE+DdSXMo0e6EH3"

	// DefaultScriptURL is the jsDelivr URL of the optional enhancement script.
	DefaultScriptURL = "https://cdn.jsdelivr.net/npm/discord-transcript-ui@" + ContractVersion + "/dist/discord-transcript.min.js"
	// DefaultScriptIntegrity is its Subresource Integrity hash.
	DefaultScriptIntegrity = "sha384-aPG5pvJ6e+7rRJ6qYbneL8xBPZZP24BUxBezSvQB0tAgpeKdea6D9jllfke4JZce"
)

// Assets says where the browser loads the stylesheet, and optionally the
// enhancement script, from.
//
// Point it at your own host when you would rather not depend on a public CDN:
//
//	transcript.WithCSS("https://cdn.example.com/discord-transcript.min.css", "sha384-...")
//
// The script is optional and off by default. Turning it on adds click-to-reveal
// spoilers, copy buttons and viewer-local timestamps; it rebuilds nothing,
// because the generated markup is already complete.
type Assets struct {
	// CSSURL is the stylesheet location. Empty omits the stylesheet entirely.
	CSSURL string
	// CSSIntegrity is the stylesheet's SRI hash. Empty omits the attribute.
	CSSIntegrity string
	// ScriptURL is the enhancement script location. Empty omits the script.
	ScriptURL string
	// ScriptIntegrity is the script's SRI hash.
	ScriptIntegrity string
	// CrossOrigin adds crossorigin="anonymous", which SRI requires for a
	// cross-origin resource.
	CrossOrigin bool
}

// DefaultAssets returns the pinned jsDelivr stylesheet with its SRI hash and no
// script: the smallest document that renders fully.
func DefaultAssets() Assets {
	return Assets{
		CSSURL:       DefaultCSSURL,
		CSSIntegrity: DefaultCSSIntegrity,
		CrossOrigin:  true,
	}
}

// AssetsWithScript returns the pinned stylesheet and the enhancement script,
// both verified with Subresource Integrity.
func AssetsWithScript() Assets {
	return Assets{
		CSSURL:          DefaultCSSURL,
		CSSIntegrity:    DefaultCSSIntegrity,
		ScriptURL:       DefaultScriptURL,
		ScriptIntegrity: DefaultScriptIntegrity,
		CrossOrigin:     true,
	}
}
