package czds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/lanrat/czds/jwt"
)

const (
	// TestAuthURL testing url endpoint
	TestAuthURL = "https://account-api-test.icann.org/api/authenticate"
	// TestBaseURL testing url endpoint
	TestBaseURL = "https://czds-api-test.icann.org"

	// AuthURL production url endpoint
	AuthURL = "https://account-api.icann.org/api/authenticate"
	// BaseURL production url endpoint
	BaseURL = "https://czds-api.icann.org/"
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

// DownloadInfo information from the HEAD request from a DownloadLink
type DownloadInfo struct {
	ContentLength int64
	LastModified  time.Time
	Filename      string
}

// DownloadLink for a single zone from the Client
type DownloadLink struct {
	URL    string
	client *Client
}

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

// Download the DownloadLink zone to destinationPath
func (dl *DownloadLink) Download(destinationPath string) error {
	return dl.client.downloadZone(dl.URL, destinationPath)
}

func (c *Client) downloadZone(url, destinationPath string) error {
	err := c.checkAuth()
	if err != nil {
		return err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.auth.AccessToken))
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error on downloadZone, got status %s", resp.Status)
	}

	// start the file download
	file, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer file.Close()

	n, err := io.Copy(file, resp.Body)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%s was empty", destinationPath)
	}

	return nil
}

// GetInfo returns DownloadInfo populated with information from HTTP headers
// in a HEAD request
func (dl *DownloadLink) GetInfo() (*DownloadInfo, error) {
	return dl.client.getDownloadInfo(dl.URL)
}

func (c *Client) getDownloadInfo(url string) (*DownloadInfo, error) {
	err := c.checkAuth()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.auth.AccessToken))
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Error on getZoneInfo, got Status %s", resp.Status)
	}

	lastModifiedStr := resp.Header.Get("Last-Modified")
	if lastModifiedStr == "" {
		return nil, fmt.Errorf("HEAD request to %s missing 'Last-Modified' header", url)
	}
	lastModifiedTime, err := time.Parse(time.RFC1123, lastModifiedStr)
	if err != nil {
		return nil, err
	}

	contentLengthStr := resp.Header.Get("Content-Length")
	if contentLengthStr == "" {
		return nil, fmt.Errorf("HEAD request to %s missing 'Content-Length' header", url)
	}
	contentLength, err := strconv.ParseInt(contentLengthStr, 10, 64)
	if err != nil {
		return nil, err
	}

	contentDisposition := resp.Header.Get("Content-Disposition")
	_, params, err := mime.ParseMediaType(contentDisposition)
	if err != nil {
		return nil, err
	}
	/*if params["filename"] == "" {
		return nil, fmt.Errorf("no filename set in Content-Disposition for %s", url)
	}*/

	info := &DownloadInfo{
		LastModified:  lastModifiedTime,
		ContentLength: contentLength,
		Filename:      params["filename"],
	}
	return info, nil
}

// GetLinks returns the DownloadLinks available to the authenticated user
func (c *Client) GetLinks() ([]DownloadLink, error) {
	err := c.checkAuth()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", c.BaseURL+"/czds/downloads/links", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.auth.AccessToken))
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Error on getAccessToken, got Status %s", resp.Status)
	}
	links := make([]string, 0, 10)
	err = json.NewDecoder(resp.Body).Decode(&links)
	if err != nil {
		return nil, err
	}
	dLinks := make([]DownloadLink, 0, len(links))
	for _, url := range links {
		dLinks = append(dLinks, DownloadLink{
			URL:    url,
			client: c,
		})
	}

	return dLinks, nil
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
		return fmt.Errorf("Error on getAccessToken, got Status %s", resp.Status)
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
