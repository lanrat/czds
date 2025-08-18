package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/lanrat/czds"
)

// StatusConfig holds the configuration for the status command.
// It contains settings for querying request status by ID or zone name,
// and for generating CSV reports of request information.
type StatusConfig struct {
	ID       string // Request ID to query for detailed status information
	Zone     string // Zone name to query for status information
	Report   string // Path to generate CSV report of all requests
	Progress bool   // Show progress for CSV report downloads
}

// statusCmd creates and configures the status subcommand for checking CZDS request status.
// It sets up command-line flags and returns a Command that handles status queries and report generation.
func statusCmd() *Command {
	var gf GlobalFlags
	var config StatusConfig

	fs := flag.NewFlagSet("status", flag.ExitOnError)

	// Add global flags
	addGlobalFlags(fs, &gf)

	// Add status-specific flags
	fs.StringVar(&config.ID, "id", "", "ID of specific zone request to lookup, defaults to printing all")
	fs.StringVar(&config.Zone, "zone", "", "same as -id, but prints the request by zone name")
	fs.StringVar(&config.Report, "report", "", "filename to save report CSV to, '-' for stdout")
	fs.BoolVar(&config.Progress, "progress", false, "show download progress for CSV reports")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: czds status [OPTIONS]\n\n")
		fmt.Fprintf(os.Stderr, "Check status of zone requests and generate reports\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  czds status                          # List all requests\n")
		fmt.Fprintf(os.Stderr, "  czds status -zone com                # Show details for com zone\n")
		fmt.Fprintf(os.Stderr, "  czds status -id REQUEST_ID           # Show details for specific request\n")
		fmt.Fprintf(os.Stderr, "  czds status -report report.csv       # Generate CSV report\n")
		fmt.Fprintf(os.Stderr, "  czds status -report report.csv -progress # Generate CSV with progress\n")
	}

	return &Command{
		Name:        "status",
		Description: "Check status of zone requests and generate reports",
		FlagSet:     fs,
		Run: func(ctx context.Context) error {
			if err := fs.Parse(os.Args[2:]); err != nil {
				return fmt.Errorf("failed to parse flags: %w", err)
			}

			// Validate flags
			if err := checkRequiredFlags(&gf); err != nil {
				return err
			}

			if (config.Report != "") && (config.ID != "" || config.Zone != "") {
				return fmt.Errorf("cannot use -report with specific zone request")
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

			return runStatus(ctx, client, &config, gf.Verbose)
		},
	}
}

// runStatus executes the status command logic based on the provided configuration.
// It handles zone-to-ID conversion, CSV report generation, and status display.
func runStatus(ctx context.Context, client *czds.Client, config *StatusConfig, verbose bool) error {
	// If zone name is provided, convert it to request ID
	if config.Zone != "" {
		zoneID, err := client.GetZoneRequestIDWithContext(ctx, config.Zone)
		if err != nil {
			return fmt.Errorf("failed to get request ID for zone %s: %w", config.Zone, err)
		}
		config.ID = zoneID
	}

	// Generate CSV report
	if config.Report != "" {
		return generateCSVReport(ctx, client, config.Report, verbose, config.Progress)
	}

	// List all requests
	if config.ID == "" {
		return listAllRequests(ctx, client, verbose)
	}

	// Show details for specific request
	return showRequestDetails(ctx, client, config.ID)
}

// listAllRequests retrieves and displays all zone requests in a tabular format.
// It fetches all requests regardless of status and prints them with headers.
func listAllRequests(ctx context.Context, client *czds.Client, verbose bool) error {
	requests, err := client.GetAllRequestsWithContext(ctx, czds.RequestAll)
	if err != nil {
		return fmt.Errorf("failed to get requests: %w", err)
	}

	if verbose {
		fmt.Printf("Total requests: %d\n", len(requests))
	}

	if len(requests) > 0 {
		printHeader()
		for _, request := range requests {
			printRequest(request)
		}
	}

	return nil
}

