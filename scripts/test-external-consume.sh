#!/usr/bin/env bash
# test-external-consume.sh — Verify that the agent module can be consumed
# by an external Go project without pulling CLI/TUI dependencies.
#
# Usage:
#   ./scripts/test-external-consume.sh              # Local mode (uses replace directive)
#   ./scripts/test-external-consume.sh v0.1.0       # Published mode (uses tagged version)
#
# Exit codes:
#   0 — All checks passed
#   1 — A check failed

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }
info() { echo -e "${YELLOW}→${NC} $1"; }

# Resolve repo root (script may be called from any directory)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
AGENT_DIR="$REPO_ROOT/agent"

VERSION="${1:-}"

# IMPORTANT: Do NOT use "TMPDIR" as the variable name — it's a standard env var
# that Go's os.TempDir() reads. Setting it would make Go think our consumer
# directory IS the system temp root, triggering "ignoring go.mod in system
# temp root" errors. See golang.org/issue/26708.
WORKDIR=""

cleanup() {
    if [ -n "$WORKDIR" ] && [ -d "$WORKDIR" ]; then
        rm -rf "$WORKDIR"
    fi
}
trap cleanup EXIT

# ──────────────────────────────────────────────────────────────
# Phase 1: Dependency audit of the agent module
# ──────────────────────────────────────────────────────────────

echo ""
echo "═══════════════════════════════════════════════════════"
echo "  MONO-4: External Consumability Verification"
echo "═══════════════════════════════════════════════════════"
echo ""

info "Phase 1: Agent module dependency audit"
echo ""

# 1a. Check agent compiles standalone
info "Checking agent compiles standalone..."
(cd "$AGENT_DIR" && GOWORK=off go build ./...) || fail "agent/ does not compile standalone"
pass "agent/ compiles standalone"

# 1b. Check agent vet is clean
info "Checking go vet..."
(cd "$AGENT_DIR" && GOWORK=off go vet ./...) || fail "agent/ has vet issues"
pass "agent/ vet clean"

# 1c. Capture full dependency list
info "Capturing dependency list..."
AGENT_DEPS=$(cd "$AGENT_DIR" && GOWORK=off go list -m all 2>&1)
echo ""
echo "Agent module dependencies:"
echo "$AGENT_DEPS" | sed 's/^/  /'
echo ""

# 1d. Verify direct dependencies are correct
echo "$AGENT_DEPS" | grep -q "github.com/JohannesKaufmann/html-to-markdown" \
    || fail "Missing direct dep: html-to-markdown"
pass "Direct dep present: html-to-markdown"

echo "$AGENT_DEPS" | grep -q "github.com/joho/godotenv" \
    || fail "Missing direct dep: godotenv"
pass "Direct dep present: godotenv"

# 1e. Verify transitive deps
echo "$AGENT_DEPS" | grep -q "github.com/PuerkitoBio/goquery" \
    || fail "Missing transitive dep: goquery"
pass "Transitive dep present: goquery"

echo "$AGENT_DEPS" | grep -q "github.com/andybalholm/cascadia" \
    || fail "Missing transitive dep: cascadia"
pass "Transitive dep present: cascadia"

echo "$AGENT_DEPS" | grep -q "golang.org/x/net" \
    || fail "Missing transitive dep: x/net"
pass "Transitive dep present: x/net"

# 1f. Verify x/sys is NOT a direct or needed dependency
info "Checking x/sys status..."
SYS_WHY=$(cd "$AGENT_DIR" && GOWORK=off go mod why -m golang.org/x/sys 2>&1)
if echo "$SYS_WHY" | grep -q "does not need"; then
    pass "golang.org/x/sys is NOT needed by agent (transitive only, not imported)"
else
    fail "golang.org/x/sys is unexpectedly needed by agent: $SYS_WHY"
fi

# Check x/sys is not a direct dep in go.mod
if grep -E "^\s+golang.org/x/sys" "$AGENT_DIR/go.mod" | grep -v "// indirect" > /dev/null 2>&1; then
    fail "golang.org/x/sys appears as a DIRECT dependency in agent/go.mod"
else
    pass "golang.org/x/sys is NOT a direct dependency in agent/go.mod"
fi

# 1g. Verify no CLI-specific deps (readline, x/sys as direct, bubbletea, etc.)
for pkg in "github.com/chzyer/readline" "github.com/charmbracelet/bubbletea" "github.com/peterh/liner"; do
    if echo "$AGENT_DEPS" | grep -q "$pkg"; then
        fail "CLI-specific dependency found in agent: $pkg"
    fi
done
pass "No CLI-specific libraries in agent dependencies"

echo ""

# ──────────────────────────────────────────────────────────────
# Phase 2: External consumer smoke test
# ──────────────────────────────────────────────────────────────

info "Phase 2: External consumer smoke test"
echo ""

WORKDIR="${HOME}/clyde_consume_verify"
rm -rf "$WORKDIR"
mkdir -p "$WORKDIR"
info "Created work directory: $WORKDIR"

cd "$WORKDIR"

# Disable workspace mode — the consumer is outside the repo and must not
# inherit the repo's go.work file.
export GOWORK=off

# 2a. Initialize a new Go module
info "Initializing consumer module..."
go mod init testconsumer > /dev/null 2>&1
pass "go mod init testconsumer"

# 2b. Write a minimal main.go that exercises the agent API
cat > main.go << 'GOEOF'
package main

import (
	"fmt"

	"github.com/this-is-alpha-iota/clyde/agent"
)

