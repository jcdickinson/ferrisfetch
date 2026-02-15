package search

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"

	"github.com/jcdickinson/ferrisfetch/internal/cas"
	"github.com/jcdickinson/ferrisfetch/internal/db"
	"github.com/jcdickinson/ferrisfetch/internal/embeddings"
	md "github.com/jcdickinson/ferrisfetch/internal/markdown"
	"github.com/jcdickinson/ferrisfetch/internal/rpc"
)

type Searcher struct {
	db          *db.DB
	voyage      *embeddings.VoyageClient
	model       string
	rerankModel string
}

func NewSearcher(database *db.DB, voyage *embeddings.VoyageClient, model, rerankModel string) *Searcher {
	if model == "" {
		model = "voyage-3.5"
	}
	if rerankModel == "" {
		rerankModel = "rerank-lite-1"
	}
	return &Searcher{db: database, voyage: voyage, model: model, rerankModel: rerankModel}
}

const (
	defaultDecay          = 0.3
	backlinkMinSimilarity = 0.3
	bfsMaxDepth           = 3
	bfsMaxQueueSize       = 500
)

// Search performs vector search with backlink traversal and reranking.
// Operates on content hashes to deduplicate across crate versions.
func (s *Searcher) Search(query string, crateNames []string, threshold float32, limit int, rerankInstruction string) ([]rpc.DocResult, error) {
	log.Printf("search: query=%q threshold=%.2f limit=%d crates=%v model=%s", query, threshold, limit, crateNames, s.model)

	queryEmb, err := s.voyage.EmbedSingle(query, s.model)
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}
	log.Printf("search: query embedded, dimension=%d", len(queryEmb))

	var crateIDs []int
	if len(crateNames) > 0 {
		crateIDs, err = s.db.GetCrateIDsByNames(crateNames)
		if err != nil {
			return nil, fmt.Errorf("resolving crate names: %w", err)
		}
		log.Printf("search: resolved crate names %v -> IDs %v", crateNames, crateIDs)
	}

	directResults, err := s.db.VectorSearch(queryEmb, threshold, limit*3, crateIDs)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	log.Printf("search: vector search returned %d direct results", len(directResults))

	// Build candidate set: content_hash -> best score
	candidates := make(map[string]float32)
	for _, r := range directResults {
		candidates[r.ContentHash] = r.Similarity
	}

	// BFS through backlinks with dampened propagation
	visited := make(map[string]bool)
	type bfsEntry struct {
		contentHash string
		score       float32
		depth       int
	}

	var queue []bfsEntry
	for _, r := range directResults {
		queue = append(queue, bfsEntry{contentHash: r.ContentHash, score: r.Similarity, depth: 0})
		visited[r.ContentHash] = true
	}

	for len(queue) > 0 {
		entry := queue[0]
		queue = queue[1:]

		if entry.depth >= bfsMaxDepth {
			continue
		}

		backlinks, err := s.db.GetBacklinks(entry.contentHash, backlinkMinSimilarity)
		if err != nil {
			log.Printf("search: GetBacklinks failed for %s: %v", entry.contentHash, err)
			continue
		}

		for _, bl := range backlinks {
			propagated := entry.score * (defaultDecay + (1-defaultDecay)*bl.Similarity)
			if propagated <= threshold {
				continue
			}

			if existing, ok := candidates[bl.ContentHash]; !ok || propagated > existing {
				candidates[bl.ContentHash] = propagated
			}

			if !visited[bl.ContentHash] && len(queue) < bfsMaxQueueSize {
				visited[bl.ContentHash] = true
				queue = append(queue, bfsEntry{contentHash: bl.ContentHash, score: propagated, depth: entry.depth + 1})
			}
		}
	}

	log.Printf("search: after BFS, %d total candidates", len(candidates))

	type candidate struct {
		contentHash string
		score       float32
	}
	var sorted []candidate
	for hash, score := range candidates {
		sorted = append(sorted, candidate{hash, score})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].score > sorted[j].score })

	if len(sorted) > limit*3 {
		sorted = sorted[:limit*3]
	}

	log.Printf("search: %d candidates after sorting/truncation", len(sorted))
	if len(sorted) == 0 {
		return nil, nil
	}

	// Resolve representative items, filtering out nil results
	type resolvedItem struct {
		item  *db.Item
		score float32 // original candidate score
	}
	var resolved []resolvedItem
	var documents []string
	for _, c := range sorted {
		item, err := s.db.GetItemForHash(c.contentHash, crateIDs)
		if err != nil || item == nil {
			continue
		}
		doc := item.Path
		if item.Signature != "" {
			doc += "\n" + item.Signature
		}
		if docsText, err := cas.Read(c.contentHash); err == nil {
			d := docsText
			if len(d) > 500 {
				d = d[:500]
			}
			doc += "\n" + d
		}
		resolved = append(resolved, resolvedItem{item: item, score: c.score})
		documents = append(documents, doc)
	}

	if len(resolved) == 0 {
		return nil, nil
	}

	// Batch-fetch crates for all resolved items
	itemIDs := make([]int, len(resolved))
	for i, r := range resolved {
		itemIDs[i] = r.item.ID
	}
	crateMap, err := s.db.GetCratesForItems(itemIDs)
	if err != nil {
		log.Printf("search: batch crate lookup failed: %v", err)
		crateMap = nil
	}

	buildResult := func(item *db.Item, score float32) rpc.DocResult {
		crateName, crateVersion := "", ""
		if c := crateMap[item.ID]; c != nil {
			crateName = c.Name
			crateVersion = c.Version
		}
		return rpc.DocResult{
			URI:          fmt.Sprintf("rsdoc://%s/%s/%s", crateName, crateVersion, item.Path),
			CrateName:    crateName,
			CrateVersion: crateVersion,
			Path:         item.Path,
			Kind:         item.Kind,
			Score:        score,
			Snippet:      snippetForItem(item),
		}
	}

	reranked, err := s.voyage.Rerank(query, documents, s.rerankModel, limit, rerankInstruction)
	if err != nil {
		log.Printf("search: reranking failed (falling back to vector scores): %v", err)
		reranked = nil
	} else {
		log.Printf("search: reranking returned %d results", len(reranked))
	}

	var results []rpc.DocResult
	if len(reranked) > 0 {
		for _, rr := range reranked {
			if rr.OriginalIndex >= len(resolved) {
				continue
			}
			r := resolved[rr.OriginalIndex]
			results = append(results, buildResult(r.item, rr.RelevanceScore))
		}
	} else {
		for i, r := range resolved {
			if i >= limit {
				break
			}
			results = append(results, buildResult(r.item, r.score))
		}
	}

	return results, nil
}

func snippetForItem(item *db.Item) string {
	if item.ContentHash == "" {
		return ""
	}
	docsText, err := cas.Read(item.ContentHash)
	if err != nil {
		return ""
	}
	docsText = rewriteItemLinks(docsText, item.DocLinks)
	return truncate(docsText, 200)
}

func rewriteItemLinks(text, docLinksJSON string) string {
	if docLinksJSON == "" {
		return text
	}
	var linkMap map[string]string
	if err := json.Unmarshal([]byte(docLinksJSON), &linkMap); err != nil {
		return text
	}
	return md.RewriteLinks(text, linkMap)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
