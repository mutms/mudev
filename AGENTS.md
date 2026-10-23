# mudev — AI agent starting point

Neutral bootstrap document for AI agents working in this repository.

`mudev` manages git checkouts for developing [MuTMS](https://github.com/mutms) plugins for
Moodle (and, longer term, general Moodle development).

## Scope & platform

- **Linux only.** macOS/Windows developers use `mdl-demo`, an OCI container that ships mudev
  at the same `/opt/mudev` and drives `clone` from a web UI recipe-picker.
- **Git-based workflows only.** mudev assembles a Moodle code tree from a recipe (Moodle
  core + plugins). It does **not** manage composer projects, create a database or run
  Moodle's phpunit/behat init — that is the caller's job (CI) or `mpd`'s (dev VM).
- License: GPL-3.0-or-later (`LICENSE.txt`).

## Canonical layout

| Path                     | What                                                               |
|--------------------------|--------------------------------------------------------------------|
| `/opt/mudev`             | this source tree; rebuild with `make install`                      |
| `/srv/extra/mdl-plugins` | plugin metadata YAML (separate repo) — the default `--plugins-dir` |
| `/srv/extra/mdl-recipes` | public recipe YAML (separate repo) — the default `--recipes-dir`   |
| `/srv/extra/dev-recipes` | private recipe YAML (separate repo) — may not be present           |

`/srv/extra` is mudev's catalogue location in every environment, not an mpd path; `mdl-demo`
creates it too. The catalogues stay siblings, so a recipe's relative path to the plugin
catalogue (`../../../mdl-plugins`) resolves.

`mdl-recipes` is public: `https://…` remotes, read-only, consumed by CI and others.
`dev-recipes` holds the MuTMS working recipes: `git@…` remotes and extra mirrors
(`fetch_order`). Reach it by path
(`mudev clone /srv/extra/dev-recipes/mutms/dev/5.2.yaml`) or via `MUDEV_RECIPES_DIRECTORY`.
**Never move a `git@…` dev recipe into the public catalogue.**

On the mpd VM, `mpd --vm-setup` clones this repo and runs `make install`. Runtime containers
have no Go; they get `/opt/mudev` bind-mounted read-only. CI uses the static binaries from
`make build-static`.

## Configuration (env → flag → default)

| Env var                   | Flag            | Default                  |
|---------------------------|-----------------|--------------------------|
| `MUDEV_PLUGINS_DIRECTORY` | `--plugins-dir` | `/srv/extra/mdl-plugins` |
| `MUDEV_RECIPES_DIRECTORY` | `--recipes-dir` | `/srv/extra/mdl-recipes` |

- Directory values may be relative; they are made absolute at startup.
- Relative paths *inside* a YAML file are anchored to that file's directory.
- **No git authentication configuration, and adding some would be a regression.** A recipe
  names the URL it means and git receives it verbatim; credentials are git's job. See
  `docs/recipe-format.md` § Remote URLs and credentials.

Formats: `docs/plugin-format.md` (identity, `source` kinds, `relpath`, branch resolution) and
`docs/recipe-format.md` (refs, `extra.mudev`, flavours, clone/init/update flows). Commands:
`docs/usage.md`.

**No release tooling in this version.** Tagging, `version.php` bumps and changelogs are planned
for Moodle 5.3 only; MuTMS releases up to 5.2 are made with an older tool. mudev only reads
`extra.mudev.release` and warns on a flavour not in `knownFlavours`
(`internal/workspace/clone.go`). The recommended release recipe is the `release` edition: one
subtree-merged repository (`mutms/mutms.git`) at a tag, `plugins: []` — no per-plugin pins.

## Package layout

All Go code lives under `go/` (the module root); `docs/` stays at the repo root.

```
go/
  cmd/mudev/main.go     thin entry point → cli.Execute()
  internal/
    config/    env/flag resolution, defaults, path anchoring
    schema/    JSON Schema validation helper
    plugin/    load/parse/validate a plugin YAML
    recipe/    load/parse/validate a recipe YAML
    moodle/    pure Moodle-tree knowledge: mdlbranch → path layout, version.php parse
    exec/      the single process gateway — ONLY package that imports os/exec
    git/       git wrapper over exec; URLs pass through untouched
    workspace/ orchestration, split by job rather than by command:
                 clone.go    assemble a recipe into the current directory
                 resolve.go  recipe entry + catalogue → install-ready description
                 live.go     .mudev.json (the live recipe)
                 list.go     enumerate a tree; columns.go renders it
                 fanout.go   run git across every checkout (fetch, pull, status)
                 add/prune/set/export/push/production/camp.go — one job each
    cli/       cobra commands, one file per command
  schema/      plugin.schema.json, recipe.schema.json + the go:embed that carries them
```

Dependency direction: `cli → workspace → {plugin, recipe, moodle, git}`; `git → exec`;
everything reads `config`. Leaf packages do not import each other, so `recipe` duplicates the
plugin-shaped structs instead of importing `plugin`. `workspace` merges catalogue and recipe
entries on the decoded `Raw` maps, so fields mudev does not model survive into the live recipe.

- **Single exec gateway (mandatory).** Only `internal/exec` may import `os/exec` or spawn a
  process — the same rule as mpd's `go/internal/exec`.
- **Shell out to `git`, not go-git.** SSH agents, submodules and arbitrary URLs (`ssh://`,
  `file://`, bare `user@host:path`) work, and no hosting platform is assumed.
- `moodle` stays pure functions; acquiring core is git work, so `workspace` does it.
- `base.patches` is parsed but not implemented; a recipe that uses it is rejected.
- The `go` directive in `go/go.mod` picks the compiler (`GOTOOLCHAIN=auto`, set in the
  Makefile). Raise it on purpose, not as a side effect of adding a dependency.

## How the commands behave, and why

- **Assembly is idempotent.** Every step checks whether it is done, so an interrupted `clone`
  resumes by re-running it. The live recipe is written incrementally (temp file + rename), so an
  interruption leaves an accurate partial record. A workspace from a *different* recipe is refused.
- **Never `git clone`.** Core and plugins use init → remotes → fetch → checkout of exactly the
  recipe's ref. `git clone -b` fails on a commit pin, refuses core's non-empty directory, and
  leaves a stray default branch.
- **Core is verified or the run dies.** An empty repository is an interrupted fetch and gets
  completed. Core's `version.php`, at the path `strippublic` implies, must declare the recipe's
  `mdlbranch` — every plugin path and branch derives from it.
- **Fetch every remote, in the recipe's order.** A LAN mirror first leaves `origin` only the
  difference. Only `origin` must be reachable; an unreachable mirror is reported and skipped.
- **Stop at the first failure, and name the checkout.** `pull` is `--ff-only`: a divergence is
  a human decision.
- **`status` shows only checkouts with something to say** (uncommitted or unpushed) and counts
  the rest. Being behind is shown by `list` as `N↓`.
- **mudev never deletes plugin code.** The developer deletes the directory; `recipe prune`
  reconciles the record.
- **A flattened recipe is used exactly as written.** An entry with its own `relpath` and remotes
  never consults the catalogue, so an exported recipe builds the same tree anywhere.
- **Requirements are reported, not enforced.** `recipe add` lists missing dependencies; Moodle
  validates at install time.

## Code style

The audience is Moodle (PHP) developers, but the code stays canonical Go:

- `gofmt` is non-negotiable (`make fmt`; `make fmt-check` is the gate). Set your editor's tab
  width to 4.
- Prefer expanded function bodies over one-liners.
- Use blank lines to separate logical blocks; document with doc comments.
- Go naming stays Go: `MixedCaps`, never `snake_case`.

## Working agreement

- After changes: `make fmt`, then `make build`, `make vet`, `make test`.
- Keep new code idiomatic Go; match surrounding style.
- **Tests use real git.** The suite builds repositories in `t.TempDir()` and runs real `git`,
  because the interesting behaviour is git behaviour. Prefer that over mocking `exec`.
- Network tests are fine: the VM is disposable and public GitHub clones over HTTPS need no keys.
