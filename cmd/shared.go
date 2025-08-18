package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/lanrat/czds"
)

// GlobalFlags contains common flags that most commands need
type GlobalFlags struct {
	Username string
	Password string
	Passin   string
	Verbose  bool
}

// Command represents a subcommand with its own flag set and execution logic
type Command struct {
	Name        string
	Description string
	FlagSet     *flag.FlagSet
	Run         func(ctx context.Context) error
}

// addGlobalFlags adds the common authentication and verbose flags to a FlagSet.
// It checks for CZDS_USERNAME and CZDS_PASSWORD environment variables and uses them as defaults.
func addGlobalFlags(fs *flag.FlagSet, gf *GlobalFlags) {
	// Check for username and password from environment variables first
	defaultUsername := os.Getenv("CZDS_USERNAME")
	defaultPassword := os.Getenv("CZDS_PASSWORD")

	fs.StringVar(&gf.Username, "username", defaultUsername, "username to authenticate with (or set CZDS_USERNAME env var)")
	fs.StringVar(&gf.Password, "password", defaultPassword, "password to authenticate with (or set CZDS_PASSWORD env var)")
	fs.StringVar(&gf.Passin, "passin", "", "password source (default: prompt on tty; other options: cmd:command, env:var, file:path, keychain:name, lpass:name, op:name)")
	fs.BoolVar(&gf.Verbose, "verbose", false, "enable verbose logging")
}

// createClient creates a CZDS client using the global flags for authentication
func createClient(gf *GlobalFlags) (*czds.Client, error) {
	var password string
	var err error

	if gf.Passin != "" {
		password, err = Getpass(gf.Passin)
	} else if gf.Password != "" {
		password = gf.Password
	} else {
		password, err = Getpass()
	}

	if err != nil {
		return nil, err
	}

	client := czds.NewClient(gf.Username, password)

	if gf.Verbose {
		client.SetLogger(log.Default())
	}

	return client, nil
}

// checkRequiredFlags validates that required authentication flags are provided
func checkRequiredFlags(gf *GlobalFlags) error {
	if gf.Username == "" {
		return fmt.Errorf("username is required")
	}
	return nil
}

// getContext creates a context that cancels on SIGINT or SIGTERM signals for graceful shutdown.
func getContext() context.Context {
	// catch signals to end context
	ctx, cancel := context.WithCancel(context.Background())

	// Create a channel to listen for OS signals.
	sigs := make(chan os.Signal, 1)

	// Register the channel to receive SIGINT (Ctrl+C) and SIGTERM signals.
	// SIGTERM is often used for graceful shutdowns by process managers.
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	// Start a goroutine to handle the received signals.
	go func() {
		sig := <-sigs // Block until a signal is received
		log.Printf("\nReceived signal: %s. Performing graceful shutdown...\n", sig)
		signal.Stop(sigs) // allow a second signal to kill

		// Perform cleanup or other actions here before exiting.
		cancel()
	}()

	return ctx
}

// excludeListToMap parses a comma-separated exclude string into a map for O(1) lookups.
// It trims whitespace and converts to lowercase for case-insensitive matching.
func excludeListToMap(excludeStr string) map[string]bool {
	if excludeStr == "" {
		return nil
	}

	excludeList := strings.Split(excludeStr, ",")
	out := make(map[string]bool, len(excludeList))

	for _, s := range excludeList {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			out[strings.ToLower(trimmed)] = true
		}
	}
	return out
}
