# CZDS

[![Go Report Card](https://goreportcard.com/badge/github.com/lanrat/czds)](https://goreportcard.com/report/lanrat/czds)
[![](https://godoc.org/github.com/lanrat/czds?status.svg)](https://godoc.org/github.com/lanrat/czds)

A utility and golang library implementing a client to the [CZDS REST API](https://github.com/icann/czds-api-client-java/blob/master/docs/ICANN_CZDS_api.pdf)
using both the documented and undocumented API endpoints

Should allow you to perform almost any action you can in the web interface via this API

## CZDS-DL

Implements a client for the officially documented [CZDS REST API](https://github.com/icann/czds-api-client-java/blob/master/docs/ICANN_CZDS_api.pdf)

### Download zone files from [czds.icann.org](https://czds.icann.org) in parallel

### Features

 * Can be used as a standalone client or as an API for another client
 * Automatically refreshes authorization token if expired during download
 * Can save downloaded zones as named by `Content-Disposition` or URL name
 * Can compare local and remote files size and modification time to skip redownloading unchanged zones
 * Can download multiple zones in parallel
 * [Docker](#docker) image available

### Usage
```
Usage of ./czds-dl:
  -authurl string
        authenticate url for JWT token (default "https://account-api.icann.org/api/authenticate")
  -baseurl string
        base URL for CZDS service (default "https://czds-api.icann.org/")
  -out string
        path to save downloaded zones to (default ".")
  -parallel uint
        number of zones to download in parallel (default 5)
  -password string
        password to authenticate with
  -redownload
        force redownloading the zone even if it already exists on local disk with same size and modification date
  -urlname
        use the filename from the url link as the saved filename instead of the file header
  -username string
        username to authenticate with
  -verbose
        enable verbose logging
```

### Example
```
$ ./czds-dl -out /zones -username "$USERNAME" -password "$PASSWORD" -verbose
2019/01/12 16:23:51 Authenticating to https://account-api.icann.org/api/authenticate
2019/01/12 16:23:52 'zones' does not exist, creating
2019/01/12 16:23:52 requesting download links
2019/01/12 16:23:54 received 5 zone links
2019/01/12 16:23:54 starting 5 parallel downloads
2019/01/12 16:23:54 downloading 'https://czds-api.icann.org/czds/downloads/example2.zone'
2019/01/12 16:23:54 downloading 'https://czds-api.icann.org/czds/downloads/example4.zone'
2019/01/12 16:23:54 downloading 'https://czds-api.icann.org/czds/downloads/example1.zone'
2019/01/12 16:23:54 downloading 'https://czds-api.icann.org/czds/downloads/example3.zone'
2019/01/12 16:23:54 downloading 'https://czds-api.icann.org/czds/downloads/example5.zone'
```

## CZDS-REPORT

Download the CSV report for current zone status

### Usage
```
Usage of ./czds-report:
  -authurl string
        authenticate url for JWT token (default "https://account-api.icann.org/api/authenticate")
  -baseurl string
        base URL for CZDS service (default "https://czds-api.icann.org")
  -file string
        filename to save report to, '-' for stdout (default "report.csv")
  -password string
        password to authenticate with
  -username string
        username to authenticate with
  -verbose
        enable verbose logging
```

### Example
```
$ ./czds-report -username "$USERNAME" -password "$PASSWORD" -verbose -file report.csv
2019/02/02 17:43:37 Authenticating to https://account-api.icann.org/api/authenticate
2019/02/02 17:43:38 Saving to report.csv
```

## Building

Just run make!
Building from source requires go >= 1.11 for module support

```
$ make
```

## [Docker](https://hub.docker.com/r/lanrat/czds-dl/)

```
docker run --rm -v /path/to/zones/:/zones lanrat/czds-dl czds-dl -out /zones -username "$USERNAME" -password "$PASSWORD"
```
