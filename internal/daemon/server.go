package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jcdickinson/ferrisfetch/internal/cas"
	"github.com/jcdickinson/ferrisfetch/internal/config"
	"github.com/jcdickinson/ferrisfetch/internal/db"
	"github.com/jcdickinson/ferrisfetch/internal/docs"
	"github.com/jcdickinson/ferrisfetch/internal/embeddings"
	md "github.com/jcdickinson/ferrisfetch/internal/markdown"
	"github.com/jcdickinson/ferrisfetch/internal/rpc"
	"github.com/jcdickinson/ferrisfetch/internal/search"
	"golang.org/x/sync/singleflight"
)

type versionCacheEntry struct {
	version  string // resolved real version; empty for 404s
	notFound bool
	expiry   time.Time
}

type Server struct {
	db            *db.DB
	voyage        *embeddings.VoyageClient
	batchEmbedder *embeddings.BatchEmbedder
	searcher      *search.Searcher
	cfg           *config.Config
	socketPath    string
	httpServer    *http.Server
	listener      net.Listener

	mu         sync.Mutex
	expTimer   *time.Timer
	expiration time.Duration

	versionCache   map[string]versionCacheEntry
	versionCacheMu sync.RWMutex
	addCrateGroup  singleflight.Group

	crateCache   map[string]*docs.RustdocCrate
	crateCacheMu sync.RWMutex
}

func NewServer(cfg *config.Config, database *db.DB, socketPath string) *Server {
	voyage := embeddings.NewVoyageClient(cfg.VoyageAI.ApiKey.Value)
	batchEmbedder := embeddings.NewBatchEmbedder(voyage, 50, 200*time.Millisecond)
	searcher := search.NewSearcher(database, voyage, cfg.VoyageAI.Model, cfg.VoyageAI.RerankModel)

	expSec := cfg.Daemon.ExpirationSeconds
	if expSec <= 0 {
		expSec = 600
	}

	return &Server{
		db:            database,
		voyage:        voyage,
		batchEmbedder: batchEmbedder,
		searcher:      searcher,
		cfg:           cfg,
		socketPath:    socketPath,
		expiration:    time.Duration(expSec) * time.Second,
		versionCache:  make(map[string]versionCacheEntry),
		crateCache:    make(map[string]*docs.RustdocCrate),
	}
}

func (s *Server) Start(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0755); err != nil {
		return fmt.Errorf("creating socket directory: %w", err)
	}
	os.Remove(s.socketPath)

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listening on socket: %w", err)
	}
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		listener.Close()
		return fmt.Errorf("setting socket permissions: %w", err)
	}
	s.listener = listener

	mux := http.NewServeMux()
	mux.HandleFunc("POST /add-crates", s.withExpReset(s.handleAddCrates))
	mux.HandleFunc("POST /search", s.withExpReset(s.handleSearch))
	mux.HandleFunc("POST /get-doc", s.withExpReset(s.handleGetDoc))
	mux.HandleFunc("GET /status", s.withExpReset(s.handleStatus))
	mux.HandleFunc("POST /search-crates", s.withExpReset(s.handleSearchCrates))
	mux.HandleFunc("POST /clear-cache", s.withExpReset(s.handleClearCache))
	mux.HandleFunc("POST /shutdown", s.handleShutdown)

	s.httpServer = &http.Server{Handler: mux}

	s.mu.Lock()
	s.expTimer = time.AfterFunc(s.expiration, s.expire)
	s.mu.Unlock()

	log.Printf("daemon: listening on %s (expires after %s of inactivity)", s.socketPath, s.expiration)

	if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serving: %w", err)
	}
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	var errs []error
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			log.Printf("daemon: shutdown error: %v", err)
			errs = append(errs, err)
		}
	}
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			log.Printf("daemon: listener close error: %v", err)
			errs = append(errs, err)
		}
	}
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		log.Printf("daemon: socket remove error: %v", err)
		errs = append(errs, err)
	}
	if err := s.db.Close(); err != nil {
		log.Printf("daemon: db close error: %v", err)
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (s *Server) expire() {
	log.Printf("daemon: expiring due to inactivity")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.Stop(ctx)
	os.Exit(0)
}

func (s *Server) resetExpiration() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.expTimer != nil {
		s.expTimer.Stop()
		s.expTimer.Reset(s.expiration)
	}
}

