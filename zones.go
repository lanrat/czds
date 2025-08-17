package czds

import (
	"fmt"
	"io"
	"mime"
	"os"
	"strconv"
	"time"
)

// DownloadInfo contains information from the HEAD request from a DownloadLink
type DownloadInfo struct {
	ContentLength int64
	LastModified  time.Time
	Filename      string
}

// DownloadZoneToWriter is analogous to DownloadZone but writes to a provided io.Writer instead of a file.
// It returns the number of bytes written to dest and any error that was encountered.
func (c *Client) DownloadZoneToWriter(url string, dest io.Writer) (int64, error) {
	c.v("downloading zone from %q", url)
	resp, err := c.apiRequest(true, "GET", url, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.v("Error closing response body: %v", err)
		}
	}()
	w, err := io.Copy(dest, resp.Body)
	if err != nil {
		return w, err
	}

	c.v("downloading %d bytes finished from %q", resp.ContentLength, url)
	if w != resp.ContentLength {
		return w, fmt.Errorf("downloaded bytes: %d, while request content-length is: %d ", w, resp.ContentLength)
	}
	return w, nil
}

// DownloadZone downloads the zone file from the given zone download URL (retrieved from GetLinks) and
// saves it to the specified destination path.
func (c *Client) DownloadZone(url, destinationPath string) error {
	// start the file download
	file, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			c.v("Error closing file: %v", err)
		}
	}()

	n, err := c.DownloadZoneToWriter(url, file)
	if err != nil {
		if removeErr := os.Remove(destinationPath); removeErr != nil {
			c.v("Error removing file %s: %v", destinationPath, removeErr)
		}
		return err
	}
	if n == 0 {
		if removeErr := os.Remove(destinationPath); removeErr != nil {
			c.v("Error removing file %s: %v", destinationPath, removeErr)
		}
		return fmt.Errorf("%s was empty", destinationPath)
	}

	return nil
}

// GetDownloadInfo performs a HEAD request to the zone at url and populates a DownloadInfo struct
// with the information returned by the headers
func (c *Client) GetDownloadInfo(url string) (*DownloadInfo, error) {
	c.v("GetDownloadInfo for %q", url)
	resp, err := c.apiRequest(true, "HEAD", url, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.v("Error closing response body: %v", err)
		}
	}()

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

// GetLinks returns the download links available to the authenticated user.
func (c *Client) GetLinks() ([]string, error) {
	links := make([]string, 0, 10)
	c.v("GetLinks called")
	err := c.jsonAPI("GET", "/czds/downloads/links", nil, &links)
	if err != nil {
		return nil, err
	}

	dLinks := make([]string, 0, len(links))
	dLinks = append(dLinks, links...)
	c.v("GetLinks returned %d links", len(dLinks))

	return dLinks, nil
}
