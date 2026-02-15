package db

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/habedi/hann/core"
	"github.com/habedi/hann/hnsw"
	_ "github.com/mattn/go-sqlite3"
)

const (
	embeddingDim = 1024
	hnswM        = 16
	hnswEf       = 100
)

type DB struct {
	conn     *sql.DB
	hnsw     *hnsw.HNSWIndex
	hnswPath string
}

func New(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	// If an existing file isn't SQLite, delete it (stale DuckDB file).
	if info, err := os.Stat(dbPath); err == nil && info.Size() >= 4 {
		f, err := os.Open(dbPath)
		if err == nil {
			header := make([]byte, 4)
			n, _ := f.Read(header)
			f.Close()
			if n >= 4 && string(header) != "SQLi" {
				log.Printf("Removing non-SQLite database file at %s", dbPath)
				os.Remove(dbPath)
			}
		}
	}

	hnswPath := strings.TrimSuffix(dbPath, filepath.Ext(dbPath)) + ".hnsw"

	dsn := "file:" + dbPath + "?_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	d := &DB{conn: conn, hnswPath: hnswPath}
	if err := d.initSchema(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("initializing schema: %w", err)
	}

	if err := d.loadOrCreateHNSW(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("initializing HNSW index: %w", err)
	}

	return d, nil
}

func (db *DB) Close() error {
	db.saveHNSW()
	return db.conn.Close()
}

