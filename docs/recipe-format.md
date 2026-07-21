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
- **Release** streams are **editions** — always **pinned** (never rolling branches; a rolling
  edition is useless, end users take a fixed release and upgrade via composer or a future
  mudev `switch`/`upgrade`). Files are named by the release version
  `<moodle-point-release>.<mutms-increment>`, e.g. `mutms/moodle/4.5.12.01` (built on Moodle
  4.5.12, MuTMS increment 01). Editions so far:
  - `full` — full MuTMS: patched core + multi-tenancy, all plugins.
  - `moodle` — MuTMS without core Moodle changes and no multi-tenancy support (stock Moodle
    core; tenancy plugins omitted).
  - `programs` — programs/learning subset, e.g. for demoing MuTMS Programs to clients.

  **Per-plugin refs are heterogeneous** — a plugin's release tag depends on *its* branch, not
  the recipe's Moodle version. On Moodle 5.2, `tool_mulib` is pinned to `v5.0.8.01` (its
  `MOODLE_500_STABLE` branch serves 5.0/5.1/5.2) while `tool_mutenancy` is `v5.2.1.01` (it has
  a dedicated `MOODLE_502_STABLE` branch). The `tool_certificate` fork has no MuTMS tag at all
  and is pinned to a commit. So release recipes must be **generated** (mudev resolves each
  plugin's branch → its latest release tag), not hand-written with a uniform version. The 4.5
  line happens to be clean (every plugin has a dedicated 405 branch, so tags synchronize at
  `v4.5.12.01`), which is why `mutms/moodle/4.5.12.01` could be written directly as a sample.

Besides `mutms/`, a **`moodle/` vendor** holds plain, plugin-less Moodle base recipes
(`moodle/release/5.2.1` → stock core at `v5.2.1`, `plugins: []`) — a clean base and a nod to
mudev's longer-term general-Moodle goal.

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
needed; `mudev export` emits a clean YAML source recipe from it for sharing.

## Example

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/mutms/mudev/main/schema/recipe.schema.json
name: MuTMS dev on Moodle 5.2
catalog: ../../../mdl-plugins       # where bare-name references resolve (relative to this file)
extra:
  mudev: {release: mutms}          # release workspace — enables tag + version.php + changelog, using the `mutms` flavour
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
  - name: mutms/tool_mulib           # reference (branch auto-resolved from the catalogue)
    extra: {mudev: {release: mutms}} # release-managed under the `mutms` flavour
  - name: mutms/tool_mutenancy       # reference pinned to a tag
    source: {git: {ref: v5.2.1.01}}  #   version lives in source.git.ref (remotes from catalogue)
  - mutms/tool_certificate           # bare reference (not release-managed)
```

## Fields

| Field                | Req | Meaning                                                              |
|----------------------|-----|----------------------------------------------------------------------|
| `name`               | no  | Human label.                                                         |
| `contributed_by`     | no  | Attribution party (name or list); also asserts recipe authorship. CC BY 4.0 → retained on reuse. In a live recipe, this is the *project owner*. |
| `based_on_recipe`    | no  | Live-recipe only: what `clone` was given — a catalogue identifier *or* a recipe file path (provenance); also the CC BY credit to a catalogue source (see below). |
| `catalog`            | no  | Where bare-name references resolve; path relative to the file, or absolute. Defaults to `MUDEV_PLUGINS_DIRECTORY`. |
| `extra`              | no  | Tool-namespaced config (composer-style), also allowed per plugin. `extra.mudev.release` = release flavour. See below. |
| `base.mdlbranch`     | yes | Moodle `$branch` code (e.g. `502`). Drives plugin-branch resolution. |
| `base.source`        | yes | How to acquire Moodle core — same shape as a plugin `source` (kinds by key; see below). Normally `git`: `git.remotes.origin` is the clone remote (name → URL) and `git.ref` is what to check out. |
| `base.localbranch`   | no  | Local branch name to create from a branch `source.git.ref` (default = the remote branch name). Lives *outside* `source`. See Refs. |
| `base.strippublic`   | no  | Strip the leading `public/` from every plugin `relpath` (pre-5.1 Moodle: 4.5, 5.0). |
| `base.patches`       | no  | List of `{ repo, ref }` to merge over `ref`, in order. **Not implemented** — a recipe using it is rejected with a clear error. MuTMS ships a pre-merged `patch/mutms/*` core branch instead, so point `base.source` at that. |
| `plugins`            | yes | List of plugin entries; each is a **string** or an **object** (below). |

## Plugin entries — reference or inline

Each entry is one of:

1. **Bare identifier** — `mutms/tool_mulib`, resolved against `catalog`.
2. **Reference + annotations** — `{ name, source?, localbranch?, extra? }` (pin/override the
   version via `source.git.ref`, with remotes merged from the catalogue; tool flags under `extra`).
3. **Inline definition** — the full plugin spec (any plugin field) plus `source`/`localbranch`/`extra`,
   so the recipe is self-contained and needs no catalogue. The entry's own fields match the plugin
   data model; mudev flags stay in `extra`.

Per-plugin release state lives in `extra.mudev` (e.g. `{release: mutms}`) — **owner state**:
hand-authored in a source recipe, and mudev-managed in the checkout's `.mudev.json` (set via mudev,
not by hand); never in the public plugin catalogue. It marks which plugins are release-managed; it
is gated by the recipe-level `extra.mudev.release`.

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
schema types every key it reads (`release` a string, `fetch_order` a list of strings) and rejects
anything else. A mistyped `fetchorder:` or `releases:` would otherwise be a setting that silently
does nothing — which is the worst kind of mistake, because the tool keeps working and just ignores
you. The schema and the binary ship together, so a new mudev key arrives with the version that
reads it; anyone else reusing this format writes their own `extra.<tool>` namespace, which the
schema does not constrain.

mudev reads two keys. First `extra.mudev.release`, in the config precedence chain:

> **flag > env var > recipe `extra.mudev` > built-in default**

`release` is a **flavour name** (a string), never a bare boolean:

The two levels do different jobs:

- Recipe-level `extra.mudev.release: <flavour>` — names **what project this workspace is**. It
  makes the tree a **release workspace** under that flavour's rules (tag + `version.php`
  `$version`/`$release` + changelog). It says nothing about how the repositories are reached:
  a workspace you push from names `git@…` remotes because that is what you want, not because a
  flag inferred it.
- Plugin-level `extra.mudev.release: <flavour>` — **selects which plugins are released**. Its
  presence flags the plugin in; its absence leaves the plugin in the tree but unreleased. The
  flavour it names **must match the recipe level** (see single-flavour rule below) — that match is
  a consistency check; the plugin flag's real payload is its presence.

The flavour names a versioning/tagging ruleset that mudev applies. mudev ships the built-in
`mutms` flavour; another project reuses this format with its own — `release: acme` — and mudev
applies that flavour's handler. A flavour mudev doesn't know is **not** an error: the plugin still
clones and assembles normally, but its **release subcommands are unavailable** (no tag / no
`version.php` bump / no changelog) and mudev prints a warning naming the unknown flavour. So a
recipe referencing a foreign flavour stays fully usable for build/consume and per-plugin git work;
only its release automation is inert until that flavour's handler exists.

**One flavour per recipe.** The recipe-level `extra.mudev.release` is authoritative for the whole
workspace — a release tree has exactly one flavour. A plugin is release-managed only if its own
`extra.mudev.release` **equals** the recipe-level flavour. A plugin that names a *different*
flavour is **not** treated as that other flavour: mudev **ignores** the plugin's release flag
(the plugin is left unmanaged) and warns, naming the mismatch. This is deliberate — mixing tagging
rulesets in one tree produces silent, hard-to-unwind messes, so the divergent flag is dropped
rather than honoured. (A plugin release flag with no recipe-level release at all is likewise
inert: no release workspace, nothing to manage.)

Release needs a resolved flavour handler, so a release subcommand (`mudev release` / `tag` / …)
**errors whenever there is no handler** — either because the recipe declares no flavour (*"not a
release workspace"*) or because it names one mudev doesn't know (*"unknown flavour"*). The command
refuses rather than guessing a tagging scheme; only clone / assembly / plain git keep working.

Absent `extra.mudev.release`, a recipe is **build/consume**: nothing to tag, nothing to version.
So dev workspaces set `extra.mudev: {release: mutms}`; editions omit `extra` entirely. A live
recipe carries whichever matches that checkout. Other consumers (composer assembler, catalogue
site) use their own `extra.<tool>` namespace.

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

## `extra.mudev.fetch_order` — which remote is asked first

mudev fetches **every** remote of every checkout, on `clone` and on `fetch` alike. A development
recipe carries mirrors and fork upstreams that a developer expects to be current, and a catalogue
recipe has nothing but `origin`, so the rule costs it nothing.

`fetch_order` decides the order:

```yaml
extra:
  mudev: {release: mutms, fetch_order: [backup, origin]}
```

Git objects are content-addressed, so this is not cosmetic. Fetching a **mirror on the local
network first** fills the object store, and the subsequent fetch from the remote over the
internet has only the difference left to send — for a full Moodle core, that is the difference
between a LAN copy and a 1.4 GB download. (Measured on a plugin repo: after priming from a local
mirror, the github fetch transferred zero objects and the resulting pack was identical.)

Rules:

- Remotes **not** named are still fetched, after the ones that are (`origin` leading the
  remainder, then alphabetically — a run is reproducible).
- A name a particular checkout does not have is skipped, so a workspace-wide order naming
  `backup` is harmless for a catalogue plugin that only has `origin`.
- Only `origin` is load-bearing: a mirror that cannot be reached (laptop off the office network,
  backup host down) is **reported and stepped over**, because failing an assembly over a copy of
  data that origin also has would make the mirror a single point of failure. An unreachable
  `origin` stops the run.
- The order is recipe-level and applies to core and every plugin. It travels into `.mudev.json`
  with the rest of `extra`, so `mudev fetch` uses it without re-reading the source recipe.

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

## Attribution in the live recipe

The live recipe (`.mudev.json`) is an *adaptation* of the source recipe, so CC BY
attribution is carried in two layers rather than by copying the source's `contributed_by`:

- **`based_on_recipe`** records what `clone` was given — a catalogue identifier or a recipe file
  path. For a catalogue source it credits the source recipe (and, being "based on", indicates
  adaptation) — the CC BY attribution to the recipe author; for a loose file it's provenance
  ("cloned from that file").
- Each **inlined plugin entry retains its own `contributed_by`** — reusing a catalogue plugin
  keeps that contributor's credit, even inside a project checkout.
- A top-level **`contributed_by`** on the live recipe, if present, is the **project owner** who
  assembled it — never carried over from the source.

## Relative paths

Relative paths inside a recipe (`catalog`, a local `source.git.remotes.*` URL) are anchored to the **directory
of the recipe file**, then made absolute. With the `<vendor>/<stream>/` grouping, reaching
sibling catalogues under `/opt` needs `../../../` (e.g. `mdl-recipes/mutms/dev/5.2.yaml` →
`../../../mdl-plugins`).
