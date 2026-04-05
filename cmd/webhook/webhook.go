// Package webhook provides a generic HTTP webhook client for batch download approval and notifications.
//
// The client sends POST requests with JSON payloads for both pre-download batch checks and
// post-download notifications. Network errors, rate limits (429), service unavailable (503),
// and server errors (5XX) are automatically retried with exponential backoff.
//
// Basic usage:
//
//	client, _ := webhook.NewFromEnv()
//	if client != nil {
//	    approved, _ := client.BatchPreDownloadCheck(ctx, []string{"com", "net"}, "2025-11-24")
//	    // ... download approved zones ...
//	    zones := []webhook.BatchAddZone{{Zone: "com", FilePath: "/path/to/com.zone.gz"}}
//	    client.BatchPostDownloadNotify(ctx, zones, "2025-11-24")
//	}
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	// DefaultMaxRetries is the default maximum number of retry attempts for transient failures
	DefaultMaxRetries = 3
	// DefaultRetryDelay is the default base delay between retry attempts
	DefaultRetryDelay = 1 * time.Second
	// DefaultTimeout is the default HTTP request timeout
	DefaultTimeout = 30 * time.Second
)

// Logger is an interface for logging webhook operations.
// It matches the standard library's log.Logger interface.
type Logger interface {
	Printf(format string, v ...any)
}

// Client provides HTTP webhook functionality for batch download approval and notifications.
// BatchPreDownloadCheck and BatchPostDownloadNotify are safe for concurrent use.
// Configuration methods (SetHeader, SetRetries, SetTimeout, SetLogger) should not be
// called concurrently with request methods or each other.
type Client struct {
	precheckURL string
	notifyURL   string
	headers     map[string]string
	client      *http.Client
	maxRetries  int
	retryDelay  time.Duration
	logger      Logger
}

// BatchCheckRequest is the request body for POST batch pre-download check.
type BatchCheckRequest struct {
	Date  string   `json:"date"`
	Zones []string `json:"zones"`
}

// BatchCheckResult represents a single zone's check result.
type BatchCheckResult struct {
	Zone           string `json:"zone"`
	ShouldDownload bool   `json:"should_download"`
	Reason         string `json:"reason"`
}

// BatchCheckResponse is the response from POST batch pre-download check.
type BatchCheckResponse struct {
	Date    string             `json:"date"`
	Results []BatchCheckResult `json:"results"`
	Summary struct {
		Total          int `json:"total"`
		ShouldDownload int `json:"should_download"`
		Skip           int `json:"skip"`
	} `json:"summary"`
}

// BatchAddZone represents a single zone to add.
type BatchAddZone struct {
	Zone     string `json:"zone"`
	FilePath string `json:"file_path"`
}

// BatchAddRequest is the request body for POST batch add import.
type BatchAddRequest struct {
	Date  string         `json:"date"`
	Zones []BatchAddZone `json:"zones"`
}

// BatchAddResult represents a single zone's add result.
type BatchAddResult struct {
	Zone        string `json:"zone"`
	Status      string `json:"status"` // added, exists, deleted, rejected, error
	ImportID    int    `json:"import_id,omitempty"`
	Message     string `json:"message,omitempty"`
	FileDeleted bool   `json:"file_deleted,omitempty"`
}

// BatchAddResponse is the response from POST batch add import.
type BatchAddResponse struct {
	Date    string           `json:"date"`
	Source  string           `json:"source"`
	Results []BatchAddResult `json:"results"`
	Summary struct {
		Total    int `json:"total"`
		Added    int `json:"added"`
		Exists   int `json:"exists"`
		Rejected int `json:"rejected"`
		Deleted  int `json:"deleted"`
		Errors   int `json:"errors"`
	} `json:"summary"`
}

// New creates a new webhook client with separate URLs for precheck and notification.
// Either URL can be empty to disable that functionality.
// The client uses 30-second timeouts and retries up to 3 times with exponential backoff.
// Use SetRetries, SetTimeout, and SetHeader to customize behavior.
func New(precheckURL, notifyURL string) (*Client, error) {
	// Validate URLs if provided
	if precheckURL != "" {
		if _, err := url.Parse(precheckURL); err != nil {
			return nil, fmt.Errorf("invalid precheck URL: %w", err)
		}
	}
	if notifyURL != "" {
		if _, err := url.Parse(notifyURL); err != nil {
			return nil, fmt.Errorf("invalid notification URL: %w", err)
		}
	}

	return &Client{
		precheckURL: precheckURL,
		notifyURL:   notifyURL,
		headers:     make(map[string]string),
		maxRetries:  DefaultMaxRetries,
		retryDelay:  DefaultRetryDelay,
		client: &http.Client{
			Timeout: DefaultTimeout,
		},
	}, nil
}

