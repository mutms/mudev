# Roadmap

What mudev does today, and what is planned. Design rationale for what already exists lives in
[docs/design.md](docs/design.md); the file formats are in
[docs/plugin-format.md](docs/plugin-format.md) and [docs/recipe-format.md](docs/recipe-format.md).

## Today

mudev assembles a Moodle code tree from a recipe and manages the checkouts in it:

| Command                         | What it does                                                      |
|---------------------------------|-------------------------------------------------------------------|
| `clone <recipe>`                | assemble a tree into the current directory; idempotent, resumable |
| `list`                          | one line per checkout — state, branch, version, release           |
| `status [git args…]`            | git status, only where there is something to report               |
| `fetch`                         | every remote of every checkout, in the recipe's order             |
| `pull`                          | `git pull --ff-only` everywhere; stops at a divergence            |
| `recipe init`                   | reconstruct `.mudev.json` from the checkouts already in a tree    |
| `recipe update <relpath>`       | fold one checkout's current state (adopt or refresh) into the record |
| `recipe add <plugin>`           | check a plugin out and record it                                  |
| `recipe prune`                  | forget plugins whose directories are gone                         |
| `recipe set <key> <value>`      | name the workspace: `name`, `description`, `contributed_by`       |
| `recipe export [--file x.yaml]` | render the workspace as a portable recipe                         |

## Next: release management (`mudev release`)

The MuTMS release process, carried over from the old PHP tool's `release.php`. This is the
largest remaining piece and is deliberately left until a real release is pending, so the
commands are shaped by an actual release rather than by guesswork.

It is **flavour-gated**: a recipe opts in with `extra.mudev.release: <flavour>`, and the flavour
names a compiled-in ruleset (mudev ships `mutms`). The plugin-level flag then selects which
plugins are released. See [docs/recipe-format.md](docs/recipe-format.md) for the model, which is
implemented already — only the release commands themselves are missing.

- `release set-version <v>` — set `$plugin->version` across the released plugins **and** update
  the inter-plugin dependency versions in `version.php`; validate the format.
- `release set-release <r>` — set `$plugin->release`, drop `MATURITY_BETA`, roll each
  `CHANGELOG.md` `[Unreleased]` section to `[<release>]` with a date.
- `release tag` — **atomic and validated**: check the whole tree first (every `version.php`
  parses, `$version` is `YYYYMMDD` + the branch suffix, `$release` is `vX.Y.Z.NN`, the tree is
  clean, each plugin is on its expected branch). If any plugin is off, tag **nothing** and name
  the offender — a partially tagged release is worse than none.
- `release tag-push` / `tag-delete` — only MuTMS-owned tags, never a fork's upstream tags.
- `release changelog` — aggregate each plugin's latest changelog section into one file.
- Generate pinned edition recipes by resolving each plugin's branch to its latest release tag.
  Editions are heterogeneous (a plugin's tag follows *its* branch, not the recipe's Moodle
  version), which is why they must be generated rather than hand-written.

`mudev --version` should carry the build info and the compiled-in flavours, so an unknown
flavour is diagnosed as "wrong binary" rather than "broken recipe".

## Smaller things

- `push`, `diff`, `log`, `branch` on the existing fan-out — one small function each, waiting to
  see which are actually wanted.
- `mudev validate` — schema-check a whole catalogue as a CI lint gate.
- `mudev recipe list [--json]` — enumerate available recipes with their metadata; the
  machine-readable form would drive `mdl-demo`'s recipe picker.
- `mudev recipe diff` — compare the workspace against the recipe it was assembled from, in both
  directions (the source moved on ↔ plugins were added locally). A preview for a future
  `switch`/`upgrade` between recipe versions.
- `recipe add` reading an *existing* checkout rather than the catalogue: `recipe update <relpath>`
  now adopts an unmanaged checkout (the `?` a listing flags) by reading its own remotes and
  component, and `recipe init` does a whole tree at once — so the remaining gap is only that
  `recipe add`, which *creates* a checkout from the catalogue, still would not reuse one already
  on disk. Largely covered; kept as a note.
- Detect unmerged local branches (`list` currently flags only HEAD ≠ recorded).
- Gather `list` status concurrently — one checkout costs about five git processes.
- `backup` — push branches and tags to a mirror remote.
- Shallow clones, for CI.
- Subtree distro build: core plus every plugin `git subtree add`-ed at its path, producing the
  single-repo artifact a sysadmin clones to build a production image. The other assembly mode,
  and the reason plugins are installed ancestors-first.
- `make build-static` for linux amd64 + arm64, and publishing the schemas at stable URLs so the
  editor `$schema` modeline resolves.

## Deliberately not planned

- **`recipe remove`.** Deleting a directory yourself and running `recipe prune` is the whole
  interface. A wrapper around `rm -rf` would have to be trusted about uncommitted changes,
  unpushed commits and stashes before it earned its place.
- **Installing a plugin's dependencies automatically.** `requirements` is information; composing
  a dev site is the developer's decision, and Moodle validates dependencies at install time.
  `recipe add` reports what a workspace is missing and stops there.
- **`base.patches`.** Parsed and rejected with a clear error. MuTMS ships a pre-merged
  `patch/mutms/*` core branch instead, so there is no need yet.
- **Pinning an export to exact commits.** `export` records what the workspace was assembled to
  be. Freezing a tree to commit hashes is a different promise and would want its own command.
