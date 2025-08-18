// Package czds implements a client to the CZDS REST API using both the documented and undocumented API endpoints.
package czds

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lanrat/czds/jwt"
)

const (
	// AuthURL is the production URL endpoint for authentication
	AuthURL = "https://account-api.icann.org/api/authenticate"
	// BaseURL is the production URL endpoint for the API
	BaseURL = "https://czds-api.icann.org"

	// TestAuthURL is the testing URL endpoint for authentication
	TestAuthURL = "https://account-api-test.icann.org/api/authenticate"
	// TestBaseURL is the testing URL endpoint for the API
	TestBaseURL = "https://czds-api-test.icann.org"

	// Default retry and timing constants
	defaultRetries = 3
	retryDelay     = 10 * time.Second
)

var (
	defaultHTTPClient = createOptimizedHTTPClient()
	// bufferPool provides reusable buffers for file I/O operations to reduce allocations
	bufferPool = sync.Pool{
		New: func() interface{} {
			// 64KB buffer size - good balance between memory usage and I/O efficiency
			buf := make([]byte, 64*1024)
			return &buf
		},
	}
)

// createOptimizedHTTPClient creates an HTTP client optimized for CZDS operations
// with connection pooling and keep-alive settings.
func createOptimizedHTTPClient() *http.Client {
	transport := &http.Transport{
		// Connection pooling settings - optimized for concurrent downloads to same hosts
		IdleConnTimeout: 90 * time.Second, // How long idle connections stay open

		// Compression - ensure gzip is enabled for better transfer efficiency
		DisableCompression: false,
	}

	return &http.Client{
		Transport: transport,
		// No timeout set here - let context control request timeouts
	}
}

// Client stores all session information for czds authentication
// and manages token renewal
type Client struct {
	HTTPClient *http.Client
	AuthURL    string
	BaseURL    string
	auth       authResponse
	authExp    time.Time
	Creds      Credentials
	authMutex  sync.Mutex
	log        Logger
}

// Credentials used by the czds.Client
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// String returns a string representation of credentials with the password redacted for security.
func (c Credentials) String() string {
	return fmt.Sprintf("Credentials{Username: %q, Password: [REDACTED]}", c.Username)
}

// GoString returns a Go-syntax representation with password redacted, used by %#v and %+v formatting.
func (c Credentials) GoString() string {
	return fmt.Sprintf("Credentials{Username: %q, Password: \"[REDACTED]\"}", c.Username)
}

type authResponse struct {
	AccessToken string `json:"accessToken"`
	Message     string `json:"message"`
}

// String returns a string representation of authResponse with the access token redacted for security.
func (a authResponse) String() string {
	return fmt.Sprintf("authResponse{AccessToken: [REDACTED], Message: %q}", a.Message)
}

// GoString returns a Go-syntax representation with access token redacted, used by %#v and %+v formatting.
func (a authResponse) GoString() string {
	return fmt.Sprintf("authResponse{AccessToken: \"[REDACTED]\", Message: %q}", a.Message)
}

type errorResponse struct {
	Message    string `json:"message"`
	HTTPStatus int    `json:"httpStatus"`
}

// NewClient returns a new instance of the CZDS Client with the default production URLs
func NewClient(username, password string) *Client {
	client := &Client{
		AuthURL: AuthURL,
		BaseURL: BaseURL,
		Creds: Credentials{
			Username: username,
			Password: password,
		},
	}
	return client
}

// checkAuth verifies the authentication state and renews the token if necessary.
// It does NOT make network requests if the auth is believed to be valid.
func (c *Client) checkAuth(ctx context.Context) error {
	// uses a mutex to prevent multiple threads from authenticating at the same time
	c.authMutex.Lock()
	defer c.authMutex.Unlock()
	if c.auth.AccessToken == "" {
		// no token yet
		c.v("no auth token")
		return c.AuthenticateWithContext(ctx)
	}
	// Add 30-second buffer to prevent race conditions
	bufferTime := 30 * time.Second
	if time.Now().Add(bufferTime).After(c.authExp) {
		// token expired or expiring soon, renew
		c.v("auth token expired or expiring soon")
		return c.AuthenticateWithContext(ctx)
	}
	return nil
}

