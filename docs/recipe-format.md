# Recipe format

A recipe is a complete Moodle site definition: core (at a branch/tag/commit) and a selection of
plugins. The core it names may already be patched — MuTMS points `base.source` at a
pre-merged `patch/mutms/*` branch — so mudev merges nothing itself. Recipes live in the recipes directory
(`MUDEV_RECIPES_DIRECTORY`, default `/srv/extra/mdl-recipes`), grouped by vendor like plugins:

```
<recipes-dir>/<vendor>/<stream>/<version>.yaml   →  identifier  vendor/stream/version
```

Two levels below the vendor: a **stream** and a **version**.

- `dev/` is the single rolling **workspace** stream — one full dev tree per Moodle version,
  always the complete plugin set (no partial dev). Files are named by Moodle version:
  `mutms/dev/5.2`, `mutms/dev/4.5`. The `mdlbranch` code lives inside the file.
- **Release** streams are **editions** — always **pinned** to a tag, never a rolling branch.
  Files are named by the release version `<moodle-point-release>.<mutms-increment>`, e.g.
  `mutms/release/5.2.4.01` (built on Moodle 5.2.4, MuTMS increment 01). Editions so far:
  - `release` — full MuTMS: patched core + multi-tenancy, all plugins. **This is the
    recommended release recipe.** It names one repository,
    `https://github.com/mutms/mutms.git`, at a release tag (`MuTMS-5.2.4-01`), with every
    plugin already merged in as a git subtree — so `plugins: []`, and nothing is pinned per
    plugin.
  - `moodle` — MuTMS plugins on stock Moodle core, without multi-tenancy.

Besides `mutms/`, the catalogue holds plugin-less base recipes for other distributions:
`moodle/` (stock core, e.g. `moodle/release/5.2.4` → `v5.2.4`) and `iomad/` (IOMAD dev
branches). Both use `plugins: []`.

