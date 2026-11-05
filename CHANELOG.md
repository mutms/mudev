# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `release set version <YYYYMMDDxx|today>`: sets `$plugin->version`, and the dependencies between
  them, in every release-managed plugin; Moodle 5.3 and later only. The value must end with the
  branch suffix (`53` on 503), and a downgrade writes nothing.

### Removed

- `camp init` command and camp registry listing generation.

## [0.3.0] - 2026-11-04

### Added

- Local Moodle mirror `/srv/extra/moodle.git`: `clone` creates it when `/srv/extra` is writable,
  refreshes it, and links every new Moodle core checkout to it, so workspaces download and store
  only what upstream Moodle does not have. `MUDEV_MOODLE_MIRROR=0` turns it off.

### Removed

- `clone --shallow`, the local mirror is faster and shallow clones are expensive for GitHub.
- `extra.mudev.fetch_order`, still accepted in recipes but ignored; remotes are fetched origin
  first, then alphabetically.

## [0.2.0] - 2026-10-23

### Added

- Added --shallow parameter to clone command
- Added push command: pushes plugin commits to origin, never Moodle core

### Changed

- Switched to Go 1.27.3
- Documentation recommends the `release` recipe edition (one subtree-merged repository at a
  release tag) and no longer describes per-plugin release pinning or release tagging
- README explains how to install mudev outside mpd and uses recipe identifiers that exist

### Fixed

- Added links to mdl-plugins and mdl-recipes repositories
- Fixed the `$id` URL of the recipe JSON schema

## [0.1.0] - 2026-08-17

### Added

- First preview release