func (s *Server) withExpReset(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.resetExpiration()
		handler(w, r)
	}
}

func (s *Server) handleAddCrates(w http.ResponseWriter, r *http.Request) {
	var req rpc.AddCratesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)

	enc := json.NewEncoder(w)
	send := func(line rpc.ProgressLine) bool {
		log.Printf("daemon: %s", line.Message)
		if err := enc.Encode(line); err != nil {
			log.Printf("daemon: client disconnected: %v", err)
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}
		return true
	}

	for _, spec := range req.Crates {
		progress := func(msg string) {
			send(rpc.ProgressLine{Type: "progress", Message: msg})
		}
		result := s.addCrate(spec, progress)
		if !send(rpc.ProgressLine{Type: "result", Result: &result}) {
			return
		}
	}
}

const versionCacheTTL = 10 * time.Minute

func (s *Server) getCachedVersion(name string) (versionCacheEntry, bool) {
	s.versionCacheMu.RLock()
	defer s.versionCacheMu.RUnlock()
	entry, ok := s.versionCache[name]
	if !ok || time.Now().After(entry.expiry) {
		return versionCacheEntry{}, false
	}
	return entry, true
}

func (s *Server) setCachedVersion(name, version string, notFound bool) {
	s.versionCacheMu.Lock()
	defer s.versionCacheMu.Unlock()
	s.versionCache[name] = versionCacheEntry{
		version:  version,
		notFound: notFound,
		expiry:   time.Now().Add(versionCacheTTL),
	}
}

func (s *Server) clearVersionCache() {
	s.versionCacheMu.Lock()
	defer s.versionCacheMu.Unlock()
	s.versionCache = make(map[string]versionCacheEntry)
}

// getCachedCrate returns a cached RustdocCrate, checking in-memory first then disk.
func (s *Server) getCachedCrate(name, version string) *docs.RustdocCrate {
	key := name + "@" + version
	s.crateCacheMu.RLock()
	c, ok := s.crateCache[key]
	s.crateCacheMu.RUnlock()
	if ok {
		return c
	}

	c, err := docs.LoadCrateCache(name, version)
	if err != nil {
		return nil
	}

	s.crateCacheMu.Lock()
	s.crateCache[key] = c
	s.crateCacheMu.Unlock()
	return c
}

func (s *Server) addCrate(spec rpc.CrateSpec, progress func(string)) rpc.CrateResult {
	version := spec.Version
	if version == "" {
		version = "latest"
	}

	result := rpc.CrateResult{Name: spec.Name, Version: version}

	// Check version cache for "latest" requests
	if version == "latest" {
		if entry, ok := s.getCachedVersion(spec.Name); ok {
			if entry.notFound {
				result.Error = fmt.Sprintf("crate %s not found on docs.rs (cached)", spec.Name)
				return result
			}
			// Use cached real version — check DB
			existing, err := s.db.GetCrate(spec.Name, entry.version)
			if err != nil {
				result.Error = err.Error()
				return result
			}
			if existing != nil && existing.ProcessedAt != nil {
				result.Version = existing.Version
				result.Items, _ = s.db.CountItems(existing.ID)
				return result
			}
		}
	}

	// For "latest", check if we already have any processed version in DB
	if version == "latest" {
		existing, err := s.db.GetLatestCrate(spec.Name)
		if err != nil {
			result.Error = err.Error()
			return result
		}
		if existing != nil {
			result.Version = existing.Version
			result.Items, _ = s.db.CountItems(existing.ID)
			return result
		}
	} else {
		// Exact version: check if already processed
		existing, err := s.db.GetCrate(spec.Name, version)
		if err != nil {
			result.Error = err.Error()
			return result
		}
		if existing != nil && existing.ProcessedAt != nil {
			result.Items, _ = s.db.CountItems(existing.ID)
			return result
		}
	}

	// Singleflight: dedup concurrent fetches for the same crate@version
	key := spec.Name + "@" + version
	v, _, _ := s.addCrateGroup.Do(key, func() (interface{}, error) {
		return s.addCrateWork(spec.Name, version, progress), nil
	})
	return v.(rpc.CrateResult)
}

