package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanrat/czds"
	"golang.org/x/sync/errgroup"
)

const (
	// downloadRetryDelay is the delay between download retry attempts
	downloadRetryDelay = 15 * time.Second
)

// DownloadConfig holds the configuration for the download command.
// It contains settings for parallel downloads, output directory, file naming,
// retry behavior, and zone filtering options.
type DownloadConfig struct {
	Parallel   uint     // Number of concurrent downloads
	OutDir     string   // Output directory for downloaded zone files
	URLName    bool     // Use URL-based naming for files
	Force      bool     // Force download even if file exists
	Redownload bool     // Re-download files that already exist
	Exclude    string   // Comma-separated list of zones to exclude
	Retries    uint     // Maximum number of retry attempts per zone
	Zone       string   // Single zone to download (deprecated, use Zones)
	Quiet      bool     // Suppress non-error output
	Progress   bool     // Show progress for large file downloads
	Zones      []string // List of zones to download (from command line args)
}

// zoneInfo contains information about a zone file download task,
// including the zone name, download URL, and local file path.
type zoneInfo struct {
	Name     string // Zone name (e.g., "com", "org")
	Dl       string // Download URL for the zone file
	FullPath string // Full local file path where zone will be saved
}

// downloadCmd creates and configures the download subcommand for the czds CLI.
// It sets up command-line flags and returns a Command that handles zone file downloads.
func downloadCmd() *Command {
	var gf GlobalFlags
	var config DownloadConfig

	fs := flag.NewFlagSet("download", flag.ExitOnError)

	// Add global flags
	addGlobalFlags(fs, &gf)

	// Add download-specific flags
	fs.UintVar(&config.Parallel, "parallel", 5, "number of zones to download in parallel")
	fs.StringVar(&config.OutDir, "out", "zones", "path to save downloaded zones to")
	fs.BoolVar(&config.URLName, "urlname", false, "use the filename from the url link as the saved filename instead of the file header")
	fs.BoolVar(&config.Force, "force", false, "force redownloading the zone even if it already exists on local disk with same size and modification date")
	fs.BoolVar(&config.Redownload, "redownload", false, "redownload zones that are newer on the remote server than local copy")
	fs.StringVar(&config.Exclude, "exclude", "", "don't fetch these zones")
	fs.UintVar(&config.Retries, "retries", 3, "max retry attempts per zone file download")
	fs.StringVar(&config.Zone, "zones", "", "comma separated list of zones to download, defaults to all")
	fs.BoolVar(&config.Quiet, "quiet", false, "suppress progress printing")
	fs.BoolVar(&config.Progress, "progress", false, "show download progress for large files (>50MB)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: czds download [OPTIONS] [zones...]\n\n")
		fmt.Fprintf(os.Stderr, "Download zone files from CZDS\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  czds download                                # Download all available zones\n")
		fmt.Fprintf(os.Stderr, "  czds download -zones com,org                  # Download specific zones\n")
		fmt.Fprintf(os.Stderr, "  czds download -parallel 10 -out ./zones     # Download with 10 parallel workers\n")
		fmt.Fprintf(os.Stderr, "  czds download -force -zone com               # Force redownload of com zone\n")
		fmt.Fprintf(os.Stderr, "  czds download -exclude com,net               # Download all except com and net\n")
		fmt.Fprintf(os.Stderr, "  czds download -progress -zone com            # Download with progress reporting\n")
		fmt.Fprintf(os.Stderr, "\nZones can also be specified as positional arguments:\n")
		fmt.Fprintf(os.Stderr, "  czds download com org net                    # Download com, org, and net zones\n")
	}

	return &Command{
		Name:        "download",
		Description: "Download zone files from CZDS",
		FlagSet:     fs,
		Run: func(ctx context.Context) error {
			if err := fs.Parse(os.Args[2:]); err != nil {
				return fmt.Errorf("failed to parse flags: %w", err)
			}

			// Get remaining args as zones to download
			config.Zones = fs.Args()

			// Validate flags
			if err := checkRequiredFlags(&gf); err != nil {
				return err
			}

			if config.Parallel < 1 {
				return fmt.Errorf("parallel must be positive")
			}
			if config.Parallel > 100 {
				return fmt.Errorf("parallel downloads limited to 100 to prevent resource exhaustion")
			}

			// Create client
			client, err := createClient(&gf)
			if err != nil {
				return fmt.Errorf("failed to create client: %w", err)
			}

			// Authenticate
			if gf.Verbose {
				fmt.Printf("Authenticating to %s\n", client.AuthURL)
			}
			err = client.AuthenticateWithContext(ctx)
			if err != nil {
				return fmt.Errorf("authentication failed: %w", err)
			}

			return runDownload(ctx, client, &config, gf.Verbose)
		},
	}
}

