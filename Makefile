.PHONY: build test vet release test-external clean

# Default target
all: build

# Build the CLI binary
build:
	go build -o clyde .

# Run all tests
test:
	cd tests && go test ./... -count=1 -timeout 120s

# Run go vet
vet:
	go vet ./...

# Clean build artifacts
clean:
	rm -f clyde

# Release a new version
# Usage: make release VERSION=0.1.0
# Dry run: make release VERSION=0.1.0 DRY_RUN=1
release:
ifndef VERSION
	$(error VERSION is required. Usage: make release VERSION=0.1.0)
endif
	@DRY_RUN=$(DRY_RUN) ./scripts/release.sh $(VERSION)

# Run external consumer smoke test
# Usage: make test-external
#        make test-external VERSION=0.1.0
test-external:
ifdef VERSION
	@./scripts/test-external-consume.sh $(VERSION)
else
	@./scripts/test-external-consume.sh
endif
