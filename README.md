# mudev — Moodle/MuTMS development tool

A single Go binary that assembles a Moodle code tree from a **recipe** — Moodle core plus a
selection of plugins, each as its own git checkout at its place in the tree — and then manages
those twenty-odd repositories as one workspace. Written for developing
[MuTMS](https://github.com/mutms) plugins, and for building Moodle test-site trees in CI.

A recipe is YAML, and it says what a site *is*: which Moodle, which plugins, which versions.
mudev turns that into a working tree and records what it did in `.mudev.json`.

mudev is Linux only. The [mdl-demo](https://github.com/mutms/mdl-demo) container uses mudev
internally to prepare its Moodle demo site. It is not meant to be used directly in production
environments.

## What it does for you

Two things cover most of what people need:

**Clone a recipe** into a directory and you get a ready-to-use Moodle tree — core plus every
plugin checked out at its place:

```sh
mkdir -p /srv/projects/mutms52 && cd /srv/projects/mutms52
mudev clone mutms/release/5.2.2.01
```

The recipe is either a catalogue identifier (`vendor/stream/version`) or a path to a recipe
file. `clone` is idempotent — run it again to resume an interrupted clone or to pick up plugins
the recipe has gained.

**Add a plugin** to a workspace you already have, and it lands at the right place on the right
branch for that Moodle:

```sh
mudev recipe add mutms/tool_mulib
```

From there mudev manages the whole set of checkouts as one workspace — listing their state,
fetching, pulling, exporting a portable recipe, and more. **See [docs/usage.md](docs/usage.md)
for the full command reference and git workflow.**

## Catalogues

Plugin metadata and recipes live in their own repositories, so they can be read by tools other
than mudev — a composer-based assembler, a catalogue website — and extended without changing
this one. Both are CC0 (public domain): reuse freely — keeping an entry's `contributed_by`
credit is a courtesy, not a condition.

- `mdl-plugins/<vendor>/<package>.yaml` — what a plugin is: where it installs, where the code
  is, which git branch serves which Moodle version. Identifier `mutms/tool_mulib`.
- `mdl-recipes/<vendor>/<stream>/<version>.yaml` — what a site is. Identifier
  `mutms/release/5.2.2.01`.

A recipe does not have to come from a catalogue at all: `mudev clone ./mysite.yaml` takes a
self-contained file, which is what `mudev recipe export` produces.

## Docs

- [docs/usage.md](docs/usage.md) — full command reference and git workflow
- [docs/plugin-format.md](docs/plugin-format.md) — plugin metadata YAML
- [docs/recipe-format.md](docs/recipe-format.md) — recipe YAML
- [AGENTS.md](AGENTS.md) — architecture and design rationale (for contributors and AI agents)

## AI disclosure

Majority of this project was written with the help of Claude (Anthropic). Everything it
produced was reviewed, corrected where needed and accepted by a human maintainer before
being committed; the design decisions and the final state of the code are the maintainers'.

## License

GPL-3.0-or-later — see [LICENSE.txt](LICENSE.txt).

## Trademarks

Moodle™ is a registered trademark of Moodle Pty Ltd. This project is independent and is not
affiliated with, endorsed by, or sponsored by Moodle Pty Ltd. "Moodle" is used here only to
refer to the genuine Moodle software (e.g. an unmodified `MOODLE_502_STABLE` tree with
additional plugins).