// runDownload executes the download command logic with parallel workers and retry handling.
// It manages output directory creation, link retrieval, worker coordination, and error handling.
func runDownload(ctx context.Context, client *czds.Client, config *DownloadConfig, verbose bool) error {
	// Create output directory if it does not exist
	_, err := os.Stat(config.OutDir)
	if err != nil {
		if os.IsNotExist(err) {
			if verbose {
				fmt.Printf("'%s' does not exist, creating\n", config.OutDir)
			}
			err = os.MkdirAll(config.OutDir, 0770)
			if err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}
		} else {
			return fmt.Errorf("failed to check output directory: %w", err)
		}
	}

	// Get download links
	downloads, err := getDownloadLinks(ctx, client, config, verbose)
	if err != nil {
		return err
	}

	if len(downloads) == 0 {
		fmt.Println("No zones to download")
		return nil
	}

	// Shuffle download links to better distribute load on CZDS
	downloads = shuffle(downloads)

	// Set up channels and sync - buffer channel based on parallel workers for better throughput
	loadDone := make(chan bool)
	inputChan := make(chan *zoneInfo, int(config.Parallel)*2)
	g, ctx := errgroup.WithContext(ctx)

	// Start workers
	g.Go(func() error {
		return addLinks(ctx, downloads, inputChan, loadDone)
	})
	if verbose {
		fmt.Printf("Starting %d parallel downloads\n", config.Parallel)
	}
	for i := uint(0); i < config.Parallel; i++ {
		g.Go(func() error {
			return worker(ctx, client, config, inputChan, verbose)
		})
	}

	// Wait for workers to finish
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-loadDone:
	}

	// Wait for workers with proper context cancellation handling
	return g.Wait()
}

// getDownloadLinks retrieves the list of zone download URLs based on command configuration.
// It always uses API-provided links and filters them based on the requested zones and exclusions.
func getDownloadLinks(ctx context.Context, client *czds.Client, config *DownloadConfig, verbose bool) ([]string, error) {
	// Always get all available download links from API
	if verbose {
		fmt.Println("Requesting download links")
	}
	downloads, err := client.GetLinksWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get download links: %w", err)
	}

	if verbose {
		fmt.Printf("Received %d zone links\n", len(downloads))
	}

	// If zones specified via args or -zones flag, filter to those zones
	var zonesToDownload []string
	if len(config.Zones) > 0 {
		zonesToDownload = config.Zones
	} else if config.Zone != "" {
		zonesToDownload = strings.Split(config.Zone, ",")
	}

	if len(zonesToDownload) > 0 {
		// Filter links to match requested zones
		zoneSet := make(map[string]bool)
		for _, zoneName := range zonesToDownload {
			zoneSet[strings.ToLower(zoneName)] = true
		}

		var filteredDownloads []string
		for _, link := range downloads {
			// Extract zone name from URL (e.g., "com.zone" -> "com")
			fileName := path.Base(link)
			zoneName := strings.TrimSuffix(fileName, ".zone")
			if zoneSet[strings.ToLower(zoneName)] {
				filteredDownloads = append(filteredDownloads, link)
			}
		}

		// Check if any requested zones were not found
		if len(filteredDownloads) < len(zonesToDownload) {
			foundZones := make(map[string]bool)
			for _, link := range filteredDownloads {
				fileName := path.Base(link)
				zoneName := strings.TrimSuffix(fileName, ".zone")
				foundZones[strings.ToLower(zoneName)] = true
			}

			var missingZones []string
			for _, zoneName := range zonesToDownload {
				if !foundZones[strings.ToLower(zoneName)] {
					missingZones = append(missingZones, zoneName)
				}
			}

			if len(missingZones) > 0 {
				return nil, fmt.Errorf("zones not available for download: %s", strings.Join(missingZones, ", "))
			}
		}

		downloads = filteredDownloads
	}

	// Apply exclusions if specified
	if config.Exclude != "" {
		downloads = pruneLinks(downloads, config.Exclude)
	}

	return downloads, nil
}