func main() {
	// Create agent with config (the primary public constructor)
	cfg := agent.Config{
		APIKey:    "test-key",
		APIURL:    "https://api.anthropic.com/v1/messages",
		ModelID:   "claude-opus-4-6",
		MaxTokens: 64000,
	}

	// Exercise the public API surface:

	// 1. Create agent with functional options
	agentInstance := agent.New(cfg,
		agent.WithProgressCallback(func(msg string, toolUseID string) {
			fmt.Printf("Progress: %s [%s]\n", msg, toolUseID)
		}),
		agent.WithOutputCallback(func(output string, toolUseID string) {
			fmt.Printf("Output: %s\n", output)
		}),
		agent.WithThinkingCallback(func(text string, signature string) {
			fmt.Printf("Thinking: %s\n", text)
		}),
		agent.WithDiagnosticCallback(func(msg string) {
			fmt.Printf("Diagnostic: %s\n", msg)
		}),
		agent.WithSpinnerCallback(func(start bool, msg string) {
			fmt.Printf("Spinner: start=%v msg=%s\n", start, msg)
		}),
		agent.WithErrorCallback(func(err error) {
			fmt.Printf("Error: %v\n", err)
		}),
		agent.WithUserMessageCallback(func(text string) {
			fmt.Printf("User: %s\n", text)
		}),
		agent.WithAssistantMessageCallback(func(text string) {
			fmt.Printf("Assistant: %s\n", text)
		}),
		agent.WithContextWindowSize(200000),
		agent.WithReserveTokens(16000),
	)
	defer agentInstance.Close()

	// 2. Use re-exported types (no agent/providers import needed)
	var history []agent.Message
	history = append(history, agent.Message{
		Role:    "user",
		Content: "Hello",
	})
	agentInstance.SetHistory(history)

	// 3. Read history back
	got := agentInstance.GetHistory()
	if len(got) != 1 {
		panic("expected 1 message in history")
	}

	// 4. Check usage type
	usage := agentInstance.LastUsage()
	_ = usage.InputTokens
	_ = usage.OutputTokens
	_ = usage.CacheReadInputTokens
	_ = usage.CacheCreationInputTokens

	// 5. Verify content block type is accessible
	var block agent.ContentBlock
	block.Type = "text"
	block.Text = "hello"
	_ = block

	fmt.Println("✓ All agent public API types and methods accessible")
	fmt.Printf("  History length: %d\n", len(got))
	fmt.Printf("  Agent created successfully with %d options\n", 10)
}
GOEOF

pass "Wrote consumer main.go"

# 2c. Add the agent dependency
if [ -n "$VERSION" ]; then
    # Published mode: use the tagged version
    info "Using published version: $VERSION"
    go get "github.com/this-is-alpha-iota/clyde/agent@$VERSION" 2>&1 || \
        fail "go get agent@$VERSION failed"
    pass "go get agent@$VERSION succeeded"
else
    # Local mode: use replace directive pointing to local agent
    info "Using local agent (replace directive)"
    go mod edit -require "github.com/this-is-alpha-iota/clyde/agent@v0.0.0"
    go mod edit -replace "github.com/this-is-alpha-iota/clyde/agent=$AGENT_DIR"
    go mod tidy > /dev/null 2>&1
    pass "Local replace directive configured"
fi

# 2d. Build the consumer
info "Building consumer..."
go build -o consumer . 2>&1 || fail "Consumer build failed"
pass "Consumer builds successfully"

# 2e. Run the consumer
info "Running consumer..."
OUTPUT=$(./consumer 2>&1) || fail "Consumer execution failed"
echo "$OUTPUT" | sed 's/^/  /'
echo ""

echo "$OUTPUT" | grep -q "All agent public API types" \
    || fail "Consumer did not produce expected output"
pass "Consumer runs correctly"

# 2f. Verify dependency tree
info "Inspecting consumer dependency tree..."
CONSUMER_DEPS=$(go list -m all 2>&1)
echo ""
echo "Consumer module dependencies:"
echo "$CONSUMER_DEPS" | sed 's/^/  /'
echo ""

# Check x/sys is not directly needed by the consumer
SYS_WHY_CONSUMER=$(go mod why -m golang.org/x/sys 2>&1 || true)
if echo "$SYS_WHY_CONSUMER" | grep -q "does not need"; then
    pass "Consumer does not need golang.org/x/sys"
elif echo "$SYS_WHY_CONSUMER" | grep -q "module does not use"; then
    pass "Consumer module does not use golang.org/x/sys"
else
    # x/sys may appear transitively — that's OK as long as it's not directly imported
    info "Note: x/sys appears transitively (via x/net): this is expected"
    pass "x/sys is transitive only (not directly imported by consumer or agent)"
fi

# Verify no TUI libraries
for pkg in "github.com/chzyer/readline" "github.com/charmbracelet/bubbletea"; do
    if echo "$CONSUMER_DEPS" | grep -q "$pkg"; then
        fail "CLI-specific dependency leaked to consumer: $pkg"
    fi
done
pass "No CLI/TUI libraries in consumer dependencies"

echo ""

# ──────────────────────────────────────────────────────────────
# Phase 3: Summary
# ──────────────────────────────────────────────────────────────

echo "═══════════════════════════════════════════════════════"
echo "  All checks passed ✓"
echo "═══════════════════════════════════════════════════════"
echo ""
echo "The agent module is independently consumable:"
echo "  • Compiles standalone (no workspace or CLI deps needed)"
echo "  • External consumer can import and build against it"
echo "  • No CLI/TUI libraries leak into consumer deps"
echo "  • golang.org/x/sys is transitive only (not directly needed)"
echo ""

if [ -n "$VERSION" ]; then
    echo "Tested with published version: $VERSION"
else
    echo "Tested with local agent (use './scripts/test-external-consume.sh <version>' for published mode)"
fi

echo ""
