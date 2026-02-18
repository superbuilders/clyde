# Project Rename to "Clyde" - Complete! ✅

## What Was Done

Successfully renamed the project from "claude-repl" / "go-coding-agent" to "clyde".

### Code Changes ✅
- [x] Updated go.mod module path: `github.com/this-is-alpha-iota/clyde`
- [x] Updated all Go file imports to use new module path
- [x] Renamed binary from `claude-repl` to `clyde`
- [x] Changed config directory from `~/.claude-repl/` to `~/.clyde/`
- [x] Updated README.md with new name and instructions
- [x] Updated main.go startup banner: "Clyde - AI Coding Agent"
- [x] Updated browse tool User-Agent: `clyde/1.0`
- [x] Updated .gitignore to ignore `clyde` binary
- [x] Documented rename in progress.md
- [x] All tests pass (25 tests)
- [x] Changes pushed to GitHub

### Commits
1. `60c3702` - Start renaming project to clyde: update go.mod and .gitignore
2. `b35c5de` - Add untracked files before multi-patch
3. `1a39f0c` - Update imports to use clyde package name
4. `0265303` - Complete rename to clyde: update test imports, README, config paths, and binary name
5. `c845701` - Rename project to 'clyde' - standardize naming across repo and code
6. `c1b3778` - Remove obsolete install-error.txt

## Next Step: Rename GitHub Repository 🔧

To complete the rename, you should rename the GitHub repository itself:

### Option 1: Via GitHub Web UI (Recommended)
1. Go to https://github.com/this-is-alpha-iota/go-coding-agent
2. Click "Settings" (top right)
3. Scroll to "Repository name"
4. Change "go-coding-agent" to "clyde"
5. Click "Rename"

GitHub will automatically:
- Redirect old URLs to new name
- Update clone URLs
- Keep all issues, PRs, and history intact

### Option 2: Via GitHub CLI
```bash
gh repo rename clyde --repo this-is-alpha-iota/go-coding-agent
```

### After Renaming on GitHub

Update your local git remote:
```bash
git remote set-url origin https://github.com/this-is-alpha-iota/clyde.git
```

Verify it worked:
```bash
git remote -v
```

## Installation Instructions

Once the GitHub repo is renamed, users can install with:

```bash
go install github.com/this-is-alpha-iota/clyde@latest
```

Then set up config:
```bash
mkdir -p ~/.clyde
cat > ~/.clyde/config << 'EOF'
TS_AGENT_API_KEY=your-anthropic-api-key
BRAVE_SEARCH_API_KEY=your-brave-api-key  # Optional
EOF
```

Run it:
```bash
clyde
```

## Benefits of This Rename

1. **Consistency**: Module path, repo name, and binary name all match
2. **No Install Errors**: `go install` will work correctly now
3. **Memorable**: "Clyde" is shorter and easier to remember than "claude-repl"
4. **Professional**: Clean, branded naming across the project
5. **SEO**: Single name makes project easier to find and reference

## Status

✅ **Code rename: COMPLETE**  
⏳ **GitHub repo rename: TODO** (requires repository settings change)

Once you rename the GitHub repository, the entire project will be consistently named "clyde"!
