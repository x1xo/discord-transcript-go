package transcript

import (
	"context"
	"time"
)

// Theme selects the colour tokens the document starts with.
type Theme string

// Supported themes.
const (
	ThemeDark  Theme = "dark"
	ThemeLight Theme = "light"
)

// Options configures a render. The zero value is valid once Options.Assets is
// set; use Apply or the With* helpers to get the defaults.
type Options struct {
	// Title overrides the document title. Defaults to the channel name.
	Title string
	// Channel describes the conversation header.
	Channel Channel
	// Theme is the initial colour scheme.
	Theme Theme
	// Assets says where the stylesheet and the optional script come from.
	Assets Assets
	// ShortTags emits the compact tag vocabulary (dm, dme, dsp) instead of
	// discord-message, discord-mention, discord-spoiler. The document then points
	// at the short-name stylesheet, so set Assets yourself if you also host the
	// stylesheet somewhere unusual.
	ShortTags bool
	// Media resolves attachment, avatar and emoji URLs. Defaults to base64
	// inlining so the document keeps working without the network.
	Media MediaStore
	// MaxMediaBytes caps a single download. Zero means DefaultMaxMediaBytes.
	MaxMediaBytes int64
	// Locale and TimeZone format timestamps. The generated text is absolute and
	// stays correct when the file is read later.
	Locale   string
	TimeZone *time.Location
	// SelfUserID marks messages that mention this user as highlighted.
	SelfUserID string
	// ShowHeader adds a page-level <h1> above the conversation, for readers who
	// want the title as document content. Note that <discord-messages
	// channel-name="..."> already renders a channel header inside the panel, so
	// enabling both shows the name twice.
	ShowHeader bool
	// ShowMeta adds a metadata line under the header: off by default to keep the
	// document as small as possible.
	ShowMeta bool
	// Generator is recorded in the page metadata when ShowMeta is set.
	Generator string
	// Warn receives non-fatal problems (a failed media download, for example).
	// Rendering always continues.
	Warn func(error)

	// assetsConfigured records that the caller chose the stylesheet, so
	// WithShortTags does not override it.
	assetsConfigured bool
}

// DefaultMaxMediaBytes is the per-file download cap used when
// Options.MaxMediaBytes is not set. Oversized media keeps its original URL.
const DefaultMaxMediaBytes = 8 << 20

// Option mutates Options.
type Option func(*Options)

func defaultOptions() Options {
	return Options{
		Theme:         ThemeDark,
		Assets:        DefaultAssets(),
		Media:         InlineMedia(),
		MaxMediaBytes: DefaultMaxMediaBytes,
		Locale:        "en-US",
		TimeZone:      time.UTC,
	}
}

// Apply returns the options produced by passing opts over the defaults.
func Apply(opts ...Option) Options {
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}

// WithTitle sets the document title.
func WithTitle(title string) Option { return func(o *Options) { o.Title = title } }

// WithChannel sets the channel header.
func WithChannel(c Channel) Option { return func(o *Options) { o.Channel = c } }

// WithTheme selects the initial theme.
func WithTheme(t Theme) Option { return func(o *Options) { o.Theme = t } }

// WithAssets replaces the whole asset configuration.
func WithAssets(a Assets) Option {
	return func(o *Options) {
		o.Assets = a
		o.assetsConfigured = true
	}
}

// WithShortTags emits the compact tag vocabulary and, unless the stylesheet was
// set explicitly, points the document at the matching short-name stylesheet.
func WithShortTags() Option {
	return func(o *Options) {
		o.ShortTags = true
		if !o.assetsConfigured {
			o.Assets.CSSURL = DefaultShortCSSURL
			o.Assets.CSSIntegrity = DefaultShortCSSIntegrity
		}
	}
}

// WithCSS points the document at a stylesheet, with an optional SRI hash.
// Pass an empty integrity to omit the attribute.
func WithCSS(url, integrity string) Option {
	return func(o *Options) {
		o.Assets.CSSURL = url
		o.Assets.CSSIntegrity = integrity
		o.assetsConfigured = true
	}
}

// WithScript adds the enhancement script that ships with the pinned UI release,
// SRI hash and all. The URL and the hash come from the generated pins, so they
// follow the UI every time this module is rebuilt against a new release; the
// document still renders if the script never loads, because the markup is
// already complete.
//
// This is the common way to opt in:
//
//	tr.WriteFile("transcript.html", transcript.WithScript())
//
// Use WithScriptURL to point at a copy you host yourself.
func WithScript() Option {
	return func(o *Options) {
		o.Assets.ScriptURL = DefaultScriptURL
		o.Assets.ScriptIntegrity = DefaultScriptIntegrity
		// SRI on a cross-origin script needs crossorigin="anonymous" or the
		// browser refuses to run it. A caller who replaced the whole Assets keeps
		// their own choice: only the pinned pair is turned on here.
		if !o.assetsConfigured {
			o.Assets.CrossOrigin = true
		}
	}
}

// WithScriptURL points the document at an enhancement script somewhere else,
// with an optional SRI hash. Pass an empty integrity to omit the attribute.
func WithScriptURL(url, integrity string) Option {
	return func(o *Options) {
		o.Assets.ScriptURL = url
		o.Assets.ScriptIntegrity = integrity
	}
}

// WithoutScript drops the enhancement script, leaving a stylesheet-only
// document. This is already the default.
func WithoutScript() Option {
	return func(o *Options) {
		o.Assets.ScriptURL = ""
		o.Assets.ScriptIntegrity = ""
	}
}

// WithoutStylesheet omits the stylesheet link entirely, for callers that inject
// their own.
func WithoutStylesheet() Option {
	return func(o *Options) {
		o.Assets.CSSURL = ""
		o.Assets.CSSIntegrity = ""
	}
}

// WithMedia sets the media store used for attachments, avatars and emoji.
func WithMedia(store MediaStore) Option { return func(o *Options) { o.Media = store } }

// WithMaxMediaBytes caps a single media download.
func WithMaxMediaBytes(n int64) Option { return func(o *Options) { o.MaxMediaBytes = n } }

// WithTimeZone sets the zone used for timestamp text.
func WithTimeZone(loc *time.Location) Option { return func(o *Options) { o.TimeZone = loc } }

// WithLocale sets the locale used for timestamp text.
func WithLocale(locale string) Option { return func(o *Options) { o.Locale = locale } }

// WithSelfUserID marks messages mentioning this user as highlighted.
func WithSelfUserID(id string) Option { return func(o *Options) { o.SelfUserID = id } }

// WithPageHeader adds a page-level <h1> with the document title.
func WithPageHeader() Option { return func(o *Options) { o.ShowHeader = true } }

// WithMeta adds the metadata line under the page title.
func WithMeta() Option { return func(o *Options) { o.ShowMeta = true } }

// WithGenerator overrides the recorded generator string.
func WithGenerator(name string) Option { return func(o *Options) { o.Generator = name } }

// WithWarn installs a callback for non-fatal problems.
func WithWarn(fn func(error)) Option { return func(o *Options) { o.Warn = fn } }

func (o *Options) warn(err error) {
	if err != nil && o.Warn != nil {
		o.Warn(err)
	}
}

func (o *Options) maxMediaBytes() int64 {
	if o.MaxMediaBytes > 0 {
		return o.MaxMediaBytes
	}
	return DefaultMaxMediaBytes
}

// fetchTimeout bounds media downloads so one dead CDN cannot hang an export.
const fetchTimeout = 30 * time.Second

// contextWithTimeout bounds a media download without overriding a deadline the
// caller already set.
func contextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, fetchTimeout)
}
