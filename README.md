# CZDS-DL

## Download zone files from [czds.icann.org](https://czds.icann.org) in parallel

Implements a client for the [CZDS REST API](https://github.com/icann/czds-api-client-java/blob/master/docs/ICANN_CZDS_api.pdf)

## Features

 * Can be used as a standalone client or as an API for another client
 * Automatically refreshes authorization token if expired during download
 * Can save download zones as names by `Content-Disposition` or URL name
 * Can compare local and remote files size and modification time to skip redownloading unchanged zones
 * Can download multiple zones in parallel
 * [Docker](#docker) image available

### Usage
```
Usage of ./czds-dl:
  -authurl string
        authenticate url for JWT token (default "https://account-api-test.icann.org/api/authenticate")
  -baseurl string
        base URL for CZDS service (default "https://czds-api-test.icann.org")
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
2019/01/12 16:23:51 Authenticating to https://account-api-test.icann.org/api/authenticate
2019/01/12 16:23:52 'zones' does not exist, creating
2019/01/12 16:23:52 requesting download links
2019/01/12 16:23:54 received 5 zone links
2019/01/12 16:23:54 starting 5 parallel downloads
2019/01/12 16:23:54 attempting to download 'https://czds-api-test.icann.org/czds/downloads/example2.zone'
2019/01/12 16:23:54 attempting to download 'https://czds-api-test.icann.org/czds/downloads/example4.zone'
2019/01/12 16:23:54 attempting to download 'https://czds-api-test.icann.org/czds/downloads/example1.zone'
2019/01/12 16:23:54 attempting to download 'https://czds-api-test.icann.org/czds/downloads/example3.zone'
2019/01/12 16:23:54 attempting to download 'https://czds-api-test.icann.org/czds/downloads/example5.zone'
```

### Building

Building from source requires go > 1.11 for module support

```
$ make
go build -o czds-dl cmd/czds-dl.go
```

### [Docker](https://hub.docker.com/r/lanrat/czds-dl/)

```
docker run --rm -v /path/to/zones/:/zones lanrat/czds-dl -out /zones -username "$USERNAME" -password "$PASSWORD"
```
