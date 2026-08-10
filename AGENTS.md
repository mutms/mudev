# mudev — AI agent starting point

Neutral bootstrap document for AI agents working in this repository. Single source of truth;
`CLAUDE.md` imports this file via `@AGENTS.md` so Claude Code, Codex, Aider, and other tools
that read `AGENTS.md` natively all see the same instructions.

`mudev` manages git checkouts for developing [MuTMS](https://github.com/mutms) plugins for
Moodle (and, longer term, general Moodle development). It replaces the old PHP tool at
`/srv/projects/old_mudev/`. It is a single Go binary — no PHP runtime.

## Scope & platform

- **Linux only.** No macOS/Windows support. Windows/macOS developers use the separate
  `mdl-demo` tool — a fat container (Apple Containers / WSL Containers) that ships mudev at the
  same `/opt/mudev` plus a Go **web UI recipe-picker**: pick a recipe → mudev clones it → managed
  demo site.
- **Git-based workflows only.** mudev assembles a Moodle code tree from a recipe (Moodle
  core + optional patches + plugins). It does **not** manage composer projects, and it does
  **not** create a database or run Moodle's phpunit/behat init — that is the caller's job
  (in CI) or `mpd`'s job (dev VM).
- License: GPL-3.0-or-later (`LICENSE.txt`).

## Canonical layout

The source tree lives under `/opt`; the three YAML catalogues are siblings
under `/srv/extra`:

| Path                     | What                                                                   |
|--------------------------|------------------------------------------------------------------------|
| `/opt/mudev`             | this source tree — developers keep it here and `make build`            |
| `/srv/extra/mdl-plugins` | plugin metadata YAML (separate repo) — the default `--plugins-dir`     |
| `/srv/extra/mdl-recipes` | public recipe YAML (separate repo) — the default `--recipes-dir`       |
| `/srv/extra/dev-recipes` | private recipe YAML (separate repo) — may not be present               |

The split is deliberate. `/opt/mudev` is a build artefact: delete it and
`make build` reproduces it from a public remote. The catalogues are *content*.

`/srv/extra` is **mudev's canonical catalogue location in any environment**,
not an mpd path that mudev happens to borrow. mpd is mudev's home, so the
layout is named after what the catalogues are rather than after who provides
them; another host — the `mdl-demo` fat container, say — creates `/srv/extra`
in turn. That is also why the defaults are hardcoded rather than injected via
`MUDEV_*_DIRECTORY` by whoever provisions the machine.

It happens to fall out well on mpd, where `/srv` is the data volume: visible
from the VM and from every runtime container at the same path, and outliving
any single runtime, which `/opt` inside a container does not. `dev-recipes` is
the one that makes that matter — hand-maintained, private, and with nowhere to
push.

The catalogues stay siblings of each other, so a recipe's relative path to the
plugin catalogue (`../../../mdl-plugins`) resolves exactly as it did under the
old all-of-it-under-`/opt` layout.

The two recipe repositories differ in kind, not just in visibility. `mdl-recipes` is the public
catalogue: `https://…` remotes, read-only, what CI and other people consume. `dev-recipes` holds
the working recipes for MuTMS development — `git@…` remotes for push access, extra remotes like
a LAN mirror (`fetch_order`), `extra.mudev: {release: mutms}`. It is not a default, so reach it
by path (`mudev clone /srv/extra/dev-recipes/mutms/dev/5.2.yaml`) or by pointing
`MUDEV_RECIPES_DIRECTORY` at it. **Never move a `git@…` dev recipe into the public catalogue.**

Developers build mudev from source (fixing bugs = recompile). CI uses a self-contained
static binary with plugin/recipe YAML checked into the CI workspace.

## Configuration (env → flag → default)

| Env var                   | Flag            | Default                  |
|---------------------------|-----------------|--------------------------|
| `MUDEV_PLUGINS_DIRECTORY` | `--plugins-dir` | `/srv/extra/mdl-plugins` |
| `MUDEV_RECIPES_DIRECTORY` | `--recipes-dir` | `/srv/extra/mdl-recipes` |

That is the whole of it. **mudev has no git authentication configuration, and adding some would
be a regression** — no URL rewriting, no token injection. A recipe names the URL it means
(`git@github.com:…` to push, `https://…` to read) and git receives it verbatim; credentials are
git's job, via an SSH agent, a credential helper or `url.<base>.insteadOf` in `~/.gitconfig`,
all of which serve every tool on the machine rather than this one. The MuTMS dev recipes spell
out `git@…` remotes, which is what made the rewrite unnecessary.

- Directory config values may be relative; they are resolved to absolute at startup.
- **Relative paths *inside* a YAML file** are anchored to the directory of that YAML file,
  then made absolute. (Binary config vs in-file paths are two separate rules.)

A recipe declares its context via `extra.mudev` — a composer-style, tool-namespaced bag (also
per plugin), opaque to the schema so any tool can extend recipes via `extra.<tool>`. mudev reads
one key, `extra.mudev.release`, a **flavour name** (string). The two levels do different jobs:
recipe-level `release: <flavour>` names **what project** the workspace is and makes it a release
workspace under that flavour's rules (tagging + version.php + changelog);
plugin-level `release: <flavour>` **selects which plugins are released** (presence flags the plugin
in, absence leaves it unreleased). There is no separate `development` flag and no bare boolean —
presence of a flavour *is* release-managed, absence is not. Dev recipes set
`extra.mudev: {release: mutms}`; CI/edition recipes omit `extra` entirely. The flavour says
nothing about how repositories are reached: that is decided solely by the URLs the recipe names.
mudev ships the built-in `mutms` flavour; other projects reuse the format with
their own flavour name. **One flavour per recipe:** the recipe-level flavour is authoritative; a
plugin whose `release` names a different flavour has its release flag **ignored** (with a warning),
never applied — so a tree never mixes release rulesets.

Catalogues are vendor-grouped: `mdl-plugins/<vendor>/<package>.yaml` (identifier
`vendor/package`) and `mdl-recipes/<vendor>/<stream>/<version>.yaml` (identifier
`vendor/stream/version`). `dev/` is the rolling full workspace (`mutms/dev/5.2`); release
editions are pinned (`mutms/full/5.2.1.01`). See `docs/plugin-format.md` and
`docs/recipe-format.md`.

## Package layout

```
cmd/mudev/main.go     thin entry point → cli.Execute()
internal/
  config/    env/flag resolution, defaults, path anchoring
  schema/    JSON Schema validation helper (Decode + Validate)
  plugin/    plugin metadata: load/parse/validate a plugin YAML
  recipe/    recipe: load/parse/validate a recipe YAML
  moodle/    pure Moodle-tree knowledge: mdlbranch→path-layout, version.php parse
  exec/      the single process gateway — ONLY package that imports os/exec
  git/       git domain wrapper over exec; URLs pass through untouched
  workspace/ orchestration, split by job rather than by command:
               clone.go    assemble a recipe into the current directory
               resolve.go  recipe entry + catalogue → install-ready description
               live.go     .mudev.json (the live recipe)
               list.go     enumerate a tree; columns.go renders it
               fanout.go   run git across every checkout (fetch, pull, status)
               add/prune/set/export.go — one file each, on top of the above
  cli/       cobra commands (one file per command)
schema/      plugin.schema.json, recipe.schema.json + the go:embed that carries them
             (go:embed cannot reach outside its own directory, so the embed lives here
             and internal/schema does the validating)
docs/        format specs, design notes, CI + build guides
```

Dependency direction: `cli → workspace → {plugin, recipe, moodle, git}`; `git` runs
subprocesses via `exec`; everything reads `config`. The leaf packages do not depend on
each other — `recipe` therefore **duplicates** the plugin-shaped structs (`Source`,
`GitSource`, `Requirement`) rather than importing `plugin`. Nothing is converted between the
two: `workspace` merges a catalogue entry with a recipe entry on the *decoded documents*
(`Raw` maps), which also means catalogue fields mudev does not model survive into the live
recipe untouched.

`moodle` is deliberately a leaf of pure functions — acquiring a core checkout is git work, so
`workspace` orchestrates it. `base.patches` is parsed but **not implemented** (MuTMS ships a
pre-merged `patch/mutms/*` core branch); a recipe that uses it is rejected with a clear error.

**Go 1.24 / dependencies.** The module targets Go 1.24, matching Debian trixie's `golang-go`,
so `make build` never needs a toolchain download. That constrains dependencies: JSON Schema
validation uses `github.com/google/jsonschema-go` because santhosh-tekuri's v6 pulls in
`golang.org/x/text`, whose current releases require Go ≥ 1.25. Check a new dependency's
minimum Go version before adding it.

**Single exec gateway (mandatory).** `internal/exec` is the only package that may import
`os/exec` or otherwise spawn a process. `git`, `moodle`, and anything else that shells out
must go through it (mirrors mpd's `VM/Exec.swift` rule). This keeps process execution
uniformly testable and consistent, and makes a future mpd→Go port map onto the same base.

## Key model decisions

- **Plugin identifier** is composer-style `vendor/package` (e.g. `mutms/tool_mulib`), and is
  the map key everywhere. File location derives from it: `<plugins-dir>/<vendor>/<package>.yaml`.
- **`relpath` is mandatory and explicit** — the install path relative to the Moodle code root,
  never derived from the frankenstyle name. It stores the *newest* Moodle layout (currently
  `public/…`, introduced in 5.1; a future 5.3 prefix would become canonical).
  For older Moodle branches that lack a prefix, mudev **strips** leading segments (never prepends).
- **`mdlbranch`** is Moodle's `$branch` code (e.g. `"502"`, a quoted string), not a version. A
  recipe pins one `mdlbranch`; each plugin's `requirements` block is keyed by git branch, each entry
  listing the `mdlbranches` it serves (and optional per-branch `plugins` deps — it merges the old
  `supported` + `require`, since deps are a property of a branch, not the plugin globally). Keys are
  branch names, not the numeric codes (numeric JSON keys are unsafe across languages), so mudev
  **inverts** it to an `mdlbranch → branch` lookup and resolves a plugin's branch directly (recipe
  may override per plugin via `source.git.ref`, which skips resolution). The pick is advisory —
  Moodle validates at install.
- Git operations **shell out to real `git`** (not go-git) so the SSH agent, subtrees, and
  normal git behavior all work.
- YAML is validated against embedded **JSON Schemas** at load time (structure/types/typos);
  cross-file logic (does a referenced plugin exist, does a branch resolve) stays in Go.

## Reference

- [`ROADMAP.md`](ROADMAP.md) — what is planned, and what has been decided against (read this
  before proposing a command; several absences are deliberate).
- [`docs/design.md`](docs/design.md) — architecture and the reasoning behind the command
  behaviour (idempotent assembly, never `git clone`, fatal core verification, and so on).
- Old PHP tool: `/srv/projects/old_mudev/` (`lib/functions.php`, `lib/plugins.php` show the
  original clone/fetch/pull/push/backup + release/tagging operations). The release flow is the
  part not yet carried over.
- mudev is built **on the mpd VM**, not inside a runtime container: `mpd --vm-setup`
  clones this repo to `/opt/mudev` and runs `make install`. The VM has `golang-go`
  (Debian 13 trixie, Go 1.24) and `make`; runtime containers deliberately have neither,
  and get `/opt/mudev` bind-mounted read-only instead, so every runtime shares the one
  binary. Rebuild after editing: `cd /opt/mudev && make install`.

## Code style

The audience is Moodle (PHP) developers, but the code stays canonical Go — `gofmt` is
non-negotiable (run `make fmt`; `make fmt-check` gates CI). Readability for PHP readers comes
from the levers that don't fight the tooling:

- **Tabs, rendered at width 4.** gofmt uses tabs; set your editor's tab width to 4 so it reads
  like Moodle's indentation. (Display only — the files are unchanged.)
- **Multiline over one-liners.** Prefer expanded function bodies; avoid one-line bodies like
  `func F() bool { return x }` even though gofmt allows them.
- **Generous vertical grouping + thorough comments.** gofmt keeps single blank lines, so use
  them to separate logical blocks; document liberally with doc comments.
- **Go naming stays Go.** `MixedCaps`, not `snake_case` — exported identifiers must be
  capitalized (that's Go's visibility mechanism, not a style choice).

## Working agreement

- After changes: `make fmt`, then `make build`, `make vet`, `make test`. `make fmt-check` is the
  gate — unformatted code does not land.
- Keep new code idiomatic Go; match surrounding style.
- **Tests use real git.** The suite builds actual repositories in `t.TempDir()` and runs real
  `git` against them, because the interesting behaviour *is* git behaviour — a stray default
  branch, an unborn HEAD, a diverged upstream. Prefer that over mocking the exec layer.
- Testing against the network is easy here too: this VM is disposable and public GitHub clones
  over HTTPS need no keys.