// httpClient returns the configured HTTP client for the CZDS client,
// or the default HTTP client if none is configured.
func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return defaultHTTPClient
}

// apiRequest makes an authenticated HTTP request to the CZDS API endpoint with retry logic.
// It handles authentication, retries on failure, and context cancellation.
func (c *Client) apiRequest(ctx context.Context, auth bool, method, url string, request io.Reader) (*http.Response, error) {
	c.v("HTTP API Request: %s %q", method, url)
	if auth {
		err := c.checkAuth(ctx)
		if err != nil {
			return nil, err
		}
	}

	totalTrys := defaultRetries
	var err error
	var req *http.Request
	var resp *http.Response
	for try := 1; try <= totalTrys; try++ {
		// check context
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		// perform http request
		req, err = http.NewRequestWithContext(ctx, method, url, request)
		if err != nil {
			return nil, err
		}
		if request != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")
		if auth {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.auth.AccessToken))
		}

		resp, err = c.httpClient().Do(req)
		if err != nil {
			err = fmt.Errorf("error on request [%d/%d] %s, got error %w: %+v", try, totalTrys, url, err, resp)
			c.v("HTTP API Request error: %s", err)
		} else {
			return resp, nil
		}

		// sleep only if we will try again
		if try < totalTrys {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelay):
			}
		}
	}

	return nil, err
}

// jsonAPI performs an authenticated JSON API request to the CZDS base URL with the given path.
// It marshals the request object to JSON and unmarshals the response.
func (c *Client) jsonAPI(ctx context.Context, method, path string, request, response any) error {
	return c.jsonRequest(ctx, true, method, c.BaseURL+path, request, response)
}

// jsonRequest performs an HTTP request to the specified URL, sending and receiving JSON objects.
// It handles JSON marshaling/unmarshaling and error response processing.
func (c *Client) jsonRequest(ctx context.Context, auth bool, method, url string, request, response any) error {
	var payloadReader io.Reader
	if request != nil {
		jsonPayload, err := json.Marshal(request)
		if err != nil {
			return err
		}
		payloadReader = bytes.NewReader(jsonPayload)
	}

	resp, err := c.apiRequest(ctx, auth, method, url, payloadReader)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.v("Error closing response body: %v", err)
		}
	}()

	// got an error, decode it
	if resp.StatusCode != http.StatusOK {
		var errorResp errorResponse
		err := fmt.Errorf("error on request %q: got Status %s %s", url, resp.Status, http.StatusText(resp.StatusCode))
		if resp.ContentLength != 0 {
			jsonError := json.NewDecoder(resp.Body).Decode(&errorResp)
			if jsonError != nil {
				return fmt.Errorf("error decoding json %w on errored request: %s", jsonError, err.Error())
			}
			err = fmt.Errorf("%w HTTP Status: %d Message: %q", err, errorResp.HTTPStatus, errorResp.Message)
		}
		return err
	}

	if response != nil {
		err = json.NewDecoder(resp.Body).Decode(&response)
		if err != nil {
			return err
		}
	}

	return nil
}

// Authenticate tests the client's credentials and gets an authentication token from the server.
// Calling this is optional. All other functions will check the auth state on their own first and authenticate if necessary.
// This function uses a background context.
//
// Deprecated: Use AuthenticateWithContext for context cancellation support.
func (c *Client) Authenticate() error {
	return c.AuthenticateWithContext(context.Background())
}

// AuthenticateWithContext authenticates the client with CZDS using the provided credentials.
// It obtains and stores an authentication token that will be used for subsequent API calls.
// The operation can be cancelled using the provided context.
func (c *Client) AuthenticateWithContext(ctx context.Context) error {
	c.v("authenticating")
	authResp := authResponse{}
	err := c.jsonRequest(ctx, false, http.MethodPost, c.AuthURL, c.Creds, &authResp)
	if err != nil {
		return err
	}
	c.auth = authResp
	c.authExp, err = authResp.getExpiration()
	if err != nil {
		return err
	}

	if !c.authExp.After(time.Now()) {
		return fmt.Errorf("unable to authenticate")
	}

	return nil
}

