# Design: Separating Audit from Clyde

## Problem

The audit package currently lives inside `Super-OAR/clyde-private` (on master, as a full copy of the clyde monorepo with an `audit/` directory added). We need:

1. **Public contributors** see only vanilla clyde — no audit code visible
2. **Internal team** sees both clyde and audit, contributes to both
3. **Internal team's local clyde** accesses the audit binary via a skill

The current setup (a private fork of the full clyde repo with audit bolted on) is fragile — it requires keeping `clyde-private` in sync with the public repo, and the audit code isn't structured as an independent project.

## Proposed Architecture: Two Repos + Skill Bridge

```
┌─────────────────────────────────┐     ┌──────────────────────────────────┐
│  superbuilders/clyde            │     │  Super-OAR/clyde-audit           │
│  (PUBLIC)                       │     │  (PRIVATE)                       │
│                                 │     │                                  │
│  main.go                        │     │  main.go          ← audit CLI   │
│  cli/                           │     │  audits/                         │
│  agent/           ← Go module   │◄────│  pipeline/                       │
│    tools/                       │     │  config/                         │
│    skills/                      │     │  testdata/                       │
│    prompts/                     │     │  go.mod  ← imports clyde/agent   │
│  tests/                         │     │  skill/                          │
│  go.mod                         │     │    SKILL.md  ← skill definition  │
│  go.work                        │     │                                  │
│                                 │     │                                  │
└─────────────────────────────────┘     └──────────────────────────────────┘
                                              │
                                              │  make install
                                              ▼
                                   ~/.local/bin/clyde-audit  (binary)
                                   ~/.agents/skills/security-audit/SKILL.md
```

### What each repo contains

**`superbuilders/clyde`** (public, unchanged):
- The clyde TUI binary
- The `agent/` Go module (library, importable by anyone)
- The skills discovery system (already built)
- Zero audit code

**`Super-OAR/clyde-audit`** (private, new — replaces `Super-OAR/clyde-private`):
- The audit CLI binary (currently in `audit/` on `Super-OAR/clyde-private` master)
- Its own `go.mod` importing `github.com/superbuilders/clyde/agent`
- A `skill/SKILL.md` that teaches clyde how to invoke the audit binary
- Self-installing via `clyde-audit install-skill` subcommand

### What `Super-OAR/clyde-private` becomes

**It gets archived or deleted.** It currently holds a full copy of the clyde repo with audit code bolted on. Once the audit code is extracted into `Super-OAR/clyde-audit` as a standalone project, `clyde-private` has no purpose. The `private` git remote gets removed from the local clyde repo. Internal team members contribute to:
- `superbuilders/clyde` (public) for clyde features
- `Super-OAR/clyde-audit` (private) for audit features

## The Skill Bridge

The skill is the key connective tissue. It's a SKILL.md file that gets installed to `~/.agents/skills/security-audit/` and teaches clyde how to invoke the audit binary.

### `skill/SKILL.md` (lives in clyde-audit repo)

```markdown
---
name: security-audit
description: "Run security and performance audits on repositories using AI agents"
triggers:
  - audit
  - security audit
  - security scan
  - vulnerability
  - performance audit
---

# Security Audit Skill

Run security and performance audits on codebases using the `clyde-audit` CLI.

## Prerequisites

The `clyde-audit` binary must be installed on the system. Verify with:
\`\`\`bash
which clyde-audit && clyde-audit --help
\`\`\`

If not installed, the user needs access to the Super-OAR/clyde-audit repo:
\`\`\`bash
go install github.com/Super-OAR/clyde-audit@latest
\`\`\`

## Usage

Use the `run_bash` tool to invoke clyde-audit. Key flags:

\`\`\`bash
# Full audit (security + performance)
clyde-audit --repo /path/to/repo

# Security only
clyde-audit --repo /path/to/repo --type security

# Performance only
clyde-audit --repo /path/to/repo --type performance

# Diff-scoped audit (only changed files)
clyde-audit --repo /path/to/repo --diff main

# Dry run (show plan without executing)
clyde-audit --repo /path/to/repo --dry-run

# Custom output directory
clyde-audit --repo /path/to/repo --output ./reports/

# Resume interrupted audit
clyde-audit --repo /path/to/repo --resume

# Check status of ongoing audit
clyde-audit --repo /path/to/repo --status

# JSON output for CI/CD
clyde-audit --repo /path/to/repo --output-format json

# Fail on severity (for CI gates)
clyde-audit --repo /path/to/repo --fail-on-severity high
\`\`\`

## Workflow

1. Ask the user which repo to audit (or use the current working directory)
2. Ask what type of audit (security, performance, or both)
3. Run the audit with `run_bash`
4. The audit produces reports in `./docs/reports/{reponame}/`
5. Read the generated reports with `read_file` and summarize findings
6. If the user wants to fix issues, use the standard file editing tools

## Output

Reports are written to `./docs/reports/{reponame}/` by default:
- `security-report.md` — Security findings
- `performance-report.md` — Performance findings
- Each finding includes severity, location, description, and remediation
```