// addLinks feeds download tasks to workers through the input channel.
// It signals completion via loadDone channel and handles context cancellation.
func addLinks(ctx context.Context, downloads []string, inputChan chan<- *zoneInfo, loadDone chan<- bool) error {
	for _, dl := range downloads {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case inputChan <- &zoneInfo{
			Name: path.Base(dl),
			Dl:   dl,
		}:
		}
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case loadDone <- true:
	}
	close(inputChan) // Signal workers to stop
	return nil
}

// worker is a goroutine that processes zone download tasks from inputChan.
// It downloads zones with retry logic and handles context cancellation gracefully.
func worker(ctx context.Context, client *czds.Client, config *DownloadConfig, inputChan <-chan *zoneInfo, verbose bool) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case zi, more := <-inputChan:
			if !more {
				// noting left to do
				return nil
			}

			var err error
			for attempt := 1; attempt <= int(config.Retries); attempt++ {
				// Check context cancellation
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				err = zoneDownload(ctx, client, config, zi, verbose)
				if err == nil {
					// Success - exit retry loop
					break
				}

				// Log the error for this attempt
				// don't stop on an error that only affects a single zone
				// fixes occasional HTTP 500s from CZDS
				if verbose {
					fmt.Printf("[%s] Attempt %d/%d failed: %s\n", path.Base(zi.Dl), attempt, config.Retries, err)
				}

				// If this was the last attempt, don't sleep
				if attempt >= int(config.Retries) {
					break
				}

				// Context-aware sleep before retry
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(downloadRetryDelay):
				}
			}

			// Handle final failure after all retries exhausted
			if err != nil {
				fmt.Printf("[%s] Max fail count hit after %d attempts; not downloading.\n", path.Base(zi.Dl), config.Retries)
				// cleanup partial file if it exists
				if _, statErr := os.Stat(zi.FullPath); !os.IsNotExist(statErr) {
					if removeErr := os.Remove(zi.FullPath); removeErr != nil {
						// log but continue; not fatal
						fmt.Printf("[%s] Error removing partial file: %s\n", zi.Dl, removeErr)
					}
				}
			}
		}
	}
}

// zoneDownload handles the download of a single zone with local file checks and validation.
// It manages file existence checks, redownload logic, and path safety.
func zoneDownload(ctx context.Context, client *czds.Client, config *DownloadConfig, zi *zoneInfo, verbose bool) error {
	if verbose {
		fmt.Printf("Downloading '%s'\n", zi.Dl)
	}

	info, err := client.GetDownloadInfoWithContext(ctx, zi.Dl)
	if err != nil {
		return fmt.Errorf("%s [%s]", err, zi.Dl)
	}

	// use filename from url or header?
	localFileName := info.Filename
	if config.URLName {
		localFileName = path.Base(zi.Dl)
	}

	// Always use filepath.Base to prevent path traversal attacks - ensures we only get the filename
	safeFileName := filepath.Base(localFileName)
	if safeFileName == "." || safeFileName == ".." {
		return fmt.Errorf("invalid filename: %q", localFileName)
	}

	zi.FullPath = filepath.Join(config.OutDir, safeFileName)

	// Extra safety check: ensure the resolved path is still within the output directory
	absOutDir, err := filepath.Abs(config.OutDir)
	if err != nil {
		return fmt.Errorf("failed to resolve output directory: %w", err)
	}
	absFullPath, err := filepath.Abs(zi.FullPath)
	if err != nil {
		return fmt.Errorf("failed to resolve target path: %w", err)
	}
	if !strings.HasPrefix(absFullPath, absOutDir+string(filepath.Separator)) && absFullPath != absOutDir {
		return fmt.Errorf("resolved path %q is outside output directory %q", absFullPath, absOutDir)
	}

	localFileInfo, err := os.Stat(zi.FullPath)
	if config.Force {
		if verbose {
			fmt.Printf("Forcing download of '%s'\n", zi.Dl)
		}
		return downloadTime(ctx, client, zi, info.ContentLength, config.Quiet, config.Progress)
	}

	// check if local file already exists
	if err == nil && config.Redownload {
		// check local file size
		if localFileInfo.Size() != info.ContentLength {
			// size differs, redownload
			if verbose {
				fmt.Printf("Size of local file (%d) differs from remote (%d), redownloading %s\n",
					localFileInfo.Size(), info.ContentLength, localFileName)
			}
			return downloadTime(ctx, client, zi, info.ContentLength, config.Quiet, config.Progress)
		}
		// check local file modification date
		if localFileInfo.ModTime().Before(info.LastModified) {
			// remote file is newer, redownload
			if verbose {
				fmt.Println("Remote file is newer than local, redownloading")
			}
			return downloadTime(ctx, client, zi, info.ContentLength, config.Quiet, config.Progress)
		}
		// local copy is good, skip download
		if verbose {
			fmt.Printf("Local file '%s' matched remote, skipping\n", localFileName)
		}
		return nil
	}

	if os.IsNotExist(err) {
		// file does not exist, download
		return downloadTime(ctx, client, zi, info.ContentLength, config.Quiet, config.Progress)
	}

	return err
}

