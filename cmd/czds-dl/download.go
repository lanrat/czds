package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/lanrat/czds"
)

// flags
var (
	username    = flag.String("username", "", "username to authenticate with")
	password    = flag.String("password", "", "password to authenticate with")
	passin      = flag.String("passin", "", "password source (default: prompt on tty; other options: cmd:command, env:var, file:path, keychain:name, lpass:name, op:name)")
	parallel    = flag.Uint("parallel", 5, "number of zones to download in parallel")
	outDir      = flag.String("out", ".", "path to save downloaded zones to")
	urlName     = flag.Bool("urlname", false, "use the filename from the url link as the saved filename instead of the file header")
	force       = flag.Bool("force", false, "force redownloading the zone even if it already exists on local disk with same size and modification date")
	redownload  = flag.Bool("redownload", false, "redownload zones that are newer on the remote server than local copy")
	exclude     = flag.String("exclude", "", "don't fetch these zones")
	verbose     = flag.Bool("verbose", false, "enable verbose logging")
	retries     = flag.Uint("retries", 3, "max retry attempts per zone file download")
	zone        = flag.String("zone", "", "comma separated list of zones to download, defaults to all")
	quiet       = flag.Bool("quiet", false, "suppress progress printing")
	showVersion = flag.Bool("version", false, "print version and exit")
)

var (
	version   = "unknown"
	loadDone  = make(chan bool)
	inputChan = make(chan *zoneInfo, 100)
	work      sync.WaitGroup
	client    *czds.Client
)

type zoneInfo struct {
	Name     string
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
	if *showVersion {
		fmt.Printf("Version: %s\n", version)
		os.Exit(0)
	}
	flagError := false
	if *parallel < 1 {
		log.Printf("parallel must be positive")
		flagError = true
	}
	if len(*username) == 0 {
		log.Printf("must pass username")
		flagError = true
	}
	if len(*password) == 0 && len(*passin) == 0 {
		log.Printf("must pass either 'password' or 'passin'")
		flagError = true
	}
	if len(*zone) != 0 && len(*exclude) != 0 {
		log.Printf("'-zone' and '-exclude' cannot be combined")
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
	var downloads []string
	if *zone == "" {
		v("requesting download links")
		downloads, err = client.GetLinks()
		if err != nil {
			log.Fatal(err)
		}
		if len(*exclude) != 0 {
			downloads = pruneLinks(downloads)
		}
		v("received %d zone links", len(downloads))
	} else {
		// this url path is not known for sure to be constant and may break in the future
		for _, zoneName := range strings.Split(*zone, ",") {
			u, _ := url.Parse(czds.BaseURL)
			u.Path = path.Join(u.Path, "/czds/downloads/", fmt.Sprintf("%s.zone", strings.ToLower(zoneName)))
			downloads = append(downloads, u.String())
		}
	}

	// shuffle download links to better distribute load on CZDS
	downloads = shuffle(downloads)

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
		inputChan <- &zoneInfo{
			Name:  path.Base(dl),
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
					// requeue in another goroutine to prevent blocking
					go func() {
						inputChan <- zi
					}()
				} else {
					log.Printf("[%s] Max fail count hit; not downloading.", path.Base(zi.Dl))
					if _, err := os.Stat(zi.FullPath); !os.IsNotExist(err) {
						err = os.Remove(zi.FullPath)
						if err != nil {
							// log but continue; not fatal
							log.Printf("[%s] %s", zi.Dl, err)
						}
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

func zoneDownload(zi *zoneInfo) error {
	v("downloading '%s'", zi.Dl)
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
	if *force {
		v("forcing download of '%s'", zi.Dl)
		return downloadTime(zi)
	}
	// check if local file already exists
	if err == nil && *redownload {
		// check local file size
		if localFileInfo.Size() != info.ContentLength {
			// size differs, redownload
			v("size of local file (%d) differs from remote (%d), redownloading %s", localFileInfo.Size(), info.ContentLength, localFileName)
			return downloadTime(zi)
		}
		// check local file modification date
		if localFileInfo.ModTime().Before(info.LastModified) {
			// remote file is newer, redownload
			v("remote file is newer than local, redownloading")
			return downloadTime(zi)
		}
		// local copy is good, skip download
		v("local file '%s' matched remote, skipping", localFileName)
	}
	if os.IsNotExist(err) {
		// file does not exist, download
		return downloadTime(zi)
	}
	return err
}

// downloadTime downloads the zoneInfo and prints the time taken
func downloadTime(zi *zoneInfo) error {
	// file does not exist, download
	start := time.Now()
	err := client.DownloadZone(zi.Dl, zi.FullPath)
	if err != nil {
		return err
	}
	if !*quiet {
		delta := time.Since(start).Round(time.Millisecond)
		fmt.Printf("downloaded %s in %s\n", zi.Name, delta)
	}
	return nil
}

func shuffle(src []string) []string {
	final := make([]string, len(src))
	rand.Seed(time.Now().UTC().UnixNano())
	perm := rand.Perm(len(src))

	for i, v := range perm {
		final[v] = src[i]
	}
	return final
}

func pruneLinks(downloads []string) []string {
	newlist := []string{}
	for _, u := range downloads {
		found := false
		for _, e := range strings.Split(*exclude, ",") {
			sfx := fmt.Sprintf("%s.zone", e)
			if strings.HasSuffix(u, sfx) {
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
