package search

import (
	"encoding/json"
	"fmt"
	"log"

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

// Search performs vector search with reranking.
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

	candidates, err := s.db.VectorSearch(queryEmb, threshold, limit*3, crateIDs)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	log.Printf("search: vector search returned %d candidates", len(candidates))
	if len(candidates) == 0 {
		return nil, nil
	}

	// Resolve representative items for each candidate.
	type resolvedItem struct {
		item  *db.Item
		score float32
	}
	var resolved []resolvedItem
	var documents []string
	for _, c := range candidates {
		item, err := s.db.GetItemForHash(c.ContentHash, crateIDs)
		if err != nil || item == nil {
			continue
		}
		doc := item.Path
		if item.Signature != "" {
			doc += "\n" + item.Signature
		}
		if docsText, err := cas.Read(c.ContentHash); err == nil {
			d := docsText
			if len(d) > 500 {
				d = d[:500]
			}
			doc += "\n" + d
		}
		resolved = append(resolved, resolvedItem{item: item, score: c.Similarity})
		documents = append(documents, doc)
	}

	if len(resolved) == 0 {
		return nil, nil
	}

	// Batch-fetch crates for all resolved items.
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
