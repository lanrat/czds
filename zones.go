package czds

import (
	"fmt"
	"io"
	"mime"
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

// DownloadZone provided the zone download URL retrieved from GetLinks() downloads the zone file and
// saves it to local disk at destinationPath
func (c *Client) DownloadZone(url, destinationPath string) error {
	resp, err := c.apiRequest(true, "GET", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

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

// GetDownloadInfo Performs a HEAD request to the zone at url and populates a DownloadInfo struct
// with the information returned by the headers
func (c *Client) GetDownloadInfo(url string) (*DownloadInfo, error) {
	resp, err := c.apiRequest(true, "HEAD", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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

	info := &DownloadInfo{
		LastModified:  lastModifiedTime,
		ContentLength: contentLength,
		Filename:      params["filename"],
	}
	return info, nil
}

// GetLinks returns the DownloadLinks available to the authenticated user
func (c *Client) GetLinks() ([]string, error) {
	links := make([]string, 0, 10)
	err := c.jsonAPI("GET", "/czds/downloads/links", nil, &links)
	if err != nil {
		return nil, err
	}

	dLinks := make([]string, 0, len(links))
	for _, url := range links {
		dLinks = append(dLinks, url)
	}

	return dLinks, nil
}
