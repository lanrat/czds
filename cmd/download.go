package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net/url"
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
	Zones      []string // List of zones to download (from command line args)
}

// zoneInfo contains information about a zone file download task,
// including the zone name, download URL, local file path, and retry count.
type zoneInfo struct {
	Name     string // Zone name (e.g., "com", "org")
	Dl       string // Download URL for the zone file
	FullPath string // Full local file path where zone will be saved
	Count    int    // Current retry attempt count
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
	fs.StringVar(&config.OutDir, "out", ".", "path to save downloaded zones to")
	fs.BoolVar(&config.URLName, "urlname", false, "use the filename from the url link as the saved filename instead of the file header")
	fs.BoolVar(&config.Force, "force", false, "force redownloading the zone even if it already exists on local disk with same size and modification date")
	fs.BoolVar(&config.Redownload, "redownload", false, "redownload zones that are newer on the remote server than local copy")
	fs.StringVar(&config.Exclude, "exclude", "", "don't fetch these zones")
	fs.UintVar(&config.Retries, "retries", 3, "max retry attempts per zone file download")
	fs.StringVar(&config.Zone, "zone", "", "comma separated list of zones to download, defaults to all")
	fs.BoolVar(&config.Quiet, "quiet", false, "suppress progress printing")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: czds download [OPTIONS] [zones...]\n\n")
		fmt.Fprintf(os.Stderr, "Download zone files from CZDS\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  czds download                                # Download all available zones\n")
		fmt.Fprintf(os.Stderr, "  czds download -zone com,org                  # Download specific zones\n")
		fmt.Fprintf(os.Stderr, "  czds download -parallel 10 -out ./zones     # Download with 10 parallel workers\n")
		fmt.Fprintf(os.Stderr, "  czds download -force -zone com               # Force redownload of com zone\n")
		fmt.Fprintf(os.Stderr, "  czds download -exclude com,net               # Download all except com and net\n")
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

	// Set up channels and sync
	loadDone := make(chan bool)
	inputChan := make(chan *zoneInfo, 100)
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

	// Wait for workers with context cancellation check
	done := make(chan error, 1)
	go func() {
		done <- g.Wait()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// getDownloadLinks retrieves the list of zone download URLs based on command configuration.
// It handles both specific zone requests and fetching all available zones with exclusions.
func getDownloadLinks(ctx context.Context, client *czds.Client, config *DownloadConfig, verbose bool) ([]string, error) {
	var downloads []string

	// If zones specified via args or -zone flag, download those specifically
	var zonesToDownload []string
	if len(config.Zones) > 0 {
		zonesToDownload = config.Zones
	} else if config.Zone != "" {
		zonesToDownload = strings.Split(config.Zone, ",")
	}

	if len(zonesToDownload) > 0 {
		// Build URLs for specific zones
		for _, zoneName := range zonesToDownload {
			u, _ := url.Parse(czds.BaseURL)
			u.Path = path.Join(u.Path, "/czds/downloads/", fmt.Sprintf("%s.zone", strings.ToLower(zoneName)))
			downloads = append(downloads, u.String())
		}
	} else {
		// Get all available download links
		if verbose {
			fmt.Println("Requesting download links")
		}
		var err error
		downloads, err = client.GetLinksWithContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get download links: %w", err)
		}

		// Apply exclusions if specified
		if config.Exclude != "" {
			downloads = pruneLinks(downloads, config.Exclude)
		}

		if verbose {
			fmt.Printf("Received %d zone links\n", len(downloads))
		}
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
			Name:  path.Base(dl),
			Dl:    dl,
			Count: 1,
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

			for uint(zi.Count) <= config.Retries {
				// Check for context cancellation
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				err := zoneDownload(ctx, client, config, zi, verbose)
				zi.Count++
				if err != nil {
					// don't stop on an error that only affects a single zone
					// fixes occasional HTTP 500s from CZDS
					if verbose {
						fmt.Printf("[%s] err: %s\n", path.Base(zi.Dl), err)
					}
				} else {
					// got the zone, exit the retry loop
					break
				}

				if uint(zi.Count) <= config.Retries {
					// Context-aware sleep
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(downloadRetryDelay):
					}
					continue
				}

				fmt.Printf("[%s] Max fail count hit; not downloading.\n", path.Base(zi.Dl))
				if _, err := os.Stat(zi.FullPath); !os.IsNotExist(err) {
					err = os.Remove(zi.FullPath)
					if err != nil {
						// log but continue; not fatal
						fmt.Printf("[%s] %s\n", zi.Dl, err)
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

	zi.FullPath = path.Join(config.OutDir, safeFileName)

	localFileInfo, err := os.Stat(zi.FullPath)
	if config.Force {
		if verbose {
			fmt.Printf("Forcing download of '%s'\n", zi.Dl)
		}
		return downloadTime(ctx, client, zi, config.Quiet)
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
			return downloadTime(ctx, client, zi, config.Quiet)
		}
		// check local file modification date
		if localFileInfo.ModTime().Before(info.LastModified) {
			// remote file is newer, redownload
			if verbose {
				fmt.Println("Remote file is newer than local, redownloading")
			}
			return downloadTime(ctx, client, zi, config.Quiet)
		}
		// local copy is good, skip download
		if verbose {
			fmt.Printf("Local file '%s' matched remote, skipping\n", localFileName)
		}
		return nil
	}

	if os.IsNotExist(err) {
		// file does not exist, download
		return downloadTime(ctx, client, zi, config.Quiet)
	}

	return err
}

// downloadTime downloads a zone file and reports the time taken for the operation.
// It provides timing feedback unless quiet mode is enabled.
func downloadTime(ctx context.Context, client *czds.Client, zi *zoneInfo, quiet bool) error {
	// file does not exist, download
	start := time.Now()
	err := client.DownloadZoneWithContext(ctx, zi.Dl, zi.FullPath)
	if err != nil {
		return err
	}
	if !quiet {
		delta := time.Since(start).Round(time.Millisecond)
		fmt.Printf("Downloaded %s in %s\n", zi.Name, delta)
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

	// Pre-compute excluded suffixes once
	excludeList := strings.Split(exclude, ",")
	excludeSuffixes := make([]string, len(excludeList))
	for i, e := range excludeList {
		excludeSuffixes[i] = strings.TrimSpace(e) + ".zone"
	}

	newlist := make([]string, 0, len(downloads))
	for _, u := range downloads {
		found := false
		for _, suffix := range excludeSuffixes {
			if strings.HasSuffix(u, suffix) {
				found = true
				break
			}
		}
		if !found {
			newlist = append(newlist, u)
		}
	}
	return newlist
}
