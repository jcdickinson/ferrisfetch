package rpc

// AddCratesRequest is the request body for POST /add-crates.
type AddCratesRequest struct {
	Crates []CrateSpec `json:"crates"`
}

type CrateSpec struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// AddCratesResponse is the response body for POST /add-crates.
type AddCratesResponse struct {
	Results []CrateResult `json:"results"`
}

type CrateResult struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Items   int    `json:"items"`
	Error   string `json:"error,omitempty"`
}

// ProgressLine is a single line of NDJSON streamed from the add-crates endpoint.
type ProgressLine struct {
	Type    string       `json:"type"` // "progress" or "result"
	Message string       `json:"message,omitempty"`
	Result  *CrateResult `json:"result,omitempty"`
}

// SearchRequest is the request body for POST /search.
type SearchRequest struct {
	Query             string   `json:"query"`
	Crates            []string `json:"crates,omitempty"`
	Threshold         float32  `json:"threshold,omitempty"`
	Limit             int      `json:"limit,omitempty"`
	RerankInstruction string   `json:"rerank_instruction,omitempty"`
}

// SearchResponse is the response body for POST /search.
type SearchResponse struct {
	Results []DocResult `json:"results"`
}

type DocResult struct {
	URI          string  `json:"uri"`
	CrateName    string  `json:"crate_name"`
	CrateVersion string  `json:"crate_version"`
	Path         string  `json:"path"`
	Kind         string  `json:"kind"`
	Score        float32 `json:"score"`
	Snippet      string  `json:"snippet"`
}

// GetDocRequest is the request body for POST /get-doc.
type GetDocRequest struct {
	Crate    string `json:"crate"`
	Version  string `json:"version"`
	Path     string `json:"path"`
	Fragment string `json:"fragment,omitempty"`
}

// GetDocResponse is the response body for POST /get-doc.
type GetDocResponse struct {
	Markdown string `json:"markdown"`
}

// SearchCratesRequest is the request body for POST /search-crates.
type SearchCratesRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

// SearchCratesResponse is the response body for POST /search-crates.
type SearchCratesResponse struct {
	Results []CrateSearchResult `json:"results"`
}

type CrateSearchResult struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	MaxVersion     string `json:"max_version"`
	Downloads      int    `json:"downloads"`
	Semantic       bool   `json:"semantic"`
	IndexedVersion string `json:"indexed_version,omitempty"`
}

// StatusResponse is the response body for GET /status.
type StatusResponse struct {
	Crates []CrateStatus `json:"crates"`
}

type CrateStatus struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Processed bool   `json:"processed"`
}
