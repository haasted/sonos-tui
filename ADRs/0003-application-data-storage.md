# 3. Application data storage

## Status

Accepted — implemented in `internal/store` and wired into `cmd/sonos`, which opens the database at startup and fails to start without it. The speaker cache has moved off JSON and into it, superseding ADR-0001's default.

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

**No ORM, and no query builder either: `database/sql` with hand-written SQL.** This is the generic-work question CLAUDE.md asks to be raised, and it was raised rather than skipped. The finding that settled it: no ORM targets an embedded local SQLite. GORM, ent and bun are multi-database abstractions whose value is portability across engines this project does not have and does not want; the libraries that *are* SQLite-specific (`ncruces/go-sqlite3`, `zombiezen.com/go/sqlite`) are drivers, not ORMs, so choosing an ORM buys nothing SQLite-shaped.

That leaves the cost side alone to decide it. The schema here is a handful of tables this project owns completely, with no third-party consumer and no second engine on any horizon. `sqlx` would remove some scanning boilerplate for a dependency; `sqlc` would give compile-time checking against the schema, which genuinely suits this codebase's habits, but adds a code-generation step and a pinned tool to the build. Neither pays for itself at this size.

This is not the same shape as the argument that carried the day above, and the difference matters. "N hand-rolled file formats beat one shared one" was about *formats* — each JSON file needing its own parser, its own versioning, its own load/save. SQL is already the one shared format; an ORM would be a second layer on top of it, not a replacement for N layers. Revisit if the query count grows past what is comfortable to read, with `sqlc` as the first step rather than an ORM.

**Migrations: `PRAGMA user_version` plus hand-rolled steps, not `golang-migrate`.** SQLite keeps four bytes in the file header that it never reads itself, so the version costs no table and has no bootstrap problem — a fresh file reads 0, which is already the correct starting state. Each step runs with its version bump in one transaction, and SQLite's DDL is transactional, so an interrupted migration leaves the file at a version it really has. `golang-migrate` and `goose` solve locking across instances, many environments and a CLI workflow; there is one file on one machine here. Forward-only, with no down migrations: the recovery for a bad one is to delete the file.

**Location: one file at `os.UserConfigDir()/sonos-tui/sonos.db`** — `~/Library/Application Support` on macOS, `~/.config` on Linux. Not the cache directory the speaker cache uses today, although that was the obvious candidate: `tmutil isexcluded` shows `~/Library/Caches` excluded from Time Machine and `~/Library/Application Support` included, and XDG says in as many words that a cache directory's contents may be deleted at any time. That is right for a regenerable speaker list and wrong for the only copy of a playlist, and this ADR put both in one file, so the file inherits the stricter requirement. Known imprecision, recorded rather than fixed: `UserConfigDir` means config on Linux, where XDG would want `~/.local/share` for data — the property that matters is that the directory is not disposable, and that holds either way.

**Cache isolation: a handle, not a naming convention.** The mitigation this ADR promised is `Store.Cache()`, which returns a type carrying no method that accepts SQL. Code holding one can clear the cache and can reach nothing else; widening that blast radius means editing `internal/store/cache.go`, not calling it wrongly. The tables it clears are a package-level list beside it. Being honest about the limit: Go cannot restrict a `*sql.DB` by table, so the guarantee is the shape of the API rather than anything the database enforces. A test keeps it true by clearing a populated cache with a non-cache table present and asserting that table survives.

**Connection settings, which matter more than the library choice.** WAL journalling so readers do not block the writer, a five-second `busy_timeout` so a held write lock is waited for rather than failed on, `synchronous=NORMAL` (safe against a crash under WAL), and foreign keys on, which SQLite leaves off for compatibility. All four are set through the DSN rather than with `Exec`, because a connection the pool opens later would never see an `Exec`. `_txlock=immediate` takes the write lock when a transaction begins rather than when it first writes, which is what lets `busy_timeout` apply instead of two writers deadlocking on a lock upgrade. This program has background event goroutines writing while the UI reads, so none of this is theoretical.

## Open questions remaining

- **Schema design for the features**: the cache half is settled — migration 1 creates `speaker_cache` and `speaker_cache_session`, mirroring what `speakers.json` held, because the isolation mechanism above needed real tables to scope to. `playlists`, `playlist_tracks` and `remembered_tracks` are still deliberately deferred until the features needing them are actually being built, so the schema reflects real requirements rather than a guess.
- **Playlist management and remembered-track features themselves**: not designed here at all — this ADR exists only to settle the storage foundation those features and the cache will eventually share. Feature design is out of scope until those tasks are actively worked.

## Consequences

New dependency: `modernc.org/sqlite`, this project's first storage dependency beyond the standard library and the first time a database driver has been evaluated here. It arrives with a transitive tail (`modernc.org/libc`, `modernc.org/memory`, `modernc.org/mathutil` and a few more) — the price of a C library transpiled to Go, and the price of keeping `CGO_ENABLED=0`. The `database/sql` decision keeps that tail the only thing added: no ORM, no query builder, no migration library, no code generator.

The storage layer lives in `internal/store`, below `internal/sonos` in the layering: it knows nothing about Sonos or about terminals, and callers hand it values and get values back. `internal/sonos` maps between the two, which means it now imports `internal/store` — and, since the `experiments` module imports `internal/sonos`, SQLite is compiled into the experiments too. A build-time weight only, and the price of keeping the mapping next to `Speaker` rather than in the application layer.

**Opening the database is fatal if it fails.** Considered and rejected: carrying on without it. The store holds the speaker cache today and will hold playlists, so a program that runs without it runs degraded in ways that get harder to see as more moves in. Losing the cache alone would have been survivable; that is no longer the whole of what is at stake.

Nothing migrates the old `speakers.json`. The cache was disposable by design, discovery rebuilds it in about five seconds, and an importer would be dead code the moment it had run once. Any leftover file in the cache directory is simply ignored.