// NewFromEnv creates a webhook client from environment variables.
// Returns a single client configured with both URLs.
// If both env vars are unset, returns (nil, nil).
func NewFromEnv() (*Client, error) {
	precheckURL := os.Getenv("PRECHECK_WEBHOOK_URL")
	notifyURL := os.Getenv("NOTIFICATION_WEBHOOK_URL")

	// If both are empty, return nil client
	if precheckURL == "" && notifyURL == "" {
		return nil, nil
	}

	client, err := New(precheckURL, notifyURL)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// SetHeader sets a custom header included in all requests.
func (c *Client) SetHeader(key, value string) {
	c.headers[key] = value
}

// SetRetries configures retry behavior. The retryDelay increases exponentially with each attempt.
func (c *Client) SetRetries(maxRetries int, retryDelay time.Duration) {
	if maxRetries < 1 {
		maxRetries = 1
	}
	c.maxRetries = maxRetries
	c.retryDelay = retryDelay
}

// SetTimeout sets the HTTP request timeout for each attempt.
func (c *Client) SetTimeout(timeout time.Duration) {
	c.client.Timeout = timeout
}

// SetLogger enables logging of retries and errors.
func (c *Client) SetLogger(logger Logger) {
	c.logger = logger
}

// PrecheckEnabled returns true if the precheck webhook is configured.
func (c *Client) PrecheckEnabled() bool {
	return c != nil && c.precheckURL != ""
}

// NotifyEnabled returns true if the notification webhook is configured.
func (c *Client) NotifyEnabled() bool {
	return c != nil && c.notifyURL != ""
}

// PreDownloadCheck checks if a single zone should be downloaded.
// Returns true if download is approved, false otherwise.
// This is a convenience wrapper around BatchPreDownloadCheck.
func (c *Client) PreDownloadCheck(ctx context.Context, zone, date string) (bool, error) {
	approved, err := c.BatchPreDownloadCheck(ctx, []string{zone}, date)
	if err != nil {
		return false, err
	}
	return approved[zone], nil
}

// BatchPreDownloadCheck sends POST request with ALL zones in a single batch.
// Returns map[zoneName]bool indicating which zones should be downloaded.
// Returns error only on request failure. If request succeeds but server returns
// should_download=false for some zones, those are marked false in map.
func (c *Client) BatchPreDownloadCheck(ctx context.Context, zones []string, date string) (map[string]bool, error) {
	if !c.PrecheckEnabled() {
		// Webhook disabled - approve all zones
		result := make(map[string]bool, len(zones))
		for _, zone := range zones {
			result[zone] = true
		}
		return result, nil
	}

	// Create request payload
	request := BatchCheckRequest{
		Date:  date,
		Zones: zones,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshaling batch check request: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= c.maxRetries; attempt++ {
		// Check context cancellation before each attempt
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.precheckURL, bytes.NewReader(jsonData))
		if err != nil {
			return nil, fmt.Errorf("creating batch pre-download request: %w", err)
		}

		// Add required headers
		c.setHeaders(req)

		resp, err := c.client.Do(req)
		if err != nil {
			// Network error - retry
			lastErr = fmt.Errorf("batch pre-download webhook request failed: %w", err)
			if c.logger != nil {
				c.logger.Printf("[webhook] Batch pre-download check failed (attempt %d/%d): %v", attempt, c.maxRetries, err)
			}
			if attempt < c.maxRetries {
				if err := c.sleep(ctx, c.retryDelay*time.Duration(attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, lastErr
		}

		// Read response body
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("reading batch pre-download response: %w", err)
			if attempt < c.maxRetries {
				if err := c.sleep(ctx, c.retryDelay*time.Duration(attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, lastErr
		}

		// Success: 200 OK
		if resp.StatusCode == http.StatusOK {
			var response BatchCheckResponse
			if err := json.Unmarshal(body, &response); err != nil {
				return nil, fmt.Errorf("unmarshaling batch check response: %w", err)
			}

			// Build result map
			result := make(map[string]bool, len(response.Results))
			for _, r := range response.Results {
				result[r.Zone] = r.ShouldDownload
			}

			if c.logger != nil && attempt > 1 {
				c.logger.Printf("[webhook] Batch pre-download check succeeded on attempt %d", attempt)
			}

			return result, nil
		}

		// Retryable errors: 429 Too Many Requests, 503 Service Unavailable, 5XX server errors
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("webhook temporary error with status %d", resp.StatusCode)
			if c.logger != nil {
				c.logger.Printf("[webhook] Temporary error %d (attempt %d/%d)", resp.StatusCode, attempt, c.maxRetries)
			}
			if attempt < c.maxRetries {
				if err := c.sleep(ctx, c.retryDelay*time.Duration(attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, lastErr
		}

		// All other status codes (4XX, 3XX, etc.) - do NOT retry
		return nil, fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil, lastErr
}

// PostDownloadNotify sends a notification for a single zone download.
// Returns the result for the zone or an error if the request failed.
// This is a convenience wrapper around BatchPostDownloadNotify.
func (c *Client) PostDownloadNotify(ctx context.Context, zone, filePath, date string) (*BatchAddResult, error) {
	results, err := c.BatchPostDownloadNotify(ctx, []BatchAddZone{{Zone: zone, FilePath: filePath}}, date)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}

// BatchPostDownloadNotify sends POST request with one or more zones.
// Returns slice of results for each zone. Logs errors but doesn't block.
func (c *Client) BatchPostDownloadNotify(ctx context.Context, zones []BatchAddZone, date string) ([]BatchAddResult, error) {
	if !c.NotifyEnabled() {
		return nil, nil // Webhook disabled
	}

	// Create request payload
	request := BatchAddRequest{
		Date:  date,
		Zones: zones,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshaling batch add request: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= c.maxRetries; attempt++ {
		// Check context cancellation before each attempt
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.notifyURL, bytes.NewReader(jsonData))
		if err != nil {
			return nil, fmt.Errorf("creating batch post-download request: %w", err)
		}

		// Add required headers
		c.setHeaders(req)

		resp, err := c.client.Do(req)
		if err != nil {
			// Network error - retry
			lastErr = fmt.Errorf("batch post-download webhook request failed: %w", err)
			if c.logger != nil {
				c.logger.Printf("[webhook] Batch post-download notification failed (attempt %d/%d): %v", attempt, c.maxRetries, err)
			}
			if attempt < c.maxRetries {
				if err := c.sleep(ctx, c.retryDelay*time.Duration(attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, lastErr
		}

		// Read response body
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("reading batch post-download response: %w", err)
			if attempt < c.maxRetries {
				if err := c.sleep(ctx, c.retryDelay*time.Duration(attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, lastErr
		}

		// Success: 200 OK
		if resp.StatusCode == http.StatusOK {
			var response BatchAddResponse
			if err := json.Unmarshal(body, &response); err != nil {
				return nil, fmt.Errorf("unmarshaling batch add response: %w", err)
			}

			if c.logger != nil && attempt > 1 {
				c.logger.Printf("[webhook] Batch post-download notification succeeded on attempt %d", attempt)
			}

			return response.Results, nil
		}

		// Retryable errors: 429 Too Many Requests, 503 Service Unavailable, 5XX server errors
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("webhook temporary error with status %d", resp.StatusCode)
			if c.logger != nil {
				c.logger.Printf("[webhook] Temporary error %d (attempt %d/%d)", resp.StatusCode, attempt, c.maxRetries)
			}
			if attempt < c.maxRetries {
				if err := c.sleep(ctx, c.retryDelay*time.Duration(attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, lastErr
		}

		// All other status codes (4XX, 3XX, etc.) - do NOT retry
		return nil, fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil, lastErr
}

// setHeaders adds custom headers to the request.
// Content-Type is automatically set to application/json for POST requests.
func (c *Client) setHeaders(req *http.Request) {
	// Set custom headers
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	// Always set Content-Type for JSON requests
	if req.Method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
}

// sleep sleeps for the specified duration while respecting context cancellation.
// Returns ctx.Err() if the context is cancelled during sleep, nil otherwise.
func (c *Client) sleep(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
		return nil
	}
}
