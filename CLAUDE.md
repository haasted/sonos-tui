# Project Instructions for AI Agents

This file provides instructions and context for AI coding agents working on this project.

## Task Tracking

No dedicated issue tracker is configured for this project. Use TodoWrite for session-scoped task tracking as needed.

## Git

Do not run git commits, pushes, or remote sync unless explicitly asked. At handoff, report changed files, validation performed, and suggested next commands; wait for approval before committing or pushing.

## Build & Test

Prefer Makefile targets over raw command-line invocations for building and testing (e.g. `make test` over `go test ./...`). Only fall back to the command line when no target covers what's needed.

Current targets: `build`, `vet`, `test`, `test-verbose`, `test-race`, `test-live`, `run`, `tidy` — see `Makefile`.

`test-live` runs tests gated behind the `-live` flag, which require a real network with real Sonos speakers. The default `test`/`test-verbose`/`test-race` targets never touch the network.

## Architecture Overview

_Add a brief overview of your project architecture_

## Conventions & Patterns

_Add your project-specific conventions here_
