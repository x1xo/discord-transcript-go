package transcript

import (
	"strings"
	"testing"
	"time"
)

// Discord uses two timestamp shapes: message headers drop the space before
// AM/PM, everything else keeps it and leads with a short date.
func TestHeaderStampMatchesDiscord(t *testing.T) {
	r := &renderer{o: &Options{TimeZone: time.UTC}}
	now := time.Now().UTC()
	at := func(daysAgo int) time.Time {
		day := now.AddDate(0, 0, -daysAgo)
		return time.Date(day.Year(), day.Month(), day.Day(), 11, 49, 0, 0, time.UTC)
	}

	cases := []struct {
		name string
		when time.Time
		want string
	}{
		{"today is just the time", at(0), "11:49AM"},
		{"yesterday is prefixed", at(1), "Yesterday at 11:49AM"},
		{"older keeps the short date", time.Date(2024, 3, 14, 11, 57, 0, 0, time.UTC), "3/14/24, 11:57 AM"},
	}
	for _, c := range cases {
		if got := r.headerStamp(c.when); got != c.want {
			t.Errorf("%s: headerStamp = %q, want %q", c.name, got, c.want)
		}
	}

	if got := r.shortStamp(time.Date(2024, 3, 14, 14, 32, 0, 0, time.UTC)); got != "2:32PM" {
		t.Errorf("shortStamp = %q, want %q", got, "2:32PM")
	}
	if got := r.headerStamp(time.Time{}); got != "" {
		t.Errorf("a zero timestamp should render nothing, got %q", got)
	}
}

func TestInlineStampFlags(t *testing.T) {
	r := &renderer{o: &Options{TimeZone: time.UTC}}
	when := time.Date(2024, 3, 14, 11, 57, 30, 0, time.UTC)

	cases := map[byte]string{
		't': "11:57 AM",
		'T': "11:57:30 AM",
		'd': "3/14/24",
		'D': "March 14, 2024",
		'f': "3/14/24, 11:57 AM",
		's': "3/14/24, 11:57 AM",
		'S': "3/14/24, 11:57:30 AM",
		'F': "Thursday, March 14, 2024 at 11:57 AM",
		'R': "3/14/24, 11:57 AM", // the script turns this relative; the fallback is absolute
		0:   "3/14/24, 11:57 AM",
	}
	for flag, want := range cases {
		if got := r.inlineStamp(when, flag); got != want {
			t.Errorf("inlineStamp(%q) = %q, want %q", string(flag), got, want)
		}
	}
}

func TestEmbedFooterUsesTheShortDateTime(t *testing.T) {
	// Footers are not message headers, so they keep the space before AM/PM.
	tr := &Transcript{
		Channel: Channel{Name: "general", Type: ChannelText},
		Messages: []Message{{
			Author: Author{Key: "1", Name: "piton"},
			Embeds: []Embed{{
				Footer: &EmbedFooter{Text: "Footer", Timestamp: time.Date(2024, 3, 14, 11, 57, 0, 0, time.UTC)},
			}},
		}},
	}
	doc, err := tr.HTML(WithMedia(URLMedia()))
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	if want := "3/14/24, 11:57 AM"; !strings.Contains(string(doc), want) {
		t.Errorf("embed footer should render %q:\n%s", want, doc)
	}
}
