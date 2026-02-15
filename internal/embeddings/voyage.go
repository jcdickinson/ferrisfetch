package embeddings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type VoyageClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewVoyageClient(apiKey string) *VoyageClient {
	return &VoyageClient{
		apiKey:  apiKey,
		baseURL: "https://api.voyageai.com/v1",
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

type EmbedRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type EmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

func (c *VoyageClient) EmbedTexts(texts []string, model string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("no texts provided")
	}
	if model == "" {
		model = "voyage-3.5"
	}

	reqData := EmbedRequest{Input: texts, Model: model}
	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+"/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("voyage API returned %d: %s", resp.StatusCode, string(body))
	}

	var embedResp EmbedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	embeddings := make([][]float32, len(texts))
	for _, item := range embedResp.Data {
		if item.Index >= len(embeddings) {
			return nil, fmt.Errorf("invalid embedding index: %d", item.Index)
		}
		embeddings[item.Index] = item.Embedding
	}

	return embeddings, nil
}

func (c *VoyageClient) EmbedSingle(text string, model string) ([]float32, error) {
	results, err := c.EmbedTexts([]string{text}, model)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	return results[0], nil
}

type BatchEmbedder struct {
	client    *VoyageClient
	batchSize int
	delay     time.Duration
}

func NewBatchEmbedder(client *VoyageClient, batchSize int, delay time.Duration) *BatchEmbedder {
	if batchSize <= 0 {
		batchSize = 50
	}
	if delay <= 0 {
		delay = 200 * time.Millisecond
	}
	return &BatchEmbedder{client: client, batchSize: batchSize, delay: delay}
}

func (b *BatchEmbedder) EmbedAll(texts []string, model string, progress func(done, total int)) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("no texts provided")
	}

	var all [][]float32
	for i := 0; i < len(texts); i += b.batchSize {
		end := i + b.batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		embeddings, err := b.client.EmbedTexts(batch, model)
		if err != nil {
			return nil, fmt.Errorf("embedding batch at offset %d: %w", i, err)
		}

		all = append(all, embeddings...)

		if progress != nil {
			progress(end, len(texts))
		}

		if end < len(texts) {
			time.Sleep(b.delay)
		}
	}

	return all, nil
}

type RerankRequest struct {
	Query       string   `json:"query"`
	Documents   []string `json:"documents"`
	Model       string   `json:"model"`
	TopK        int      `json:"top_k,omitempty"`
	Instruction string   `json:"instruction,omitempty"`
}

type RerankResponse struct {
	Data []struct {
		Index          int     `json:"index"`
		RelevanceScore float32 `json:"relevance_score"`
	} `json:"data"`
}

type RerankResult struct {
	OriginalIndex  int
	RelevanceScore float32
}

func (c *VoyageClient) Rerank(query string, documents []string, model string, topK int, instruction string) ([]RerankResult, error) {
	if len(documents) == 0 {
		return nil, fmt.Errorf("no documents provided")
	}
	if model == "" {
		model = "rerank-lite-1"
	}

	reqData := RerankRequest{Query: query, Documents: documents, Model: model, TopK: topK, Instruction: instruction}
	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+"/rerank", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("voyage rerank API returned %d: %s", resp.StatusCode, string(body))
	}

	var rerankResp RerankResponse
	if err := json.Unmarshal(body, &rerankResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	var results []RerankResult
	for _, item := range rerankResp.Data {
		results = append(results, RerankResult{
			OriginalIndex:  item.Index,
			RelevanceScore: item.RelevanceScore,
		})
	}

	return results, nil
}
