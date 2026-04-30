# Release Process

## Versioning Convention

Clyde uses a **multi-module monorepo** with two Go modules that are versioned independently using prefixed tags:

| Module | Tag prefix | Example | Install command |
|--------|-----------|---------|-----------------|
| CLI binary (root) | `v` | `v0.1.0` | `go install github.com/this-is-alpha-iota/clyde@v0.1.0` |
| Agent library | `agent/v` | `agent/v0.1.0` | `go get github.com/this-is-alpha-iota/clyde/agent@v0.1.0` |

### Lockstep releases

Both tags are created on the **same commit** for lockstep releases. The root `go.mod` pins `require agent@vX.Y.Z` (with no `replace` directive — `go.work` handles local development).

### Semantic versioning

- **Pre-v1**: Use `v0.x.y`. No compatibility guarantees per Go semver convention.
- **Post-v1** (future): Follow standard semver rules — breaking changes bump major, features bump minor, fixes bump patch.

## Creating a Release

### Automated (recommended)

```bash
# Dry run — shows what would happen without executing
make release VERSION=0.1.0 DRY_RUN=1

# Real release
make release VERSION=0.1.0
```

Or use the script directly:

```bash
# Dry run
DRY_RUN=1 ./scripts/release.sh 0.1.0

# Real release
./scripts/release.sh 0.1.0
```

### What the release script does

1. **Verifies** the working tree is clean (no uncommitted changes).
2. **Builds** both modules (`go build ./...`).
3. **Runs** unit tests (integration tests with API keys are optional).
4. **Updates** root `go.mod` to pin `agent@vX.Y.Z` and removes the `replace` directive.
5. **Commits** the `go.mod`/`go.sum` changes.
6. **Tags** both `agent/vX.Y.Z` and `vX.Y.Z` on the same commit.
7. **Pushes** both tags to `origin`.
8. **Triggers** Go module proxy indexing.
9. **Prints** post-release verification commands.

### Manual release (if script fails)

```bash
VERSION=0.1.0

# 1. Ensure clean tree
git status

# 2. Build and test
go build ./...
cd tests && go test ./... -short -count=1; cd ..

# 3. Update go.mod
sed -i '' "s|github.com/this-is-alpha-iota/clyde/agent v.*|github.com/this-is-alpha-iota/clyde/agent v${VERSION}|" go.mod
sed -i '' '/^replace github.com\/this-is-alpha-iota\/clyde\/agent/d' go.mod
go mod tidy

# 4. Commit
git add go.mod go.sum
git commit -m "release: pin agent to v${VERSION}"

# 5. Tag
git tag "agent/v${VERSION}"
git tag "v${VERSION}"

# 6. Push
git push origin master "agent/v${VERSION}" "v${VERSION}"

# 7. Trigger proxy indexing
GOPROXY=https://proxy.golang.org go list -m "github.com/this-is-alpha-iota/clyde/agent@v${VERSION}"
GOPROXY=https://proxy.golang.org go list -m "github.com/this-is-alpha-iota/clyde@v${VERSION}"
```

## Post-Release Verification

After a release, verify with:

```bash
# Verify proxy has indexed both modules
GOPROXY=https://proxy.golang.org go list -m github.com/this-is-alpha-iota/clyde/agent@v0.1.0
GOPROXY=https://proxy.golang.org go list -m github.com/this-is-alpha-iota/clyde@v0.1.0

# Verify external consumer smoke test
./scripts/test-external-consume.sh v0.1.0

# Verify CLI installation
go install github.com/this-is-alpha-iota/clyde@v0.1.0

# Verify agent library
go get github.com/this-is-alpha-iota/clyde/agent@v0.1.0
```

## Post-Release Development

After a release, the `go.work` file at repo root ensures local development continues seamlessly:

```
go 1.24.0

use (
    .
    ./agent
)
```

This means:
- Local changes to `agent/` are immediately reflected when building the CLI.
- No need to publish a new version between releases.
- `go.mod` shows the pinned release version, but `go.work` overrides it locally.
- Running `GOWORK=off go build .` fetches the published agent version (useful for testing the published module).

## Release Checklist

- [ ] All unit tests pass (`cd tests && go test ./... -count=1`)
- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` is clean
- [ ] Working tree is clean (`git status`)
- [ ] `docs/progress.md` is updated
- [ ] Run `make release VERSION=x.y.z`
- [ ] Verify proxy indexing
- [ ] Run external consumer smoke test with tagged version
