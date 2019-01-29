package czds

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	BaseURL = "https://czds-api.icann.org/"

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

// this function dowa NOT make network requests if the auth is valid
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

// Authenticate tests the client's credentials and gets an authentication token from the server
// calling this is optional. All other functions will check the auth state on their own first and authenticate if necessary.
func (c *Client) Authenticate() error {
	jsonCreds, err := json.Marshal(c.Creds)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", c.AuthURL, bytes.NewReader(jsonCreds))
	if err != nil {
		return err
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Error on getAccessToken, got Status %s %s", resp.Status, http.StatusText(resp.StatusCode))
	}

	authResp := authResponse{}
	err = json.NewDecoder(resp.Body).Decode(&authResp)
	if err != nil {
		return err
	}
	c.auth = authResp
	c.authExp, err = authResp.getExpiration()

	return err
}

func (ar *authResponse) getExpiration() (time.Time, error) {
	token, err := jwt.DecodeJWT(ar.AccessToken)
	exp := time.Unix(token.Data.Exp, 0)
	return exp, err
}
