# Design

## Goal

Replace the old PHP tool (`/srv/projects/old_mudev/`) with a single Go binary that manages
git checkouts for MuTMS / Moodle plugin development, and that can also assemble a full Moodle
test-site code tree in CI (forgejo-runner) for PHPUnit and Behat.

## Non-goals

- No composer project management (mudev assembles via git). But note the catalogue is *also*
  consumed by a live composer-based path: `mutms/seed` + `mutms/seed-mutenancy` are composer
  project templates using Moodle 5.1+'s `moodle/moodle` package (`moodle/composer-installer`),
  with the **patched core pulled through composer** from the same `mutms/patches` repo (a `vcs`
  repository + `"moodle/moodle": "dev-patch/mutms/…"`). So one patches repo serves both mudev
  (git subtree/branch) and composer (vcs). A recipe → `composer.json` seed is a natural future
  output (`source.composer` = package name, release version = constraint, `base` block =
  core + patches vcs) — but generating those seeds is out of mudev's initial git-focused scope.
- No database setup or Moodle phpunit/behat init (caller/`mpd` does that).
- No macOS/Windows support (see `mdl-demo`).

## Two run modes

| Mode | Where          | Catalogs                                           | Git access                     | Binary                   |
|------|----------------|----------------------------------------------------|--------------------------------|--------------------------|
| Dev  | `/opt/mudev`   | `/srv/extra/mdl-plugins`, `/srv/extra/mdl-recipes` | `git@…` remotes + an SSH agent | `make build` from source |
| CI   | forgejo-runner | YAML checked into CI workspace                     | `https://…` remotes, anonymous | pinned static binary     |

CI overrides everything via env/flags; dev relies on the shared catalogue defaults
under `/srv/extra`.

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
  a `# yaml-language-server: $schema=…` modeline. Cross-file logic stays in Go. The library
  choice is constrained: the module targets **Go 1.24**, matching Debian trixie's `golang-go` so
  a build never downloads a toolchain, and santhosh-tekuri's v6 pulls in `golang.org/x/text`,
  whose current releases need Go ≥ 1.25. Check a new dependency's minimum Go version.
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
- **Explicit `relpath`, canonical newest layout, strip-down for old branches**: avoids the
  fragile frankenstyle→path derivation and keeps a single source of truth per plugin.
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

## Lineage

The old PHP tool (`lib/functions.php`) provided list, clone, fetch, pull, push and backup, plus
the release flow — set version/release, tag, tag-push, tag-delete, changelog. The git-facing
verbs are implemented here; the release flow is the main piece still to come (see
[ROADMAP.md](../ROADMAP.md)). The recipe/live-recipe layer — a site as a declared composition,
with `add`, `prune` and `export` on top — is new.