type embeddable struct {
	contentHash string
	preamble    string
	docLinks    map[string]string // only set for main item docs
}

func (s *Server) addCrateWork(name, version string, progress func(string)) rpc.CrateResult {
	result := rpc.CrateResult{Name: name, Version: version}

	realVersion, rustdocCrate, items, err := s.resolveVersion(name, version, progress)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	// Check if this resolved version is already processed
	if realVersion != version {
		existing, err := s.db.GetCrate(name, realVersion)
		if err != nil {
			result.Error = err.Error()
			return result
		}
		if existing != nil && existing.ProcessedAt != nil {
			result.Version = realVersion
			result.Items, _ = s.db.CountItems(existing.ID)
			s.setCachedVersion(name, realVersion, false)
			return result
		}
	}
	result.Version = realVersion
	s.setCachedVersion(name, realVersion, false)

	crate, err := s.db.UpsertCrate(name, realVersion)
	if err != nil {
		result.Error = fmt.Sprintf("upserting crate: %v", err)
		return result
	}
	s.db.MarkCrateFetched(crate.ID)

	toEmbed, err := s.indexItems(crate, rustdocCrate, items, name, progress)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	if err := s.embedAndBacklink(toEmbed, name, realVersion, progress); err != nil {
		result.Error = err.Error()
		return result
	}

	s.db.MarkCrateProcessed(crate.ID)
	result.Items = len(items)
	progress(fmt.Sprintf("finished indexing %s@%s (%d items)", name, realVersion, len(items)))
	return result
}

// resolveVersion fetches rustdoc JSON, parses it, and resolves "latest" to a real version.
func (s *Server) resolveVersion(name, version string, progress func(string)) (string, *docs.RustdocCrate, []docs.ParsedItem, error) {
	progress(fmt.Sprintf("fetching rustdoc for %s@%s", name, version))
	data, err := docs.FetchRustdocJSON(name, version)
	if err != nil {
		if version == "latest" {
			s.setCachedVersion(name, "", true)
		}
		return "", nil, nil, fmt.Errorf("fetching docs: %w", err)
	}

	progress(fmt.Sprintf("parsing rustdoc for %s@%s", name, version))
	rustdocCrate, items, err := docs.Parse(data, name, version)
	if err != nil {
		return "", nil, nil, fmt.Errorf("parsing docs: %w", err)
	}

	realVersion := version
	if rustdocCrate.CrateVersion != nil && *rustdocCrate.CrateVersion != "" {
		realVersion = *rustdocCrate.CrateVersion
	}

	// Re-parse with real version so generated URIs use it
	if realVersion != version {
		_, items, err = docs.Parse(data, name, realVersion)
		if err != nil {
			return "", nil, nil, fmt.Errorf("parsing docs: %w", err)
		}
	}

	// Cache rustdoc JSON to disk for on-the-fly fragment generation
	if err := docs.SaveCrateCache(data, name, realVersion); err != nil {
		log.Printf("daemon: failed to cache rustdoc JSON for %s@%s: %v", name, realVersion, err)
	}

	return realVersion, rustdocCrate, items, nil
}

