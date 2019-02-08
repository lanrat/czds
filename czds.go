package czds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
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
	httpClient = &http.Client{
		//Timeout: time.Minute * 120, // this timeout also included reading resp body,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
			//MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
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
	return httpClient
}

// apiRequest makes a request to the client's API endpoint
func (c *Client) apiRequest(auth bool, method, url string, request io.Reader) (*http.Response, error) {
	if auth {
		err := c.checkAuth()
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(method, url, request)
	if err != nil {
		return nil, err
	}
	if request != nil {
		req.Header.Add("Content-Type", "application/json")
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.auth.AccessToken))
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("Error on request %s, got Status %s %s", url, resp.Status, http.StatusText(resp.StatusCode))
	}

	return resp, nil
}

// jsonAPI performes an authenticated json API request
func (c *Client) jsonAPI(method, path string, request, response interface{}) error {
	return c.jsonRequest(true, method, c.BaseURL+path, request, response)
}

// jsonRequest performes a request to the API endpoint sending and receiving JSON objects
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

	if !c.authExp.After(time.Now()) {
		return fmt.Errorf("Unable to authenticate")
	}

	return nil
}

// getExpiration returns the expiration of the authentication token
func (ar *authResponse) getExpiration() (time.Time, error) {
	token, err := jwt.DecodeJWT(ar.AccessToken)
	exp := time.Unix(token.Data.Exp, 0)
	return exp, err
}
