package fenrir

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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
	return c.doRequest(ctx, method, path, "application/json", bodyReader, target)
}

// doRequest sends an HTTP request with specified Content-Type and body reader.
func (c *Client) doRequest(ctx context.Context, method, path string, contentType string, bodyReader io.Reader, target any) error {
	requestURL := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, requestURL, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	} else {
		req.Header.Set("Content-Type", "application/json")
	}
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

// GetHealth fetches daemon health status from GET /api/v1/health (with /health fallback).
func (c *Client) GetHealth(ctx context.Context) (*HealthStatus, error) {
	var status HealthStatus
	if err := c.do(ctx, http.MethodGet, "/api/v1/health", nil, &status); err != nil {
		// Try fallback to /health if /api/v1/health returned error
		if errFallback := c.do(ctx, http.MethodGet, "/health", nil, &status); errFallback == nil {
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

// GetHost queries host telemetry and hardware metrics from Fenrir via GET /api/v1/host (with /api/v1/metrics/host fallback).
func (c *Client) GetHost(ctx context.Context) (*HostMetrics, error) {
	var metrics HostMetrics
	if err := c.do(ctx, http.MethodGet, "/api/v1/host", nil, &metrics); err != nil {
		// Fallback to /api/v1/metrics/host
		if errFallback := c.do(ctx, http.MethodGet, "/api/v1/metrics/host", nil, &metrics); errFallback == nil {
			return &metrics, nil
		}
		return nil, err
	}
	return &metrics, nil
}

// GetHostMetrics queries CPU, RAM, and Disk metrics from Fenrir (alias for GetHost).
func (c *Client) GetHostMetrics(ctx context.Context) (*HostMetrics, error) {
	return c.GetHost(ctx)
}

// GetProjectCompose retrieves the raw compose definition for a project.
func (c *Client) GetProjectCompose(ctx context.Context, name string) (string, error) {
	// 1. Try GET /api/v1/projects/{name}/compose
	url := c.baseURL + fmt.Sprintf("/api/v1/projects/%s/compose", name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err == nil {
		req.Header.Set("User-Agent", "Skoll-Companion/1.0")
		if c.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
			req.Header.Set("X-API-Key", c.apiKey)
		}
		resp, err := c.httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				bodyBytes, _ := io.ReadAll(resp.Body)
				var payload struct {
					Compose string `json:"compose"`
					Content string `json:"content"`
					Data    any    `json:"data"`
				}
				if json.Unmarshal(bodyBytes, &payload) == nil {
					if payload.Compose != "" {
						return payload.Compose, nil
					}
					if payload.Content != "" {
						return payload.Content, nil
					}
					if strData, ok := payload.Data.(string); ok && strData != "" {
						return strData, nil
					}
					if mapData, ok := payload.Data.(map[string]any); ok {
						if comp, ok := mapData["compose"].(string); ok && comp != "" {
							return comp, nil
						}
					}
				}
				if len(bodyBytes) > 0 {
					return string(bodyBytes), nil
				}
			}
		}
	}

	// 2. Fallback: try GET /api/v1/projects/{name}
	proj, err := c.GetProject(ctx, name)
	if err == nil && proj != nil {
		if proj.Compose != "" {
			return proj.Compose, nil
		}
		if proj.ComposeYAML != "" {
			return proj.ComposeYAML, nil
		}
		// 3. Fallback: Reconstruct YAML from services if available
		if len(proj.Services) > 0 {
			return ReconstructCompose(proj), nil
		}
	}

	return "", fmt.Errorf("compose file not found for project %s", name)
}

// ReconstructCompose generates a fallback compose YAML representation from project services.
func ReconstructCompose(project *Project) string {
	if project == nil || len(project.Services) == 0 {
		return "# No compose configuration or service metadata available."
	}
	var sb strings.Builder
	sb.WriteString("# Compose definition reconstructed from Fenrir service telemetry\n")
	sb.WriteString("version: '3.8'\n\nservices:\n")
	for _, svc := range project.Services {
		sb.WriteString(fmt.Sprintf("  %s:\n", svc.Name))
		if svc.Image != "" {
			sb.WriteString(fmt.Sprintf("    image: %s\n", svc.Image))
		} else {
			sb.WriteString("    image: alpine:latest\n")
		}
		if len(svc.Ports) > 0 {
			sb.WriteString("    ports:\n")
			for _, port := range svc.Ports {
				sb.WriteString(fmt.Sprintf("      - \"%s\"\n", port))
			}
		}
		if svc.State != "" || svc.Status != "" {
			sb.WriteString(fmt.Sprintf("    # State: %s | Status: %s\n", svc.State, svc.Status))
		}
	}
	return sb.String()
}

// UpdateProjectCompose updates a project's compose configuration in Fenrir Daemon via PUT /api/v1/projects/{name}/compose.
// It accepts raw YAML or JSON body with compose, yaml, content, or composeContent,
// and supports query parameters like ?redeploy=false or ?dry_run=true.
func (c *Client) UpdateProjectCompose(ctx context.Context, name string, content string, opts *ComposeUpdateOptions) error {
	path := fmt.Sprintf("/api/v1/projects/%s/compose", url.PathEscape(name))
	queryParams := url.Values{}

	rawYAML := false
	jsonField := "compose"

	if opts != nil {
		if opts.Redeploy != nil {
			queryParams.Set("redeploy", strconv.FormatBool(*opts.Redeploy))
		}
		if opts.DryRun != nil {
			queryParams.Set("dry_run", strconv.FormatBool(*opts.DryRun))
		}
		if opts.RawYAML {
			rawYAML = true
		}
		if opts.JSONField != "" {
			jsonField = opts.JSONField
		}
	}

	if len(queryParams) > 0 {
		path += "?" + queryParams.Encode()
	}

	if rawYAML {
		return c.doRequest(ctx, http.MethodPut, path, "text/yaml", strings.NewReader(content), nil)
	}

	body := map[string]string{
		jsonField: content,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal compose JSON payload: %w", err)
	}
	return c.doRequest(ctx, http.MethodPut, path, "application/json", bytes.NewReader(data), nil)
}

// UpdateProjectYAML updates an auxiliary YAML file for a project in Fenrir Daemon via PUT /api/v1/projects/{name}/yaml/{filename}.
// It supports optional ?restart=true query parameter and raw YAML or JSON body.
func (c *Client) UpdateProjectYAML(ctx context.Context, name, filename string, content string, opts *YAMLUpdateOptions) error {
	path := fmt.Sprintf("/api/v1/projects/%s/yaml/%s", url.PathEscape(name), url.PathEscape(filename))
	queryParams := url.Values{}

	rawYAML := false
	jsonField := "content"

	if opts != nil {
		if opts.Restart != nil {
			queryParams.Set("restart", strconv.FormatBool(*opts.Restart))
		}
		if opts.RawYAML {
			rawYAML = true
		}
		if opts.JSONField != "" {
			jsonField = opts.JSONField
		}
	}

	if len(queryParams) > 0 {
		path += "?" + queryParams.Encode()
	}

	if rawYAML {
		return c.doRequest(ctx, http.MethodPut, path, "text/yaml", strings.NewReader(content), nil)
	}

	body := map[string]string{
		jsonField: content,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal yaml JSON payload: %w", err)
	}
	return c.doRequest(ctx, http.MethodPut, path, "application/json", bytes.NewReader(data), nil)
}

// GetProjectYAML retrieves an auxiliary YAML configuration file for a project via GET /api/v1/projects/{name}/yaml/{filename}.
func (c *Client) GetProjectYAML(ctx context.Context, name, filename string) (string, error) {
	urlPath := fmt.Sprintf("/api/v1/projects/%s/yaml/%s", url.PathEscape(name), url.PathEscape(filename))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+urlPath, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("User-Agent", "Skoll-Companion/1.0")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("X-API-Key", c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch yaml file %s: %w", filename, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrNotFound
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fenrir returned status %d fetching %s", resp.StatusCode, filename)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var payload struct {
		Content string `json:"content"`
		YAML    string `json:"yaml"`
		Compose string `json:"compose"`
		Data    any    `json:"data"`
	}
	if json.Unmarshal(bodyBytes, &payload) == nil {
		if payload.Content != "" {
			return payload.Content, nil
		}
		if payload.YAML != "" {
			return payload.YAML, nil
		}
		if payload.Compose != "" {
			return payload.Compose, nil
		}
		if strData, ok := payload.Data.(string); ok && strData != "" {
			return strData, nil
		}
		if mapData, ok := payload.Data.(map[string]any); ok {
			for _, k := range []string{"content", "yaml", "compose"} {
				if v, ok := mapData[k].(string); ok && v != "" {
					return v, nil
				}
			}
		}
	}
	return string(bodyBytes), nil
}

