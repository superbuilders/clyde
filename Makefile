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

# ---------------------------------------------------------------------------
# Bonnie (branch `bonnie` only) — see BONNIE.md
#
# These targets exist so Bonnie development can NEVER overwrite the clyde
# binary installed on this machine. They build to ./bin/ under different
# names. Do not "fix" them by simplifying them back to `clyde`.
# ---------------------------------------------------------------------------
.PHONY: bonnie clyde-next bonnie-dev bonnie-guard bonnie-clean install

# The session viewer, built as `bonnie`. This is where the work happens.
bonnie:
	@mkdir -p bin
	cd session-viewer && go build -o ../bin/bonnie .
	@echo "built ./bin/bonnie — run it by absolute path, never install it"

# The agent, built as `clyde-next`. Only needed for the umask change.
clyde-next:
	@mkdir -p bin
	go build -o bin/clyde-next .
	@echo "built ./bin/clyde-next — NOT installed, NOT on PATH"

# Run against a scratch HOME, port 8788, tmux socket bonnie-dev.
bonnie-dev: bonnie
	@./scripts/bonnie-dev.sh

# Assert the installed clyde binary is byte-identical to the M0 baseline.
bonnie-guard:
	@./scripts/bonnie-guard.sh

bonnie-clean:
	rm -rf bin "$${BONNIE_SANDBOX:-$$HOME/.bonnie-sandbox}"

# Tripwire: there is no install target on this branch, on purpose.
install:
	@echo ""
	@echo "  ⛔ REFUSING TO INSTALL."
	@echo ""
	@echo "  You are on the 'bonnie' branch. Installing from here would replace"
	@echo "  the clyde binary this machine (and possibly your own session) is"
	@echo "  running on. See BONNIE.md."
	@echo ""
	@echo "  Build with 'make bonnie' or 'make clyde-next' and run ./bin/<name>"
	@echo "  by absolute path instead."
	@echo ""
	@exit 1

# Headless auth e2e (PLAN.md §5). Targets a *running* instance; set BONNIE_URL
# to the deployed URL for a milestone gate.
.PHONY: e2e
e2e:
	@./scripts/bonnie-e2e.sh
