package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/lanrat/czds"
)

var (
	// flags
	username = flag.String("username", "", "username to authenticate with")
	password = flag.String("password", "", "password to authenticate with")
	verbose  = flag.Bool("verbose", false, "enable verbose logging")
	id       = flag.String("id", "", "ID of specific zone request to lookup, defaults to printing all")
	zone     = flag.String("zone", "", "same as -id, but prints the request by zone name")
	cancel   = flag.Bool("cancel", false, "cancel the request. Requires -id or -zone")

	client *czds.Client
)

const (
	pageSize = 100
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

	client = czds.NewClient(*username, *password)

	// validate credentials
	v("Authenticating to %s", client.AuthURL)
	err := client.Authenticate()
	if err != nil {
		log.Fatal(err)
	}

	if *zone != "" {
		// get id from zone name
		zoneID, err := getZoneRequestID(*zone)
		if err != nil {
			log.Fatal(err)
		}
		id = &zoneID
	}

	if *cancel {
		if *id == "" {
			log.Fatal("Need to pass ID or Zone to cancel request")
		}
		// get zone name/id/info from ID
		info, err := client.GetRequestInfo(*id)
		if err != nil {
			log.Fatal(err)
		}
		cancelRequest(info.RequestID, info.TLD.TLD)
		return
	}

	if *id == "" {
		listAll()
		return
	}

	info, err := client.GetRequestInfo(*id)
	if err != nil {
		log.Fatal(err)
	}
	printRequestInfo(info)
}

func cancelRequest(id, zone string) {
	cancelRequest := &czds.CancelRequestSubmission{
		RequestID: id,
		TLDName:   zone,
	}
	info, err := client.CancelRequest(cancelRequest)
	if err != nil {
		log.Fatal(err)
	}
	printRequestInfo(info)
}

func getZoneRequestID(zone string) (string, error) {
	filter := czds.RequestsFilter{
		Status: czds.RequestAll,
		Filter: strings.ToLower(zone),
		Pagination: czds.RequestsPagination{
			Size: 1,
			Page: 0,
		},
		Sort: czds.RequestsSort{
			Field:     czds.SortByLastUpdated,
			Direction: czds.SortDesc,
		},
	}
	requests, err := client.GetRequests(&filter)
	if err != nil {
		return "", err
	}
	if requests.TotalRequests == 0 {
		return "", fmt.Errorf("no request found for zone %s", zone)
	}
	return requests.Requests[0].RequestID, nil
}

func printRequestInfo(info *czds.RequestsInfo) {
	fmt.Printf("ID:\t%s\n", info.RequestID)
	fmt.Printf("TLD:\t%s (%s)\n", info.TLD.TLD, info.TLD.ULabel)
	fmt.Printf("Status:\t%s\n", info.Status)
	fmt.Printf("Created:\t%s\n", info.Created.Format(time.ANSIC))
	fmt.Printf("Updated:\t%s\n", info.LastUpdated.Format(time.ANSIC))
	fmt.Printf("Expires:\t%s\n", expiredTime(info.Expired))
	fmt.Printf("Request IP:\t%s\n", info.RequestIP)
	fmt.Println("FTP IPs:\t", info.FtpIps)
	fmt.Printf("Reason:\t%s\n", info.Reason)
	fmt.Printf("History:\n")
	for _, event := range info.History {
		fmt.Printf("\t%s\t%s\n", event.Timestamp.Format(time.ANSIC), event.Action)
	}
}

func listAll() {
	filter := czds.RequestsFilter{
		Status: czds.RequestAll,
		Filter: "",
		Pagination: czds.RequestsPagination{
			Size: pageSize,
			Page: 0,
		},
		Sort: czds.RequestsSort{
			Field:     czds.SortByCreated,
			Direction: czds.SortDesc,
		},
	}

	requests, err := client.GetRequests(&filter)
	if err != nil {
		log.Fatal(err)
	}

	v("Total requests: %d", requests.TotalRequests)
	if len(requests.Requests) > 0 {
		printHeader()
	}
	for len(requests.Requests) != 0 {
		for _, request := range requests.Requests {
			printRequest(request)
		}
		filter.Pagination.Page++
		requests, err = client.GetRequests(&filter)
		if err != nil {
			log.Fatal(err)
		}
	}
}

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

func printHeader() {
	fmt.Printf("TLD\tID\tUnicodeTLD\tStatus\tCreated\tUpdated\tExpires\tSFTP\n")
}

func expiredTime(t time.Time) string {
	if t.Unix() > 0 {
		return t.Format(time.ANSIC)
	}
	return ""
}
