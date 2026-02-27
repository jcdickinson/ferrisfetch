# rsdoc

Semantic search for Rust crate documentation. Fetches rustdoc JSON from docs.rs, parses it into markdown, vectorizes it with Voyage AI, and stores everything in SQLite with HNSW indexing.

## Features

- **Semantic Search**: Find documentation using natural language queries
- **Content-Addressable Storage**: Deduplicates docs across crate versions — re-indexing identical docs costs zero API calls
- **Auto-Fetch on Read**: Request docs for a crate you haven't indexed yet and it fetches automatically
- **Re-export Resolution**: Follows `pub use` chains to find canonical documentation
- **crates.io Search**: Search for crates by name or keyword
- **Background Daemon**: Heavy work runs in a background daemon that auto-exits after inactivity

## Quick Start

### Prerequisites

- Go 1.24+
- A [Voyage AI](https://voyageai.com) API key

### Installation

#### Build from Source

```bash
git clone https://github.com/jcdickinson/ferrisfetch
cd ferrisfetch
go build -o rsdoc ./cmd/rsdoc
```

#### Nix

```bash
nix run github:jcdickinson/ferrisfetch
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

## Usage

When `CLAUDECODE=1` or `AGENT=1` is set, `rsdoc --help` outputs markdown instructions tailored for AI agents. To make an agent aware of the tool, add a `CLI Tools` section to your `CLAUDE.md` or `AGENTS.md`:

```md
## CLI Tools

These tools are intended to be used in the same way that MCPs are. stderr is for logging and you should typically ignore it.

- `rsdoc --help`: used to search for crates, as well as semantic search across indexed crates. Use this instead of fetching documentation from doc.rs.
```

[CLI tools typically use fewer tokens than MCPs.](https://kanyilmaz.me/2026/02/23/cli-vs-mcp.html)

### Commands

```bash
rsdoc add serde tokio            # Index crates (latest version)
rsdoc add tokio@1.44.2           # Index a specific version
rsdoc search "async runtime"     # Semantic search
rsdoc search-crates serde        # Search crates.io
rsdoc get serde/latest/serde::Serialize  # Read a doc item (rsdoc:// prefix optional)
rsdoc status                     # Show indexed crates
rsdoc logs                       # Tail daemon log
rsdoc stop                       # Stop the daemon
rsdoc clear-cache                # Clear version resolution cache
```

Use `--debug` to run the daemon in-process with visible log output.

## Architecture

Single binary, two modes:

1. **CLI** (`rsdoc <command>`): Thin client that forwards requests to the daemon.
2. **Daemon** (`rsdoc daemon`): Background process that does the heavy lifting — fetching docs, generating embeddings, running searches. Communicates over a Unix socket. Auto-spawned if not running, auto-exits after 10 minutes of inactivity.

Data lives in `~/.cache/ferrisfetch/`:
- `db.db` — SQLite database with HNSW index
- `cas/` — Content-addressable storage for documentation markdown
- `json/` — Cached rustdoc JSON from docs.rs
- `daemon.log` — Daemon log output

## License

See [LICENSE](LICENSE).

## Credits

- [Voyage AI](https://voyageai.com) embeddings
- [hann](https://github.com/habedi/hann) HNSW indexing
- [docs.rs](https://docs.rs) rustdoc JSON
