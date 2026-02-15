# Ferrisfetch — Rust Documentation Search

Ferrisfetch indexes Rust crate documentation from docs.rs and provides semantic search. Use it instead of fetching documentation manually.

## Important Constraints

- **Do NOT use curl, wget, or web fetching to access docs.rs directly.** Ferrisfetch parses rustdoc JSON and provides clean, searchable markdown. Raw docs.rs HTML is noisy and wastes tokens. Always use Ferrisfetch tools instead.
- **`core`, `std`, `alloc`, `proc_macro`, `test`, and other standard library crates are NOT on docs.rs and will always 404.** Do not attempt to index them. Rely on your training data for standard library documentation.
- Ferrisfetch auto-fetches crates on read if they haven't been indexed yet, so you can often skip `add_crates` for one-off lookups.

## Workflow

1. Use `search_crates` to find crates by name or keyword if you're not sure what's available
2. Use `add_crates` to index the crates relevant to your task (version defaults to "latest")
3. Use `search_docs` to find relevant items with natural language queries
4. Read the returned `rsdoc://` resource URIs for full documentation

## Tools

### `add_crates`
Index one or more crates. Call this to ensure crates are indexed before searching.
```json
{"crates": [{"name": "serde"}, {"name": "tokio", "version": "1.0"}]}
```

### `search_docs`
Semantic search across indexed documentation. Returns `rsdoc://` resource URIs you can read for full docs. Use `crates` to filter; omit to search everything indexed.
```json
{"query": "serialize a struct to JSON", "crates": ["serde", "serde_json"]}
```

### `search_crates`
Search crates.io for Rust crates by name or keyword. Results indicate which crates are already indexed locally.
```json
{"query": "async http client"}
```

## Resources

Search results return `rsdoc://` URIs (e.g. `rsdoc://serde/1.0.219/serde::Serialize`). Read these to get full markdown documentation. Fragment suffixes like `#fields`, `#variants`, and `#implementations` return specific sections of an item's documentation.
