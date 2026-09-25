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

// AssetMode decides how the stylesheet and the enhancement script are referenced.
type AssetMode string

// Supported asset modes.
const (
	// AssetsCDN links the pinned CDN files, with Subresource Integrity and the
	// multi-mirror fallback chain. Smallest documents; the browser caches the
	// assets across every transcript.
	AssetsCDN AssetMode = "cdn"
	// AssetsLocal writes relative paths, for a folder you ship (or archive)
	// alongside the HTML.
	AssetsLocal AssetMode = "local"
	// AssetsInline embeds the stylesheet and script in the document, so it needs
	// no network at all. Costs roughly 50 KB per file before compression.
	AssetsInline AssetMode = "inline"
)

// Options configures a render. Use the With* helpers; the zero value is valid
// and renders a dark, CDN-linked transcript with base64-inlined media.
type Options struct {
	// Title overrides the document title. Defaults to the channel name.
	Title string
	// Channel describes the conversation header.
	Channel Channel
	// Theme is the initial colour scheme.
	Theme Theme
	// Assets selects how the stylesheet and script are referenced.
	Assets AssetMode
	// AssetsBase is the path prefix used by AssetsLocal.
	AssetsBase string
	// Media resolves attachment, avatar and emoji URLs. Defaults to base64
	// inlining; pass URLMedia() to keep the original (expiring) CDN URLs.
	Media MediaStore
	// MaxMediaBytes caps a single download. Larger media keeps its original URL.
	// Zero means DefaultMaxMediaBytes.
	MaxMediaBytes int64
	// Locale is used for the no-script timestamp fallback text.
	Locale string
	// TimeZone is used for the no-script timestamp fallback text.
	TimeZone *time.Location
	// SelfUserID marks messages that mention this user as highlighted.
	SelfUserID string
	// WithoutMeta drops the metadata line under the page title.
	ShowMeta bool
	// Generator is recorded in the page metadata and the recovery comment.
	Generator string
	// Warn receives non-fatal problems (a failed media download, an
	// unwritable cache entry). Rendering always continues.
	Warn func(error)
}

// DefaultMaxMediaBytes is the per-file download cap used when Options.MaxMediaBytes
// is not set. Discord's own attachment limit for non-boosted servers is 10 MiB,
// but a transcript is not the place for a 10 MiB video, so the cap is lower and
// oversized media keeps its original URL instead.
const DefaultMaxMediaBytes = 8 << 20

// Option mutates Options.
type Option func(*Options)

func defaultOptions() Options {
	return Options{
		Theme:         ThemeDark,
		Assets:        AssetsCDN,
		AssetsBase:    "../dist/",
		Media:         InlineMedia(),
		MaxMediaBytes: DefaultMaxMediaBytes,
		Locale:        "en-US",
		TimeZone:      time.UTC,
		ShowMeta:      true,
		Generator:     "discord-transcript-go " + Version,
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

// WithAssets selects how the stylesheet and script are referenced.
func WithAssets(mode AssetMode) Option { return func(o *Options) { o.Assets = mode } }

// WithAssetsBase sets the prefix used by AssetsLocal.
func WithAssetsBase(base string) Option { return func(o *Options) { o.AssetsBase = base } }

// WithMedia sets the media store used for attachments, avatars and emoji.
func WithMedia(store MediaStore) Option { return func(o *Options) { o.Media = store } }

// WithMaxMediaBytes caps a single media download.
func WithMaxMediaBytes(n int64) Option { return func(o *Options) { o.MaxMediaBytes = n } }

// WithTimeZone sets the zone used for fallback timestamp text.
func WithTimeZone(loc *time.Location) Option { return func(o *Options) { o.TimeZone = loc } }

// WithLocale sets the locale used for fallback timestamp text.
func WithLocale(locale string) Option { return func(o *Options) { o.Locale = locale } }

// WithSelfUserID marks messages mentioning this user as highlighted.
func WithSelfUserID(id string) Option { return func(o *Options) { o.SelfUserID = id } }

// WithoutMeta hides the metadata line under the page title.
func WithoutMeta() Option { return func(o *Options) { o.ShowMeta = false } }

// WithGenerator overrides the recorded generator string.
func WithGenerator(name string) Option { return func(o *Options) { o.Generator = name } }

// WithWarn installs a callback for non-fatal problems.
func WithWarn(fn func(error)) Option { return func(o *Options) { o.Warn = fn } }

// warn reports a non-fatal problem if a callback is configured.
func (o *Options) warn(err error) {
	if err != nil && o.Warn != nil {
		o.Warn(err)
	}
}

// maxMediaBytes returns the effective download cap.
func (o *Options) maxMediaBytes() int64 {
	if o.MaxMediaBytes > 0 {
		return o.MaxMediaBytes
	}
	return DefaultMaxMediaBytes
}

// connectTimeout and fetchTimeout bound media downloads so one dead CDN cannot
// hang a whole export.
const (
	fetchTimeout = 30 * time.Second
)

// contextWithTimeout is a small helper so callers of the media stores get a
// bounded context without having to think about it.
func contextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, fetchTimeout)
}
