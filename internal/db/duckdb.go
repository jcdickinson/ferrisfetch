package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/marcboeker/go-duckdb"
)

type DB struct {
	conn *sql.DB
}

func New(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	conn, err := sql.Open("duckdb", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.initSchema(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("initializing schema: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) initSchema() error {
	queries := []string{
		`INSTALL vss;`,
		`LOAD vss;`,

		`CREATE SEQUENCE IF NOT EXISTS seq_crate_id START 1;`,
		`CREATE SEQUENCE IF NOT EXISTS seq_item_id START 1;`,
		`CREATE SEQUENCE IF NOT EXISTS seq_embedding_id START 1;`,
		`CREATE SEQUENCE IF NOT EXISTS seq_backlink_id START 1;`,
		`CREATE SEQUENCE IF NOT EXISTS seq_reexport_id START 1;`,

		`CREATE TABLE IF NOT EXISTS crates (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			version TEXT NOT NULL,
			fetched_at TIMESTAMP,
			processed_at TIMESTAMP,
			last_used_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(name, version)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_crates_name ON crates (name)`,

		`CREATE TABLE IF NOT EXISTS items (
			id INTEGER PRIMARY KEY,
			crate_id INTEGER REFERENCES crates(id),
			rustdoc_id TEXT NOT NULL,
			name TEXT NOT NULL,
			path TEXT NOT NULL,
			kind TEXT NOT NULL,
			content_hash TEXT,
			signature TEXT,
			doc_links TEXT,
			fragment_names TEXT,
			UNIQUE(crate_id, rustdoc_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_items_crate ON items (crate_id)`,
		`CREATE INDEX IF NOT EXISTS idx_items_path ON items (path)`,
		`CREATE INDEX IF NOT EXISTS idx_items_hash ON items (content_hash)`,

		`CREATE TABLE IF NOT EXISTS embeddings (
			id INTEGER PRIMARY KEY,
			content_hash TEXT NOT NULL,
			chunk_text TEXT NOT NULL,
			chunk_index INTEGER NOT NULL,
			embedding FLOAT[1024] NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_embeddings_hash ON embeddings (content_hash)`,

		`CREATE TABLE IF NOT EXISTS semantic_backlinks (
			id INTEGER PRIMARY KEY,
			hash_a TEXT NOT NULL,
			hash_b TEXT NOT NULL,
			similarity_score FLOAT NOT NULL,
			UNIQUE(hash_a, hash_b),
			CHECK(hash_a < hash_b)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_backlinks_a ON semantic_backlinks (hash_a)`,
		`CREATE INDEX IF NOT EXISTS idx_backlinks_b ON semantic_backlinks (hash_b)`,

		`CREATE TABLE IF NOT EXISTS reexports (
			id INTEGER PRIMARY KEY,
			crate_id INTEGER NOT NULL REFERENCES crates(id),
			local_prefix TEXT NOT NULL,
			source_crate TEXT NOT NULL,
			source_prefix TEXT NOT NULL,
			UNIQUE(crate_id, local_prefix)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_reexports_crate ON reexports (crate_id)`,
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return fmt.Errorf("executing %q: %w", q, err)
		}
	}
	return nil
}

// --- Crate operations ---

type Crate struct {
	ID          int
	Name        string
	Version     string
	FetchedAt   *time.Time
	ProcessedAt *time.Time
	LastUsedAt  time.Time
}

func (db *DB) UpsertCrate(name, version string) (*Crate, error) {
	var c Crate
	err := db.conn.QueryRow(
		`SELECT id, name, version, fetched_at, processed_at, last_used_at FROM crates WHERE name = ? AND version = ?`,
		name, version,
	).Scan(&c.ID, &c.Name, &c.Version, &c.FetchedAt, &c.ProcessedAt, &c.LastUsedAt)

	if err == nil {
		return &c, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("checking crate: %w", err)
	}

	_, err = db.conn.Exec(
		`INSERT INTO crates (id, name, version) VALUES (nextval('seq_crate_id'), ?, ?)`,
		name, version,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting crate: %w", err)
	}

	var id int
	if err := db.conn.QueryRow("SELECT currval('seq_crate_id')").Scan(&id); err != nil {
		return nil, fmt.Errorf("getting crate id: %w", err)
	}

	now := time.Now()
	return &Crate{ID: id, Name: name, Version: version, LastUsedAt: now}, nil
}

func (db *DB) MarkCrateFetched(crateID int) error {
	_, err := db.conn.Exec(`UPDATE crates SET fetched_at = CURRENT_TIMESTAMP WHERE id = ?`, crateID)
	return err
}

func (db *DB) MarkCrateProcessed(crateID int) error {
	_, err := db.conn.Exec(`UPDATE crates SET processed_at = CURRENT_TIMESTAMP WHERE id = ?`, crateID)
	return err
}

func (db *DB) TouchCrate(crateID int) error {
	_, err := db.conn.Exec(`UPDATE crates SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`, crateID)
	return err
}

func (db *DB) GetCrate(name, version string) (*Crate, error) {
	var c Crate
	err := db.conn.QueryRow(
		`SELECT id, name, version, fetched_at, processed_at, last_used_at FROM crates WHERE name = ? AND version = ?`,
		name, version,
	).Scan(&c.ID, &c.Name, &c.Version, &c.FetchedAt, &c.ProcessedAt, &c.LastUsedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetLatestCrate returns the most recently processed crate with the given name.
func (db *DB) GetLatestCrate(name string) (*Crate, error) {
	var c Crate
	err := db.conn.QueryRow(
		`SELECT id, name, version, fetched_at, processed_at, last_used_at
		 FROM crates WHERE name = ? AND processed_at IS NOT NULL
		 ORDER BY processed_at DESC LIMIT 1`, name,
	).Scan(&c.ID, &c.Name, &c.Version, &c.FetchedAt, &c.ProcessedAt, &c.LastUsedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (db *DB) ListCrates() ([]Crate, error) {
	rows, err := db.conn.Query(`SELECT id, name, version, fetched_at, processed_at, last_used_at FROM crates ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var crates []Crate
	for rows.Next() {
		var c Crate
		if err := rows.Scan(&c.ID, &c.Name, &c.Version, &c.FetchedAt, &c.ProcessedAt, &c.LastUsedAt); err != nil {
			return nil, err
		}
		crates = append(crates, c)
	}
	return crates, nil
}

// --- Item operations ---

type Item struct {
	ID            int
	CrateID       int
	RustdocID     string
	Name          string
	Path          string
	Kind          string
	ContentHash   string
	Signature     string
	DocLinks      string // JSON-encoded map[string]string
	FragmentNames string // JSON-encoded []string
}

func (db *DB) InsertItem(item *Item) error {
	_, err := db.conn.Exec(
		`INSERT INTO items (id, crate_id, rustdoc_id, name, path, kind, content_hash, signature, doc_links, fragment_names)
		 VALUES (nextval('seq_item_id'), ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.CrateID, item.RustdocID, item.Name, item.Path, item.Kind, item.ContentHash, item.Signature, item.DocLinks, item.FragmentNames,
	)
	if err != nil {
		return fmt.Errorf("inserting item: %w", err)
	}

	return db.conn.QueryRow(
		`SELECT id FROM items WHERE crate_id = ? AND rustdoc_id = ?`,
		item.CrateID, item.RustdocID,
	).Scan(&item.ID)
}

func (db *DB) GetItem(itemID int) (*Item, error) {
	var it Item
	err := db.conn.QueryRow(
		`SELECT id, crate_id, rustdoc_id, name, path, kind, content_hash, signature, doc_links, fragment_names FROM items WHERE id = ?`,
		itemID,
	).Scan(&it.ID, &it.CrateID, &it.RustdocID, &it.Name, &it.Path, &it.Kind, &it.ContentHash, &it.Signature, &it.DocLinks, &it.FragmentNames)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

func (db *DB) GetItemByPath(crateID int, path string) (*Item, error) {
	var it Item
	err := db.conn.QueryRow(
		`SELECT id, crate_id, rustdoc_id, name, path, kind, content_hash, signature, doc_links, fragment_names
		 FROM items WHERE crate_id = ? AND path = ?`,
		crateID, path,
	).Scan(&it.ID, &it.CrateID, &it.RustdocID, &it.Name, &it.Path, &it.Kind, &it.ContentHash, &it.Signature, &it.DocLinks, &it.FragmentNames)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

// GetItemForHash picks a representative item for a content hash.
// When crateIDs are specified, it prefers items from those crates.
func (db *DB) GetItemForHash(contentHash string, crateIDs []int) (*Item, error) {
	if len(crateIDs) > 0 {
		placeholders := make([]string, len(crateIDs))
		params := make([]interface{}, 0, len(crateIDs)+1)
		params = append(params, contentHash)
		for i, id := range crateIDs {
			placeholders[i] = "?"
			params = append(params, id)
		}
		var it Item
		err := db.conn.QueryRow(
			fmt.Sprintf(`SELECT id, crate_id, rustdoc_id, name, path, kind, content_hash, signature, doc_links, fragment_names
			 FROM items WHERE content_hash = ? AND crate_id IN (%s) LIMIT 1`,
				strings.Join(placeholders, ",")),
			params...,
		).Scan(&it.ID, &it.CrateID, &it.RustdocID, &it.Name, &it.Path, &it.Kind, &it.ContentHash, &it.Signature, &it.DocLinks, &it.FragmentNames)
		if err == nil {
			return &it, nil
		}
		if err != sql.ErrNoRows {
			return nil, err
		}
		// Fall through to any crate
	}

	var it Item
	err := db.conn.QueryRow(
		`SELECT id, crate_id, rustdoc_id, name, path, kind, content_hash, signature, doc_links, fragment_names
		 FROM items WHERE content_hash = ? LIMIT 1`, contentHash,
	).Scan(&it.ID, &it.CrateID, &it.RustdocID, &it.Name, &it.Path, &it.Kind, &it.ContentHash, &it.Signature, &it.DocLinks, &it.FragmentNames)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

func (db *DB) DeleteItemsByCrate(crateID int) error {
	_, err := db.conn.Exec(`DELETE FROM items WHERE crate_id = ?`, crateID)
	return err
}

// --- Embedding operations ---

const expectedEmbeddingDim = 1024

func (db *DB) InsertEmbedding(contentHash string, chunkText string, chunkIndex int, embedding []float32) error {
	if len(embedding) != expectedEmbeddingDim {
		return fmt.Errorf("expected embedding dimension %d, got %d", expectedEmbeddingDim, len(embedding))
	}
	embStr, err := formatEmbedding(embedding)
	if err != nil {
		return fmt.Errorf("formatting embedding: %w", err)
	}
	_, err = db.conn.Exec(
		`INSERT INTO embeddings (id, content_hash, chunk_text, chunk_index, embedding)
		 VALUES (nextval('seq_embedding_id'), ?, ?, ?, ?::FLOAT[1024])`,
		contentHash, chunkText, chunkIndex, embStr,
	)
	return err
}

// HasEmbeddings checks if a content hash already has embeddings stored.
func (db *DB) HasEmbeddings(contentHash string) bool {
	var count int
	db.conn.QueryRow(`SELECT COUNT(*) FROM embeddings WHERE content_hash = ?`, contentHash).Scan(&count)
	return count > 0
}

// --- Vector search ---

type SearchResult struct {
	ContentHash string
	CrateID     int
	Name        string
	Path        string
	Kind        string
	Signature   string
	Similarity  float32
}

func (db *DB) VectorSearch(embedding []float32, threshold float32, limit int, crateIDs []int) ([]SearchResult, error) {
	embStr, err := formatEmbedding(embedding)
	if err != nil {
		return nil, fmt.Errorf("formatting embedding: %w", err)
	}

	var crateFilter string
	var params []interface{}
	if len(crateIDs) > 0 {
		placeholders := make([]string, len(crateIDs))
		for i, id := range crateIDs {
			placeholders[i] = "?"
			params = append(params, id)
		}
		crateFilter = fmt.Sprintf(` AND EXISTS (SELECT 1 FROM items i WHERE i.content_hash = dists.content_hash AND i.crate_id IN (%s))`,
			strings.Join(placeholders, ","))
	}

	query := fmt.Sprintf(`
		WITH dists AS (
			SELECT e.content_hash, (1 - (e.embedding <=> %s)) as sim
			FROM embeddings e
		)
		SELECT content_hash, MAX(sim) as similarity
		FROM dists
		WHERE sim > ?%s
		GROUP BY content_hash
		ORDER BY similarity DESC
		LIMIT ?`, embStr, crateFilter)

	params = append([]interface{}{threshold}, params...)
	params = append(params, limit)

	rows, err := db.conn.Query(query, params...)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.ContentHash, &r.Similarity); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}
		results = append(results, r)
	}
	return results, nil
}

// --- Backlink operations ---

func (db *DB) UpsertBacklink(hashA, hashB string, similarity float32) error {
	if hashA > hashB {
		hashA, hashB = hashB, hashA
	}
	_, err := db.conn.Exec(
		`INSERT INTO semantic_backlinks (id, hash_a, hash_b, similarity_score)
		 VALUES (nextval('seq_backlink_id'), ?, ?, ?)
		 ON CONFLICT (hash_a, hash_b) DO UPDATE SET similarity_score = EXCLUDED.similarity_score`,
		hashA, hashB, similarity,
	)
	return err
}

func (db *DB) GetBacklinks(contentHash string, minSimilarity float32) ([]struct {
	ContentHash string
	Similarity  float32
}, error) {
	query := `
		SELECT CASE WHEN hash_a = ? THEN hash_b ELSE hash_a END as other_hash,
		       similarity_score
		FROM semantic_backlinks
		WHERE (hash_a = ? OR hash_b = ?) AND similarity_score >= ?
		ORDER BY similarity_score DESC`

	rows, err := db.conn.Query(query, contentHash, contentHash, contentHash, minSimilarity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []struct {
		ContentHash string
		Similarity  float32
	}
	for rows.Next() {
		var r struct {
			ContentHash string
			Similarity  float32
		}
		if err := rows.Scan(&r.ContentHash, &r.Similarity); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// FindSimilarContent finds content hashes similar to the given embedding for backlink generation.
func (db *DB) FindSimilarContent(embedding []float32, threshold float32, limit int, excludeHash string) ([]struct {
	ContentHash string
	Similarity  float32
}, error) {
	embStr, err := formatEmbedding(embedding)
	if err != nil {
		return nil, fmt.Errorf("formatting embedding: %w", err)
	}

	query := fmt.Sprintf(`
		WITH dists AS (
			SELECT e.content_hash, (1 - (e.embedding <=> %s)) as sim
			FROM embeddings e
		)
		SELECT content_hash, MAX(sim) as similarity
		FROM dists
		WHERE content_hash != ? AND sim > ?
		GROUP BY content_hash
		ORDER BY similarity DESC
		LIMIT ?`, embStr)

	rows, err := db.conn.Query(query, excludeHash, threshold, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []struct {
		ContentHash string
		Similarity  float32
	}
	for rows.Next() {
		var r struct {
			ContentHash string
			Similarity  float32
		}
		if err := rows.Scan(&r.ContentHash, &r.Similarity); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// GetCratesForItems returns a map from item ID to Crate for the given item IDs in a single query.
func (db *DB) GetCratesForItems(itemIDs []int) (map[int]*Crate, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(itemIDs))
	params := make([]interface{}, len(itemIDs))
	for i, id := range itemIDs {
		placeholders[i] = "?"
		params[i] = id
	}
	query := fmt.Sprintf(`
		SELECT i.id, c.id, c.name, c.version, c.fetched_at, c.processed_at, c.last_used_at
		FROM items i JOIN crates c ON c.id = i.crate_id
		WHERE i.id IN (%s)`, strings.Join(placeholders, ","))
	rows, err := db.conn.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int]*Crate, len(itemIDs))
	for rows.Next() {
		var itemID int
		var c Crate
		if err := rows.Scan(&itemID, &c.ID, &c.Name, &c.Version, &c.FetchedAt, &c.ProcessedAt, &c.LastUsedAt); err != nil {
			return nil, err
		}
		result[itemID] = &c
	}
	return result, nil
}

func (db *DB) GetCrateIDsByNames(names []string) ([]int, error) {
	if len(names) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(names))
	params := make([]interface{}, len(names))
	for i, n := range names {
		placeholders[i] = "?"
		params[i] = n
	}
	query := fmt.Sprintf(`SELECT id FROM crates WHERE name IN (%s)`, strings.Join(placeholders, ","))
	rows, err := db.conn.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// GetIndexedVersions returns name→version for processed crates matching the given names.
// If multiple versions exist for the same name, the one with the latest processed_at wins.
func (db *DB) GetIndexedVersions(names []string) (map[string]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(names))
	params := make([]interface{}, len(names))
	for i, n := range names {
		placeholders[i] = "?"
		params[i] = n
	}
	query := fmt.Sprintf(`
		SELECT name, version
		FROM (
			SELECT name, version, ROW_NUMBER() OVER (PARTITION BY name ORDER BY processed_at DESC) as rn
			FROM crates
			WHERE name IN (%s) AND processed_at IS NOT NULL
		)
		WHERE rn = 1`, strings.Join(placeholders, ","))

	rows, err := db.conn.Query(query, params...)
	if err != nil {
		return nil, fmt.Errorf("getting indexed versions: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var name, version string
		if err := rows.Scan(&name, &version); err != nil {
			return nil, err
		}
		result[name] = version
	}
	return result, nil
}

func (db *DB) CountItems(crateID int) (int, error) {
	var count int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM items WHERE crate_id = ?`, crateID).Scan(&count)
	return count, err
}

// --- Reexport operations ---

func (db *DB) InsertReexport(crateID int, localPrefix, sourceCrate, sourcePrefix string) error {
	_, err := db.conn.Exec(
		`INSERT INTO reexports (id, crate_id, local_prefix, source_crate, source_prefix)
		 VALUES (nextval('seq_reexport_id'), ?, ?, ?, ?)
		 ON CONFLICT (crate_id, local_prefix) DO UPDATE SET source_crate = ?, source_prefix = ?`,
		crateID, localPrefix, sourceCrate, sourcePrefix, sourceCrate, sourcePrefix,
	)
	return err
}

func (db *DB) DeleteReexportsByCrate(crateID int) error {
	_, err := db.conn.Exec(`DELETE FROM reexports WHERE crate_id = ?`, crateID)
	return err
}

// ResolveReexport checks if the given path matches a re-export in this crate.
// Tries exact match first, then longest prefix match (for glob re-exports).
// Returns the source crate name and resolved source path.
func (db *DB) ResolveReexport(crateID int, path string) (sourceCrate, sourcePath string, found bool) {
	var localPrefix, srcCrate, srcPrefix string
	err := db.conn.QueryRow(
		`SELECT local_prefix, source_crate, source_prefix FROM reexports
		 WHERE crate_id = ? AND (local_prefix = ? OR ? LIKE local_prefix || '::%')
		 ORDER BY length(local_prefix) DESC LIMIT 1`,
		crateID, path, path,
	).Scan(&localPrefix, &srcCrate, &srcPrefix)
	if err != nil {
		return "", "", false
	}

	if localPrefix == path {
		return srcCrate, srcPrefix, true
	}
	suffix := path[len(localPrefix):]
	return srcCrate, srcPrefix + suffix, true
}

func formatEmbedding(embedding []float32) (string, error) {
	strs := make([]string, len(embedding))
	for i, v := range embedding {
		if v != v || v-v != 0 { // NaN or Inf check without math import
			return "", fmt.Errorf("embedding contains NaN or Inf at index %d", i)
		}
		strs[i] = fmt.Sprintf("%g", v)
	}
	return "[" + strings.Join(strs, ",") + "]", nil
}
