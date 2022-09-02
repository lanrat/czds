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

// DownloadZoneToWriter is analogous to DownloadZone but instead of writing it to a file, it will
// write it to a provided io.Writer. It returns the number of bytes written to dest and any error
// that was encountered.
func (c *Client) DownloadZoneToWriter(url string, dest io.Writer) (int64, error) {
	c.v("downloading zone from %q", url)
	resp, err := c.apiRequest(true, "GET", url, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
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

// DownloadZone provided the zone download URL retrieved from GetLinks() downloads the zone file and
// saves it to local disk at destinationPath
func (c *Client) DownloadZone(url, destinationPath string) error {
	// start the file download
	file, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer file.Close()

	n, err := c.DownloadZoneToWriter(url, file)
	if err != nil {
		os.Remove(destinationPath)
		return err
	}
	if n == 0 {
		os.Remove(destinationPath)
		return fmt.Errorf("%s was empty", destinationPath)
	}

	return nil
}

// GetDownloadInfo Performs a HEAD request to the zone at url and populates a DownloadInfo struct
// with the information returned by the headers
func (c *Client) GetDownloadInfo(url string) (*DownloadInfo, error) {
	c.v("GetDownloadInfo for %q", url)
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
