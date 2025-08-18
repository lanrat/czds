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
)

var (
	defaultHTTPClient = &http.Client{}
)

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

type authResponse struct {
	AccessToken string `json:"accessToken"`
	Message     string `json:"message"`
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

// checkAuth does NOT make network requests if the auth is believed to be valid
func (c *Client) checkAuth(ctx context.Context) error {
	// uses a mutex to prevent multiple threads from authenticating at the same time
	c.authMutex.Lock()
	defer c.authMutex.Unlock()
	if c.auth.AccessToken == "" {
		// no token yet
		c.v("no auth token")
		return c.AuthenticateWithContext(ctx)
	}
	if time.Now().After(c.authExp) {
		// token expired, renew
		c.v("auth token expired")
		return c.Authenticate()
	}
	return nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return defaultHTTPClient
}

// apiRequest makes a request to the client's API endpoint
func (c *Client) apiRequest(ctx context.Context, auth bool, method, url string, request io.Reader) (*http.Response, error) {
	c.v("HTTP API Request: %s %q", method, url)
	if auth {
		err := c.checkAuth(ctx)
		if err != nil {
			return nil, err
		}
	}

	totalTrys := 3
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
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.auth.AccessToken))

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
			case <-time.After(time.Second * 10):
			}
		}
	}

	return resp, err
}

// jsonAPI performs an authenticated JSON API request
func (c *Client) jsonAPI(ctx context.Context, method, path string, request, response interface{}) error {
	return c.jsonRequest(ctx, true, method, c.BaseURL+path, request, response)
}

// jsonRequest performs a request to the API endpoint sending and receiving JSON objects
func (c *Client) jsonRequest(ctx context.Context, auth bool, method, url string, request, response interface{}) error {
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

// getExpiration returns the expiration of the authentication token
func (ar *authResponse) getExpiration() (time.Time, error) {
	token, err := jwt.DecodeJWT(ar.AccessToken)
	exp := time.Unix(token.Data.Exp, 0)
	return exp, err
}

// GetZoneRequestID returns the most recent RequestID for the given zone
//
// Deprecated: Use GetZoneRequestIDWithContext
func (c *Client) GetZoneRequestID(zone string) (string, error) {
	return c.GetZoneRequestIDWithContext(context.Background(), zone)
}

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

// GetAllRequests returns the request information for all requests with the given status.
// Status should be one of the constant czds.Status* strings.
// Warning: for a large number of results, may be slow.
//
// Deprecated: Use GetAllRequestsWithContext
func (c *Client) GetAllRequests(status string) ([]Request, error) {
	return c.GetAllRequestsWithContext(context.Background(), status)
}

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
