package czds

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"time"
)

// DownloadInfo information from the HEAD request from a DownloadLink
type DownloadInfo struct {
	ContentLength int64
	LastModified  time.Time
	Filename      string
}

func (c *Client) DownloadZone(url, destinationPath string) error {
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
		return fmt.Errorf("error on downloadZone, got status %s %s", resp.Status, http.StatusText(resp.StatusCode))
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

func (c *Client) GetDownloadInfo(url string) (*DownloadInfo, error) {
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
		return nil, fmt.Errorf("Error on getZoneInfo, got Status %s (%s)", resp.Status, http.StatusText(resp.StatusCode))
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
func (c *Client) GetLinks() ([]string, error) {
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
		return nil, fmt.Errorf("Error on getAccessToken, got Status %s %s", resp.Status, http.StatusText(resp.StatusCode))
	}
	links := make([]string, 0, 10)
	err = json.NewDecoder(resp.Body).Decode(&links)
	if err != nil {
		return nil, err
	}
	dLinks := make([]string, 0, len(links))
	for _, url := range links {
		dLinks = append(dLinks, url)
	}

	return dLinks, nil
}
