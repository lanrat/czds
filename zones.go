package czds

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
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

// DownloadZoneToWriter downloads a zone file from the given URL and writes it to the provided io.Writer.
// It returns the number of bytes written and any error encountered.
//
// Deprecated: Use DownloadZoneToWriterWithContext for context cancellation support.
func (c *Client) DownloadZoneToWriter(url string, dest io.Writer) (int64, error) {
	return c.DownloadZoneToWriterWithContext(context.Background(), url, dest)
}

// DownloadZoneToWriterWithContext downloads a zone file from the given URL and writes it to the provided io.Writer.
// It returns the number of bytes written and any error encountered. The download can be cancelled using the provided context.
func (c *Client) DownloadZoneToWriterWithContext(ctx context.Context, url string, dest io.Writer) (int64, error) {
	c.v("downloading zone from %q", url)
	resp, err := c.apiRequest(ctx, true, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.v("Error closing response body: %v", err)
		}
	}()

	// Use buffer pool for more efficient copying
	w, err := copyWithBuffer(dest, newReaderCtx(ctx, resp.Body))
	if err != nil {
		return w, err
	}

	c.v("downloading %d bytes finished from %q", resp.ContentLength, url)
	// Only validate Content-Length if server provided it (> 0)
	if resp.ContentLength > 0 && w != resp.ContentLength {
		return w, fmt.Errorf("downloaded bytes: %d, while request content-length is: %d ", w, resp.ContentLength)
	}
	return w, nil
}

// copyWithBuffer copies from src to dst using a pooled buffer for better performance
func copyWithBuffer(dst io.Writer, src io.Reader) (int64, error) {
	// Get buffer from pool
	buf := bufferPool.Get().([]byte)
	defer bufferPool.Put(buf)

	return io.CopyBuffer(dst, src, buf)
}

// DownloadZone downloads a zone file from the given URL and saves it to the specified file path.
// The URL should be retrieved from GetLinks(). If an error occurs, any partially downloaded file is removed.
//
// Deprecated: Use DownloadZoneWithContext for context cancellation support.
func (c *Client) DownloadZone(url, destinationPath string) error {
	return c.DownloadZoneWithContext(context.Background(), url, destinationPath)
}

// DownloadZoneWithContext downloads a zone file from the given URL and saves it to the specified file path.
// The operation can be cancelled using the provided context. If an error occurs, any partially downloaded file is removed.
func (c *Client) DownloadZoneWithContext(ctx context.Context, url, destinationPath string) error {
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

	// Use buffered writer for better I/O performance on large files
	bufferedWriter := bufio.NewWriterSize(file, 64*1024) // 64KB buffer
	defer func() {
		if flushErr := bufferedWriter.Flush(); flushErr != nil {
			c.v("Error flushing buffered writer: %v", flushErr)
		}
	}()

	n, err := c.DownloadZoneToWriterWithContext(ctx, url, bufferedWriter)
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

// GetDownloadInfo retrieves metadata about a zone file download without downloading the file itself.
// It performs a HEAD request to get information like file size, last modified time, and filename.
//
// Deprecated: Use GetDownloadInfoWithContext for context cancellation support.
func (c *Client) GetDownloadInfo(url string) (*DownloadInfo, error) {
	return c.GetDownloadInfoWithContext(context.Background(), url)
}

// GetDownloadInfoWithContext retrieves metadata about a zone file download without downloading the file itself.
// It performs a HEAD request to get information like file size, last modified time, and filename.
// The operation can be cancelled using the provided context.
func (c *Client) GetDownloadInfoWithContext(ctx context.Context, url string) (*DownloadInfo, error) {
	c.v("GetDownloadInfo for %q", url)
	resp, err := c.apiRequest(ctx, true, "HEAD", url, nil)
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

	// Validate Content-Length is not negative
	if contentLength < 0 {
		return nil, fmt.Errorf("invalid Content-Length: %d bytes", contentLength)
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

// GetLinks returns all zone download links available to the authenticated user.
// The returned URLs can be used with the download functions to retrieve zone files.
//
// Deprecated: Use GetLinksWithContext for context cancellation support.
func (c *Client) GetLinks() ([]string, error) {
	return c.GetLinksWithContext(context.Background())
}

// GetLinksWithContext retrieves all zone download links available to the authenticated user.
// It returns a slice of URLs that can be used with the download functions.
// The operation can be cancelled using the provided context.
func (c *Client) GetLinksWithContext(ctx context.Context) ([]string, error) {
	// Pre-allocate with more realistic capacity - most users have access to 50-500+ zones
	links := make([]string, 0, 100)
	c.v("GetLinks called")
	err := c.jsonAPI(ctx, http.MethodGet, "/czds/downloads/links", nil, &links)
	if err != nil {
		return nil, err
	}

	c.v("GetLinks returned %d links", len(links))

	return links, nil
}
