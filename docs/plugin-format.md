# Plugin metadata format

Plugin definitions live in the plugins directory (`MUDEV_PLUGINS_DIRECTORY`, default
`/srv/extra/mdl-plugins`), one YAML file per plugin, grouped by vendor. The file path derives from
the identifier:

```
<plugins-dir>/<vendor>/<package>.yaml
```

e.g. identifier `mutms/tool_mulib` → `/srv/extra/mdl-plugins/mutms/tool_mulib.yaml`.

This is a **public, tool-neutral catalogue**: it holds generic, stable facts about a plugin.
It is consumed by mudev (git assembly), by a future composer-based site assembler, and
eventually by a plugins catalogue website. So it must not carry anything that is mutable or
specific to one consumer's workflow — e.g. *whether you tag/release a plugin lives in a
recipe, not here* (see `recipe-format.md`).

## Example

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/mutms/mudev/main/schema/plugin.schema.json
name: mutms/tool_mulib
title: MuTMS shared library
description: SQL builder, context map, notifications, AJAX forms, upsert, extdb.
relpath: public/admin/tool/mulib
source:
  git:
    remotes:
      origin: https://github.com/mutms/moodle-tool_mulib.git
  composer: mutms/moodle-tool_mulib      # Packagist package name (a source kind)
homepage: https://github.com/mutms/moodle-tool_mulib
license: GPL-3.0-or-later
requirements:
  MOODLE_405_STABLE:
    mdlbranches: ["405"]
  MOODLE_500_STABLE:
    mdlbranches: ["500", "501", "502"]   # one git branch serves three Moodle versions
    # plugins: [mutms/tool_mulib]        # per-branch deps go here (tool_mulib itself has none)
```

## Fields

| Field             | Req | Meaning                                                                                                                                                                                                                                                                                                                  |
|-------------------|-----|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `name`            | yes | Composer-style `vendor/package` identifier and the map key. See Identity.                                                                                                                                                                                                                                                |
| `title`           | yes | Human display name for a catalogue UI (`Programs`, not the slug `tool_muprog`). English fallback; localized names come from the plugin's lang packs. See Title.                                                                                                                                                          |
| `description`     | no  | Short one-line human summary (the composer `description`). Long-form copy is the repo README, not stored here.                                                                                                                                                                                                           |
| `relpath`         | yes | Explicit install path relative to the Moodle root, newest (`public/`) layout. See Relpath.                                                                                                                                                                                                                               |
| `source`          | no  | Acquisition methods keyed by kind (`git` + `composer` today; `zip` reserved) — kinds can coexist. `git.remotes` is a name → URL map (`origin` clone remote; `upstream` forks, `backup` mirror); `composer` is the Packagist package name. Optional overall (advertise-only); clone fails only if git is actually needed. |
| `source.composer` | no  | Packagist package name; presence = published on Packagist. See Composer.                                                                                                                                                                                                                                                 |
| `homepage`        | no  | Project homepage.                                                                                                                                                                                                                                                                                                        |
| `license`         | no  | SPDX id, e.g. `GPL-3.0-or-later`.                                                                                                                                                                                                                                                                                        |
| `contributed_by`  | no  | Credit party (name or list). The catalogue is CC0 — keeping this credit on reuse is a courtesy, not a condition.                                                                                                                                                                                                                        |
| `requirements`    | yes | Per-git-branch support + dependencies, keyed by git branch. Each entry has `mdlbranches` (the Moodle `$branch` codes that branch serves) and optional `plugins` (dependency identifiers). Merges the old `supported` + `require`. See Branch resolution.                                                                 |

The schema is intentionally **open** (`additionalProperties: true`) so a catalogue website can add
presentation metadata without breaking mudev. mudev only acts on the functional fields (`name`,
`relpath`, `source`, `requirements`); everything else is for other consumers. Neutral facts any
catalogue would show (`title`, `homepage`, `license`) sit top-level; heavier presentation assets go
under `extra.catalogue` (see Catalogue presentation).

## Source and remotes

`source` is a map of **acquisition methods keyed by kind** — the kind is the key (`git` and
`composer` today; `zip` is a reserved sibling), *not* a `type` field, so an entry can advertise
several ways to get the same code at once and each consumer picks the kind it supports (mudev →
`git`; a composer assembler → `composer`). No `git.ref` here — the version is resolved per recipe,
not stored in the catalogue.

Under `git`, `remotes` is a **name → URL** map. `origin` is the primary clone remote; `upstream`
(forks) and any others (`backup` mirror, etc.) are optional — mudev sets them all up on `clone`.
The **public** catalogue normally carries just `origin` (and `upstream` for forks); **internal**
remotes like backup mirrors belong in a **private overlay** (a private plugins catalogue or a
private recipe that inlines the plugins with extra remotes), never the public catalogue.

## Identity

`name` is composer-style `vendor/package` using the **bare frankenstyle** component — never a
`moodle-` prefix (that trades on the trademarked "Moodle" name). It is the clean **canonical
identifier**, and it need **not** equal the plugin's Packagist package name: Moodle's plugin
guidelines push a `moodle-` prefixed package name (e.g. `vendor/moodle-tool_foo`), so the two
routinely differ. The **actual** registered package name lives in the `composer` field, not in
`name`.

## Title

`title` is the **human display name** a catalogue UI shows — `Programs`, not the slug `tool_muprog`.
It is required, because the frankenstyle identifier is meaningless to a browsing human (and doubly so
to non-English speakers). Composer has **no** title field — Packagist just renders the package name —
so this is a deliberate mudev/catalogue extension, mirroring Moodle's own plugin directory, which
shows display names rather than components.

Store only the **canonical English** title here; it's the fallback. Do **not** put translations in
the catalogue — every Moodle plugin already ships localized names in its lang packs
(`lang/<xx>/<component>.php` → `$string['pluginname']`), so a catalogue site resolves the visitor's
language from there and falls back to `title`. Duplicating them in YAML would only drift.

## Catalogue presentation

Heavier presentation assets — a logo and screenshots — are **not** plugin facts mudev needs, and
they'd bloat the top level, so they live under the `extra.catalogue` namespace (the same
`extra.<tool>` pattern used elsewhere; a catalogue site is just another consumer):

```yaml
extra:
  catalogue:
    logo: assets/muprog-logo.svg      # path (anchored to this file) or URL
    screenshots:
      - assets/muprog-list.png
      - assets/muprog-detail.png
