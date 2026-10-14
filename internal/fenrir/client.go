package fenrir

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	ErrNotFound     = errors.New("resource not found")
	ErrUnauthorized = errors.New("unauthorized by Fenrir Daemon")
)

// Client is an HTTP client for interacting with the Fenrir Daemon API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new Fenrir API client.
func NewClient(baseURL, apiKey string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// SetHTTPClient allows overriding the http.Client (e.g. for testing).
func (c *Client) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// do sends an HTTP request with authentication headers and decodes the response.
func (c *Client) do(ctx context.Context, method, path string, body any, target any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Skoll-Companion/1.0")

	// Set both Bearer token and X-API-Key for maximum compatibility
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fenrir request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read fenrir response body: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrUnauthorized
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBytes, &apiErr) == nil && (apiErr.Error != "" || apiErr.Message != "") {
			errMsg := apiErr.Error
			if errMsg == "" {
				errMsg = apiErr.Message
			}
			return fmt.Errorf("fenrir API error (status %d): %s", resp.StatusCode, errMsg)
		}
		return fmt.Errorf("fenrir API error with status code %d: %s", resp.StatusCode, string(respBytes))
	}

	if target != nil && len(respBytes) > 0 {
		// Fenrir responses can either be {"data": ...} or direct payload
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(respBytes, &envelope); err == nil && len(envelope.Data) > 0 {
			if err := json.Unmarshal(envelope.Data, target); err == nil {
				return nil
			}
		}
		// Direct decode fallback
		if err := json.Unmarshal(respBytes, target); err != nil {
			return fmt.Errorf("failed to decode response payload: %w", err)
		}
	}

	return nil
}

// GetHealth fetches daemon health status.
func (c *Client) GetHealth(ctx context.Context) (*HealthStatus, error) {
	var status HealthStatus
	if err := c.do(ctx, http.MethodGet, "/health", nil, &status); err != nil {
		// Try fallback to /api/v1/health if /health returned error
		if errFallback := c.do(ctx, http.MethodGet, "/api/v1/health", nil, &status); errFallback == nil {
			return &status, nil
		}
		return nil, err
	}
	return &status, nil
}

// ListProjects returns all registered stacks/projects.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var projects []Project
	if err := c.do(ctx, http.MethodGet, "/api/v1/projects", nil, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

// GetProject returns details of a single stack.
func (c *Client) GetProject(ctx context.Context, name string) (*Project, error) {
	var project Project
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s", name), nil, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// GetProjectStatus queries real-time status of containers in a project.
func (c *Client) GetProjectStatus(ctx context.Context, name string) (*Project, error) {
	var project Project
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/status", name), nil, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// RestartProject signals Fenrir to restart containers in a project.
func (c *Client) RestartProject(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/projects/%s/restart", name), nil, nil)
}

// StartProject signals Fenrir to bring up containers in a project.
func (c *Client) StartProject(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/projects/%s/up", name), nil, nil)
}

// StopProject signals Fenrir to stop containers in a project.
func (c *Client) StopProject(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/projects/%s/down", name), nil, nil)
}

// DeployProject creates or updates a Docker Compose stack.
func (c *Client) DeployProject(ctx context.Context, name, composeYAML string) error {
	req := CreateProjectRequest{
		Name:    name,
		Compose: composeYAML,
	}
	return c.do(ctx, http.MethodPost, "/api/v1/projects", req, nil)
}

// GetHostMetrics queries CPU, RAM, and Disk metrics from Fenrir.
func (c *Client) GetHostMetrics(ctx context.Context) (*HostMetrics, error) {
	var metrics HostMetrics
	if err := c.do(ctx, http.MethodGet, "/api/v1/metrics/host", nil, &metrics); err != nil {
		return nil, err
	}
	return &metrics, nil
}
