package disgo

import (
	"bytes"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/x1xo/discord-transcript-go/transcript"
)

// This example is compiled by `go test` but never run, so it doubles as a check
// that the documented API stays accurate. It needs no network: identity comes
// from overrides and media is left as-is.
func Example() {
	messages := []discord.Message{
		{
			ID:        snowflake.ID(200000000000000001),
			ChannelID: snowflake.ID(300000000000000001),
			Author:    discord.User{ID: snowflake.ID(100000000000000001), Username: "piton"},
			Content:   "Hello **world**, welcome <@100000000000000002>!",
			CreatedAt: time.Date(2024, 3, 15, 14, 28, 0, 0, time.UTC),
		},
		{
			ID:        snowflake.ID(200000000000000002),
			ChannelID: snowflake.ID(300000000000000001),
			Author:    discord.User{ID: snowflake.ID(100000000000000002), Username: "miona", Bot: true},
			Content:   "Thanks!",
			CreatedAt: time.Date(2024, 3, 15, 14, 29, 0, 0, time.UTC),
		},
	}

	adapter := New(WithUsers(transcript.Profiles{
		"100000000000000001": {Name: "piton", RoleColor: "#57f287"},
		"100000000000000002": {Name: "miona", RoleColor: "#eb459e", Bot: true, Verified: true},
	}))

	tr := adapter.Transcript(
		transcript.Channel{Name: "general", Type: transcript.ChannelText, Guild: "Test Server"},
		messages,
	)
	tr.Title = "general — 2024-03-15"

	// WriteTo accepts any io.Writer, so a transcript can go straight to an HTTP
	// response, object storage or a file.
	var out bytes.Buffer
	if err := tr.WriteTo(&out, transcript.WithMedia(transcript.URLMedia())); err != nil {
		panic(err)
	}
}
