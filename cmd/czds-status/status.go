package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"

	"github.com/lanrat/czds"
)

// flags
var (
	username    = flag.String("username", "", "username to authenticate with")
	password    = flag.String("password", "", "password to authenticate with")
	passin      = flag.String("passin", "", "password source (default: prompt on tty; other options: cmd:command, env:var, file:path, keychain:name, lpass:name, op:name)")
	verbose     = flag.Bool("verbose", false, "enable verbose logging")
	id          = flag.String("id", "", "ID of specific zone request to lookup, defaults to printing all")
	zone        = flag.String("zone", "", "same as -id, but prints the request by zone name")
	showVersion = flag.Bool("version", false, "print version and exit")
	report      = flag.String("report", "", "filename to save report CSV to, '-' for stdout")
)

var (
	version = "unknown"
	client  *czds.Client
)

func checkFlags() {
	flag.Parse()
	if *showVersion {
		fmt.Printf("Version: %s\n", version)
		os.Exit(0)
	}
	flagError := false
	if len(*username) == 0 {
		log.Printf("must pass username")
		flagError = true
	}
	if len(*password) == 0 && len(*passin) == 0 {
		log.Printf("must pass either 'password' or 'passin'")
		flagError = true
	}
	if (len(*report) > 0) && ((*id != "") || (*zone != "")) {
		log.Printf("can not use -report with specific zone request")
		flagError = true
	}
	if flagError {
		flag.PrintDefaults()
		os.Exit(1)
	}
}

func main() {
	checkFlags()

	p := *password
	if len(p) == 0 {
		pass, err := czds.Getpass(*passin)
		if err != nil {
			log.Fatal("Unable to get password from user: ", err)
		}
		p = pass
	}

	client = czds.NewClient(*username, p)
	if *verbose {
		client.SetLogger(log.Default())
	}

	// validate credentials
	v("Authenticating to %s", client.AuthURL)
	err := client.Authenticate()
	if err != nil {
		log.Fatal(err)
	}

	if *zone != "" {
		// get id from zone name
		zoneID, err := client.GetZoneRequestID(*zone)
		if err != nil {
			log.Fatal(err)
		}
		id = &zoneID
	}

	// save CSV report
	if len(*report) > 0 {
		csvReport()
		return
	}

	// list status of all zones
	if *id == "" {
		listAll()
		return
	}

	// list details of a single zone request
	info, err := client.GetRequestInfo(*id)
	if err != nil {
		log.Fatal(err)
	}
	printRequestInfo(info)
}

func listAll() {
	requests, err := client.GetAllRequests(czds.RequestAll)
	if err != nil {
		log.Fatal(err)
	}

	v("Total requests: %d", len(requests))
	if len(requests) > 0 {
		printHeader()
		for _, request := range requests {
			printRequest(request)
		}
	}
}

func csvReport() {
	out := os.Stdout
	if *report != "-" {
		v("Saving to %s", *report)
		dir := path.Dir(*report)
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
		file, err := os.Create(*report)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		out = file
	} else {
		v("Printing to StdOut")
	}

	// CSV report to out
	err := client.DownloadAllRequests(out)
	if err != nil {
		log.Fatal(err)
	}
}
