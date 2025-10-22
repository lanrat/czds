package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/lanrat/czds"
)

// RequestConfig holds the configuration for the request command.
// It contains settings for requesting access to zones, extending existing requests,
// canceling requests, and viewing terms and status information.
type RequestConfig struct {
	Reason      string // Reason text required for zone access requests
	PrintTerms  bool   // Print terms of use and exit
	RequestTLDs string // Comma-separated list of TLDs to request
	RequestAll  bool   // Request access to all available TLDs
	Status      bool   // Show status of TLD requests
	ExtendTLDs  string // Comma-separated list of TLDs to extend
	ExtendAll   bool   // Extend all expiring TLD requests
	Exclude     string // Comma-separated list of TLDs to exclude from operations
	CancelTLDs  string // Comma-separated list of TLD requests to cancel
}

// requestCmd creates and configures the request subcommand for managing CZDS zone requests.
// It sets up command-line flags and returns a Command that handles zone access requests, extensions, and cancellations.
func requestCmd() *Command {
	var gf GlobalFlags
	var config RequestConfig

	fs := flag.NewFlagSet("request", flag.ExitOnError)

	// Add global flags
	addGlobalFlags(fs, &gf)

	// Add request-specific flags
	fs.StringVar(&config.Reason, "reason", "", "reason to request zone access")
	fs.BoolVar(&config.PrintTerms, "terms", false, "print CZDS Terms & Conditions")
	fs.StringVar(&config.RequestTLDs, "request", "", "comma separated list of zones to request")
	fs.BoolVar(&config.RequestAll, "request-all", false, "request all available zones")
	fs.BoolVar(&config.Status, "status", false, "print status of zones")
	fs.StringVar(&config.ExtendTLDs, "extend", "", "comma separated list of zones to request extensions")
	fs.BoolVar(&config.ExtendAll, "extend-all", false, "extend all possible zones")
	fs.StringVar(&config.Exclude, "exclude", "", "comma separated list of zones to exclude from request-all or extend-all")
	fs.StringVar(&config.CancelTLDs, "cancel", "", "comma separated list of zones to cancel outstanding requests for")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: czds request [OPTIONS]\n\n")
		fmt.Fprintf(os.Stderr, "Request access to zones, extensions, cancellations\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  czds request -terms                              # Print terms and conditions\n")
		fmt.Fprintf(os.Stderr, "  czds request -status                             # Show TLD status\n")
		fmt.Fprintf(os.Stderr, "  czds request -request com,org -reason \"Research\" # Request specific TLDs\n")
		fmt.Fprintf(os.Stderr, "  czds request -request-all -reason \"Research\"     # Request all available TLDs\n")
		fmt.Fprintf(os.Stderr, "  czds request -extend com,org                     # Extend specific TLDs\n")
		fmt.Fprintf(os.Stderr, "  czds request -extend-all                         # Extend all possible TLDs\n")
		fmt.Fprintf(os.Stderr, "  czds request -cancel com,org                     # Cancel requests for TLDs\n")
	}

	return &Command{
		Name:        "request",
		Description: "Request access to zones, extensions, cancellations",
		FlagSet:     fs,
		Run: func(ctx context.Context) error {
			if err := fs.Parse(os.Args[2:]); err != nil {
				return fmt.Errorf("failed to parse flags: %w", err)
			}

			// Validate flags
			if err := checkRequiredFlags(&gf); err != nil {
				return err
			}

			doRequest := (config.RequestAll || len(config.RequestTLDs) > 0)
			doExtend := (config.ExtendAll || len(config.ExtendTLDs) > 0)
			doCancel := len(config.CancelTLDs) > 0

			if !config.PrintTerms && !config.Status && !doRequest && !doExtend && !doCancel {
				return fmt.Errorf("nothing to do! Must specify one of: -terms, -status, -request/-request-all, -extend/-extend-all, or -cancel")
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

			return runRequest(ctx, client, &config, gf.Verbose)
		},
	}
}

// runRequest executes the request command logic based on the provided configuration.
// It handles printing terms, showing TLD status, and processing requests, extensions, and cancellations.
func runRequest(ctx context.Context, client *czds.Client, config *RequestConfig, verbose bool) error {
	excludeMap := excludeListToMap(config.Exclude)

	// Print terms
	if config.PrintTerms {
		if err := printTerms(ctx, client, verbose); err != nil {
			return err
		}
	}

	// Print status
	if config.Status {
		if err := printTLDStatus(ctx, client); err != nil {
			return err
		}
	}

	// Handle requests
	if config.RequestAll || len(config.RequestTLDs) > 0 {
		if err := handleRequests(ctx, client, config, excludeMap, verbose); err != nil {
			return err
		}
	}

	// Handle extensions
	if config.ExtendAll || len(config.ExtendTLDs) > 0 {
		if err := handleExtensions(ctx, client, config, excludeMap, verbose); err != nil {
			return err
		}
	}

	// Handle cancellations
	if len(config.CancelTLDs) > 0 {
		if err := handleCancellations(ctx, client, config, verbose); err != nil {
			return err
		}
	}

	return nil
}