```

mudev ignores `extra.catalogue` entirely. Purely site-editorial choices (featured, pricing,
collection ordering) are **not** plugin facts at all and belong under a per-site namespace
(`extra.<site>`), not here.

## Relpath

`relpath` is **mandatory and authoritative** — mudev never derives it from the frankenstyle
name. It is the install path **relative to the Moodle code root**, stored in the **newest**
Moodle layout (currently the `public/` prefix, introduced in Moodle **5.1** — 4.5 and 5.0 have
no `public/`). For older Moodle branches without `public/`, the **recipe** sets
`base.strippublic: true` and mudev strips the prefix — mudev never prepends. This keeps a
single authoritative relpath per plugin and makes the transform explicit in the recipe rather
than hidden in mudev.

## Branch resolution

`requirements` is a map **keyed by git branch**; each entry's `mdlbranches` lists the Moodle
`$branch` codes that branch serves (and optional `plugins` its per-branch dependencies). mudev
**inverts** it to an `mdlbranch → branch` map at load (erroring on a duplicate `mdlbranch` across
branches), so resolution is a **direct lookup**: given a recipe pinned to `mdlbranch: "502"`, mudev
finds the branch whose `mdlbranches` contains `"502"` and checks it out (e.g. `MOODLE_500_STABLE`).
One Moodle version maps to exactly one branch, and the union of all `mdlbranches` doubles as the
plugin's compatibility declaration.

Keying by git branch collapses the many-to-one that a per-code list would repeat
(`500`/`501`/`502` all under `MOODLE_500_STABLE`) and — the reason `requirements` merges the old
`supported` + `require` — lets **dependencies vary per branch**, since a dep is a property of a
branch, not of the plugin globally. The numeric `mdlbranch` codes stay **values** inside
`mdlbranches`, never object keys, because numeric object keys are unsafe across languages (JS
reorders them, PHP coerces them to ints); the keys here are branch *names*, which are safe strings.

The pick is **advisory**: Moodle validates at install, and a recipe that pins `source.git.ref` skips
resolution entirely — plugin git-branch *names* are inconsistent across the Moodle world, which is
why the branch is a value, never derived.

## Composer

`source.composer` is the **Packagist package name** — one of the plugin's source kinds (see Source
and remotes). Its **presence** means the plugin is published on Packagist; **absence** means it is
not (a git-only plugin). The package name is usually Moodle's `moodle-`prefixed form, which differs
from the frankenstyle `name` — e.g. `mutms/moodle-tool_mulib` vs `mutms/tool_mulib` — and during a
rename the two simply differ. No version constraint is stored here: a composer-based assembler
derives it from the recipe's release version.

mudev does not act on `source.composer`; it's there for the composer-based assembler and the
catalogue website.

## Relative paths

Any relative path inside a plugin YAML (e.g. a local `source.git.remotes.*` URL) is resolved relative to the
**directory of that YAML file**, then made absolute.
