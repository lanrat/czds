package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	parallel   = flag.Uint("parallel", 5, "Number of concurrent downloads to run")
	token      = flag.String("token", "", "Authorization token for CZDS api")
	out        = flag.String("out", ".", "Path to save downloaded zones to")
	keepName   = flag.Bool("keepname", false, "Use filename from http header and not {ZONE}.zone.gz")
	redownload = flag.Bool("redownload", false, "Force redownloading zone if it already exists on local disk")

	errNoFile  = fmt.Errorf("Unknown Filename")
	filenameRe = regexp.MustCompile("\\d{8}-(.*?)-zone-data.txt.gz")
	loadDone   = make(chan bool)
	inputChan  = make(chan string, 100)
	work       sync.WaitGroup
	numZones   int
	savedZones int32
)

const (
	base     = "https://czds.icann.org"
	listPath = base + "/en/user-zone-data-urls.json?token=%s"
	timeout  = 600 * time.Second // 10 min
)

// given the filename from CZDS in the format {date}-{zone}-zone-data.txt.gz
// return zone
func zoneFromFilename(filename string) string {
	match := filenameRe.FindStringSubmatch(filename)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func main() {
	// check flags
	flag.Parse()
	if *parallel < 1 {
		fmt.Println("parallel must be a positive number")
		return
	}
	if token == nil || len(*token) == 0 {
		fmt.Println("Must pass authorization token")
		return
	}

	// create output directory if it does not exist
	_, err := os.Stat(*out)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(*out, 0770)
			if err != nil {
				fmt.Println(err)
				return
			}
		} else {
			fmt.Println(err)
			return
		}
	}

	// start worker threads
	go loadList()
	for i := uint(0); i < *parallel; i++ {
		go worker()
	}

	<-loadDone
	work.Wait()
	fmt.Printf("Saved %d/%d zones\n", savedZones, numZones)
}

// connect to CZDS, get the domain list, and add each url to the inputChan
func loadList() {
	list, err := getList()
	if err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
	//fmt.Printf("found %d zones\n", len(list))
	numZones = len(list)
	for _, url := range list {
		work.Add(1)
		inputChan <- url
	}
	close(inputChan)
	loadDone <- true
}

// worker
// gets from from the input queue and downloads the result
// gets new work till input chan is closed
func worker() {
	for {
		url, more := <-inputChan
		if more {
			// do work
			err := zoneDL(url)
			if err != nil {
				fmt.Println(err)
				os.Exit(2)
			} else {
				atomic.AddInt32(&savedZones, 1)
			}
			work.Done()
		} else {
			// done
			return
		}
	}
}

// given a full url, get the filename and size
// if there is not already a local file with the same size
// download it
func zoneDL(url string) error {
	httpClient := http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("%s (%d)", res.Status, res.StatusCode)
	}

	cd := res.Header.Get("Content-Disposition")
	if cd == "" {
		return errNoFile
	}
	hm := headerMap(cd)
	filenameHeader := hm["filename"]
	filename := filenameHeader

	cl := res.Header.Get("Content-Length")
	sizeHeader, err := strconv.ParseInt(cl, 10, 64)
	if err != nil {
		return err
	}

	if !*keepName {
		zone := zoneFromFilename(filenameHeader)
		if zone == "" {
			return fmt.Errorf("%s has no zone name", url)
		}

		filename = fmt.Sprintf("%s.zone.gz", zone)
	}
	fullPath := path.Join(*out, filename)

	// test file existence and size
	fi, err := os.Stat(fullPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		} /*else { // ELSE: file is new, download it
			//fmt.Printf("%s downloading\n", fullPath)
		}*/
	} else {
		if *redownload {
			if fi.Size() == sizeHeader {
				// file is already downloaded; skip it
				//fmt.Printf("%s already exists\n", fullPath)
				return nil
			}
			// ELSE file is wrong size, re-download
			fmt.Printf("%s is wrong size, re-downloading\n", fullPath)
		} else {
			return nil
		}
	}

	// start the file download
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	n, err := io.Copy(file, res.Body)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%s was empty", filename)
	}

	return nil
}

// given a HTTP header value, return a map of the value contents
func headerMap(data string) map[string]string {
	m := make(map[string]string)
	parts := strings.Split(data, ";")

	for _, v := range parts {
		v = strings.TrimSpace(v)
		vp := strings.SplitN(v, "=", 2)
		if len(vp) == 1 {
			m[vp[0]] = ""
		} else {
			s := strings.TrimSpace(vp[1])
			if len(s) > 0 && s[0] == '"' && s[len(s)-1] == '"' {
				s = s[1 : len(s)-1]
			}
			m[vp[0]] = s
		}
	}

	return m
}

// gets a list of zone URLs from the CZDS api
func getList() ([]string, error) {
	list := make([]string, 0, 10)
	httpClient := http.Client{
		Timeout: timeout,
	}

	url := fmt.Sprintf(listPath, *token)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return list, err
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return list, err
	}

	if res.StatusCode != 200 {
		return list, fmt.Errorf("%s (%d)", res.Status, res.StatusCode)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return list, err
	}

	err = json.Unmarshal(body, &list)
	if err != nil {
		return list, err
	}

	for i := range list {
		list[i] = base + list[i]
	}

	return list, nil
}
