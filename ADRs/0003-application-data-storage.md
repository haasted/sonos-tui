# 3. Application data storage

## Status

Proposed

## Context

ADR-0001 designed a speaker cache and defaulted to JSON, since a handful of small speaker records didn't obviously need more. Since then the intended feature set grew beyond that: playlist management and remembering tracks for easy later re-addition are both planned, and both need real durable storage — arbitrary user-created lists, growing history, data that must never be silently lost the way a regenerable cache can be.

JSON-per-feature would mean N ad hoc formats accumulating over time, each with its own hand-rolled load/save code, no shared query capability, and no shared story for evolving a schema later without a bespoke migration script per file. That's the generic-utility-vs-dependency question CLAUDE.md already asks to be raised for exactly this kind of problem.

The user's proposal, inspired by [SQLite as an Application File Format](https://sqlite.org/appfileformat.html): use a single SQLite database as the shared storage layer for this data, rather than one-off files per feature.

That page's argument, fetched directly rather than assumed: a single SQLite file gets the portability of "one file" plus the structural power of a real relational database, still readable with off-the-shelf tools (the `sqlite3` CLI, any SQL client) instead of a bespoke parser. Stated benefits: no custom I/O code; SQL as a high-level query interface instead of hand-rolled seek/parse logic; cross-platform; atomic all-or-nothing writes even across a crash; incremental updates (only changed pages rewritten); schema can extend while staying backward compatible; often faster than raw filesystem operations for small blobs; safe concurrent multi-process reads; bindings in effectively every language. The page's own caveat: "SQLite is not the perfect application file format for every situation," and accepting untrusted database files from other users needs care — not a real concern here, since this project only ever reads and writes its own file.

## Decision

Adopt SQLite as the shared durable storage layer for this application's data — the speaker cache (superseding ADR-0001's JSON default), playlists, and remembered-track history all land here rather than each inventing its own file format.

**Driver: a pure-Go SQLite implementation (`modernc.org/sqlite`), not CGo-based (`mattn/go-sqlite3`).** This isn't a fresh call — it's the same reasoning ADR-0001 already used to reject reading the Wi-Fi SSID and the gateway MAC (both required a subprocess or CGo), applied consistently. Every prior library evaluation in this codebase (`go-termimg`, `koron/go-ssdp`) has landed on pure Go, no CGo; this should too.

**One shared database file, not one file per concern.** This was a deliberate choice, made after the trade-off was raised explicitly: ADR-0001 established the speaker cache as disposable — never a source of truth, safe to delete, `Discover` always rebuilds it from scratch. Playlists and remembered tracks are the opposite: real user data that must never be silently lost. Putting both in one file means a careless "clear cache" operation has a blast radius it wouldn't have if the cache lived in its own throwaway file. The user chose the shared file anyway — most likely for the simplicity of one file to reason about and back up — but this ADR is not pretending the risk isn't real. It needs a mitigation, not a shrug:

- Cache data should live in its own clearly-named table(s) (e.g. a `speaker_cache` prefix or a dedicated schema/table group), so "clear the cache" is a `DELETE FROM`/`DROP TABLE` scoped to exactly those tables, never a blanket file-level operation.
- Whatever "clear cache" ends up meaning in code should be structurally incapable of touching playlist or track-history tables — enforced by which tables the cache-clearing code path even has a handle to, not just by convention or a comment.

## Open questions before implementation

- **Schema design**: table-per-concern (`speaker_cache`, `playlists`, `playlist_tracks`, `remembered_tracks`, ...) under one shared schema, versus some other layout. Not designed here — deliberately deferred until the features needing it (playlists, track history) are actually being built, so the schema reflects real requirements rather than a guess.
- **Migration/versioning strategy**: SQLite's own `PRAGMA user_version` plus hand-rolled migration steps, versus a library (e.g. `golang-migrate`) — for a schema this project fully controls (no third-party consumers of the file), hand-rolled `user_version` bumps may be sufficient, but not decided here.
- **Concrete cache-isolation mechanism**: the table-naming/access-scoping approach described above needs an actual design once the schema exists, not just the principle stated here.
- **Migration timing for the speaker cache specifically**: the cache (implementing ADR-0001) proceeds now on JSON rather than waiting for this ADR, since the two were decided independently and blocking the cache on the full storage layer wasn't judged worth the delay. A follow-up task tracks migrating it to this shared store once this ADR is implemented (see cross-reference below).
- **Playlist management and remembered-track features themselves**: not designed here at all — this ADR exists only to settle the storage foundation those features and the cache will eventually share. Feature design is out of scope until those tasks are actively worked.

## Consequences

New dependency: `modernc.org/sqlite`, this project's first storage dependency beyond the standard library and the first time a database driver has been evaluated here. The speaker cache's JSON implementation is expected to be temporary, superseded by a migration to this store once it exists — that's accepted up front, not a surprise to be discovered later. Playlist and track-history features are blocked on this ADR being implemented, not on any specific schema being pre-designed.
