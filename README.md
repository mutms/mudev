# mudev — Moodle/MuTMS development tool

A single Go binary that assembles a Moodle code tree from a **recipe** — Moodle core plus a
selection of plugins, each as its own git checkout at its place in the tree — and then manages
those twenty-odd repositories as one workspace. Written for developing
[MuTMS](https://github.com/mutms) plugins, and for building Moodle test-site trees in CI.

A recipe is YAML, and it says what a site *is*: which Moodle, which plugins, which versions.
mudev turns that into a working tree, records what it did in `.mudev.json`, and gives you one
command each for the things you would otherwise run twenty times by hand.

**Linux only.** Windows/macOS developers use `mdl-demo`, a Linux container that ships mudev.

## Commands

| Command                      | What it does                                                     |
|------------------------------|------------------------------------------------------------------|
| `clone <recipe>`             | assemble a tree into the current directory; idempotent, resumable |
| `list`                       | one line per checkout — state, branch, version, release           |
| `status [git args…]`         | git status, only where there is something to report               |
| `fetch`                      | every remote of every checkout, in the recipe's order             |
| `pull`                       | `git pull --ff-only` everywhere; stops at a divergence            |
| `recipe add <plugin>`        | check a plugin out and record it                                  |
| `recipe prune`               | forget plugins whose directories are gone                         |
| `recipe set <key> <value>`   | name the workspace                                                |
| `export [--file x.yaml]`     | render the workspace as a portable recipe                         |

## Quick start

```sh
cd /opt/mudev
make build            # → ./bin/mudev  (needs Go 1.24+: apt-get install -y golang-go)
make install          # optional: symlinks ~/.local/bin/mudev at it
mudev --help
```

With the standard `/opt` layout, mudev finds `/opt/mdl-plugins` and `/opt/mdl-recipes`
automatically. See [docs/build.md](docs/build.md) for CI binaries.

## Configuration

There are two settings, and each resolves **flag > environment > built-in default**.

| Environment               | Flag            | Default            | What             |
|---------------------------|-----------------|--------------------|------------------|
| `MUDEV_PLUGINS_DIRECTORY` | `--plugins-dir` | `/opt/mdl-plugins` | plugin catalogue |
| `MUDEV_RECIPES_DIRECTORY` | `--recipes-dir` | `/opt/mdl-recipes` | recipe catalogue |

**There is no git authentication configuration, deliberately.** A recipe names the URL it means
— `git@github.com:…` for a checkout you push from, `https://…` for one you only read — and mudev
hands it to git untouched, whatever the host. Anything else is git's own job and works the same
for every tool on your machine: an SSH agent, a credential helper, or a rewrite rule in
`~/.gitconfig`:

```sh
git config --global url."git@github.com:".insteadOf "https://github.com/"
```

## Assembling a workspace

```sh
mkdir -p ~/sites/mutms52 && cd ~/sites/mutms52   # mudev never creates the directory,
mudev clone mutms/full/5.2.1.01                  # so it keeps your owner/permissions
```

`clone` takes either a catalogue identifier (`vendor/stream/version`) or a path to a recipe
file. It checks Moodle core out into the current directory and every plugin as its own git
repository at its place in the tree, excluded in the surrounding repo so `git status` stays
clean. What it did is recorded step by step in `.mudev.json` (the *live recipe*), and the
command is idempotent: run it again to resume an interrupted clone, or to pick up plugins the
recipe has gained since.

## Daily driver

```sh
mudev list                       # every checkout, its state, branch and release
mudev list --columns path,state,recorded
```

```
.                                      *     MOODLE_502_STABLE...origin/*  2026071600.00  5.2.1+
public/admin/tool/certificate                MOODLE_502_STABLE...origin/*  2026042100     5.0.7
public/admin/tool/mulib                1↑ ≠  MDL-1234-fix...origin/MOOD…   2026060550     v5.0.8.01
public/admin/tool/mutenancy                  (detached)                    2026060552     v5.2.1.01  v5.2.1.01
public/admin/tool/scratch              ?     master
```

Rows are the checkout's path in the tree (`.` is Moodle core). The state column marks `N↑`
unpushed, `N↓` incoming, `*` uncommitted, `≠` on a different branch from the one the recipe
recorded, `?` not recorded in the recipe at all, `✗` recorded but not checked out.

```sh
mudev status           # only the checkouts with something to report
mudev status -s        # …arguments go straight to git
mudev fetch            # every remote of every checkout, with tags
mudev pull             # git pull --ff-only in every checkout
```

`status` shows a checkout when it has uncommitted changes or unpushed commits, and counts the
quiet ones at the end (`--all` shows everything). Being merely *behind* is not reported there —
that is incoming work, and `mudev list` shows it as `N↓`.

These fan out over the whole workspace, printing a `---- <path> ----` separator above each
checkout's git output. `pull` accepts fast-forwards only: a checkout that has diverged stops the
run right there, so you fix that one repository and run it again — nothing after it was touched.
Checkouts with nothing to pull into (a pinned edition is detached; a new branch may have no
upstream) are skipped rather than treated as failures.

## Adding a plugin by hand

```sh
mudev recipe add mutms/tool_mulib          # from the catalogue
mudev recipe add ./tool_scratch.yaml       # a plugin that is not in it yet
mudev recipe add mutms/tool_mulib --ref v5.0.8.01
```

The plugin lands at its `relpath` inside the tree, on the branch that serves this workspace's
Moodle, and is recorded in `.mudev.json` — so `list`, `status`, `fetch` and `pull` pick it up
from then on. A plugin already in the workspace is left untouched, which makes the command safe
to repeat, and resolution happens before anything is checked out, so a plugin that serves no
branch of this Moodle is refused with the tree still clean.

Exactly the plugin you asked for is added. What its branch declares it **requires** is reported
when the workspace does not have it, but never installed: composing a dev site is your decision,
and Moodle checks dependencies at install time anyway.

Removing one goes the other way round — you delete it, mudev catches up:

```sh
rm -rf admin/tool/mulib
mudev recipe prune
```

mudev does not delete plugin code. A checkout can hold uncommitted changes, unpushed commits or
a stash that no recipe knows about and nothing here could give back, so that call is yours.
`prune` then drops from `.mudev.json` every plugin that is recorded but no longer in the tree,
and cleans up the exclude that hid each one from its containing repository. It touches no git
repository and no working tree, and `mudev list` marks exactly what it would remove with `✗`, so
the listing is the preview. A directory that is still there but is no longer a git checkout is
reported and left recorded — that is a plugin whose `.git` went missing, not a deleted one.

This deliberately leaves the workspace differing from the recipe it was assembled from; the
closing line says so.

`fetch` contacts **every** remote a checkout has. A recipe with a mirror on the local network can
say who is asked first:

```yaml
extra:
  mudev: {release: mutms, fetch_order: [backup, origin]}
```

Git objects are content-addressed, so priming from the near copy leaves the remote over the
internet with only the difference to send — for a full Moodle core that is a LAN copy instead of
a 1.4 GB download. A mirror that is unreachable is reported and stepped over; an unreachable
`origin` stops the run.

## Naming what you built

A workspace starts out describing the recipe it was cloned from, which stops being true the
moment you add or prune a plugin:

```sh
mudev recipe set name "MuTMS dev 5.2"
mudev recipe set description "programs + certifications on a patched core"
mudev recipe set description ""            # an empty value clears it
```

One key and one value per call, the way `git config user.name "…"` works. Settable: `name`,
`description`, `contributed_by`. Everything else in the recipe is a record of what mudev actually
did and changes by cloning, adding and pruning — and `based_on_recipe` is provenance, crediting
the recipe this workspace was adapted from, so it is deliberately not editable.

## Exporting what you built

```sh
mudev export                                  # to stdout, so it pipes and diffs
mudev export --file mysite-5.2.yaml           # .yaml or .yml required
```

`export` renders `.mudev.json` as recipe YAML, with the schema header that makes an editor
validate and complete it. Every plugin in it is flattened, so it needs no plugin catalogue in
reach and `mudev clone` of the exported file assembles the same tree anywhere — that round trip
is what the command promises, and what its tests check.

It exports what the workspace was *assembled* to be, not what its checkouts are doing at this
moment; for that, use `mudev list`. Writing over an existing file is allowed and reported as
`replaced`, since comments in a hand-written recipe do not survive the trip.

## Catalogues

Plugin metadata and recipes live in their own repositories, so they can be read by tools other
than mudev — a composer-based assembler, a catalogue website — and extended without changing
this one. Both are CC BY 4.0: reusing an entry means keeping its `contributed_by` credit.

- `mdl-plugins/<vendor>/<package>.yaml` — what a plugin is: where it installs, where the code
  is, which git branch serves which Moodle version. Identifier `mutms/tool_mulib`.
- `mdl-recipes/<vendor>/<stream>/<version>.yaml` — what a site is. Identifier
  `mutms/full/5.2.1.01`.

A recipe does not have to come from a catalogue at all: `mudev clone ./mysite.yaml` takes a
self-contained file, which is what `mudev export` produces.

## Docs

- [docs/design.md](docs/design.md) — architecture and why things are the way they are
- [docs/plugin-format.md](docs/plugin-format.md) — plugin metadata YAML
- [docs/recipe-format.md](docs/recipe-format.md) — recipe YAML
- [docs/build.md](docs/build.md) — building & installing
- [docs/ci.md](docs/ci.md) — forgejo-runner / CI usage
- [ROADMAP.md](ROADMAP.md) — what is planned, and what is deliberately not

## License

GPL-3.0-or-later — see [LICENSE.txt](LICENSE.txt).

## Trademarks

Moodle™ is a registered trademark of Moodle Pty Ltd. This project is independent and is not
affiliated with, endorsed by, or sponsored by Moodle Pty Ltd. "Moodle" is used here only to
refer to the genuine Moodle software (e.g. an unmodified `MOODLE_502_STABLE` tree with
additional plugins).
