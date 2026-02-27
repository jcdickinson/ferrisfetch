# rsdoc — Rust Documentation Search

rsdoc indexes Rust crate documentation from docs.rs and provides semantic search. Use it instead of fetching documentation manually.

## Important Constraints

- **Do NOT use curl, wget, or web fetching to access docs.rs directly.** rsdoc parses rustdoc JSON and provides clean, searchable markdown. Raw docs.rs HTML is noisy and wastes tokens. Always use rsdoc instead.
- **`core`, `std`, `alloc`, `proc_macro`, `test`, and other standard library crates are NOT on docs.rs and will always 404.** Do not attempt to index them. Rely on your training data for standard library documentation.
- rsdoc auto-fetches crates on read if they haven't been indexed yet, so you can often skip `rsdoc add` for one-off lookups.
- Consider using sub-agents to find specific answers using rsdoc, instead of bringing multiple documentation pages into the primary chat context, unless you think that documentation could be useful multiple times.

## Workflow

1. Use `rsdoc search-crates <query>` to find crates by name or keyword if you're not sure what's available
2. Use `rsdoc add <crate[@version]>` to index the crates relevant to your task (version defaults to "latest")
3. Use `rsdoc search <query>` to find relevant items with natural language queries
4. Use `rsdoc get <uri>` to read the returned `rsdoc://` URIs for full documentation

## Commands

### `rsdoc add <crate[@version] ...>`

Index one or more crates. Ensure crates are indexed before searching. Version defaults to "latest". Pin a specific version with `@version`.

```
rsdoc add serde
rsdoc add tokio@1.44.2
rsdoc add serde@1.0 tokio@1.0
```

### `rsdoc search <query>`

Semantic search across indexed documentation. Returns `rsdoc://` URIs. Use `--crate` to filter; omit to search everything indexed.

```
rsdoc search "serialize a struct to JSON"
rsdoc search --crate serde "derive macro"
```

### `rsdoc search-crates <query>`

Search crates.io for Rust crates by name or keyword. Results indicate which crates are already indexed locally.

```
rsdoc search-crates "async http client"
```

### `rsdoc get <crate/version/path>`

Read a specific documentation item by URI. The `rsdoc://` prefix is optional and can be omitted (recommended).

```
rsdoc get serde/latest/serde::Serialize
rsdoc get tokio/1.44.2/tokio::spawn
rsdoc get serde/1.0.219/serde::Serialize#implementations
```

## URIs

Search results return `rsdoc://` URIs (e.g. `rsdoc://serde/1.0.219/serde::Serialize`). Read these with `rsdoc get` — the `rsdoc://` prefix can be omitted. Fragment suffixes like `#fields`, `#variants`, and `#implementations` return specific sections of an item's documentation.