func (db *DB) initSchema() error {
	queries := []string{
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
			embedding BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_embeddings_hash ON embeddings (content_hash)`,

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

	result, err := db.conn.Exec(
		`INSERT INTO crates (name, version) VALUES (?, ?)`,
		name, version,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting crate: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting crate id: %w", err)
	}

	now := time.Now()
	return &Crate{ID: int(id), Name: name, Version: version, LastUsedAt: now}, nil
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
	result, err := db.conn.Exec(
		`INSERT INTO items (crate_id, rustdoc_id, name, path, kind, content_hash, signature, doc_links, fragment_names)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.CrateID, item.RustdocID, item.Name, item.Path, item.Kind, item.ContentHash, item.Signature, item.DocLinks, item.FragmentNames,
	)
	if err != nil {
		return fmt.Errorf("inserting item: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting item id: %w", err)
	}
	item.ID = int(id)
	return nil
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
	query := `SELECT id, crate_id, rustdoc_id, name, path, kind, content_hash, signature, doc_links, fragment_names
		 FROM items WHERE content_hash = ?`
	var params []interface{}
	params = append(params, contentHash)

	if len(crateIDs) > 0 {
		placeholders := make([]string, len(crateIDs))
		for i, id := range crateIDs {
			placeholders[i] = "?"
			params = append(params, id)
		}
		query += fmt.Sprintf(` AND crate_id IN (%s)`, strings.Join(placeholders, ","))
	}
	query += ` LIMIT 1`

	var it Item
	err := db.conn.QueryRow(query, params...).Scan(&it.ID, &it.CrateID, &it.RustdocID, &it.Name, &it.Path, &it.Kind, &it.ContentHash, &it.Signature, &it.DocLinks, &it.FragmentNames)
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

func (db *DB) InsertEmbedding(contentHash string, chunkText string, chunkIndex int, embedding []float32) error {
	if len(embedding) != embeddingDim {
		return fmt.Errorf("expected embedding dimension %d, got %d", embeddingDim, len(embedding))
	}
	if err := validateEmbedding(embedding); err != nil {
		return err
	}

	blob := serializeFloat32(embedding)
	result, err := db.conn.Exec(
		`INSERT INTO embeddings (content_hash, chunk_text, chunk_index, embedding) VALUES (?, ?, ?, ?)`,
		contentHash, chunkText, chunkIndex, blob,
	)
	if err != nil {
		return fmt.Errorf("inserting embedding: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting embedding id: %w", err)
	}

	// Copy to avoid hann's in-place normalization mutating our slice.
	vec := make([]float32, len(embedding))
	copy(vec, embedding)
	if err := db.hnsw.Add(int(id), vec); err != nil {
		return fmt.Errorf("adding to HNSW index: %w", err)
	}

	return nil
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

// knnSearch runs a KNN query against the HNSW index and returns content_hash + similarity pairs,
// grouped by content_hash (keeping the best similarity per hash).
func (db *DB) knnSearch(embedding []float32, fetchLimit int, threshold float32, allowedHashes map[string]bool) (map[string]float32, error) {
	stats := db.hnsw.Stats()
	if stats.Count == 0 {
		return nil, nil
	}

	topK := fetchLimit
	if topK > stats.Count {
		topK = stats.Count
	}

	hits, err := db.hnsw.Search(embedding, topK)
	if err != nil {
		return nil, fmt.Errorf("HNSW search: %w", err)
	}
	if len(hits) == 0 {
		return nil, nil
	}

	// Batch lookup content_hash for matched IDs.
	placeholders := make([]string, len(hits))
	params := make([]interface{}, len(hits))
	for i, h := range hits {
		placeholders[i] = "?"
		params[i] = h.ID
	}
	hashRows, err := db.conn.Query(
		fmt.Sprintf(`SELECT id, content_hash FROM embeddings WHERE id IN (%s)`, strings.Join(placeholders, ",")),
		params...,
	)
	if err != nil {
		return nil, fmt.Errorf("looking up content hashes: %w", err)
	}
	defer hashRows.Close()

	idToHash := make(map[int]string, len(hits))
	for hashRows.Next() {
		var id int
		var hash string
		if err := hashRows.Scan(&id, &hash); err != nil {
			return nil, err
		}
		idToHash[id] = hash
	}

	// Post-process: convert distance → similarity, filter, group by content_hash.
	best := make(map[string]float32)
	for _, h := range hits {
		hash, ok := idToHash[h.ID]
		if !ok {
			continue
		}
		sim := float32(1 - h.Distance)
		if sim <= threshold {
			continue
		}
		if allowedHashes != nil && !allowedHashes[hash] {
			continue
		}
		if prev, ok := best[hash]; !ok || sim > prev {
			best[hash] = sim
		}
	}

	return best, nil
}

func (db *DB) VectorSearch(embedding []float32, threshold float32, limit int, crateIDs []int) ([]SearchResult, error) {
	// Load allowed content hashes if filtering by crate.
	var allowedHashes map[string]bool
	if len(crateIDs) > 0 {
		var err error
		allowedHashes, err = db.contentHashesForCrates(crateIDs)
		if err != nil {
			return nil, fmt.Errorf("loading crate hashes: %w", err)
		}
		if len(allowedHashes) == 0 {
			return nil, nil
		}
	}

	fetchLimit := limit * 10
	if fetchLimit > 5000 {
		fetchLimit = 5000
	}

	best, err := db.knnSearch(embedding, fetchLimit, threshold, allowedHashes)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(best))
	for hash, sim := range best {
		results = append(results, SearchResult{ContentHash: hash, Similarity: sim})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// contentHashesForCrates returns the set of content hashes belonging to the given crate IDs.
func (db *DB) contentHashesForCrates(crateIDs []int) (map[string]bool, error) {
	placeholders := make([]string, len(crateIDs))
	params := make([]interface{}, len(crateIDs))
	for i, id := range crateIDs {
		placeholders[i] = "?"
		params[i] = id
	}
	query := fmt.Sprintf(
		`SELECT DISTINCT content_hash FROM items WHERE crate_id IN (%s) AND content_hash IS NOT NULL`,
		strings.Join(placeholders, ","))
	rows, err := db.conn.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hashes := make(map[string]bool)
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		hashes[h] = true
	}
	return hashes, nil
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

// GetIndexedVersions returns name->version for processed crates matching the given names.
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
		`INSERT INTO reexports (crate_id, local_prefix, source_crate, source_prefix)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (crate_id, local_prefix) DO UPDATE SET source_crate = EXCLUDED.source_crate, source_prefix = EXCLUDED.source_prefix`,
		crateID, localPrefix, sourceCrate, sourcePrefix,
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

func newHNSW() *hnsw.HNSWIndex {
	return hnsw.NewHNSW(embeddingDim, hnswM, hnswEf, core.Distances["cosine"], "cosine")
}

// loadOrCreateHNSW loads the HNSW index from disk, or creates a new one.
// If embeddings exist in SQLite but the HNSW file is missing, rebuilds from SQLite.
func (db *DB) loadOrCreateHNSW() error {
	if f, err := os.Open(db.hnswPath); err == nil {
		db.hnsw = newHNSW()
		if err := db.hnsw.Load(f); err != nil {
			f.Close()
			return fmt.Errorf("loading HNSW index: %w", err)
		}
		f.Close()
		return nil
	}

	db.hnsw = newHNSW()

	// Rebuild from SQLite if embeddings exist.
	var count int
	db.conn.QueryRow(`SELECT COUNT(*) FROM embeddings`).Scan(&count)
	if count == 0 {
		return nil
	}

	log.Printf("rebuilding HNSW index from %d embeddings in SQLite", count)

	rows, err := db.conn.Query(`SELECT id, embedding FROM embeddings`)
	if err != nil {
		return fmt.Errorf("reading embeddings for HNSW rebuild: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			return fmt.Errorf("scanning embedding row: %w", err)
		}
		vec := deserializeFloat32(blob)
		if len(vec) != embeddingDim {
			log.Printf("skipping embedding id=%d: dimension %d != %d", id, len(vec), embeddingDim)
			continue
		}
		if err := db.hnsw.Add(id, vec); err != nil {
			log.Printf("skipping embedding id=%d: %v", id, err)
		}
	}

	db.saveHNSW()
	return nil
}

// SaveHNSW persists the HNSW index to disk.
func (db *DB) SaveHNSW() {
	db.saveHNSW()
}

func (db *DB) saveHNSW() {
	if db.hnsw == nil {
		return
	}
	f, err := os.Create(db.hnswPath)
	if err != nil {
		log.Printf("failed to create HNSW file: %v", err)
		return
	}
	if err := db.hnsw.Save(f); err != nil {
		log.Printf("failed to save HNSW index: %v", err)
	}
	f.Close()
}

func serializeFloat32(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func deserializeFloat32(buf []byte) []float32 {
	v := make([]float32, len(buf)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return v
}

func validateEmbedding(embedding []float32) error {
	for i, v := range embedding {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return fmt.Errorf("embedding contains NaN or Inf at index %d", i)
		}
	}
	return nil
}
