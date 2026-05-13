**Agent Skills Design Document**  
**Clyde – Open Agent Skills Support**  

**Status**: Draft / Proposal  
**Date**: May 2026  
**Author**: Grok (on behalf of the team)  
**Target**: `main` branch (post v0.x)  

---

### 1. Goals

We want Clyde to support the **open Agent Skills standard** (agentskills.io / SKILL.md format) in the exact same lightweight, portable way that Pi, Cursor, Codex CLI, and others do.

**Specifically**:
- Follow **Pi’s philosophy** exactly (no dedicated `load_skill` tool).
- Model loads full skills **using the existing `read_file` tool** (the de-facto standard way).
- **No slash commands** (`/skill:name`) – keep the harness minimal and model-driven.
- **Only use the universal `.agents/` discovery paths** (no `.clyde/` folders).
- Skills support is **enabled by default**.
- Zero breaking changes to existing tools, prompt caching, or REPL behavior.
- Full compatibility with community skill repos (drop a skill folder anywhere and it just works).

**Non-goals**:
- Adding new tool schemas
- Slash-command parser
- Custom `skill://` URI scheme

---

### 2. High-Level Architecture (Harnesses vs. System Prompt Split)

We keep the exact split that has become the ecosystem standard:

| Component     | Responsibility                              | Where in Clyde                     |
|---------------|---------------------------------------------|------------------------------------|
| **Harness**   | Discovery, metadata parsing, catalog building | New `agent/skills/` package       |
| **System Prompt** | Tiny catalog + one-paragraph usage rule   | Dynamically appended to `system.txt` |
| **Model**     | Decides when to load; calls `read_file`     | No changes needed                 |

This keeps context tiny (only metadata lives in the initial prompt) and makes skills work on Claude 3.5/4, Opus, etc. without any post-training.

---

### 3. Discovery Locations (in priority order)

At startup (and on `reload` / `skills reload` command) Clyde will scan **only** the universal `.agents/` locations (in priority order):

1. Project-local: `./.agents/skills/`  
2. User-global: `~/.agents/skills/`

Skills are **folders** containing at minimum a `SKILL.md` file.

---

### 4. SKILL.md Parsing

For every discovered folder:

- Read `SKILL.md`
- Parse **only the YAML frontmatter** (using `gopkg.in/yaml.v3`)
- Extract:
  ```yaml
  name: frontend-design
  description: Apply modern, accessible, responsive UI/UX best practices...
  version: 1.2
  triggers: []          # optional
  ```
- Ignore everything else until the model asks for it.

Invalid/missing frontmatter → log warning but still load as a “raw” skill (name = folder name).

---

### 5. Skills Catalog in System Prompt

We will **dynamically append** a block to the system prompt (right before the tool definitions).  

Example catalog block (exact wording to be finalized in `agent/skills/catalog.go`):

```markdown
You have access to specialized Agent Skills.

Available skills:
• frontend-design – Apply modern, accessible, responsive UI/UX best practices...
• tdd-cycle – Follow strict red-green-refactor workflow...
• security-scan – Check for OWASP Top 10 issues...

When a user request matches a skill’s description, call the `read_file` tool on the skill’s SKILL.md path (relative or absolute) to load its full instructions. Then follow those instructions exactly.
```

This block is **~100–300 tokens** even with 50+ skills.

Implementation notes:
- Built once at startup and cached (fits perfectly with existing prompt caching).
- In dev mode: rebuild catalog on every `skills reload`.

---

### 6. How a Skill Is Loaded (Model-Driven)

1. Model sees catalog → decides a skill matches.
2. Model issues a normal tool call:
   ```json
   {
     "name": "read_file",
     "arguments": {
       "path": ".agents/skills/frontend-design/SKILL.md"
     }
   }
   ```
   (or the absolute path – both work).

3. Harness returns the **full Markdown content** of `SKILL.md` (including frontmatter – models are fine with it).
4. Content is injected into conversation history → model now follows the skill’s step-by-step instructions.

No new code paths, no new tools. `read_file` already exists and is battle-tested.

---

### 7. Implementation Plan (Go packages)

Create new package: `agent/skills/`

```
agent/skills/
├── catalog.go          # discovery + metadata parsing + catalog string builder
├── loader.go           # (optional) helper to validate paths are inside skill dirs
├── types.go            # SkillMetadata struct
├── watcher.go          # (future) optional fsnotify hot-reload
└── registry.go         # main SkillsRegistry that lives in Agent struct
```

Changes needed elsewhere:

- `agent/agent.go` – add `SkillsRegistry` to the Agent struct.
- `agent/prompts/system.txt` – add a placeholder comment:
  ```txt
  <!-- SKILLS_CATALOG will be injected here by the harness -->
  ```
- `cli/repl.go` (or wherever the prompt is built) – call `registry.BuildCatalogBlock()` and insert it.
- Add CLI command: `clyde skills list` and `clyde skills reload` (**nice-to-have**, not required for core functionality).

---

### 8. Backward Compatibility & Safety

- Agent Skills support is **enabled by default** with zero runtime overhead when no skills folders exist.
- All skill paths are sandboxed to the `.agents/skills/` discovery folders (we already do similar path validation for `read_file`).
- Existing prompt caching still works (catalog is part of the cached system prompt).
- Zero impact on non-skill users.

---

### 9. Shippable Testable User Stories with Acceptance Criteria

**US-1: Skills are automatically discovered and catalogued at startup**  
*As a developer, when I place a valid skill folder in `./.agents/skills/` or `~/.agents/skills/`, Clyde automatically discovers it and injects the skills catalog into the system prompt.*  
**AC**:
- Discovery happens silently at startup (and on `skills reload`).
- Catalog block is present in the generated system prompt (verifiable via `clyde debug prompt` or verbose logs).
- Works for both project-local and global skills.
- Zero extra tokens or performance impact when no skills are present.

**US-2: Model can load and follow a skill using existing tooling**  
*As the agent, when a user request matches a skill description, I can load the full skill via `read_file` and follow its instructions.*  
**AC**:
- Model issues a `read_file` call on the correct `SKILL.md` path.
- Full skill content is injected into context.
- Subsequent model responses follow the skill’s step-by-step instructions.
- Graceful handling of malformed skills (warning logged, raw Markdown still returned).

These two stories fully cover the feature and can be implemented and merged independently.
