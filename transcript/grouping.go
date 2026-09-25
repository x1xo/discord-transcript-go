package transcript

import "time"

// DefaultGroupWindow is how close two messages from the same author must be to
// render as one group, matching Discord and the enhancement script.
const DefaultGroupWindow = 7 * time.Minute

// grouping describes how a message relates to the one before it.
type grouping struct {
	// continuation means the author row is omitted: same author, close in time.
	continuation bool
	// groupStart means extra vertical space is added above the row.
	groupStart bool
}

// groupMessages applies the same rule the enhancement script uses, so a
// stylesheet-only document groups rows exactly as the script would.
//
// Only user messages take part; system rows are invisible to this, which keeps
// the two implementations in step.
func groupMessages(messages []Message, window time.Duration) []grouping {
	out := make([]grouping, len(messages))
	if window <= 0 {
		window = DefaultGroupWindow
	}

	var previousKey string
	var previousTime time.Time
	havePrevious := false
	var previousTimeSet bool

	for i, m := range messages {
		if m.System != nil {
			continue
		}
		key := m.Author.Key
		if key == "" {
			key = m.Author.Name
		}
		timeSet := !m.Timestamp.IsZero()

		sameAuthor := havePrevious && key != "" && key == previousKey
		closeInTime := !previousTimeSet || !timeSet ||
			m.Timestamp.Sub(previousTime) <= window && previousTime.Sub(m.Timestamp) <= window

		switch {
		case sameAuthor && closeInTime:
			out[i].continuation = true
		case havePrevious:
			out[i].groupStart = true
		}

		previousKey = key
		previousTime = m.Timestamp
		previousTimeSet = timeSet
		havePrevious = true
	}
	return out
}
