# Ferrisfetch

A vibe-coded MCP server for semantic search of Rust crate documentation.

> **Warning**: This project is completely vibe-coded. It works, but don't look too closely at the implementation details.

## What is Ferrisfetch?

Ferrisfetch fetches rustdoc JSON from docs.rs, parses it into markdown, vectorizes it with Voyage AI, and stores everything in DuckDB. It exposes semantic search over the indexed documentation via the Model Context Protocol (MCP).

Think of it as giving your AI assistant the ability to actually read Rust documentation instead of hallucinating API signatures.

## Features

- **Automatic Documentation Fetching**: Downloads and parses rustdoc JSON directly from docs.rs
- **Semantic Search**: Find documentation using natural language queries
- **Backlink Graph Traversal**: Discovers related items through documentation cross-references
- **Content-Addressable Storage**: Deduplicates docs across crate versions — re-indexing identical docs costs zero API calls
- **Auto-Fetch on Read**: Request docs for a crate you haven't indexed yet and it fetches automatically
- **Re-export Resolution**: Follows `pub use` chains to find canonical documentation
- **crates.io Search**: Search for crates by name or keyword without leaving your editor
- **Background Daemon**: Heavy work runs in a background daemon process that auto-exits after inactivity

## Quick Start

### Prerequisites

- Go 1.24+
- A [Voyage AI](https://voyageai.com) API key

### Installation

#### Option 1: Download Pre-built Binary

Download the latest release from [GitHub Releases](https://github.com/jcdickinson/ferrisfetch/releases).

#### Option 2: Build from Source

```bash
git clone https://github.com/jcdickinson/ferrisfetch
cd ferrisfetch
go build -o ferrisfetch ./cmd/ferrisfetch
```

#### Option 3: Nix

```bash
# Run directly from GitHub
nix run github:jcdickinson/ferrisfetch

# Or install to profile
nix profile install github:jcdickinson/ferrisfetch
```

### Configuration

Create `~/.config/ferrisfetch/config.toml`:

```toml
[voyage_ai]
model = "voyage-3.5"
rerank_model = "rerank-lite-1"

# Read API key from a file (recommended)
api_key = { path = "~/.config/ferrisfetch/voyage_api_key.txt" }
# Or inline (not recommended)
# api_key = "your-api-key"
```

Or use environment variables:

```bash
export FERRISFETCH_VOYAGE_AI_API_KEY="your-api-key"
```

### MCP Client Configuration

Add to your MCP client config (e.g. Claude Code `settings.json`):

```json
{
  "mcpServers": {
    "ferrisfetch": {
      "command": "ferrisfetch"
    }
  }
}
```

## MCP Tools

### `add_crates`

Index one or more crates. Version defaults to "latest".

```json
{"crates": [{"name": "serde"}, {"name": "tokio", "version": "1.0"}]}
```

### `search_docs`

Semantic search across indexed documentation. Returns `rsdoc://` resource URIs.

```json
{"query": "serialize a struct to JSON", "crates": ["serde", "serde_json"]}
```

### `search_crates`

Search crates.io for crates by name or keyword. Shows which are already indexed locally.

```json
{"query": "async http client"}
```

## MCP Resources

Search results return `rsdoc://` URIs (e.g. `rsdoc://serde/1.0.219/serde::Serialize`). Read these to get full markdown documentation for an item. Fragment suffixes like `#fields`, `#variants`, and `#implementations` give you specific sections.

## CLI Commands

```bash
ferrisfetch                  # Start as MCP server (default)
ferrisfetch add serde tokio  # Index crates from the command line
ferrisfetch search "async runtime" --crates tokio
ferrisfetch daemon           # Run the daemon in the foreground
ferrisfetch logs             # Tail the daemon log
ferrisfetch status           # Check if the daemon is running
ferrisfetch stop             # Stop the daemon
ferrisfetch clear-cache      # Clear the version resolution cache
```

Use `--debug` to run the daemon in-process with visible log output.

## Architecture

Ferrisfetch is a single binary that runs in two modes:

1. **MCP Server** (`ferrisfetch`): Communicates with your editor via stdio. Thin client that forwards requests to the daemon.
2. **Daemon** (`ferrisfetch daemon`): Background process that does all the heavy lifting — fetching docs, generating embeddings, running searches. Communicates over a Unix socket. Auto-spawned by the MCP server if not already running, auto-exits after 10 minutes of inactivity.

Data lives in `~/.cache/ferrisfetch/`:
- `db.db` — DuckDB database (items, embeddings, backlinks)
- `cas/` — Content-addressable storage for documentation markdown
- `json/` — Cached rustdoc JSON from docs.rs
- `daemon.log` — Daemon log output

## Known Issues

- No test suite (it's vibe-coded, remember?)
- Linux only for now
- Large crates can take a while to index on first fetch
- The DuckDB VSS extension needs to be available on your platform

## License

See [LICENSE](LICENSE).

## Credits

- Powered by [Voyage AI](https://voyageai.com) embeddings
- Uses [DuckDB](https://duckdb.org) with vector similarity search
- MCP integration via [mcp-go](https://github.com/mark3labs/mcp-go)
- Rustdoc JSON from [docs.rs](https://docs.rs)
