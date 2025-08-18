# CZDS

[![Go Report Card](https://goreportcard.com/badge/github.com/lanrat/czds)](https://goreportcard.com/report/lanrat/czds)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/lanrat/czds)](https://pkg.go.dev/github.com/lanrat/czds)
[![docker](https://github.com/lanrat/czds/actions/workflows/docker.yml/badge.svg)](https://github.com/lanrat/czds/actions/workflows/docker.yml)

A utility and Go library implementing a client to the [CZDS REST API](https://github.com/icann/czds-api-client-java/blob/master/docs/ICANN_CZDS_api.pdf),
using both the documented and undocumented API endpoints.

## [CZDS API](https://pkg.go.dev/github.com/lanrat/czds)

The Go API allows you to perform almost any action available in the web interface. See the [API documentation](https://pkg.go.dev/github.com/lanrat/czds) for details.

## CZDS CLI

A unified command-line interface that provides all functionality through subcommands:

- `download` (alias: `dl`) - Download zone files from [czds.icann.org](https://czds.icann.org)
- `request` (alias: `req`) - Submit new zone requests or modify existing ones
- `status` (alias: `st`) - View information about zone file requests
- `version` - Print version information

### Features

- Can be used as a standalone client or embedded as a library in other applications
- Automatically refreshes authorization token if expired during download
- Can save downloaded zones as named by `Content-Disposition` or URL name
- Can compare local and remote file size and modification time to skip redownloading unchanged zones
- Can download multiple zones in parallel
- [Docker](#docker) image available

### Usage

```console
czds - CZDS (Centralized Zone Data Service) client

Usage:
  czds <command> [options]

Available Commands:
  download, dl    Download zone files from CZDS
  request, req    Request access to zones, extensions, cancellations
  status, st      Check status of zone requests and generate reports
  version         Print version information
  help            Show this help message

Use "czds <command> -h" for more information about a command.

Global Options:
  -username string    Username to authenticate with (or set CZDS_USERNAME env var)
  -password string    Password to authenticate with (or set CZDS_PASSWORD env var)
  -verbose            Enable verbose logging

Examples:
  czds download -parallel 10 com org
  czds request -request-all -reason "Research project"
  czds status -zone com
```

### Authentication

The czds command supports multiple authentication methods:

1. **Command-line flags**: `-username` and `-password`
2. **Environment variables**: `CZDS_USERNAME` and `CZDS_PASSWORD`

Environment variables are checked first and used as defaults if the corresponding flags are not provided.

## Download Subcommand

Download zone files from CZDS in parallel.

Zones can be specified either using the `-zones` flag with a comma-separated list, or as positional arguments.

### Download Usage

```console
Usage: czds download [OPTIONS] [zones...]

Download zone files from CZDS

Options:
  -exclude string
     don't fetch these zones
  -force
     force redownloading the zone even if it already exists on local disk with same size and modification date
  -out string
     path to save downloaded zones to (default "zones")
  -parallel uint
     number of zones to download in parallel (default 5)
  -password string
     password to authenticate with (or set CZDS_PASSWORD env var)
  -progress
     show download progress for large files (>50MB)
  -quiet
     suppress progress printing
  -redownload
     redownload zones that are newer on the remote server than local copy
  -retries uint
     max retry attempts per zone file download (default 3)
  -urlname
     use the filename from the url link as the saved filename instead of the file header
  -username string
     username to authenticate with (or set CZDS_USERNAME env var)
  -verbose
     enable verbose logging
  -zones string
     comma separated list of zones to download, defaults to all
```

### Download Examples

```shell
czds download                                # Download all available zones
czds download -zones com,org                 # Download specific zones
czds download -parallel 10 -out ./zones     # Download with 10 parallel workers
czds download -force -zones com              # Force redownload of com zone
czds download -exclude com,net               # Download all except com and net
czds download -progress -zones com           # Download with progress reporting

# Zones can also be specified as positional arguments:
czds download com org net                    # Download com, org, and net zones

# Using environment variables:
export CZDS_USERNAME="your_username"
export CZDS_PASSWORD="your_password"
czds download -verbose
```

## Request Subcommand

Submit a new zone request or modify an existing CZDS request. Be sure to view and accept the terms and conditions with the `-terms` flag.

### Request Usage

```text
Usage: czds request [OPTIONS]

Request access to zones, extensions, cancellations

Options:
  -cancel string
     comma separated list of zones to cancel outstanding requests for
  -exclude string
     comma separated list of zones to exclude from request-all or extend-all
  -extend string
     comma separated list of zones to request extensions
  -extend-all
     extend all possible zones
  -password string
     password to authenticate with (or set CZDS_PASSWORD env var)
  -reason string
     reason to request zone access
  -request string
     comma separated list of zones to request
  -request-all
     request all available zones
  -status
     print status of zones
  -terms
     print CZDS Terms & Conditions
  -username string
     username to authenticate with (or set CZDS_USERNAME env var)
  -verbose
     enable verbose logging
```

### Request Examples

```text
czds request -terms                              # Print terms and conditions
czds request -status                             # Show TLD status
czds request -request com,org -reason "Research" # Request specific TLDs
czds request -request-all -reason "Research"     # Request all available TLDs
czds request -extend com,org                     # Extend specific TLDs
czds request -extend-all                         # Extend all possible TLDs
czds request -cancel com,org                     # Cancel requests for TLDs

# View zones able to be requested:
czds request -status | grep -v pending | grep -v approved
```

## Status Subcommand

View information about current zone file requests

### Status Usage

By default the status subcommand prints high-level information about all CZDS requests, like the [reports page](https://czds.icann.org/zone-requests/all) on CZDS.
Detailed information about a particular zone can be displayed with the `-zone` or `-id` flag.

```text
Usage: czds status [OPTIONS]

Check status of zone requests and generate reports

Options:
  -id string
     ID of specific zone request to lookup, defaults to printing all
  -password string
     password to authenticate with (or set CZDS_PASSWORD env var)
  -progress
     show download progress for CSV reports
  -report string
     filename to save report CSV to, '-' for stdout
  -username string
     username to authenticate with (or set CZDS_USERNAME env var)
  -verbose
     enable verbose logging
  -zone string
     same as -id, but prints the request by zone name
```

### Status Examples

```text
czds status                          # List all requests
czds status -zone com                # Show details for com zone
czds status -id REQUEST_ID           # Show details for specific request
czds status -report report.csv       # Generate CSV report
czds status -report report.csv -progress # Generate CSV with progress
```

Show all requests:

```text
$ czds status
TLD     ID      UnicodeTLD      Status  Created Updated Expires SFTP
xn--mxtq1m e59839f1-d69d-4970-9a15-7b49f3592065 政府 Approved Wed Jan 30 08:00:42 2019 Wed Jan 30 08:53:41 2019 Sat Jan 12 08:53:41 2030 false
aigo c6886423-b67d-43b6-828f-9d5a6cb3e6a3 aigo Pending Wed Jan 30 08:00:41 2019 Wed Jan 30 08:01:38 2019  false
barclaycard fa6d9c14-17ac-4b15-baf6-2d10g8e806fe barclaycard Pending Wed Jan 30 08:00:41 2019 Wed Jan 30 08:01:38 2019  false
fans 977d8589-9cec-41ef-b62e-0d3f0cf863e0 fans Pending Wed Jan 30 08:00:41 2019 Wed Jan 30 08:01:38 2019  false
live 8c95ccae-ae4d-4028-8997-655b132f542d live Approved Wed Jan 30 08:00:41 2019 Wed Jan 30 16:40:15 2019 Sat Jan 12 16:40:13 2030 false
onyourside 259aa66b-ac77-43db-a09a-9d3f57cf0e6b onyourside Pending Wed Jan 30 08:00:41 2019 Wed Jan 30 08:02:16 2019  false
wtc 67f5b31d-19f0-4071-a176-25ff71f509f7 wtc Pending Wed Jan 30 08:00:41 2019 Wed Jan 30 08:02:55 2019  false
xn--d1acj3b 69929632-ed92-437a-b140-fff4b0d771a7 дети Approved Wed Jan 30 08:00:41 2019 Wed Jan 30 10:55:03 2019 Tue Apr 30 10:55:03 2019 false
```

Lookup specific request details:

```console
$ czds status -zone red
ID:     a056b38d-0080-4097-95cb-014b35ed4cb7
TLD:    red (red)
Status: approved
Created:        Wed Jan 30 08:00:41 2019
Updated:        Thu Jan 31 20:51:22 2019
Expires:        Sun Jan 13 20:51:20 2030
Request IP:     123.456.789.123
FTP IPs:         []
Reason: ...
History:
        Wed Jan 30 08:00:41 2019        Request submitted
        Wed Jan 30 08:02:16 2019        Request status change to Pending
        Thu Jan 31 20:51:22 2019        Request status change to Approved
```

## Building

Just run make!

```console
make
```

## [Docker](https://github.com/lanrat/czds/pkgs/container/czds)

Using command-line flags:

```console
docker run --rm -v /path/to/zones/:/zones ghcr.io/lanrat/czds download -out /zones -username "$USERNAME" -password "$PASSWORD"
```

Using environment variables:

```console
docker run --rm -v /path/to/zones/:/zones -e CZDS_USERNAME="$USERNAME" -e CZDS_PASSWORD="$PASSWORD" ghcr.io/lanrat/czds -out /zones download
```