// printTerms retrieves and displays the current CZDS terms and conditions.
func printTerms(ctx context.Context, client *czds.Client, verbose bool) error {
	terms, err := client.GetTermsWithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to get terms: %w", err)
	}

	if verbose {
		fmt.Printf("Terms Version %s\n", terms.Version)
	}
	fmt.Println("Terms and Conditions:")
	fmt.Println(terms.Content)

	return nil
}

// printTLDStatus retrieves and displays the current status of all TLD requests.
func printTLDStatus(ctx context.Context, client *czds.Client) error {
	allTLDStatus, err := client.GetTLDStatusWithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to get TLD status: %w", err)
	}

	for _, tldStatus := range allTLDStatus {
		fmt.Printf("%s\t%s\n", tldStatus.TLD, tldStatus.CurrentStatus)
	}

	return nil
}

// handleRequests processes zone access requests for specific TLDs or all available TLDs.
// It requires a reason to be provided for the request.
func handleRequests(ctx context.Context, client *czds.Client, config *RequestConfig, excludeMap map[string]bool, verbose bool) error {
	if len(config.Reason) == 0 {
		return fmt.Errorf("must pass a reason to request TLDs")
	}

	var requestedTLDs []string
	var err error

	if config.RequestAll {
		if verbose {
			fmt.Println("Requesting all TLDs")
		}
		// Convert map to slice for library function
		excludeList := mapToSlice(excludeMap)
		requestedTLDs, err = client.RequestAllTLDsExceptWithContext(ctx, config.Reason, excludeList)
	} else {
		tlds := strings.Split(config.RequestTLDs, ",")
		if verbose {
			fmt.Printf("Requesting %v\n", tlds)
		}
		err = client.RequestTLDsWithContext(ctx, tlds, config.Reason)
		requestedTLDs = tlds
	}

	if err != nil {
		return fmt.Errorf("failed to request TLDs: %w", err)
	}

	if len(requestedTLDs) > 0 {
		fmt.Printf("Requested: %v\n", requestedTLDs)
	}

	return nil
}

// handleExtensions processes extension requests for TLDs based on the configuration.
// It can extend all available TLDs or specific TLDs listed in the configuration.
func handleExtensions(ctx context.Context, client *czds.Client, config *RequestConfig, excludeMap map[string]bool, verbose bool) error {
	var extendedTLDs []string
	var err error

	if config.ExtendAll {
		if verbose {
			fmt.Println("Requesting extension for all TLDs")
		}
		// Convert map to slice for library function
		excludeList := mapToSlice(excludeMap)
		extendedTLDs, err = client.ExtendAllTLDsExceptWithContext(ctx, excludeList)
	} else {
		tlds := strings.Split(config.ExtendTLDs, ",")
		for _, tld := range tlds {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if verbose {
				fmt.Printf("Requesting extension %v\n", tld)
			}
			err = client.ExtendTLDWithContext(ctx, tld)
			if err != nil {
				// stop on first error
				break
			}
		}
		extendedTLDs = tlds
	}

	if err != nil {
		return fmt.Errorf("failed to extend TLDs: %w", err)
	}

	if len(extendedTLDs) > 0 {
		fmt.Printf("Extended: %v\n", extendedTLDs)
	}

	return nil
}

// handleCancellations processes cancellation requests for the TLDs specified in the configuration.
// It cancels pending requests for each TLD listed in config.CancelTLDs.
func handleCancellations(ctx context.Context, client *czds.Client, config *RequestConfig, verbose bool) error {
	tlds := strings.Split(config.CancelTLDs, ",")
	for _, tld := range tlds {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if verbose {
			fmt.Printf("Requesting cancellation %v\n", tld)
		}
		err := cancelRequest(ctx, client, tld)
		if err != nil {
			return fmt.Errorf("failed to cancel request for %s: %w", tld, err)
		}
	}

	if len(tlds) > 0 {
		fmt.Printf("Canceled: %v\n", tlds)
	}

	return nil
}

// cancelRequest cancels a pending zone access request for the specified zone.
// It looks up the request ID for the zone and submits a cancellation request.
func cancelRequest(ctx context.Context, client *czds.Client, zone string) error {
	zoneID, err := client.GetZoneRequestIDWithContext(ctx, zone)
	if err != nil {
		return err
	}

	cancelRequest := &czds.CancelRequestSubmission{
		RequestID: zoneID,
		TLDName:   zone,
	}

	_, err = client.CancelRequestWithContext(ctx, cancelRequest)
	if err != nil {
		return err
	}

	return nil
}
