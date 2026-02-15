---
title: Code Review Fixes Applied
description: Comprehensive code review fixes applied to ferrisfetch - security, bugs, performance, structure
tags:
  project: ferrisfetch
  type: refactor
created: 2026-02-14T22:25:50.649193852-08:00
modified: 2026-02-14T22:25:50.649193852-08:00
---

# Code Review Fixes Applied

## Security
- Socket permissions: `os.Chmod(socketPath, 0600)` after `net.Listen`
- `formatEmbedding` now returns `(string, error)`, validates for NaN/Inf
- `InsertEmbedding` validates `len(embedding) == 1024`

## Bug Fixes
- `Stop()` collects all errors via `errors.Join`, logs each
- `resetExpiration` calls `Stop()` before `Reset()` (AfterFunc timer)
- Nil items filtered before reranker (parallel `resolved` + `documents` slices)
- Unchecked `json.Unmarshal` in `handleGetDoc` and `traitMethodFragments` now checked
- `os.Exit(1)` after `log.Fatalf` removed (unreachable)
- `config.go`: extracted `cacheBase()` helper with `/tmp/ferrisfetch` fallback
- Streaming encode now checks error and returns early on client disconnect
- Re-export resolution errors now logged

## Performance
- `GetCratesForItems` batch query replaces N+1 `GetCrateForItem` calls
- `VectorSearch` and `FindSimilarContent` use CTE to avoid double cosine distance
- BFS queue capped at 500 entries
- Reference link rewriting: single pass over lines with map lookup
- Removed `COUNT(*)` debug logging queries from `VectorSearch`

## Structure
- `addCrateWork` split into `resolveVersion`, `indexItems`, `embedAndBacklink`
- `fragments.go` split into 3 files: `fragments.go`, `fragments_sig.go`, `fragments_types.go`
- Fragment name constants (`FragFields`, `FragVariants`, etc.)
- `LoadConfig()` removed from `mcp/server.go` (dead code)
- `GetCrateForItem` removed (replaced by batch method)
- Backlink chunk-0 comment added

## Decision: 2i (links.go second len check)
Kept the second `if len(segments) == 0` check — it's reachable when `index.html` is the only segment.
