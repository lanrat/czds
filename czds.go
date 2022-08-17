// Package czds implementing a client to the CZDS REST API using both the documented and undocumented API endpoints
package czds

import (
	"bytes"
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
	// AuthURL production url endpoint
	AuthURL = "https://account-api.icann.org/api/authenticate"
	// BaseURL production url endpoint
	BaseURL = "https://czds-api.icann.org"

	// TestAuthURL testing url endpoint
	TestAuthURL = "https://account-api-test.icann.org/api/authenticate"
	// TestBaseURL testing url endpoint
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

// this function does NOT make network requests if the auth is valid
func (c *Client) checkAuth() error {
	// used a mutex to prevent multiple threads from authenticating at the same time
	c.authMutex.Lock()
	defer c.authMutex.Unlock()
	if c.auth.AccessToken == "" {
		// no token yet
		return c.Authenticate()
	}
	if time.Now().After(c.authExp) {
		// token expired, renew
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
// TODO add optional context to requests
func (c *Client) apiRequest(auth bool, method, url string, request io.Reader) (*http.Response, error) {
	if auth {
		err := c.checkAuth()
		if err != nil {
			return nil, err
		}
	}

	totalTrys := 3
	var err error
	var req *http.Request
	var resp *http.Response
	for try := 1; try <= totalTrys; try++ {
		req, err = http.NewRequest(method, url, request)
		if err != nil {
			return nil, err
		}
		if request != nil {
			req.Header.Add("Content-Type", "application/json")
		}
		req.Header.Add("Accept", "application/json")
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.auth.AccessToken))

		resp, err = c.httpClient().Do(req)
		if err != nil {
			err = fmt.Errorf("error on request [%d/%d] %s, got error %w: %+v", try, totalTrys, url, err, resp)
		} else {
			return resp, nil
		}

		// sleep only if we will try again
		if try < totalTrys {
			time.Sleep(time.Second * 10)
		}
	}

	return resp, err
}

// jsonAPI performs an authenticated json API request
func (c *Client) jsonAPI(method, path string, request, response interface{}) error {
	return c.jsonRequest(true, method, c.BaseURL+path, request, response)
}

// jsonRequest performs a request to the API endpoint sending and receiving JSON objects
func (c *Client) jsonRequest(auth bool, method, url string, request, response interface{}) error {
	var payloadReader io.Reader
	if request != nil {
		jsonPayload, err := json.Marshal(request)
		if err != nil {
			return err
		}
		payloadReader = bytes.NewReader(jsonPayload)
	}

	resp, err := c.apiRequest(auth, method, url, payloadReader)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// got an error, decode it
	if resp.StatusCode != http.StatusOK {
		var errorResp errorResponse
		err := fmt.Errorf("error on request %q: got Status %s %s", url, resp.Status, http.StatusText(resp.StatusCode))
		if resp.ContentLength != 0 {
			jsonError := json.NewDecoder(resp.Body).Decode(&errorResp)
			if jsonError != nil {
				return fmt.Errorf("error decoding json %w on errored request: %s", jsonError, err.Error())
			}
			err = fmt.Errorf("%w HTTPStatus: %d Message: %q", err, errorResp.HTTPStatus, errorResp.Message)
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

// Authenticate tests the client's credentials and gets an authentication token from the server
// calling this is optional. All other functions will check the auth state on their own first and authenticate if necessary.
func (c *Client) Authenticate() error {

	authResp := authResponse{}
	err := c.jsonRequest(false, "POST", c.AuthURL, c.Creds, &authResp)
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

// GetZoneRequestID returns the most request RequestID for the given zone
func (c *Client) GetZoneRequestID(zone string) (string, error) {
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
	requests, err := c.GetRequests(&filter)
	if err != nil {
		return "", err
	}
	// check if zone in returned requests
	request := findFirstZoneInRequests(zone, requests)
	// if zone is not found in requests, and there are more requests to get, iterate through them
	for request == nil && len(requests.Requests) != 0 {
		filter.Pagination.Page++
		requests, err = c.GetRequests(&filter)
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

// GetAllRequests returns the request information for all requests with the given status
// status should be one of the constant czds.Status* strings
// warning: for large number of results, may be slow
func (c *Client) GetAllRequests(status string) ([]Request, error) {
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
	requests, err := c.GetRequests(&filter)
	if err != nil {
		return out, err
	}

	for len(requests.Requests) != 0 {
		out = append(out, requests.Requests...)
		filter.Pagination.Page++
		requests, err = c.GetRequests(&filter)
		if err != nil {
			return out, err
		}
	}

	return out, nil
}
