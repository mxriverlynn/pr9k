.PHONY: build test lint format format-check vet vulncheck mod-tidy ci

build:
	rm -rf bin
	mkdir -p bin/.pr9k/workflow
	cd src && go build -o ../bin/pr9k ./cmd/pr9k
	cp -r workflow/prompts bin/.pr9k/workflow/prompts
	cp -r workflow/scripts bin/.pr9k/workflow/scripts
	cp workflow/config.json bin/.pr9k/workflow/
	cp ralph-art.txt bin/.pr9k/workflow/

test:
	cd src && go test -race -count=1 ./...

lint:
	cd src && golangci-lint run

GOFMT_PATHS := cmd internal tools.go

format:
	cd src && gofmt -w $(GOFMT_PATHS)

format-check:
	@cd src && unformatted=$$(gofmt -l $(GOFMT_PATHS)); \
	if [ -n "$$unformatted" ]; then \
		echo "Files not formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet:
	cd src && go vet ./...
	cd src && go vet -tags tools .

vulncheck:
	cd src && govulncheck ./...

mod-tidy:
	@cd src && go mod tidy && \
	files="go.mod"; \
	if [ -f go.sum ]; then files="$$files go.sum"; fi; \
	if ! git diff --exit-code $$files; then \
		echo "go.mod or go.sum is not tidy — run 'go mod tidy' and commit"; \
		exit 1; \
	fi

ci: test lint format-check vet vulncheck mod-tidy build
