# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
