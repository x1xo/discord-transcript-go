package transcript

import (
	"context"
	"strings"
	"testing"
	"time"
)

// countingStore records what a store was asked to download and inline.
type countingStore struct {
	inner MediaStore
	uses  []MediaUse
}

func (s *countingStore) Store(ctx context.Context, ref MediaRef) (string, error) {
	s.uses = append(s.uses, ref.Use)
	return s.inner.Store(ctx, ref)
}

func (s *countingStore) downloaded(use MediaUse) bool {
	for _, seen := range s.uses {
		if seen == use {
			return true
		}
	}
	return false
}

// linkFixture is a message with one of everything the policy distinguishes: an
// avatar, an embed thumbnail and image, and an attachment.
func linkFixture() *Transcript {
	at := time.Date(2024, 3, 15, 14, 28, 0, 0, time.UTC)
	return &Transcript{
		Channel: Channel{Name: "general", Type: ChannelText},
		Messages: []Message{{
			Author:    Author{Key: "1", Name: "piton", AvatarURL: "https://cdn.discordapp.com/avatars/1/abc.png"},
			Timestamp: at,
			Content:   ParseContent("look"),
			Embeds: []Embed{{
				Description: ParseContent("an embed"),
				Thumbnail:   &Media{URL: "https://images-ext-1.discordapp.net/external/xyz/thumb.png"},
				Image:       &Media{URL: "https://images-ext-1.discordapp.net/external/xyz/full.png"},
			}},
			Attachments: []Attachment{{Kind: MediaImage, URL: "https://cdn.discordapp.com/attachments/1/2/shot.png", Name: "shot.png"}},
		}},
	}
}

func TestWithCDNMediaLinksEverythingButAttachments(t *testing.T) {
	// The point of the option: a transcript served over HTTP keeps the CDN links
	// for the small, long-lived images and downloads only what expires.
	store := &countingStore{inner: MediaStoreFunc(func(_ context.Context, ref MediaRef) (string, error) {
		return "data:image/png;base64,INLINED+" + ref.URL, nil
	})}

	page := render(t, linkFixture(), WithMedia(store), WithCDNMedia())

	for _, want := range []string{
		// An avatar is asked for at the size it is drawn and left as a link.
		`<span class="dt-avatar"><img src="https://cdn.discordapp.com/avatars/1/abc.png?size=64"`,
		// Thumbnails and embed images too, exactly as the producer wrote them.
		`<div class="dt-embed-thumbnail"><img src="https://images-ext-1.discordapp.net/external/xyz/thumb.png"`,
		`<div class="dt-embed-image"><img src="https://images-ext-1.discordapp.net/external/xyz/full.png"`,
		// The attachment is downloaded and inlined.
		`src="data:image/png;base64,INLINED+https://cdn.discordapp.com/attachments/1/2/shot.png"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %s in:\n%s", want, page)
		}
	}

	// Nothing but the attachment was ever fetched.
	if store.downloaded(UseAvatar) || store.downloaded(UseThumbnail) || store.downloaded(UseEmbedImage) {
		t.Errorf("only attachments should be downloaded, got %v", store.uses)
	}
	if !store.downloaded(UseAttachment) {
		t.Errorf("the attachment should have been downloaded, got %v", store.uses)
	}
	if len(store.uses) != 1 {
		t.Errorf("expected exactly one download, got %v", store.uses)
	}
	// Nothing is inlined, so there is no blob to pool.
	if strings.Contains(page, "<style data-dt-media-pool>") {
		t.Errorf("linked media has nothing to pool")
	}
}

func TestWithLinkedMediaChoosesPerUse(t *testing.T) {
	store := func() *countingStore {
		return &countingStore{inner: MediaStoreFunc(func(_ context.Context, ref MediaRef) (string, error) {
			return "data:image/png;base64,INLINED+" + ref.URL, nil
		})}
	}

	// Only the thumbnail is linked: the avatar and the embed image are inlined.
	s := store()
	page := render(t, linkFixture(), WithMedia(s), WithLinkedMedia(UseThumbnail))
	if !strings.Contains(page, "https://images-ext-1.discordapp.net/external/xyz/thumb.png") {
		t.Errorf("the thumbnail should be linked")
	}
	if !strings.Contains(page, "INLINED+https://cdn.discordapp.com/avatars/1/abc.png?size=64") {
		t.Errorf("the avatar should be inlined when it is not in the linked set")
	}
	if s.downloaded(UseThumbnail) {
		t.Errorf("the thumbnail should not have been downloaded")
	}

	// WithoutLinkedMedia puts everything back.
	s = store()
	page = render(t, linkFixture(), WithMedia(s), WithCDNMedia(), WithoutLinkedMedia())
	if strings.Contains(page, `src="https://images-ext-1.discordapp.net/external/xyz/thumb.png"`) {
		t.Errorf("WithoutLinkedMedia should inline again:\n%s", page)
	}
	if !s.downloaded(UseAvatar) || !s.downloaded(UseThumbnail) || !s.downloaded(UseAttachment) {
		t.Errorf("everything should be downloaded again, got %v", s.uses)
	}
}
