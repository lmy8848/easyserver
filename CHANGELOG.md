# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Added `air` support for live-reloading during backend development.
- Added `-v` and `-version` command-line flags to easily print version, Go runtime version, and platform information.

### Fixed
- Fixed Makefile zombie processes issue when terminating `make dev`.
- Fixed cross-compilation target dependencies in Makefile to ensure frontend is correctly embedded.
- Fixed version injection parsing in Makefile (`LDFLAGS`) by adding quotes.

## [1.0.0] - 2026-06-29
### Added
- Initial stable release of EasyServer.
- Complete panel support for managing databases, firewall, web servers, and containers.
