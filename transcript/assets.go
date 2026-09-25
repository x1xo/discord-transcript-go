package transcript

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
