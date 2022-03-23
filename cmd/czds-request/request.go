package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/lanrat/czds"
)

var (
	// flags
	username    = flag.String("username", "", "username to authenticate with")
	password    = flag.String("password", "", "password to authenticate with")
	verbose     = flag.Bool("verbose", false, "enable verbose logging")
	reason      = flag.String("reason", "", "reason to request zone access")
	printTerms  = flag.Bool("terms", false, "print CZDS Terms & Conditions")
	requestTLDs = flag.String("request", "", "comma separated list of zones to request")
	requestAll  = flag.Bool("request-all", false, "request all available zones")
	status      = flag.Bool("status", false, "print status of zones")

	client *czds.Client
)

func v(format string, v ...interface{}) {
	if *verbose {
		log.Printf(format, v...)
	}
}

func checkFlags() {
	flag.Parse()
	flagError := false
	if len(*username) == 0 {
		log.Printf("must pass username")
		flagError = true
	}
	if len(*password) == 0 {
		log.Printf("must pass password")
		flagError = true
	}
	if flagError {
		flag.PrintDefaults()
		os.Exit(1)
	}
}

func main() {
	checkFlags()

	doRequest := (*requestAll || len(*requestTLDs) > 0)
	if !*printTerms && !*status && !doRequest {
		log.Fatal("Nothing to do!")
	}

	client = czds.NewClient(*username, *password)

	// validate credentials
	v("Authenticating to %s", client.AuthURL)
	err := client.Authenticate()
	if err != nil {
		log.Fatal(err)
	}

	// print terms
	if *printTerms {
		terms, err := client.GetTerms()
		if err != nil {
			log.Fatal(err)
		}
		v("Terms Version %s", terms.Version)
		fmt.Println("Terms and Conditions:")
		fmt.Println(terms.Content)
	}

	// print status
	if *status {
		allTLDStatus, err := client.GetTLDStatus()
		if err != nil {
			log.Fatal(err)
		}
		for _, tldStatus := range allTLDStatus {
			printTLDStatus(tldStatus)
		}
	}

	// request
	if doRequest {
		if len(*reason) == 0 {
			log.Fatal("Must pass a reason to request TLDs")
		}
		var requestedTLDs []string
		if *requestAll {
			v("Requesting All TLDs")
			requestedTLDs, err = client.RequestAllTLDs(*reason)
		} else {
			tlds := strings.Split(*requestTLDs, ",")
			v("Requesting %v", tlds)
			err = client.RequestTLDs(tlds, *reason)
			requestedTLDs = tlds
		}

		if err != nil {
			log.Fatal(err)
		}
		if len(requestedTLDs) > 0 {
			fmt.Printf("Requested: %v\n", requestedTLDs)
		}
	}
}

func printTLDStatus(tldStatus czds.TLDStatus) {
	fmt.Printf("%s\t%s\n", tldStatus.TLD, tldStatus.CurrentStatus)
}
