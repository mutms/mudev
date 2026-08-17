# mudev — AI agent starting point

Neutral bootstrap document for AI agents working in this repository. Single source of truth;
`CLAUDE.md` imports this file via `@AGENTS.md` so Claude Code, Codex, Aider, and other tools
that read `AGENTS.md` natively all see the same instructions.

`mudev` manages git checkouts for developing [MuTMS](https://github.com/mutms) plugins for
Moodle (and, longer term, general Moodle development).

## Scope & platform

- **Linux only.** No macOS/Windows support. Windows/macOS developers use the separate
  `mdl-demo` tool — an OCI container (source on GitHub, run under Apple Containers / WSL
  Containers) that ships mudev at the same `/opt/mudev` plus a Go **web UI recipe-picker**.
  Inside that container mudev assembles the Moodle web site the picker serves — the same `clone`
  path CI and the dev VM use, driven by the web UI instead of the command line.
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
be a regression** — a recipe names the URL it means (`git@…` to push, `https://…` to read) and
git receives it verbatim; credentials are git's job (SSH agent, credential helper,
`url.<base>.insteadOf`). *Why these choices* has the full rationale.

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
mudev ships the built-in `mutms` flavour; other projects reuse the format with their own flavour
name. **One flavour per recipe** — the recipe-level flavour is authoritative; see *Why these
choices* for how it is resolved and how a mismatched plugin flavour is handled.

Catalogues are vendor-grouped: `mdl-plugins/<vendor>/<package>.yaml` (identifier
`vendor/package`) and `mdl-recipes/<vendor>/<stream>/<version>.yaml` (identifier
`vendor/stream/version`). `dev/` is the rolling full workspace (`mutms/dev/5.2`); release
editions are pinned (`mutms/release/5.2.2.01`). See `docs/plugin-format.md` and
`docs/recipe-format.md`.

## Package layout

All Go code lives under `go/` (the module root — `go/go.mod`), so `make build` runs from there;
`docs/` and the top-level Markdown stay at the repo root.

```
go/
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
docs/        format specs, design notes (repo root, outside the module)
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

## Reference

- mudev is meant to be built in the mpd VM, not inside a runtime container: `mpd --vm-setup`
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


## Assembly outputs

The same recipe drives two kinds of output (mirroring the old tool's split of `plugins.php`
vs `subtree-build-*.sh`):

- **Per-plugin checkouts** — individual plugin repos cloned at their paths, for developers who
  fetch/pull/push each plugin. The dev workflow.
- **Subtree distro repo** — one consolidated git repo: patched Moodle core with every plugin
  merged in as a squashed `git subtree` at its resolved path/ref. No submodules, no deploy-time
  fetches — a single `git clone` yields the whole distro. Tagged/released builds are the
  artifact **sysadmins clone to produce production server images**; `.mudev.json` records
  provenance. `extra.mudev: {release: <flavour>}` (recipe) + per-plugin `extra.mudev: {release: <flavour>}`
  gate which checkouts/plugins are release-managed (tag + version.php + changelog).

Build mechanism (proven against public repos): clone `base.source.git.remotes.origin`@`base.source.git.ref`
(a pre-patched core branch), then `git subtree add --prefix <resolved-path> <source.git.remotes.origin>
<resolved-branch> --squash` per plugin. Branch/path come straight from the catalogue + recipe.

## Packages & data flow

```
cli ──▶ workspace ──▶ plugin   (metadata: identity, path, requirements [support + deps])
                 └──▶ recipe   (the base tree + plugin refs)
                 └──▶ moodle   (path layout, version.php parsing)
                 └──▶ git      (exec `git`; URLs passed through untouched)
                              └──▶ exec (the only package that spawns a process)
all ───▶ config   (env/flag resolution, path anchoring)
schema ─ embedded JSON Schemas; used by the plugin and recipe loaders
```

Leaf packages (`plugin`, `recipe`, `moodle`, `git`) don't depend on each other; `workspace`
composes them. Inside `workspace`, the files split by job rather than by command: `clone.go`
assembles, `resolve.go` turns a recipe entry into an install-ready description, `live.go` owns
`.mudev.json`, `list.go` enumerates a tree, `fanout.go` runs git across every checkout, and
`add`/`prune`/`set`/`export` are each one file on top of those.

## Why these choices

- **Plugin identifier** is composer-style `vendor/package` (e.g. `mutms/tool_mulib`), and is the
  map key everywhere; the file location derives from it (`<plugins-dir>/<vendor>/<package>.yaml`).
- **Shell out to `git`**, not go-git: SSH agent, subtrees, submodules, and normal git semantics
  just work; go-git's SSH/subtree support is weak. This also means **no hosting platform is
  assumed** — arbitrary git URLs clone as-is (bare `user@host:path` SSH, `ssh://`, `file://`,
  https), so mudev fits platform-less enterprise setups (bare SSH-file-access repos, private
  per-customer plugins) as well as GitHub. Recipe-driven assembly *replaces* site-level
  submodule/superrepo composition; submodules *inside* a plugin are still handled on clone.