// downloadTime downloads a zone file and reports the time taken for the operation.
// It provides timing feedback and progress reporting unless quiet mode is enabled.
// Uses atomic file operations - downloads to a temporary file first, then renames on success.
func downloadTime(ctx context.Context, client *czds.Client, zi *zoneInfo, contentLength int64, quiet, showProgress bool) error {
	// Create temporary file in same directory for atomic operation
	tempPath := zi.FullPath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return err
	}

	var downloadErr error
	defer func() {
		// Always close file first
		if closeErr := file.Close(); closeErr != nil && !quiet {
			fmt.Printf("Error closing file: %v\n", closeErr)
		}

		// Handle cleanup and atomic rename
		// Check if context was cancelled or if there was a download error
		if downloadErr != nil || ctx.Err() != nil {
			// Remove temp file on error or cancellation
			if removeErr := os.Remove(tempPath); removeErr != nil && !quiet {
				fmt.Printf("Error removing temp file %s: %v\n", tempPath, removeErr)
			}
		} else {
			// Atomically rename temp file to final name on success
			if renameErr := os.Rename(tempPath, zi.FullPath); renameErr != nil {
				downloadErr = fmt.Errorf("failed to rename temp file: %w", renameErr)
				// Clean up temp file if rename fails
				if removeErr := os.Remove(tempPath); removeErr != nil && !quiet {
					fmt.Printf("Error removing temp file after rename failure %s: %v\n", tempPath, removeErr)
				}
			}
		}
	}()

	// Use buffered writer for better I/O performance
	bufferedWriter := bufio.NewWriterSize(file, 64*1024) // 64KB buffer

	// Wrap with progress writer for large files (only if progress flag is enabled)
	progressWriter := newProgressWriter(bufferedWriter, contentLength, zi.Name, quiet || !showProgress)

	// Download with progress reporting
	start := time.Now()
	n, err := client.DownloadZoneToWriterWithContext(ctx, zi.Dl, progressWriter)
	if err != nil {
		downloadErr = err
		return err
	}

	// Flush buffered writer before checking file size
	if flushErr := bufferedWriter.Flush(); flushErr != nil {
		downloadErr = fmt.Errorf("failed to flush buffer: %w", flushErr)
		return downloadErr
	}

	if n == 0 {
		downloadErr = fmt.Errorf("%s was empty", zi.Name)
		return downloadErr
	}

	if !quiet {
		delta := time.Since(start).Round(time.Millisecond)
		fmt.Printf("Downloaded %s (%s) in %s\n", zi.Name, formatBytes(n), delta)
	}
	return nil
}

// shuffle randomly reorders a slice of strings using Go's built-in shuffle algorithm.
// This helps distribute load across CZDS servers when downloading multiple zones.
func shuffle(src []string) []string {
	result := make([]string, len(src))
	copy(result, src)
	rand.Shuffle(len(result), func(i, j int) {
		result[i], result[j] = result[j], result[i]
	})
	return result
}

// pruneLinks removes download URLs for zones specified in the exclude list.
// It supports comma-separated exclusion lists with whitespace trimming.
func pruneLinks(downloads []string, exclude string) []string {
	if exclude == "" {
		return downloads
	}

	// Parse exclude list into map for O(1) lookups
	excludeMap := excludeListToMap(exclude)
	if excludeMap == nil {
		return downloads
	}

	// Pre-allocate with exact capacity since we know the maximum possible size
	newlist := make([]string, 0, len(downloads))
	for _, u := range downloads {
		// Extract zone name from URL (e.g., "com.zone" -> "com")
		fileName := path.Base(u)
		zoneName := strings.TrimSuffix(fileName, ".zone")

		// O(1) lookup instead of O(n*m) string suffix matching
		if !excludeMap[strings.ToLower(zoneName)] {
			newlist = append(newlist, u)
		}
	}
	return newlist
}