// showRequestDetails retrieves and displays detailed information for a specific request ID.
// It shows comprehensive request details including history and status information.
func showRequestDetails(ctx context.Context, client *czds.Client, requestID string) error {
	info, err := client.GetRequestInfoWithContext(ctx, requestID)
	if err != nil {
		return fmt.Errorf("failed to get request info for ID %s: %w", requestID, err)
	}

	printRequestInfo(info)
	return nil
}

// generateCSVReport downloads a CSV report of all requests to the specified file path.
// If reportPath is "-", the report is written to stdout instead of a file.
func generateCSVReport(ctx context.Context, client *czds.Client, reportPath string, verbose bool, showProgress bool) error {
	var out io.Writer = os.Stdout

	if reportPath != "-" {
		if verbose {
			fmt.Printf("Saving to %s\n", reportPath)
		}

		dir := path.Dir(reportPath)
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		file, err := os.Create(reportPath)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", reportPath, err)
		}
		defer func() {
			if err := file.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err)
			}
		}()
		out = file

		// Wrap output with progress writer if requested (but not for stdout)
		if showProgress {
			// For CSV reports, we don't know the size ahead of time, so pass 0 for totalBytes
			// This will show bytes downloaded without percentage
			out = newProgressWriter(out, 0, reportPath, false)
		}
	}

	return client.DownloadAllRequestsWithContext(ctx, out)
}

// Utility functions for printing

// printRequestInfo displays detailed information about a single request in a formatted output.
// It includes all request metadata, status, dates, and history.
func printRequestInfo(info *czds.RequestsInfo) {
	fmt.Printf("ID:\t%s\n", info.RequestID)
	if info.TLD != nil {
		fmt.Printf("TLD:\t%s (%s)\n", info.TLD.TLD, info.TLD.ULabel)
	} else {
		fmt.Printf("TLD:\t<unknown>\n")
	}
	fmt.Printf("Status:\t%s\n", info.Status)
	fmt.Printf("Created:\t%s\n", info.Created.Format(time.ANSIC))
	fmt.Printf("Updated:\t%s\n", info.LastUpdated.Format(time.ANSIC))
	fmt.Printf("Expires:\t%s\n", expiredTime(info.Expired))
	fmt.Printf("AutoRenew:\t%t\n", info.AutoRenew)
	fmt.Printf("Extensible:\t%t\n", info.Extensible)
	fmt.Printf("ExtensionInProcess:\t%t\n", info.ExtensionInProcess)
	fmt.Printf("Cancellable:\t%t\n", info.Cancellable)
	fmt.Printf("Request IP:\t%s\n", info.RequestIP)
	fmt.Println("FTP IPs:\t", info.FtpIps)
	fmt.Printf("Reason:\t%s\n", info.Reason)
	fmt.Printf("History:\n")
	for _, event := range info.History {
		fmt.Printf("\t%s\t%s\n", event.Timestamp.Format(time.ANSIC), event.Action)
	}
}

// printRequest displays a single request in tabular format for the list view.
// It formats request information in a single line with tab separators.
func printRequest(request czds.Request) {
	fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%t\n",
		request.TLD,
		request.RequestID,
		request.ULabel,
		request.Status,
		request.Created.Format(time.ANSIC),
		request.LastUpdated.Format(time.ANSIC),
		expiredTime(request.Expired),
		request.SFTP)
}

// printHeader displays the column headers for the tabular request list view.
func printHeader() {
	fmt.Printf("TLD\tID\tUnicodeTLD\tStatus\tCreated\tUpdated\tExpires\tSFTP\n")
}

// expiredTime formats a time for display, returning an empty string for zero times.
// This is used to format expiration dates that may not be set.
func expiredTime(t time.Time) string {
	if !t.IsZero() {
		return t.Format(time.ANSIC)
	}
	return ""
}
