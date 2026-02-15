package mcp

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jcdickinson/ferrisfetch/internal/daemon"
	"github.com/jcdickinson/ferrisfetch/internal/rpc"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

//go:embed instructions.md
var instructions string

type Server struct {
	mcpServer *server.MCPServer
	client    *daemon.Client
}

func NewServer(socketPath string) (*Server, error) {
	client, err := daemon.ConnectOrSpawn(socketPath)
	if err != nil {
		return nil, fmt.Errorf("connecting to daemon: %w", err)
	}

	s := &Server{client: client}

	mcpServer := server.NewMCPServer(
		"ferrisfetch",
		"0.1.0",
		server.WithInstructions(instructions),
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, false),
	)

	s.registerTools(mcpServer)
	s.registerResources(mcpServer)

	s.mcpServer = mcpServer
	return s, nil
}

func (s *Server) registerTools(mcpServer *server.MCPServer) {
	mcpServer.AddTool(
		mcp.NewTool("add_crates",
			mcp.WithDescription("Fetch, parse, embed, and index Rust crate documentation from docs.rs. Synchronous — returns when complete. Version defaults to \"latest\"."),
			addCratesSchema,
		),
		s.handleAddCrates,
	)

	mcpServer.AddTool(
		mcp.NewTool("search_crates",
			mcp.WithDescription("Search crates.io for Rust crates by name or keyword. Results indicate which crates are already indexed locally for semantic search."),
			mcp.WithString("query",
				mcp.Description("Search query (crate name or keyword)"),
				mcp.Required(),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum number of results (default 20)"),
			),
		),
		s.handleSearchCrates,
	)

	mcpServer.AddTool(
		mcp.NewTool("search_docs",
			mcp.WithDescription("Semantic search across indexed Rust crate documentation. Returns URIs that can be read as resources. Use `crates` to filter to specific crates; omit to search all indexed crates."),
			mcp.WithString("query",
				mcp.Description("Natural language search query"),
				mcp.Required(),
			),
			mcp.WithArray("crates",
				mcp.Description("Optional list of crate names to search within"),
				mcp.Items(map[string]interface{}{"type": "string"}),
			),
			mcp.WithNumber("threshold",
				mcp.Description("Minimum similarity threshold (default 0.3)"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum number of results (default 20)"),
			),
			mcp.WithString("rerank_instruction",
				mcp.Description("Optional instruction for the reranker to guide relevance scoring"),
			),
		),
		s.handleSearchDocs,
	)
}

func addCratesSchema(t *mcp.Tool) {
	t.InputSchema.Required = append(t.InputSchema.Required, "crates")
	t.InputSchema.Properties["crates"] = map[string]any{
		"type":        "array",
		"description": "List of crates to index",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Crate name (e.g., \"serde\")",
				},
				"version": map[string]any{
					"type":        "string",
					"description": "Version (default: \"latest\")",
				},
			},
			"required": []string{"name"},
		},
	}
}

func (s *Server) registerResources(mcpServer *server.MCPServer) {
	mcpServer.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"rsdoc://{crate}/{version}/{path}",
			"Rust documentation item",
			mcp.WithTemplateDescription("Read a specific Rust documentation item. Search results return these URIs."),
			mcp.WithTemplateMIMEType("text/markdown"),
		),
		s.handleReadResource,
	)
}

func (s *Server) handleAddCrates(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	cratesRaw, ok := args["crates"]
	if !ok {
		return mcp.NewToolResultError("missing required parameter: crates"), nil
	}

	cratesJSON, err := json.Marshal(cratesRaw)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid crates parameter: %v", err)), nil
	}

	var specs []rpc.CrateSpec
	if err := json.Unmarshal(cratesJSON, &specs); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid crates format: %v", err)), nil
	}

	resp, err := s.client.AddCrates(ctx, specs, nil)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to add crates: %v", err)), nil
	}

	resultJSON, _ := json.MarshalIndent(resp.Results, "", "  ")
	return mcp.NewToolResultText(string(resultJSON)), nil
}

func (s *Server) handleSearchDocs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	query, _ := args["query"].(string)
	if query == "" {
		return mcp.NewToolResultError("missing required parameter: query"), nil
	}

	var searchReq rpc.SearchRequest
	searchReq.Query = query

	if cratesRaw, ok := args["crates"]; ok {
		cratesJSON, _ := json.Marshal(cratesRaw)
		json.Unmarshal(cratesJSON, &searchReq.Crates)
	}

	if threshold, ok := args["threshold"].(float64); ok {
		searchReq.Threshold = float32(threshold)
	}
	if limit, ok := args["limit"].(float64); ok {
		searchReq.Limit = int(limit)
	}
	if instruction, ok := args["rerank_instruction"].(string); ok {
		searchReq.RerankInstruction = instruction
	}

	resp, err := s.client.Search(ctx, searchReq)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
	}

	resultJSON, _ := json.MarshalIndent(resp.Results, "", "  ")
	return mcp.NewToolResultText(string(resultJSON)), nil
}

func (s *Server) handleSearchCrates(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	query, _ := args["query"].(string)
	if query == "" {
		return mcp.NewToolResultError("missing required parameter: query"), nil
	}

	var searchReq rpc.SearchCratesRequest
	searchReq.Query = query
	if limit, ok := args["limit"].(float64); ok {
		searchReq.Limit = int(limit)
	}

	resp, err := s.client.SearchCrates(ctx, searchReq)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
	}

	resultJSON, _ := json.MarshalIndent(resp.Results, "", "  ")
	return mcp.NewToolResultText(string(resultJSON)), nil
}

func (s *Server) handleReadResource(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	uri := req.Params.URI
	trimmed := strings.TrimPrefix(uri, "rsdoc://")
	parts := strings.SplitN(trimmed, "/", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid resource URI: %s", uri)
	}

	path := parts[2]
	var fragment string
	if idx := strings.LastIndex(path, "#"); idx >= 0 {
		fragment = path[idx+1:]
		path = path[:idx]
	}

	resp, err := s.client.GetDoc(ctx, rpc.GetDocRequest{
		Crate:    parts[0],
		Version:  parts[1],
		Path:     path,
		Fragment: fragment,
	})
	if err != nil {
		return nil, fmt.Errorf("getting doc: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      uri,
			MIMEType: "text/markdown",
			Text:     resp.Markdown,
		},
	}, nil
}

func (s *Server) Run() error {
	return server.ServeStdio(s.mcpServer)
}

func (s *Server) Shutdown(_ context.Context) error {
	return nil
}

