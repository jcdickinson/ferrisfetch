package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/jcdickinson/ferrisfetch/internal/rpc"
)

type Client struct {
	socketPath string
	httpClient *http.Client
}

func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
			Timeout: 5 * time.Minute, // add_crates can be slow
		},
	}
}

// ConnectOrSpawn tries to connect to the daemon, spawning it if necessary.
func ConnectOrSpawn(socketPath string) (*Client, error) {
	client := NewClient(socketPath)

	if client.IsAvailable() {
		return client, nil
	}

	if err := Spawn(); err != nil {
		return nil, fmt.Errorf("spawning daemon: %w", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		if client.IsAvailable() {
			return client, nil
		}
	}

	return nil, fmt.Errorf("daemon did not start within 5 seconds")
}

func (c *Client) IsAvailable() bool {
	conn, err := net.DialTimeout("unix", c.socketPath, 100*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (c *Client) AddCrates(ctx context.Context, crates []rpc.CrateSpec, onProgress func(string)) (*rpc.AddCratesResponse, error) {
	jsonData, err := json.Marshal(rpc.AddCratesRequest{Crates: crates})
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "http://unix/add-crates", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("daemon returned %d: %s", resp.StatusCode, string(body))
	}

	var result rpc.AddCratesResponse
	dec := json.NewDecoder(resp.Body)
	for dec.More() {
		var line rpc.ProgressLine
		if err := dec.Decode(&line); err != nil {
			return nil, fmt.Errorf("decoding progress: %w", err)
		}
		switch line.Type {
		case "progress":
			if onProgress != nil {
				onProgress(line.Message)
			}
		case "result":
			if line.Result != nil {
				result.Results = append(result.Results, *line.Result)
			}
		}
	}

	return &result, nil
}

func (c *Client) Search(ctx context.Context, req rpc.SearchRequest) (*rpc.SearchResponse, error) {
	var resp rpc.SearchResponse
	err := c.post(ctx, "/search", req, &resp)
	return &resp, err
}

func (c *Client) GetDoc(ctx context.Context, req rpc.GetDocRequest) (*rpc.GetDocResponse, error) {
	var resp rpc.GetDocResponse
	err := c.post(ctx, "/get-doc", req, &resp)
	return &resp, err
}

func (c *Client) Status(ctx context.Context) (*rpc.StatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://unix/status", nil)
	if err != nil {
		return nil, err
	}
	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("status request: %w", err)
	}
	defer httpResp.Body.Close()

	var resp rpc.StatusResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decoding status: %w", err)
	}
	return &resp, nil
}

func (c *Client) SearchCrates(ctx context.Context, req rpc.SearchCratesRequest) (*rpc.SearchCratesResponse, error) {
	var resp rpc.SearchCratesResponse
	err := c.post(ctx, "/search-crates", req, &resp)
	return &resp, err
}

func (c *Client) ClearCache(ctx context.Context) error {
	var resp map[string]string
	return c.post(ctx, "/clear-cache", nil, &resp)
}

func (c *Client) Shutdown(ctx context.Context) error {
	var resp map[string]string
	return c.post(ctx, "/shutdown", nil, &resp)
}

func (c *Client) post(ctx context.Context, path string, body, result interface{}) error {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "http://unix"+path, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned %d: %s", resp.StatusCode, string(respBody))
	}

	if err := json.Unmarshal(respBody, result); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}