### How it flows at runtime

```
User: "Run a security audit on this repo"
         │
         ▼
Clyde sees "audit" trigger → loads SKILL.md via read_file
         │
         ▼
Clyde follows skill instructions → run_bash("clyde-audit --repo . --type security")
         │
         ▼
clyde-audit binary runs (imports clyde/agent internally for LLM calls)
         │
         ▼
Reports generated → Clyde reads & summarizes them
```

## Installation Flow for Internal Team

### `Makefile` in clyde-audit repo:

```makefile
BINARY_NAME := clyde-audit
INSTALL_DIR := $(HOME)/.local/bin
SKILL_DIR := $(HOME)/.agents/skills/security-audit

.PHONY: build install uninstall

build:
	go build -o $(BINARY_NAME) .

install: build
	@mkdir -p $(INSTALL_DIR)
	@cp $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@mkdir -p $(SKILL_DIR)
	@cp skill/SKILL.md $(SKILL_DIR)/SKILL.md
	@echo "✓ Installed $(BINARY_NAME) to $(INSTALL_DIR)"
	@echo "✓ Installed skill to $(SKILL_DIR)"
	@echo ""
	@echo "Make sure $(INSTALL_DIR) is in your PATH."
	@echo "Restart clyde to pick up the new skill."

uninstall:
	@rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	@rm -rf $(SKILL_DIR)
	@echo "✓ Uninstalled $(BINARY_NAME) and skill"
```

### Team member setup (one time):

```bash
# 1. Clone the audit repo (requires Super-OAR access)
git clone https://github.com/Super-OAR/clyde-audit.git
cd clyde-audit

# 2. Install binary + skill
make install

# 3. Verify
clyde-audit --help          # binary works
clyde                       # start clyde — skill appears in catalog
```

After this, any clyde session automatically sees the security-audit skill in its catalog and knows how to invoke it.

## Migration Steps

1. **Create `Super-OAR/clyde-audit` repo** (private)

2. **Extract audit code from `Super-OAR/clyde-private` master**:
   - The audit source currently lives at `audit/` inside the full clyde repo on `clyde-private` master
   - Move `audit/` contents to the new repo root
   - Create proper `go.mod` with module path `github.com/Super-OAR/clyde-audit`
   - Add dependency on `github.com/superbuilders/clyde/agent@v0.2.0`
   - Rename binary from `audit` to `clyde-audit`
   - Update internal import paths (`clyde/audit/...` → `clyde-audit/...`)

3. **Create the skill**: Write `skill/SKILL.md` with frontmatter and usage docs

4. **Add `install-skill` subcommand**: Embed SKILL.md via `//go:embed`, add self-install logic

5. **Module path is already correct**: The agent module is
   `github.com/superbuilders/clyde/agent` — no changes needed on the public repo

6. **Clean up**:
   - Delete local `security-auditer` and `security-auditer-v2` branches (already deleted from private remote)
   - Archive `Super-OAR/clyde-private` (no longer needed once audit is extracted)
   - Remove the `private` git remote from local clyde repo

## Why Not a Monorepo?

| Concern | Monorepo | Separate Repos |
|---------|----------|----------------|
| Public visibility of audit code | ❌ Can't hide dirs in a public repo | ✅ Audit is in a private repo |
| Contribution model | Confusing — which repo do you PR to? | ✅ Clear: public PRs to clyde, private PRs to clyde-audit |
| Go module dependency | Circular risk if not careful | ✅ One-way: audit depends on agent, never vice versa |
| Install experience | Must build from monorepo | ✅ `go install` or `make install` |
| Skill distribution | Embedded (not useful for external users) | ✅ Installed alongside binary |
| CI/CD | Shared pipelines, complex filtering | ✅ Independent pipelines |
| Sync overhead | Must keep private fork in sync with public | ✅ No sync needed — just a dependency version |

## Dependency Direction (Critical Invariant)

```
clyde-audit ──depends-on──► clyde/agent
clyde/agent ──never-depends-on──► clyde-audit
```

This one-way dependency is what makes the whole thing work. The public `agent` module has no knowledge of the audit tool. The audit tool consumes the agent as a library, just like any other external consumer would.

## Open Questions

1. **Binary name**: `clyde-audit` vs `audit` — recommend `clyde-audit` to avoid PATH collisions *(decided: `clyde-audit`)*
2. **`go install` vs `make install`**: `go install` can't install the SKILL.md, so the binary ships a `clyde-audit install-skill` subcommand that self-installs the SKILL.md via `//go:embed`. *(decided: self-installing, no Makefile)*
3. **Versioning**: Should clyde-audit pin to a specific agent version, or track latest? Recommend pinning to tagged releases of `clyde/agent` (currently `v0.2.0` — note: `v0.1.0` is permanently poisoned on the Go module proxy with the old `this-is-alpha-iota` module path).
4. **clyde-private repo**: Create a fresh `Super-OAR/clyde-audit` repo and archive `Super-OAR/clyde-private` once extraction is complete. *(decided: fresh repo, archive old)*
