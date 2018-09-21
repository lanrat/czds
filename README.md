# CZDS-DL

## Download zone files from [czds.icann.org](https://czds.icann.org) in parallel

### Usage
```
Usage of ./czds-dl:
  -keepname
        Use filename from http header and not {ZONE}.zone.gz
  -out string
        Path to save downloaded zones to (default ".")
  -parallel uint
        Number of concurrent downloads to run (default 5)
  -token string
        Authorization token for CZDS api
```

### Example
```
$ ./czds-dl -token $API_TOKEN -out ~/zones/
Saved 1212/1212 zones
```

### Building

Install a recent version of go

```
$ make
go build -o czds-dl czds-dl.go
```

### [Docker](https://hub.docker.com/r/lanrat/czds-dl/)

```
docker run --rm -v /path/to/zones/:/zones lanrat/czds-dl -token $API_TOKEN
```
