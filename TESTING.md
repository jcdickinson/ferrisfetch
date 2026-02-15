# Test Coverage Needed

Functions and areas that need test coverage, organized by package.

## internal/db

- `InsertEmbedding` — dimension validation, NaN/Inf rejection
- `formatEmbedding` — error cases (NaN, Inf, empty)
- `VectorSearch` — CTE correctness, crate filtering, threshold/limit
- `FindSimilarContent` — exclusion, threshold
- `GetCratesForItems` — batch lookup correctness, empty input
- `ResolveReexport` — exact match, glob prefix match, no match

## internal/daemon

- `Stop` — error collection via errors.Join
- `resetExpiration` — timer race (Stop+Reset)
- `addCrateWork` / `resolveVersion` / `indexItems` / `embedAndBacklink` — integration test with mock DB
- `handleGetDoc` — re-export fallback with error logging
- Streaming encode error handling (client disconnect)
- Socket permissions (0600 after creation)

## internal/search

- `Search` — nil item filtering before rerank
- `Search` — batched crate lookup integration
- BFS queue cap (bfsMaxQueueSize)
- Reranker fallback path (when rerank fails)

## internal/docs

- `GenerateFragments` — struct, enum, trait fragment generation
- `renderFnSig` — function signature rendering
- `resolveTypeName` — all type variants (resolved_path, primitive, dyn_trait, borrowed_ref, slice, generic, qualified_path, tuple)
- `selfShorthand` — all self parameter forms
- `CollectReexports` — glob and individual re-exports

## internal/markdown

- `RewriteLinks` — reference-style link rewriting (single-pass optimization)

## internal/config

- `cacheBase` — XDG_CACHE_HOME, UserHomeDir, /tmp fallback

## internal/cas

- `Write` / `Read` — round-trip, dedup, zstd compression

## internal/embeddings

- `ChunkSections` — existing tests, extend for edge cases