- **No authentication policy, deliberately.** mudev has no credential settings: a recipe names
  the URL it means — `git@github.com:…` for a checkout you push from, `https://…` for one you
  only read — and git receives it verbatim. mudev once rewrote github https URLs to SSH and
  could inject a token into https clones; both were removed. The MuTMS dev recipes spell out
  their `git@…` remotes, which made the rewrite dead code, and every mechanism it duplicated
  already exists in git — an SSH agent, a credential helper, `url.<base>.insteadOf` in
  `~/.gitconfig` — where it applies to every tool on the machine instead of this one. Config
  that guesses at a URL's meaning is config that will eventually guess wrong, and a token in a
  recipe is a secret in a file that gets committed.
- **Single exec gateway** (`internal/exec`): the one package allowed to import `os/exec`.
  `git`/`moodle` are domain wrappers over it. Mirrors mpd's mandatory `VM/Exec.swift` rule,
  keeps process execution uniformly testable, and lets a future mpd→Go port reuse the base.
- **JSON Schema validation** of hand-authored YAML: the plugin/recipe files are edited by
  people in *separate* repos, so schema validation (google/jsonschema-go over yaml.v3-decoded
  data) gives precise structural errors, and the published schemas power editor autocomplete via
  a `# yaml-language-server: $schema=…` modeline. Cross-file logic stays in Go. (The library
  choice is constrained by the Go 1.24 floor — see *Package layout*.)
- **Neutral, adoptable data model vs. mudev-private state.** The catalogue (`mdl-plugins`) and
  source recipes (`mdl-recipes`) are tool-agnostic YAML that *anyone* can consume and extend:
  the composer-style `extra.<tool>` namespace lets a composer assembler, the catalogue website,
  or third-party/proprietary tooling attach its own config with no schema change. That is the
  shared public contract. The **live recipe** (`/.mudev.json`) is the deliberate exception — it
  reuses the recipe schema but is **mudev's own operational state** in a checkout, not part of
  that contract and not meant for other tools to build on. In the same spirit, a plugin's
  **`source` is a map of acquisition kinds keyed by name** (`git` today; `zip`/`composer`
  reserved), *not* a single `type`-tagged method — so one catalogue entry can advertise several
  ways to fetch the same code at once and each consumer reads the kind it supports (mudev → `git`,
  a composer assembler → `composer`, a plain installer → `zip`). Each kind is self-contained
  (git → `remotes` + `ref`), which is why the git version selector lives at `source.git.ref`, not
  as a method-agnostic top-level field.
- **Explicit `relpath`, canonical newest layout, strip-down for old branches**: `relpath` is the
  mandatory, explicit install path relative to the Moodle code root — never derived from the
  frankenstyle name (which avoids a fragile derivation and keeps a single source of truth). It
  stores the *newest* layout (currently `public/…`, introduced in 5.1); for older branches that
  lack the prefix mudev **strips** leading segments, never prepends.
- **`mdlbranch`-keyed branch resolution**: plugin git-branch naming is inconsistent, so a plugin's
  `requirements` block is keyed by git branch, each entry listing the `mdlbranches` it serves (and
  optional per-branch `plugins` deps); mudev **inverts** it to the `mdlbranch → branch` lookup at
  load (the branch name is a value, never derived). Keying by branch dedupes the many-to-one
  (500/501/502 → one branch) and lets **dependencies vary per branch** — the reason `requirements`
  merges the old `supported` + `require` (a dep is a property of a branch, not the plugin globally).
  The numeric `mdlbranch` codes stay values inside `mdlbranches`, never object keys, because numeric
  object keys are unsafe across languages (JS reorders them, PHP coerces them to ints); branch-name
  keys are safe strings. The pick is advisory — Moodle validates at install — and is skipped
  entirely when a recipe pins `source.git.ref`.