// getExpiration extracts the expiration time from the JWT access token.
// It returns the expiration time as a time.Time value.
func (ar *authResponse) getExpiration() (time.Time, error) {
	token, err := jwt.DecodeJWT(ar.AccessToken)
	exp := time.Unix(token.Data.Exp, 0)
	return exp, err
}

// GetZoneRequestID returns the most recent request ID for the given zone.
// It searches through paginated results to find the request for the specified zone name.
//
// Deprecated: Use GetZoneRequestIDWithContext for context cancellation support.
func (c *Client) GetZoneRequestID(zone string) (string, error) {
	return c.GetZoneRequestIDWithContext(context.Background(), zone)
}

// GetZoneRequestIDWithContext retrieves the most recent request ID for the specified zone.
// It searches through paginated results to find the request for the given zone name.
func (c *Client) GetZoneRequestIDWithContext(ctx context.Context, zone string) (string, error) {
	c.v("GetZoneRequestID: %q", zone)
	zone = strings.ToLower(zone)

	// given a RequestsResponse, return the request for the provided zone if found, otherwise nil
	findFirstZoneInRequests := func(zone string, r *RequestsResponse) *Request {
		for _, request := range r.Requests {
			if strings.ToLower(request.TLD) == zone {
				return &request
			}
		}
		return nil
	}

	filter := RequestsFilter{
		Status: RequestAll,
		Filter: zone,
		Pagination: RequestsPagination{
			Size: 100,
			Page: 0,
		},
		Sort: RequestsSort{
			Field:     SortByLastUpdated,
			Direction: SortDesc,
		},
	}

	// get all requests matching filter
	requests, err := c.GetRequestsWithContext(ctx, &filter)
	if err != nil {
		return "", err
	}
	// check if zone in returned requests
	request := findFirstZoneInRequests(zone, requests)
	// if zone is not found in requests, and there are more requests to get, iterate through them
	for request == nil && len(requests.Requests) != 0 {
		// check context
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		filter.Pagination.Page++
		c.v("GetZoneRequestID: zone %q not found yet, requesting page %d", zone, filter.Pagination.Page)
		requests, err = c.GetRequestsWithContext(ctx, &filter)
		if err != nil {
			return "", err
		}
		request = findFirstZoneInRequests(zone, requests)
	}

	if requests.TotalRequests == 0 || request == nil {
		return "", fmt.Errorf("no request found for zone %s", zone)
	}
	return request.RequestID, nil
}

// GetAllRequests returns all zone requests with the specified status.
// Status should be one of the constant czds.Status* strings.
// Warning: for a large number of results, may be slow as it handles pagination automatically.
//
// Deprecated: Use GetAllRequestsWithContext for context cancellation support.
func (c *Client) GetAllRequests(status string) ([]Request, error) {
	return c.GetAllRequestsWithContext(context.Background(), status)
}

// GetAllRequestsWithContext retrieves all zone requests with the specified status.
// It handles pagination automatically to return the complete list of requests.
func (c *Client) GetAllRequestsWithContext(ctx context.Context, status string) ([]Request, error) {
	c.v("GetAllRequests status: %q", status)
	const pageSize = 100
	filter := RequestsFilter{
		Status: status,
		Filter: "",
		Pagination: RequestsPagination{
			Size: pageSize,
			Page: 0,
		},
		Sort: RequestsSort{
			Field:     SortByCreated,
			Direction: SortDesc,
		},
	}

	out := make([]Request, 0, 100)
	c.v("GetAllRequests status: %q, page %d", status, filter.Pagination.Page)
	requests, err := c.GetRequestsWithContext(ctx, &filter)
	if err != nil {
		return out, err
	}

	for len(requests.Requests) != 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		c.v("GetAllRequests status: %q, page %d", status, filter.Pagination.Page)
		out = append(out, requests.Requests...)
		filter.Pagination.Page++
		requests, err = c.GetRequestsWithContext(ctx, &filter)
		if err != nil {
			return out, err
		}
	}

	return out, nil
}
