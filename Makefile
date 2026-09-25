.PHONY: build test vet fmt verify example clean

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

# Regenerate the example transcripts from the test fixture.
example: examples/transcript.html examples/transcript-interactive.html

examples/transcript.html: testdata/export.json
	go run ./cmd/discord-transcript -in testdata/export.json -out examples/transcript.html

examples/transcript-interactive.html: testdata/export.json
	go run ./cmd/discord-transcript -in testdata/export.json -out examples/transcript-interactive.html -script

# Full check: tests plus a real-browser render of both examples.
# Needs a Chrome or Chromium binary on PATH (or CHROME=/path/to/chrome).
verify: test example
	node scripts/verify-browser.mjs examples/transcript.html
	node scripts/verify-browser.mjs examples/transcript-interactive.html

clean:
	rm -f examples/transcript.html examples/transcript-interactive.html
	go clean -testcache
