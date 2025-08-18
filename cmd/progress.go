package main

import (
	"fmt"
	"io"
	"path"
)

// progressWriter wraps an io.Writer to provide download progress reporting for large files.
// It reports progress every 10% of the expected file size to avoid spam while providing useful feedback.
type progressWriter struct {
	writer       io.Writer
	totalBytes   int64
	writtenBytes int64
	lastReport   int64
	filename     string
	quiet        bool
	minSize      int64 // Minimum file size to show progress for
}

// newProgressWriter creates a progress-aware writer that reports download progress.
// Progress is only shown for files larger than minSize (50MB by default) and not in quiet mode.
func newProgressWriter(writer io.Writer, totalBytes int64, filename string, quiet bool) *progressWriter {
	return &progressWriter{
		writer:     writer,
		totalBytes: totalBytes,
		filename:   path.Base(filename), // Just show filename, not full path
		quiet:      quiet,
		minSize:    50 * 1024 * 1024, // 50MB threshold
	}
}

// Write implements io.Writer and reports progress periodically during large downloads.
func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	if err != nil {
		return n, err
	}

	pw.writtenBytes += int64(n)

	// Only show progress when not in quiet mode
	if !pw.quiet {
		if pw.totalBytes > 0 && pw.totalBytes > pw.minSize {
			// Known size and large file - show percentage progress
			progress := pw.writtenBytes * 100 / pw.totalBytes
			// Report every 10% progress to avoid spam
			if progress >= pw.lastReport+10 {
				// Ignore errors from progress output to prevent blocking on closed stdout
				_, _ = fmt.Printf("[%s] Progress: %d%% (%s/%s)\n",
					pw.filename,
					progress,
					formatBytes(pw.writtenBytes),
					formatBytes(pw.totalBytes))
				pw.lastReport = progress
			}
		} else if pw.totalBytes <= 0 && pw.writtenBytes > pw.minSize {
			// Unknown size but already downloaded more than minSize - show bytes progress
			// Report every 25MB to avoid spam for unknown sizes
			progressMB := pw.writtenBytes / (25 * 1024 * 1024) // 25MB chunks
			if progressMB > pw.lastReport {
				// Ignore errors from progress output to prevent blocking on closed stdout
				_, _ = fmt.Printf("[%s] Downloaded: %s...\n",
					pw.filename,
					formatBytes(pw.writtenBytes))
				pw.lastReport = progressMB
			}
		}
	}

	return n, err
}

// formatBytes formats byte counts in human-readable format (KB, MB, GB).
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
