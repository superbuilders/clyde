# Migration Plan: Extract Audit into Separate Repo

**Decisions locked in:**
- Binary name: `clyde-audit`
- Repo: Create `Super-OAR/clyde-audit` fresh (don't repurpose `clyde-private`)
- Install: Self-installing via `clyde-audit install-skill` subcommand (no Makefile)

---

## Phase 1: Create the New Repo

### Step 1.1 — Create `Super-OAR/clyde-audit` on GitHub

```bash
gh repo create Super-OAR/clyde-audit --private --description "AI-powered security and performance auditor (Clyde plugin)"
```

### Step 1.2 — Clone it locally

```bash
cd ~/Projects  # or wherever
git clone https://github.com/Super-OAR/clyde-audit.git
cd clyde-audit
```

---

## Phase 2: Extract Audit Code

### Step 2.1 — Copy audit files from the branch

From the `clyde` repo, extract the `audit/` subtree from `security-auditer-v2`:

```bash
# From the clyde repo
cd ~/Projects/clyde  # adjust path

# Export audit directory as an archive and extract into clyde-audit
git archive security-auditer-v2 -- audit/ | tar -x -C ~/Projects/clyde-audit --strip-components=1
```

This gives `clyde-audit/` the following structure:
```
clyde-audit/
├── audits/
│   ├── performance.go
│   ├── prompts/          (7 .md prompt files)
│   ├── registry.go
│   ├── registry_test.go
│   └── security.go
├── config/
│   ├── config.go
│   └── config_test.go
├── pipeline/
│   ├── checker.go / checker_test.go
│   ├── diff.go / diff_test.go
│   ├── e2e_test.go
│   ├── json_output.go / json_output_test.go
│   ├── pipeline.go / pipeline_test.go
│   ├── report.go / report_test.go
│   ├── resume_test.go
│   ├── runner.go / runner_test.go
│   ├── severity.go / severity_test.go
│   └── state.go / state_test.go
├── testdata/
│   └── sample-repo/ (4 files)
├── main.go
└── main_test.go
```

### Step 2.2 — Create `go.mod`

```bash
cd ~/Projects/clyde-audit
go mod init github.com/Super-OAR/clyde-audit
```

This creates:
```
module github.com/Super-OAR/clyde-audit

go 1.24.0
```

### Step 2.3 — Rewrite import paths

Every internal import needs to change from `github.com/superbuilders/clyde/audit/...` to `github.com/Super-OAR/clyde-audit/...`.

**Files and their import changes:**

| File | Old Import | New Import |
|------|-----------|------------|
| `main.go` | `clyde/audit/audits` | `clyde-audit/audits` |
| `main.go` | `clyde/audit/config` | `clyde-audit/config` |
| `main.go` | `clyde/audit/pipeline` | `clyde-audit/pipeline` |
| `main_test.go` | `clyde/audit/audits` | `clyde-audit/audits` |
| `main_test.go` | `clyde/audit/pipeline` | `clyde-audit/pipeline` |
| `pipeline/checker.go` | `clyde/audit/audits`, `clyde/audit/config` | (same pattern) |
| `pipeline/checker_test.go` | `clyde/audit/audits`, `clyde/audit/config` | (same pattern) |
| `pipeline/e2e_test.go` | `clyde/audit/audits`, `clyde/audit/config`, `clyde/audit/pipeline` | (same pattern) |
| `pipeline/json_output.go` | `clyde/audit/audits`, `clyde/audit/config` | (same pattern) |
| `pipeline/json_output_test.go` | `clyde/audit/audits`, `clyde/audit/config` | (same pattern) |
| `pipeline/pipeline.go` | `clyde/audit/audits`, `clyde/audit/config` | (same pattern) |
| `pipeline/pipeline_test.go` | `clyde/audit/audits`, `clyde/audit/config` | (same pattern) |
| `pipeline/report.go` | `clyde/audit/audits`, `clyde/audit/config` | (same pattern) |
| `pipeline/report_test.go` | `clyde/audit/audits`, `clyde/audit/config` | (same pattern) |
| `pipeline/runner.go` | `clyde/agent` | **No change** (stays as external dep) |
| `pipeline/severity.go` | `clyde/audit/audits`, `clyde/audit/config` | (same pattern) |
| `pipeline/severity_test.go` | `clyde/audit/audits`, `clyde/audit/config` | (same pattern) |

One-liner to do this:

```bash
find . -name '*.go' -exec sed -i '' \
  's|github.com/superbuilders/clyde/audit/|github.com/Super-OAR/clyde-audit/|g' {} +
```

**Critical: Do NOT touch the `clyde/agent` import** — that stays as `github.com/superbuilders/clyde/agent` because it's an external dependency on the public repo.

### Step 2.4 — Resolve dependencies

```bash
go mod tidy
```

This will:
- Add `github.com/superbuilders/clyde/agent v0.1.0` as a dependency
- Add `github.com/joho/godotenv` (used directly in main.go)
- Pull in transitive deps from agent (html-to-markdown, etc.)

### Step 2.5 — Update `flag.Usage` binary name

In `main.go`, update the usage string:

```go
// Old
fmt.Fprintf(os.Stderr, `Usage: audit --repo <path> [options]
// New  
fmt.Fprintf(os.Stderr, `Usage: clyde-audit --repo <path> [options]
```

And all references in the usage examples:
```
// Old: audit --repo /path/to/repo
// New: clyde-audit --repo /path/to/repo
```

### Step 2.6 — Verify it builds

```bash
go build -o clyde-audit .
./clyde-audit --help
```

### Step 2.7 — Run tests

```bash
go test ./... -v
```

---

## Phase 3: Add `install-skill` Subcommand

### Step 3.1 — Embed the SKILL.md

Create `skill/SKILL.md` in the repo (full content in the design doc).

Use `//go:embed` to bake it into the binary:

```go
// skill/embed.go
package skill

import _ "embed"

//go:embed SKILL.md
var SkillMD string
```

### Step 3.2 — Add subcommand to `main.go`

Before `flag.Parse()`, check for the subcommand:

```go
func main() {
    // Handle subcommands before flag parsing
    if len(os.Args) > 1 {
        switch os.Args[1] {
        case "install-skill":
            installSkill()
            return
        case "uninstall-skill":
            uninstallSkill()
            return
        }
    }

    // ... existing flag parsing and audit logic ...
}

func installSkill() {
    home, err := os.UserHomeDir()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: could not determine home directory: %v\n", err)
        os.Exit(1)
    }

    skillDir := filepath.Join(home, ".agents", "skills", "security-audit")
    skillPath := filepath.Join(skillDir, "SKILL.md")

    if err := os.MkdirAll(skillDir, 0755); err != nil {
        fmt.Fprintf(os.Stderr, "Error: could not create skill directory: %v\n", err)
        os.Exit(1)
    }

    if err := os.WriteFile(skillPath, []byte(skill.SkillMD), 0644); err != nil {
        fmt.Fprintf(os.Stderr, "Error: could not write SKILL.md: %v\n", err)
        os.Exit(1)
    }

    fmt.Printf("✓ Installed skill to %s\n", skillPath)
    fmt.Println("  Restart clyde to pick up the new skill.")
}

func uninstallSkill() {
    home, err := os.UserHomeDir()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: could not determine home directory: %v\n", err)
        os.Exit(1)
    }

    skillDir := filepath.Join(home, ".agents", "skills", "security-audit")

    if err := os.RemoveAll(skillDir); err != nil {
        fmt.Fprintf(os.Stderr, "Error: could not remove skill directory: %v\n", err)
        os.Exit(1)
    }

    fmt.Printf("✓ Uninstalled security-audit skill from %s\n", skillDir)
}
```

### Step 3.3 — Create `skill/SKILL.md`

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

The `clyde-audit` binary must be installed. Verify:
```bash
which clyde-audit
```

## Usage

Use the `run_bash` tool to invoke `clyde-audit`:

```bash
# Full audit (security + performance)
clyde-audit --repo /path/to/repo

# Security only
clyde-audit --repo /path/to/repo --type security

# Scoped to recent changes
clyde-audit --repo /path/to/repo --diff main

# Dry run
clyde-audit --repo /path/to/repo --dry-run

# Resume interrupted audit
clyde-audit --repo /path/to/repo --resume

# JSON output for CI/CD
clyde-audit --repo /path/to/repo --output-format json --fail-on-severity high
```

## Workflow

1. Ask the user which repo to audit (default: current working directory)
2. Ask what type of audit (security, performance, or both)
3. Run with `run_bash("clyde-audit --repo <path> --type <type>")`
4. Reports land in `./docs/reports/{reponame}/`
5. Read reports with `read_file` and summarize findings for the user
6. Offer to fix issues using standard file editing tools

## Output

Default output: `./docs/reports/{reponame}/`
- `security-report.md` — Security findings
- `performance-report.md` — Performance findings  
- `final-report.md` — Consolidated report
- Each finding includes severity, location, description, and remediation
```

### Step 3.4 — Verify the full flow

```bash
go build -o clyde-audit .
./clyde-audit install-skill
ls ~/.agents/skills/security-audit/SKILL.md   # confirm it's there
./clyde-audit --help                           # confirm audit still works
./clyde-audit uninstall-skill                  # confirm cleanup works
```

---

## Phase 4: Repo Hygiene

### Step 4.1 — Create README.md

Write a README for the audit repo covering:
- What it does
- Installation (`go install` + `clyde-audit install-skill`)
- Usage (standalone CLI + via clyde skill)
- Configuration (`~/.clyde/config` for API key, `~/.clyde/audit-config` for defaults)
- Development setup

### Step 4.2 — Add `.gitignore`

```
clyde-audit
*.exe
.env
docs/reports/
```

### Step 4.3 — Initial commit and push

```bash
git add -A
git commit -m "Initial commit: extract audit from clyde monorepo"
git push origin main
```

---

## Phase 5: Verify End-to-End

### Step 5.1 — Clean install test

```bash
# From scratch (simulates a new team member)
cd /tmp
git clone https://github.com/Super-OAR/clyde-audit.git
cd clyde-audit
go build -o clyde-audit .
./clyde-audit install-skill

# Verify clyde sees it
cd ~/Projects/some-repo
clyde
# > "Run a security audit on this repo"
# Should load the skill and invoke clyde-audit
```

### Step 5.2 — `go install` test

```bash
go install github.com/Super-OAR/clyde-audit@latest
clyde-audit install-skill
clyde-audit --repo /path/to/test-repo --dry-run
```

---

## Phase 6: Cleanup

### Step 6.1 — Delete old branches from clyde repo

```bash
cd ~/Projects/clyde

# Delete remote branches
git push origin --delete security-auditer 2>/dev/null
git push origin --delete security-auditer-v2 2>/dev/null
git push private --delete security-auditer 2>/dev/null
git push private --delete security-auditer-v2 2>/dev/null

# Delete local branches
git branch -D security-auditer
git branch -D security-auditer-v2
```

### Step 6.2 — Archive `Super-OAR/clyde-private`

The private mirror is no longer needed. Archive it:

```bash
gh repo archive Super-OAR/clyde-private --yes
```

And remove the remote from clyde:

```bash
cd ~/Projects/clyde
git remote remove private
```

### Step 6.3 — Clean up clyde's go.work (if needed)

The current `go.work` references `.` and `./agent`. No audit reference exists on master, so nothing to change.

### Step 6.4 — Update clyde README

Remove any references to audit branches or `clyde-private`. Add a note in the "Using Clyde as a Library" section mentioning `clyde-audit` as an example external consumer.

---

## Execution Order & Dependencies

```
Phase 1 ──► Phase 2 ──► Phase 3 ──► Phase 4 ──► Phase 5 ──► Phase 6
 create      extract     add          repo        verify      clean up
 repo        + rewrite   install-     hygiene     e2e         old
             imports     skill                                branches
```

**Phases 1-4 are the core work.** Phase 5 is validation. Phase 6 is cleanup (can be deferred).

## Risk Assessment

| Risk | Mitigation |
|------|-----------|
| `clyde/agent` API changes break audit | Pin to tagged version (`v0.1.0`). Update deliberately. |
| Team forgets `install-skill` after `go install` | Print reminder: "Run `clyde-audit install-skill` to enable the clyde integration" |
| Import path typo breaks builds | `go mod tidy` + `go test ./...` in Phase 2 catches this immediately |
| Old branches linger and confuse | Phase 6 cleans them up; can be done same day |

## Estimated Effort

| Phase | Time |
|-------|------|
| Phase 1: Create repo | 2 min |
| Phase 2: Extract + rewrite imports | 15 min |
| Phase 3: install-skill subcommand | 15 min |
| Phase 4: README, .gitignore, push | 10 min |
| Phase 5: E2E verification | 10 min |
| Phase 6: Cleanup | 5 min |
| **Total** | **~1 hour** |