// indexItems writes items to CAS and DB, returns embeddables for the embedding phase.
func (s *Server) indexItems(crate *db.Crate, rustdocCrate *docs.RustdocCrate, items []docs.ParsedItem, crateName string, progress func(string)) ([]embeddable, error) {
	progress(fmt.Sprintf("parsed %d items from %s@%s", len(items), crateName, crate.Version))

	s.db.DeleteItemsByCrate(crate.ID)
	s.db.DeleteReexportsByCrate(crate.ID)

	reexports := docs.CollectReexports(rustdocCrate, crateName)
	for _, re := range reexports {
		if err := s.db.InsertReexport(crate.ID, re.LocalPrefix, re.SourceCrate, re.SourcePrefix); err != nil {
			log.Printf("daemon: failed to insert reexport %s → %s/%s: %v", re.LocalPrefix, re.SourceCrate, re.SourcePrefix, err)
		}
	}

	var toEmbed []embeddable
	for _, parsed := range items {
		var contentHash string
		if parsed.Docs != "" {
			h, err := cas.Write(parsed.Docs)
			if err != nil {
				log.Printf("daemon: failed to write CAS for %s: %v", parsed.Path, err)
				continue
			}
			contentHash = h
		}

		var docLinksJSON string
		if len(parsed.DocLinks) > 0 {
			b, _ := json.Marshal(parsed.DocLinks)
			docLinksJSON = string(b)
		}

		var fragNamesJSON string
		if len(parsed.Fragments) > 0 {
			names := make([]string, len(parsed.Fragments))
			for i, f := range parsed.Fragments {
				names[i] = f.Name
			}
			b, _ := json.Marshal(names)
			fragNamesJSON = string(b)
		}

		dbItem := &db.Item{
			CrateID:       crate.ID,
			RustdocID:     parsed.RustdocID,
			Name:          parsed.Name,
			Path:          parsed.Path,
			Kind:          parsed.Kind,
			ContentHash:   contentHash,
			Signature:     parsed.Signature,
			DocLinks:      docLinksJSON,
			FragmentNames: fragNamesJSON,
		}
		if err := s.db.InsertItem(dbItem); err != nil {
			log.Printf("daemon: failed to insert item %s: %v", parsed.Path, err)
			continue
		}

		if contentHash != "" {
			preamble := parsed.Path
			if parsed.Signature != "" {
				preamble += "\n" + parsed.Signature
			}
			toEmbed = append(toEmbed, embeddable{contentHash: contentHash, preamble: preamble, docLinks: parsed.DocLinks})
		}

		for _, frag := range parsed.Fragments {
			if frag.Content == "" {
				continue
			}
			fragHash, err := cas.Write(frag.Content)
			if err != nil {
				log.Printf("daemon: failed to write CAS for %s#%s: %v", parsed.Path, frag.Name, err)
				continue
			}
			toEmbed = append(toEmbed, embeddable{contentHash: fragHash, preamble: parsed.Path + "#" + frag.Name})
		}
	}

	return toEmbed, nil
}

