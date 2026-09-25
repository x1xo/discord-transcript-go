.PHONY: build test vet fmt generate verify example clean

# Compile everything.
build:
	go build ./...

# Unit tests for the parser, renderer, media stores and the disgo adapter.
test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# Refresh the embedded discord-transcript-ui assets from ../dist.
# Override the source with DISCORD_TRANSCRIPT_UI_DIR=/path/to/transcripts.
generate:
	go generate ./...

# Regenerate the example transcript from the test fixture.
example: examples/transcript.html

examples/transcript.html: testdata/export.json
	go run ./cmd/discord-transcript -in testdata/export.json -out examples/transcript.html

# Full check: tests plus a real-browser render of the generated transcript.
# Needs a Chrome or Chromium binary on PATH (or CHROME=/path/to/chrome).
verify: test example
	node scripts/verify-browser.mjs examples/transcript.html

clean:
	rm -f examples/transcript.html
	go clean -testcache
