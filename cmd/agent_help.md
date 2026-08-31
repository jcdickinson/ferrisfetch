# `rsdoc` — Rust Documentation Search

rsdoc indexes Rust crate documentation from docs.rs and provides semantic search.

It is only for Rust crates published in the crates.io/docs.rs ecosystem. Do not use rsdoc for npm packages, PyPI packages, GitHub repositories, general web docs, or any non-Rust package source.

## Important Constraints

- **Do NOT use curl, wget, or web fetching to access docs.rs directly.** rsdoc parses rustdoc JSON and provides clean, searchable markdown. Raw docs.rs HTML is noisy and wastes tokens. Always use rsdoc instead.
- **Do NOT use rsdoc unless the target is a Rust crate from crates.io/docs.rs.** If the thing you need is an npm package, a Python package, a web API, a repo README, or anything else outside the Rust crate ecosystem, use the appropriate tool instead.
- **`core`, `std`, `alloc`, `proc_macro`, `test`, and other standard library crates are NOT on docs.rs and will always 404.** Do not attempt to index them. Rely on your training data for standard library documentation.
- rsdoc auto-fetches crates on read if they haven't been indexed yet, so you can often skip `rsdoc add` for one-off lookups.
- Consider using sub-agents to find specific answers using rsdoc, instead of bringing multiple documentation pages into the primary chat context, unless you think that documentation could be useful multiple times.

## Workflow

1. Use `rsdoc info <crate>` to check crate metadata, latest version, and links
2. Use `rsdoc search-crates <query>` to find crates by name or keyword if you're not sure what's available
3. Use `rsdoc add <crate[@version]>` to index the crates relevant to your task (version defaults to "latest")
4. Use `rsdoc search <query>` to find relevant items with natural language queries
5. Use `rsdoc get <uri>` to read the returned `rsdoc://` URIs for full documentation

## Commands

### `rsdoc add <crate[@version] ...>`

Index one or more crates. Ensure crates are indexed before searching. Version defaults to "latest". Pin a specific version with `@version`.

```
rsdoc add serde
rsdoc add tokio@1.44.2
rsdoc add serde@1.0 tokio@1.0
```

### `rsdoc search [crate[@version]] <query>`

Semantic search across indexed documentation. Returns `rsdoc://` URIs. Use `--crate` to filter; omit to search everything indexed.

```
rsdoc search "serialize a struct to JSON"
rsdoc search bevy_image@0.19.0 "ImageSampler linear ImageSamplerDescriptor"
rsdoc search --crate serde "derive macro"
```

### `rsdoc search-crates <query>`

Search crates.io for Rust crates by name or keyword. Results indicate which crates are already indexed locally. Note that documentation can lag behind crate releases.

```
rsdoc search-crates "async http client"
```

### `rsdoc get <crate/version/path>`

Read a specific documentation item by URI. The `rsdoc://` prefix is optional and can be omitted (recommended).

```
rsdoc get serde/latest/serde::Serialize
rsdoc get tokio/1.44.2/tokio::spawn
rsdoc get serde/1.0.219/serde::Serialize%implementations
```

### `rsdoc uris <crate[@version]>`

List every `rsdoc://` URI in a crate, one per line, with the item kind. Useful when a `rsdoc get` lookup failed and you want to see the canonical paths, or when you have an item name in mind but aren't sure how to spell its URI. Auto-fetches the crate if not indexed. Pipe to `grep` to narrow.

```
rsdoc uris serde | grep Serialize
rsdoc uris tokio@1.44.2
```

### `rsdoc info <crate[@version]>`

Show crate metadata from crates.io (license, MSRV, downloads, links, keywords). No daemon needed.

```
rsdoc info serde
rsdoc info tokio@1.44.2
```

### `rsdoc which <crate> --with dep@ver`

Find crate versions whose dependencies are compatible with specific versions. Useful for finding which version of a crate to use with a given dependency version.

- `--with dep@ver` — dependency constraint (repeatable)
- `--newer-than X.Y.Z` — minimum version bound (inclusive)
- `--older-than X.Y.Z` — maximum version bound (inclusive)

```
rsdoc which tokio --with mio@1.0.0
rsdoc which tokio --with mio@1.0.0 --older-than 1.40.0
rsdoc which serde --with serde_derive@1.0 --newer-than 1.0.100
```

## URIs

Search results return `rsdoc://` URIs (e.g. `rsdoc://serde/1.0.219/serde::Serialize`). Read these with `rsdoc get` — the `rsdoc://` prefix can be omitted. Fragment suffixes like `%fields`, `%variants`, and `%implementations` return specific sections of an item's documentation. Use `%` as the fragment separator, even if you are provided a link with `#`.

The canonical path segment is the Rust path (`::` separated) starting with the **lib name** (underscored), not a docs.rs URL path. For example, `https://docs.rs/openrouter-rs/0.9.0/openrouter_rs/types/stream/enum.StreamEvent.html` is `rsdoc://openrouter-rs/0.9.0/openrouter_rs::types::stream::StreamEvent` — note the underscore in `openrouter_rs`, the `::` separators, and no `enum.` prefix.

`rsdoc get` is lenient: it also accepts docs.rs-style paths (slashes, `kind.Name` prefixes, missing lib name) and full `https://docs.rs/...` URLs, resolving them to the canonical item. All of these work for the same item:

```
rsdoc get openrouter-rs/0.9.0/openrouter_rs::types::stream::StreamEvent
rsdoc get openrouter-rs/0.9.0/types/stream/enum.StreamEvent
rsdoc get https://docs.rs/openrouter-rs/0.9.0/openrouter_rs/types/stream/enum.StreamEvent.html
```

If a lookup still fails — usually because the bare item name is ambiguous across modules — run `rsdoc uris <crate> | grep <name>` to see canonical paths.