// embedAndBacklink chunks, deduplicates, embeds, and generates backlinks.
func (s *Server) embedAndBacklink(toEmbed []embeddable, name, version string, progress func(string)) error {
	model := s.cfg.VoyageAI.Model
	if model == "" {
		model = "voyage-3.5"
	}

	type chunkMeta struct {
		contentHash string
		chunkIndex  int
		chunkText   string
	}

	needsEmbedding := make(map[string]bool)
	for _, e := range toEmbed {
		if _, seen := needsEmbedding[e.contentHash]; seen {
			continue
		}
		needsEmbedding[e.contentHash] = !s.db.HasEmbeddings(e.contentHash)
	}

	skipped := 0
	for _, needs := range needsEmbedding {
		if !needs {
			skipped++
		}
	}
	if skipped > 0 {
		progress(fmt.Sprintf("%d content hashes already embedded, skipping", skipped))
	}

	var allTexts []string
	var metas []chunkMeta

	for _, e := range toEmbed {
		if !needsEmbedding[e.contentHash] {
			continue
		}
		needsEmbedding[e.contentHash] = false

		docsText, err := cas.Read(e.contentHash)
		if err != nil {
			log.Printf("daemon: failed to read CAS %s: %v", e.contentHash, err)
			continue
		}

		docsText = md.RewriteLinks(docsText, e.docLinks)

		chunks := embeddings.ChunkSections(e.preamble, docsText)
		for _, chunk := range chunks {
			allTexts = append(allTexts, chunk.Text)
			metas = append(metas, chunkMeta{
				contentHash: e.contentHash,
				chunkIndex:  chunk.Index,
				chunkText:   chunk.Text,
			})
		}
	}

	if len(allTexts) == 0 {
		return nil
	}

	progress(fmt.Sprintf("embedding %d chunks for %s@%s", len(allTexts), name, version))
	allEmbeddings, err := s.batchEmbedder.EmbedAll(allTexts, model)
	if err != nil {
		return fmt.Errorf("embedding: %w", err)
	}

	for j, emb := range allEmbeddings {
		meta := metas[j]
		if err := s.db.InsertEmbedding(meta.contentHash, meta.chunkText, meta.chunkIndex, emb); err != nil {
			log.Printf("daemon: failed to store embedding for hash %s chunk %d: %v", meta.contentHash, meta.chunkIndex, err)
		}
	}

	progress(fmt.Sprintf("generating backlinks for %s@%s", name, version))
	seen := make(map[string]bool)
	for j, emb := range allEmbeddings {
		meta := metas[j]
		// Only use chunk 0 (the summary/first section) for backlinks — it's the most
		// representative of the item's overall semantics, avoiding noisy sub-section matches.
		if meta.chunkIndex != 0 || seen[meta.contentHash] {
			continue
		}
		seen[meta.contentHash] = true

		similar, err := s.db.FindSimilarContent(emb, 0.5, 20, meta.contentHash)
		if err != nil {
			log.Printf("daemon: backlink search failed for hash %s: %v", meta.contentHash, err)
			continue
		}

		for _, sim := range similar {
			if err := s.db.UpsertBacklink(meta.contentHash, sim.ContentHash, sim.Similarity); err != nil {
				log.Printf("daemon: failed to store backlink: %v", err)
			}
		}
	}

	return nil
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req rpc.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Threshold <= 0 {
		req.Threshold = 0.3
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}

	results, err := s.searcher.Search(req.Query, req.Crates, req.Threshold, req.Limit, req.RerankInstruction)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, rpc.SearchResponse{Results: results})
}

// resolveOrFetchCrate looks up a crate, resolving "latest" and auto-fetching if needed.
func (s *Server) resolveOrFetchCrate(name, version string) (*db.Crate, error) {
	if version == "latest" || version == "" {
		// Try to find any already-processed version
		existing, err := s.db.GetLatestCrate(name)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	} else {
		existing, err := s.db.GetCrate(name, version)
		if err != nil {
			return nil, err
		}
		if existing != nil && existing.ProcessedAt != nil {
			return existing, nil
		}
	}

	// Not found — auto-fetch
	result := s.addCrate(rpc.CrateSpec{Name: name, Version: version}, func(msg string) {
		log.Printf("auto-fetch: %s", msg)
	})
	if result.Error != "" {
		return nil, fmt.Errorf("%s", result.Error)
	}

	// Retry lookup with the resolved version
	return s.db.GetCrate(name, result.Version)
}

