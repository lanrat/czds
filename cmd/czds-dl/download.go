package main

import (
	"flag"
	"fmt"
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
	outDir          = flag.String("out", ".", "path to save downloaded zones to")
	urlName         = flag.Bool("urlname", false, "use the filename from the url link as the saved filename instead of the file header")
	forceRedownload = flag.Bool("redownload", false, "force redownloading the zone even if it already exists on local disk with same size and modification date")
	verbose         = flag.Bool("verbose", false, "enable verbose logging")
	retries         = flag.Uint("retries", 3, "max retry attempts per zone file download")

	loadDone  = make(chan bool)
	inputChan = make(chan *ZoneInfo, 100)
	work      sync.WaitGroup
	client    *czds.Client
)

type ZoneInfo struct {
	Dl       string
	FullPath string
	Count    int
}

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

	client = czds.NewClient(*username, *password)

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

func addLinks(downloads []string) {
	for _, dl := range downloads {
		work.Add(1)
		inputChan <- &ZoneInfo{
			Dl:    dl,
			Count: 1,
		}
	}	
	loadDone <- true
}

func worker() {
	for {
		zi, more := <-inputChan
		if more {
			// do work
			err := zoneDownload(zi)
			if err != nil {
				// don't stop on an error that only affects a single zone
				// fixes occasional HTTP 500s from CZDS
				v("[%s] err: %s", path.Base(zi.Dl), err)
				zi.Count++
				if uint(zi.Count) < *retries {
					work.Add(1)
					inputChan <- zi // requeue
				} else {
					log.Printf("[%s] Max fail count hit; not downloading.", path.Base(zi.Dl))
					err = os.Remove(zi.FullPath)
					if err != {
						// log but continue; not fatal
						log.Printf("[%s] %s", zi.Dl, err)
					}
				}
			}
			work.Done()
		} else {
			// done
			return
		}
	}
}

func zoneDownload(zi *ZoneInfo) error {
	v("starting download '%s'", zi.Dl)
	info, err := client.GetDownloadInfo(zi.Dl)
	if err != nil {
		return fmt.Errorf("%s [%s]", err, zi.Dl)
	}
	// use filename from url or header?
	localFileName := info.Filename
	if *urlName {
		localFileName = path.Base(zi.Dl)
	}
	zi.FullPath = path.Join(*outDir, localFileName)
	localFileInfo, err := os.Stat(zi.FullPath)
	if *forceRedownload {
		v("forcing download of '%s'", zi.Dl)
		return client.DownloadZone(zi.Dl, zi.FullPath)
	}
	// check if local file already exists
	if err == nil {
		// check local file size
		if localFileInfo.Size() != info.ContentLength {
			// size differs, redownload
			v("size of local file (%d) differs from remote (%d), redownloading %s", localFileInfo.Size(), info.ContentLength, localFileName)
			return client.DownloadZone(zi.Dl, zi.FullPath)
		}
		// check local file modification date
		if localFileInfo.ModTime().Before(info.LastModified) {
			// remote file is newer, redownload
			v("remote file is newer than local, redownloading")
			return client.DownloadZone(zi.Dl, zi.FullPath)
		}
		// local copy is good, skip download
		v("local file '%s' matched remote, skipping", localFileName)
	}
	if os.IsNotExist(err) {
		// file does not exist, download
		return client.DownloadZone(zi.Dl, zi.FullPath)
	}
	return err
}
