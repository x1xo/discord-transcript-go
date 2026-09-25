package transcript

// ContractVersion is the discord-transcript-ui release whose markup contract
// this module emits.
const ContractVersion = "1.0.1"

// Pinned defaults. The stylesheet is all a transcript needs to render: this
// module emits the full markup the stylesheet targets, so avatars, author
// headers, badges, timestamps, replies, embeds, attachments and reactions all
// work without JavaScript.
const (
	// DefaultCSSURL is the jsDelivr URL of the pinned stylesheet.
	DefaultCSSURL = "https://cdn.jsdelivr.net/npm/discord-transcript-ui@" + ContractVersion + "/dist/discord-transcript.min.css"
	// DefaultCSSIntegrity is its Subresource Integrity hash.
	DefaultCSSIntegrity = "sha384-MdwneGHI4BQDRywMXXD0VVcvuU2mSlU/cJlHVjDaS6fkNJ/5+OGgmv5w4D/VAOa+"

	// DefaultScriptURL is the jsDelivr URL of the optional enhancement script.
	DefaultScriptURL = "https://cdn.jsdelivr.net/npm/discord-transcript-ui@" + ContractVersion + "/dist/discord-transcript.min.js"
	// DefaultScriptIntegrity is its Subresource Integrity hash.
	DefaultScriptIntegrity = "sha384-hxBXQMtoZvVzUDgfAw6RWEVsVW7oJ3++5VYQr2i+vU0gOBIWebUzWahpufZUpQUN"
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