- **Flavour resolved once, up front.** The recipe-level `extra.mudev.release` names *what project*
  the workspace is, so mudev resolves it to a single **flavour handler** at the very start of a
  run — before any git write — and threads that one handler through the whole execution. The
  handler encapsulates the project's release rules (tag convention, `version.php`
  `$version`/`$release`, changelog); a small registry maps flavour name → handler and is the sole
  extension seam (a fork registers one more; the built-in `mutms` is just a one-entry registry).
  This keeps the code logic simple by construction: there is **one** decision point and **one**
  place the unknown-flavour case is handled — an unresolved name yields no handler, so release
  subcommands are disabled workspace-wide (assembly/clone/git still work) with a single warning,
  rather than being re-decided or re-checked per plugin. The plugin-level flag then does no ruleset
  selection at all — it only **selects which plugins** the already-chosen handler acts on (presence =
  released; a mismatched flavour is ignored + warned). No mixed-ruleset state is representable.
  Because flavours are compiled in, an unknown flavour usually means the **wrong binary** (a fork's
  handler lives only in the fork's build), and release still *refuses* rather than mis-tagging — so
  the only real risk is misdiagnosis. To close that, the unknown-flavour warning/error must **name
  the flavours this build supports** (e.g. `unknown release flavour "acme" — this build supports:
  mutms`), turning "recipe looks broken" into the obvious "I'm running the wrong mudev." For a
  *proactive* check there is no dedicated command — `mudev --version` carries the build info
  (version + the compiled-in flavour(s)), which is where people already look to identify a binary.

## How the commands behave, and why

- **Assembly is idempotent, not one-shot.** Every step asks whether it has already been done, so
  an interrupted `clone` resumes by re-running it and a recipe that gained a plugin is caught up
  the same way. The live recipe is written **incrementally** — core first, then one plugin at a
  time, through a temp file and a rename — so an interruption leaves an accurate partial record
  rather than a lie. (The old PHP tool worked this way and it mattered every time the plugin set
  changed.) A workspace assembled from a *different* recipe is refused rather than mixed.
- **Never `git clone`.** Core *and* plugins are acquired with init → remotes → fetch → checkout
  of exactly the recipe's ref. `git clone -b` cannot cover the cases a recipe can name: it fails
  outright on a commit pin, refuses a non-empty directory (core's already exists, and holds the
  live recipe by then), and names the local branch after the remote one, so a `localbranch`
  override would need a rename afterwards. Clone-then-switch would leave a stray default branch
  in twenty repositories. One path handles all of it and leaves no branch nobody asked for.
- **Core is verified or the run dies.** A `.git` directory is not proof of a checkout: a
  repository with no commits is an interrupted fetch and gets completed. Afterwards the tree's
  own `version.php` — at the exact path `strippublic` implies, not guessed — must exist and
  declare the recipe's `mdlbranch`. A mismatch is fatal, not a warning, because every plugin
  path and branch resolution downstream derives from that claim. This is the bug that once left
  a workspace with twenty plugins installed on an empty core.
- **Fetch every remote, in the recipe's order.** Git objects are content-addressed, so priming
  from a mirror on the local network leaves the slow remote only the difference to send — for a
  full Moodle core, a LAN copy instead of a 1.4 GB download. Only `origin` is load-bearing: an
  unreachable mirror is reported and stepped over, because a backup host must not become a
  single point of failure.
- **Stop at the first failure, and say which checkout.** With twenty repositories scrolling
  past, an error collected and reported at the end is an error that gets missed. `pull` is
  `--ff-only` for the same reason: resolving a divergence is a decision for a human in one
  repository, not something to paper over twenty times.
- **Say what is worth saying, and count the rest.** `status` shows only checkouts with
  uncommitted changes or unpushed commits, and reports how many it passed over — silence about a
  repository must not look like there being nothing to say about it. Being merely *behind* is not
  reported there; that is incoming work, and `list` shows it as `N↓`.
- **mudev never deletes plugin code.** A checkout can hold uncommitted changes, unpushed commits
  or a stash, none of which the recipe knows about and none of which mudev could give back. So
  removal is split: the developer deletes the directory, `recipe prune` reconciles the record.
- **A flattened recipe is used exactly as written.** An entry carrying its own `relpath` and
  remotes does not consult the catalogue at all — otherwise cloning an exported recipe on a
  machine that happens to have a catalogue installed would merge local fields in underneath and
  build a different tree. Self-containment is the whole point of flattening.
- **Reported, not enforced.** A plugin's `requirements` lists what its branch depends on, and
  `recipe add` reports what the workspace lacks without installing it. Composing a dev site is
  the developer's decision, and Moodle validates dependencies at install time anyway.
