---
title: Re-export Resolution for pub use Items
description: How ferrisfetch handles Rust pub use re-exports so that items like tracing::field::Value resolve to their defining crate
tags:
  feature: reexport-resolution
  project: ferrisfetch
created: 2026-02-14T20:44:28.8998897-08:00
modified: 2026-02-14T20:44:28.8998897-08:00
---

# Re-export Resolution

## Problem
Rust crates heavily use `pub use` to re-export items from dependencies. Example: `tracing::field::Value` is actually defined in `tracing-core`. Without re-export handling, these paths 404.

## How Rustdoc JSON Encodes Re-exports
- **`use` items** in the index with `crate_id=0` (the use statement is local)
- `inner.use` has: `source`, `name`, `id` (target item ID), `is_glob`
- **Glob re-exports** (`pub use tracing_core::field::*`): `is_glob=true`, `id` points to the source module
- **Individual re-exports** (`pub use tracing_core::Level`): `is_glob=false`, `id` points to the specific item
- The `id` always resolves through chains — points to the final canonical item
- Target items are usually NOT in the index (only in `paths` map)

## Architecture
- **`internal/docs/reexports.go`**: `CollectReexports(crate, crateName)` walks module tree, returns `[]Reexport{LocalPrefix, SourceCrate, SourcePrefix}`
- **`reexports` DB table**: `(crate_id, local_prefix, source_crate, source_prefix)` with unique on `(crate_id, local_prefix)`
- **Resolution**: exact match first, then longest-prefix match (for globs) via `LIKE local_prefix || '::%'`
- Prefix match replaces local prefix with source prefix, appends suffix

## Crate Name Normalization
- Rustdoc JSON `ExternalCrate.Name` uses Rust lib name (underscores: `tracing_core`)
- docs.rs/Cargo uses hyphens (`tracing-core`)
- `resolveOrFetchCrate` tries both the given name and `strings.ReplaceAll(name, "_", "-")`
- DB lookup checks both forms before attempting auto-fetch

## Key Files
- `internal/docs/reexports.go` — collection logic
- `internal/db/duckdb.go` — `InsertReexport`, `DeleteReexportsByCrate`, `ResolveReexport`
- `internal/daemon/server.go` — stores during `addCrateWork`, resolves in `handleGetDoc`
