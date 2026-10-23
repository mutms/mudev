# mudev — Moodle/MuTMS development tool

mudev prepares the source code of a Moodle site. You give it a **recipe** — a small YAML file
that says what the site is made of: which Moodle version, which plugins, which releases — and
mudev fetches all of it and arranges it into a ready-to-use code tree, every plugin in its
right place. It was written for developing [MuTMS](https://github.com/mutms) plugins.

## Where you'll meet it

mudev is plumbing: most people never install it themselves, they meet it working inside
something else.

- **[mpd](https://github.com/mutms/mpd)** — the Moodle plugin development environment. Its
  Linux VM ships mudev pre-installed, and developers use it there to set up and maintain their
  project workspaces.
- **[mdl-demo](https://github.com/mutms/mdl-demo)** — the try-Moodle-on-your-own-computer
  container. When you pick a Moodle version in its web UI, it is mudev that assembles the demo
  site's code behind the scenes.
- **CI** — automated test pipelines use the standalone mudev binary to build the Moodle site
  trees that plugins are tested against.

mudev runs on Linux only. It prepares code for development, demos and testing; it does not run
on production servers itself.

## Installing it

On an mpd VM there is nothing to install: mudev and both catalogues are already there.

Anywhere else on Linux:

1. Download the binary for your architecture (`linux-amd64` or `linux-arm64`) from
   [the releases page](https://github.com/mutms/mudev/releases) and put it on your `PATH` as
   `mudev`.
2. Clone the two catalogues side by side:

   ```sh
   sudo install -d -o "$USER" /srv/extra
   git clone https://github.com/mutms/mdl-plugins.git /srv/extra/mdl-plugins
   git clone https://github.com/mutms/mdl-recipes.git /srv/extra/mdl-recipes
   ```

   mudev looks there by default. To keep them somewhere else, set `MUDEV_PLUGINS_DIRECTORY`
   and `MUDEV_RECIPES_DIRECTORY` (see [docs/usage.md](docs/usage.md#configuration)).

## Using it yourself

Two commands cover most of what people need:

**Clone a recipe** into a directory and you get a ready-to-use Moodle tree — core plus every
plugin checked out at its place:

```sh
mkdir -p /srv/projects/mutms52 && cd /srv/projects/mutms52
mudev clone mutms/release/5.2.4.01
```

The recipe is either a catalogue identifier (`vendor/stream/version`) or a path to a recipe
file. [Browse the available recipes](https://github.com/mutms/mdl-recipes). `clone` is
idempotent — run it again to resume an interrupted clone or to pick up plugins the recipe has
gained. Add `--shallow` when you only need the code, not the git history (for example in CI).

**Add a plugin** to a workspace you already have, and it lands at the right place on the right
branch for that Moodle:

```sh
mudev recipe add mutms/tool_mulib
```

From there mudev manages the whole set of checkouts as one workspace — listing their state,
fetching, pulling, pushing, exporting a portable recipe, packing a deployable `.tgz`
(`mudev production export`), and more. **See [docs/usage.md](docs/usage.md)
for the full command reference and git workflow.**

## Catalogues

Plugin metadata and recipes live in their own repositories, so they can be read by tools other
than mudev — a composer-based assembler, a catalogue website — and extended without changing
this one. Both are CC0 (public domain): reuse freely — keeping an entry's `contributed_by`
credit is a courtesy, not a condition.

- [mdl-plugins](https://github.com/mutms/mdl-plugins) — `<vendor>/<package>.yaml`, what a
  plugin is: where it installs, where the code is, which git branch serves which Moodle
  version. Identifier `mutms/tool_mulib`.
- [mdl-recipes](https://github.com/mutms/mdl-recipes) — `<vendor>/<stream>/<version>.yaml`,
  what a site is. Identifier `mutms/release/5.2.4.01`.

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