The same schema also validates the **live recipe** — a single, self-contained `/.mudev.json` at
the project root. Every plugin reference is **fully flattened**: at clone/install time mudev
merges each plugin's *complete* catalogue definition (relpath, source, requirements, composer,
contributed_by…) inline, **collapses `source` to the single kind it actually used** (`git`), and
records inside `source.git.ref` the **ref it checked out** — a **branch** for a dev workspace (so
git tracks it, and mudev can spot a manual switch to a feature branch), or a tag/commit for a
pinned edition. A source recipe may *advertise* several source kinds; the live recipe keeps only
the resolved one — the source recipe is composer.json, `.mudev.json` is the lock. (Any other
resolver — a composer assembler, mdl-demo's zip installer — applies its own heuristic to stitch up
its install instructions and records its own single-kind artifact.) So `.mudev.json` depends on **no** plugins directory and **no**
`catalog` — it fully describes and can rebuild the tree on its own, wherever it travels. The
recorded ref is the *intended baseline*; mudev diffs it against the live git state (current
branch, dirty, ahead/behind) for `status`. `base.mdlbranch` is retained mainly as a **cache** —
a fast default-branch lookup so `mudev recipe add <plugin>` can resolve a newly added plugin's branch
(via that plugin's `requirements` block) without re-deriving the target. It is **mudev-managed state**
(JSON, composer.lock-style): you change it by running mudev, not by hand-editing. `extra` namespaces
let mudev (and other tools) track per-checkout state in that one file, so no `.mudev/` directory is
needed; `mudev recipe export` emits a clean YAML source recipe from it for sharing. A live recipe
is normally written by `clone`, but `mudev recipe init` also **reconstructs one from the checkouts
already in a tree** (see below).

## Example

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/mutms/mudev/main/go/schema/recipe.schema.json
name: MuTMS dev on Moodle 5.2
catalog: ../../../mdl-plugins       # where bare-name references resolve (relative to this file)
base:
  mdlbranch: "502"                   # Moodle $branch code (quoted string) — drives branch resolution
  source:                            # same shape as a plugin source (acquisition methods by kind)
    git:                             # mudev reads the `git` kind (zip/composer are reserved siblings)
      remotes:
        origin: https://github.com/mutms/patches.git   # name → URL; may be a pre-patched branch, not vanilla moodle/moodle
      ref: origin/patch/mutms/MOODLE_502_STABLE         # git's <remote>/<branch> spelling; `origin` from remotes above
  localbranch: MOODLE_502_STABLE     # local branch to create (default: the remote branch name) — outside source
  # strippublic: true                # set on pre-5.1 branches (4.5, 5.0) that lack public/
  # patches:                         # optional EXTRA patches merged over ref, in order
  #   - { repo: https://github.com/mutms/patches.git, ref: patch/mutest/MOODLE_502_STABLE }
plugins:
  - mutms/tool_mulib                # bare reference (branch auto-resolved from the catalogue)
  - name: mutms/tool_mutenancy       # reference with an explicit ref
    source: {git: {ref: origin/MOODLE_502_STABLE}}   # remotes still come from the catalogue
```

## Fields

| Field                | Req | Meaning                                                                                                                                                                                                                      |
|----------------------|-----|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `name`               | no  | Human label.                                                                                                                                                                                                                 |
| `contributed_by`     | no  | Credit party (name or list); also asserts recipe authorship. Catalogues are CC0 — keeping it on reuse is a courtesy, not a condition. In a live recipe, this is the *project owner*.                                         |
| `based_on_recipe`    | no  | Live-recipe only: what `clone` was given — a catalogue identifier *or* a recipe file path (provenance); also the courtesy credit to a catalogue source (see below).                                                          |
| `catalog`            | no  | Where bare-name references resolve; path relative to the file, or absolute. Defaults to `MUDEV_PLUGINS_DIRECTORY`.                                                                                                           |
| `extra`              | no  | Tool-namespaced config (composer-style), also allowed per plugin. `extra.mudev.release` = release flavour. See below.                                                                                                        |
| `base.mdlbranch`     | yes | Moodle `$branch` code (e.g. `502`). Drives plugin-branch resolution.                                                                                                                                                         |
| `base.source`        | yes | How to acquire Moodle core — same shape as a plugin `source` (kinds by key; see below). Normally `git`: `git.remotes.origin` is the clone remote (name → URL) and `git.ref` is what to check out.                            |
| `base.localbranch`   | no  | Local branch name to create from a branch `source.git.ref` (default = the remote branch name). Lives *outside* `source`. See Refs.                                                                                           |
| `base.strippublic`   | no  | Strip the leading `public/` from every plugin `relpath` (pre-5.1 Moodle: 4.5, 5.0).                                                                                                                                          |
| `base.patches`       | no  | List of `{ repo, ref }` to merge over `ref`, in order. **Not implemented** — a recipe using it is rejected with a clear error. MuTMS ships a pre-merged `patch/mutms/*` core branch instead, so point `base.source` at that. |
| `plugins`            | yes | List of plugin entries; each is a **string** or an **object** (below).                                                                                                                                                       |

## Plugin entries — reference or inline

Each entry is one of:

1. **Bare identifier** — `mutms/tool_mulib`, resolved against `catalog`.
2. **Reference + annotations** — `{ name, source?, localbranch?, extra? }` (pin/override the
   version via `source.git.ref`, with remotes merged from the catalogue; tool flags under `extra`).
3. **Inline definition** — the full plugin spec (any plugin field) plus `source`/`localbranch`/`extra`,
   so the recipe is self-contained and needs no catalogue. The entry's own fields match the plugin
   data model; mudev flags stay in `extra`.

## Refs — `source.git.ref`, remote, and local branch name

The git version selector is **`source.git.ref`** — it names **what to check out**. mudev shells out
to real `git`, so the ref uses git's own vocabulary rather than inventing a scheme:

- **Branch** — git's remote-tracking spelling **`<remote>/<branch>`**, exactly as `git switch`
  takes a start point. `<remote>` is a key from that entry's `source.git.remotes` — `origin` by
  default, or `upstream` to track a fork's branch (e.g. `upstream/MOODLE_405_STABLE` for
  `tool_certificate`). Because mudev hands the string straight to git — which knows its own remote
  list — branch names with slashes (`origin/patch/mutms/MOODLE_502_STABLE`) are unambiguous; mudev
  does no parsing.
- **Tag or commit** — a plain string (a pinned edition), checked out detached. No remote prefix.

`ref` lives **inside `source.git`** because it is git-specific: a ref is meaningless without git,
and each source kind carries its own version selector (git → `ref`, a future `zip` → `sha256`,
`composer` → a version constraint). With kinds able to coexist, a single top-level ref would be
ambiguous — so the version travels with its kind.

`localbranch` stays **outside `source`** — it is *not* an acquisition detail but a local-checkout
ergonomic. From a branch ref mudev creates a local branch with `git switch -c <localbranch> <ref>`,
which also sets its upstream (so `pull`/`push`/ahead-behind Just Work, and `status` can read
tracking). `localbranch` defaults to the **remote branch name**; set it to override — most useful
for the patched core, where the remote branch is `patch/mutms/MOODLE_502_STABLE` but you want a
clean local `MOODLE_502_STABLE` matching every plugin's local branch.

**Omit `source.git.ref` to auto-resolve, or pin it to be self-contained.** A plugin entry with no
`source.git.ref` has its branch resolved from the plugin's `requirements` block at `mdlbranch` (mudev
inverts it to mdlbranch → branch; tracked from `origin`, local name = the resolved branch) — convenient, but it makes
the recipe depend on the catalogue. The MuTMS **dev recipes instead pin `source.git.ref` +
`localbranch` on every plugin**, so they carry the expected remote and branch explicitly and mirror
the flattened **live-recipe** (`.mudev.json`) structure — no `requirements` lookup needed to know what
a checkout should be on. (The pin is not uniform across plugins: on 5.2 most ride `origin/MOODLE_500_STABLE`,
while `tool_mutenancy` and `tool_certificate` have their own `origin/MOODLE_502_STABLE` — exactly
what the catalogue would resolve, made explicit.)

## The `extra` block (tool-namespaced, extensible)

`extra` is a composer-style bag of **tool-namespaced** config, present at the recipe top level
**and** on each plugin entry. Each key is a tool name; its value is opaque to the recipe schema,
so a tool extends recipes without any schema change.

**`extra.mudev` is the exception: it is defined, and closed.** mudev owns that namespace, so the
schema types every key (`release` a string) and rejects
anything else. A mistyped `fetchorder:` or `releases:` would otherwise be a setting that silently
does nothing — which is the worst kind of mistake, because the tool keeps working and just ignores
you. The schema and the binary ship together, so a new mudev key arrives with the version that
reads it; anyone else reusing this format writes their own `extra.<tool>` namespace, which the
schema does not constrain.

The schema accepts `release`, reserved for release tooling that this version does not include;
mudev only warns when the name is not one it knows. `fetch_order` is deprecated and ignored: it
is still accepted so live recipes of older workspaces load, and mudev drops it when it rewrites
them. mudev fetches `origin` first and the rest alphabetically, and Moodle core is sped up by the
local mirror instead (see `docs/usage.md`).

## Remote URLs and credentials

mudev has **no** authentication configuration — no URL rewriting, no token injection. Each URL
in `source.git.remotes` goes to git exactly as written, whatever the host: bare SSH
(`git@host:path`), `ssh://`, `file://` and `https://` all work, so no hosting platform is
assumed. Write the URL you mean:

```yaml
origin: git@github.com:mutms/moodle-tool_mulib.git      # a checkout you push from
origin: https://github.com/mutms/moodle-tool_mulib.git  # one you only read (CI, editions)
```

Which key or credential reaches that URL is git's business and yours — an SSH agent, a
credential helper, or a `url.<base>.insteadOf` rule in `~/.gitconfig`, each of which applies to
every git tool on the machine rather than only to mudev. **Never put a token in a recipe:** a
recipe is a file that gets committed and shared.


## Clone flow (`mudev clone <recipe>`)

`<recipe>` is resolved first: an existing file / `.yaml` path is loaded directly (handy for
ad-hoc private recipes — a flat `somecustomer-4.5.12.yaml` per site, self-contained, no catalogue
needed), otherwise it's a `vendor/stream/version` identifier loaded from `MUDEV_RECIPES_DIRECTORY`.
Then:

1. Read `base.mdlbranch`.
2. Check `base.source.git.remotes.origin` out at `base.source.git.ref` into the **current
   directory** (which the caller has already created with the right owner/permissions), then
   verify the result really is Moodle at that `mdlbranch` by reading its own `version.php` —
   a mismatch stops the run before a single plugin is installed.
3. Per plugin: use `source.git.ref` if given, else resolve it from the plugin's `requirements` block at
   `mdlbranch` (tracked from `origin`). For a branch ref, create the local branch (`localbranch`,
   default = the remote branch name); a tag/commit is checked out detached.
4. Resolve each plugin's install `relpath` (`strippublic` strips `public/` when set).
5. Install each plugin at `<resolved-relpath>` (under the current directory) at the resolved ref, **processed sorted by
   `relpath` (ancestors first)** so a subplugin lands inside its already-present parent
   (`…/certificate` before `…/certificate/element/muprog`). The recipe's list order is for
   humans (release grouping); it is **not** the assembly order.
6. Write `./.mudev.json`: the resolved recipe with **every plugin definition flattened
   inline** (full catalogue merge — relpath/requirements/etc. — with `source` narrowed to the resolved
   `git` kind and `source.git.ref` set to the checked-out ref, plus `extra`), so it carries no
   `catalog` and needs no plugins directory thereafter.

## Init flow (`mudev recipe init`) — reconstructing the live recipe from a tree

`init` is the reverse of `clone`: where clone reads a recipe and produces checkouts, init reads
the checkouts and produces the recipe. It is for a tree that already holds the code — assembled by
hand — but has no `.mudev.json`, so mudev does not yet know it as a
workspace. It **changes no working tree**; it only reads git state and writes the record.

1. Read core at the root: try `public/version.php` (5.1+), then `version.php` (older, which sets
   `strippublic`), for the `$branch` code → `base.mdlbranch`. Read core's remotes and its branch
   (and tracking ref) → `base.source.git`, with `base.localbranch` when the local branch differs
   from the ref's branch part. A root that is not a Moodle code tree is refused.
2. Walk the tree for every other git checkout (the same enumeration `list` uses).
3. Per checkout: read its remotes and, from its own `version.php`, its **frankenstyle component**.
   The identifier is reconstructed as **`<remote-owner>/<component>`** — the owner segment of the
   `origin` URL, and the component. A checkout of `git@github.com:mutms/moodle-tool_mulib.git`
   with component `tool_mulib` becomes `mutms/tool_mulib` (the catalogue identifier); one from a
   private forge (`git@forge.example:acme/mod_thing.git`) becomes `acme/mod_thing`. Where the
   component is unreadable, the repository name (minus a `moodle-` prefix) stands in; where there
   is no `origin` at all, the checkout is **reported and left out** rather than named on a guess.
4. Record the ref the way clone does — a branch by its upstream (`origin/…`), a detached checkout
   by the tag at its HEAD, or, failing both, by its commit. `relpath` is the checkout's path put
   back into the newest (`public/`) layout when the tree is `strippublic`.
5. Write `.mudev.json` and add the same excludes clone leaves (the live recipe, and each nested
   checkout hidden from its containing repository).

The result is a fully flattened, self-contained live recipe — the same shape clone writes — so
`recipe export` and every fan-out command work against it. The reconstructed names are **mudev's
own state keys**, not a claim about any catalogue: edit them with `recipe set` or in the file. To
keep that state safe, init **refuses to overwrite an existing `.mudev.json`** (remove it first to
reconstruct from scratch). It reads no plugin catalogue — everything comes from the checkouts —
and so records no `requirements` or `based_on_recipe`; it is a snapshot of the tree, not an
adaptation of a source recipe.

## Update flow (`mudev recipe update <relpath>`) — one checkout at a time

`init` reconstructs a whole tree, and **refuses if a live recipe already exists**. `update` is the
per-checkout follow-up for a workspace that is already initialised: it folds the current state of
one checkout into the record. `<relpath>` is the path as `mudev list` prints it — a tree-relative
path, or `.` for core — resolved to an entry through the same `strippublic` mapping the listing
uses. It reads the same on-disk facts as init and, like init, changes no working tree.

- **Adopt** — the checkout is on disk but not in the record (the `?` a listing flags). It is
  identified `<owner>/<component>`, flattened, excluded from its containing repository and
  recorded, exactly as init would have done that one row.
- **Refresh** — the checkout is already recorded. Only its **git identity** is rewritten —
  `source.git.remotes`, `source.git.ref`, `localbranch` — from what git now reports; the entry's
  `name` and every other field are preserved (a name edited by hand after init survives, and any
  catalogue field carried in survives). Re-recording a strayed checkout this way makes its current
  branch the baseline `status` compares against — the write-side inverse of the `≠` that a listing
  shows.

It **never removes** an entry: a recorded checkout whose directory is gone is reported and left
for `recipe prune`, keeping the same split that makes prune safe (you delete the code, mudev
reconciles the record). A recorded name is never re-derived on refresh, so update cannot silently
rename an entry. There is deliberately no bulk form — to re-snapshot the whole tree, delete
`.mudev.json` and run `recipe init` again.

## Credit in the live recipe

The catalogues are CC0, so none of this is a license obligation — but mudev still carries
credit for the source recipe in two layers rather than by copying the source's
`contributed_by`:

- **`based_on_recipe`** records what `clone` was given — a catalogue identifier or a recipe file
  path. For a catalogue source it credits the source recipe (and, being "based on", indicates
  adaptation); for a loose file it's provenance ("cloned from that file").
- Each **inlined plugin entry retains its own `contributed_by`** — reusing a catalogue plugin
  keeps that contributor's credit, even inside a project checkout.
- A top-level **`contributed_by`** on the live recipe, if present, is the **project owner** who
  assembled it — never carried over from the source.

## Relative paths

Relative paths inside a recipe (`catalog`, a local `source.git.remotes.*` URL) are anchored to the **directory
of the recipe file**, then made absolute. With the `<vendor>/<stream>/` grouping, reaching
sibling catalogues under `/srv/extra` needs `../../../` (e.g. `mdl-recipes/mutms/dev/5.2.yaml` →
`../../../mdl-plugins`).
