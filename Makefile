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

# Regenerate the committed examples (and the golden file) after a renderer change.
example:
	go test ./transcript -run TestGenerateExamples -update

# Full check: tests plus a real-browser render of the examples.
# Needs Chrome (CHROME=/path/to/chrome overrides the lookup). The --css fallback
# checks against the local stylesheet when the pinned release is not on the CDN.
verify: test example
	node scripts/verify-browser.mjs examples/transcript.html --css ../dist/discord-transcript.min.css
	node scripts/verify-browser.mjs examples/transcript-interactive.html --css ../dist/discord-transcript.min.css
	node scripts/verify-browser.mjs examples/transcript-short.html --css ../dist/discord-transcript.short.min.css
	node scripts/verify-browser.mjs examples/transcript-embeds.html --css ../dist/discord-transcript.min.css

clean:
	rm -f examples/*.html
	go clean -testcache
