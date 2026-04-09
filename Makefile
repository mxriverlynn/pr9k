.PHONY: build test lint format vet vulncheck mod-tidy ci

build:
	rm -rf bin
	mkdir -p bin
	cd ralph-tui && go build -o ../bin/ralph-tui ./cmd/ralph-tui
	cp -r prompts bin/prompts
	cp -r scripts bin/scripts
	cp ralph-steps.json bin/

test:
	cd ralph-tui && go test -race -count=1 ./...

lint:
	cd ralph-tui && golangci-lint run

format:
	@cd ralph-tui && unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "Files not formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet:
	cd ralph-tui && go vet ./...

vulncheck:
	cd ralph-tui && govulncheck ./...

mod-tidy:
	@cd ralph-tui && go mod tidy && \
	files="go.mod"; \
	if [ -f go.sum ]; then files="$$files go.sum"; fi; \
	if ! git diff --exit-code $$files; then \
		echo "go.mod or go.sum is not tidy — run 'go mod tidy' and commit"; \
		exit 1; \
	fi

ci: test lint format vet vulncheck mod-tidy build