func (s *Server) handleGetDoc(w http.ResponseWriter, r *http.Request) {
	var req rpc.GetDocRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Resolve crate: try exact version, then latest, then auto-fetch
	crate, err := s.resolveOrFetchCrate(req.Crate, req.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if crate == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("crate %s@%s not found", req.Crate, req.Version))
		return
	}

	item, err := s.db.GetItemByPath(crate.ID, req.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// If not found, check re-export mappings and redirect to the source crate
	if item == nil {
		srcCrate, srcPath, found := s.db.ResolveReexport(crate.ID, req.Path)
		if found {
			sourceCrate, err := s.resolveOrFetchCrate(srcCrate, "latest")
			if err != nil {
				log.Printf("daemon: re-export fetch for %s failed: %v", srcCrate, err)
			} else if sourceCrate != nil {
				item, err = s.db.GetItemByPath(sourceCrate.ID, srcPath)
				if err != nil {
					log.Printf("daemon: re-export lookup for %s in %s failed: %v", srcPath, srcCrate, err)
				} else if item != nil {
					crate = sourceCrate
					req.Crate = sourceCrate.Name
					req.Path = srcPath
				}
			}
		}
	}

	if item == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("item %s not found in %s@%s", req.Path, req.Crate, crate.Version))
		return
	}

	// Fragment request: generate on-the-fly from cached rustdoc JSON
	if req.Fragment != "" {
		cachedCrate := s.getCachedCrate(req.Crate, crate.Version)
		if cachedCrate == nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("rustdoc cache not available for %s@%s", req.Crate, crate.Version))
			return
		}
		rustdocItem, ok := cachedCrate.Index[item.RustdocID]
		if !ok {
			writeError(w, http.StatusNotFound, fmt.Sprintf("item %s not found in rustdoc cache", item.RustdocID))
			return
		}
		frags := docs.GenerateFragments(&rustdocItem, cachedCrate, req.Crate, crate.Version)
		var fragContent string
		for _, f := range frags {
			if f.Name == req.Fragment {
				fragContent = f.Content
				break
			}
		}
		if fragContent == "" {
			writeError(w, http.StatusNotFound, fmt.Sprintf("fragment #%s not found for %s", req.Fragment, req.Path))
			return
		}
		writeJSON(w, http.StatusOK, rpc.GetDocResponse{Markdown: fragContent})
		return
	}

	// Full item: build rendered markdown
	var docsText string
	if item.ContentHash != "" {
		docsText, _ = cas.Read(item.ContentHash)
	}

	var docLinks map[string]string
	if item.DocLinks != "" {
		if err := json.Unmarshal([]byte(item.DocLinks), &docLinks); err != nil {
			log.Printf("daemon: failed to unmarshal doc_links for %s: %v", item.Path, err)
		}
	}

	var content strings.Builder
	content.WriteString(fmt.Sprintf("# %s\n\n", item.Path))
	content.WriteString(fmt.Sprintf("**Kind:** %s\n\n", item.Kind))
	if item.Signature != "" {
		content.WriteString(fmt.Sprintf("```rust\n%s\n```\n\n", item.Signature))
	}
	if docsText != "" {
		content.WriteString(md.RewriteLinks(docsText, docLinks))
		content.WriteString("\n")
	}

	text := content.String()

	if item.FragmentNames != "" {
		var fragNames []string
		if json.Unmarshal([]byte(item.FragmentNames), &fragNames) == nil && len(fragNames) > 0 {
			fragURIs := make(map[string]string, len(fragNames))
			for _, name := range fragNames {
				fragURIs[name] = fmt.Sprintf("rsdoc://%s/%s/%s#%s", req.Crate, crate.Version, req.Path, name)
			}
			text = md.AddFrontMatter(text, fragURIs)
		}
	}

	writeJSON(w, http.StatusOK, rpc.GetDocResponse{Markdown: text})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	crates, err := s.db.ListCrates()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var status []rpc.CrateStatus
	for _, c := range crates {
		status = append(status, rpc.CrateStatus{
			Name:      c.Name,
			Version:   c.Version,
			Processed: c.ProcessedAt != nil,
		})
	}

	writeJSON(w, http.StatusOK, rpc.StatusResponse{Crates: status})
}

func (s *Server) handleSearchCrates(w http.ResponseWriter, r *http.Request) {
	var req rpc.SearchCratesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "missing query")
		return
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}

	cratesIO, err := docs.SearchCratesIO(req.Query, req.Limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	names := make([]string, len(cratesIO))
	for i, c := range cratesIO {
		names[i] = c.Name
	}

	indexed, err := s.db.GetIndexedVersions(names)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	results := make([]rpc.CrateSearchResult, len(cratesIO))
	for i, c := range cratesIO {
		results[i] = rpc.CrateSearchResult{
			Name:        c.Name,
			Description: c.Description,
			MaxVersion:  c.MaxVersion,
			Downloads:   c.Downloads,
		}
		if ver, ok := indexed[c.Name]; ok {
			results[i].Semantic = true
			results[i].IndexedVersion = ver
		}
	}

	writeJSON(w, http.StatusOK, rpc.SearchCratesResponse{Results: results})
}

func (s *Server) handleClearCache(w http.ResponseWriter, r *http.Request) {
	s.clearVersionCache()
	log.Printf("daemon: version cache cleared")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "shutting down"})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.Stop(ctx)
		os.Exit(0)
	}()
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
