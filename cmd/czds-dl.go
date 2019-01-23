package main

import (
	"flag"
	"log"
	"os"
	"path"
	"sync"

	"github.com/lanrat/czds"
)

var (
	// flags
	username        = flag.String("username", "", "username to authenticate with")
	password        = flag.String("password", "", "password to authenticate with")
	parallel        = flag.Uint("parallel", 5, "number of zones to download in parallel")
	authURL         = flag.String("authurl", czds.AuthURL, "authenticate url for JWT token")
	baseURL         = flag.String("baseurl", czds.BaseURL, "base URL for CZDS service")
	outDir          = flag.String("out", ".", "path to save downloaded zones to")
	urlName         = flag.Bool("urlname", false, "use the filename from the url link as the saved filename instead of the file header")
	forceRedownload = flag.Bool("redownload", false, "force redownloading the zone even if it already exists on local disk with same size and modification date")
	verbose         = flag.Bool("verbose", false, "enable verbose logging")

	loadDone  = make(chan bool)
	inputChan = make(chan czds.DownloadLink, 100)
	work      sync.WaitGroup
	client    *czds.Client
)

func v(format string, v ...interface{}) {
	if *verbose {
		log.Printf(format, v...)
	}
}

func checkFlags() {
	flag.Parse()
	flagError := false
	if *parallel < 1 {
		log.Printf("parallel must be positive")
		flagError = true
	}
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

	client = &czds.Client{
		AuthURL: *authURL,
		BaseURL: *baseURL,
		Creds: czds.Credentials{
			Username: *username,
			Password: *password,
		},
	}

	// validate credentials
	v("Authenticating to %s", client.AuthURL)
	err := client.Authenticate()
	if err != nil {
		log.Fatal(err)
	}

	// create output directory if it does not exist
	_, err = os.Stat(*outDir)
	if err != nil {
		if os.IsNotExist(err) {
			v("'%s' does not exist, creating", *outDir)
			err = os.MkdirAll(*outDir, 0770)
			if err != nil {
				log.Fatal(err)
			}
		} else {
			log.Fatal(err)
		}
	}

	// start the czds Client
	v("requesting download links")
	downloads, err := client.GetLinks()
	if err != nil {
		log.Fatal(err)
	}
	v("received %d zone links", len(downloads))

	// start workers
	go addLinks(downloads)
	v("starting %d parallel downloads", *parallel)
	for i := uint(0); i < *parallel; i++ {
		go worker()
	}

	// wait for workers to finish
	<-loadDone
	work.Wait()
}

func addLinks(downloads []czds.DownloadLink) {
	for _, dl := range downloads {
		work.Add(1)
		inputChan <- dl
	}
	close(inputChan)
	loadDone <- true
}

func worker() {
	for {
		dl, more := <-inputChan
		if more {
			// do work
			err := zoneDownload(dl)
			if err != nil {
				log.Fatal(err)
			}
			work.Done()
		} else {
			// done
			return
		}
	}
}

func zoneDownload(dl czds.DownloadLink) error {
	v("starting download '%s'", dl.URL)
	info, err := dl.GetInfo()
	if err != nil {
		return err
	}
	// use filename from url or header?
	localFileName := info.Filename
	if *urlName {
		localFileName = path.Base(dl.URL)
	}
	fullPath := path.Join(*outDir, localFileName)
	localFileInfo, err := os.Stat(fullPath)
	if *forceRedownload {
		v("forcing download of '%s'", dl.URL)
		return dl.Download(fullPath)
	}
	// check if local file already exists
	if err == nil {
		// check local file size
		if localFileInfo.Size() != info.ContentLength {
			// size differs, redownload
			v("size of local file (%d) differs from remote (%d), redownloading %s", localFileInfo.Size(), info.ContentLength, localFileName)
			return dl.Download(fullPath)
		}
		// check local file modification date
		if localFileInfo.ModTime().Before(info.LastModified) {
			// remote file is newer, redownload
			v("remote file is newer than local, redownloading")
			return dl.Download(fullPath)
		}
		// local copy is good, skip download
		v("local file '%s' matched remote, skipping", localFileName)
	}
	if os.IsNotExist(err) {
		// file does not exist, download
		return dl.Download(fullPath)
	}
	return err
}
