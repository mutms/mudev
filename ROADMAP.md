# Roadmap

What mudev does today, and what is planned. Design rationale for what already exists lives in
[docs/design.md](docs/design.md); the file formats are in
[docs/plugin-format.md](docs/plugin-format.md) and [docs/recipe-format.md](docs/recipe-format.md).

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
