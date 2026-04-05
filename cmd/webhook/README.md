# Webhook Package

The webhook package provides a generic HTTP webhook client for batch download approval and notifications. While designed for the `czds download` command, it can be reused for other applications.

## Features

- Pre-download batch approval via POST requests with all zones
- Post-download notifications via POST requests (sent immediately after each zone completes)
- Automatic retries with exponential backoff for transient errors (429, 503, 5XX, network failures)
- Context-aware with configurable timeouts, retry counts, and delays
- Optional logging and customizable headers
- Generic, reusable, stdlib-only implementation

## Usage

### Environment Variables

Enable webhooks using environment variables:

```bash
export PRECHECK_WEBHOOK_URL=https://example.com/addzone/check
export NOTIFICATION_WEBHOOK_URL=https://example.com/addzone
czds download
```

Both variables are optional. If not set, the corresponding functionality is disabled.
URLs are used exactly as provided.

### Webhook Flow

**Pre-Download Batch Check:**

```http
POST https://example.com/addzone/check
Content-Type: application/json
X-Zone-Source: czds

{
  "date": "2025-11-24",
  "zones": ["com", "net", "org"]
}
```

Response (200 OK):

```json
{
  "date": "2025-11-24",
  "results": [
    {"zone": "com", "should_download": true, "reason": "new import needed"},
    {"zone": "net", "should_download": false, "reason": "already imported"}
  ],
  "summary": {"total": 2, "should_download": 1, "skip": 1}
}
```

Zones with `should_download: false` are skipped.

**Post-Download Notification:**

```http
POST https://example.com/addzone
Content-Type: application/json
X-Zone-Source: czds

{
  "date": "2025-11-24",
  "zones": [
    {"zone": "com", "file_path": "/zones/com.zone.gz"}
  ]
}
```

Response (200 OK):

```json
{
  "date": "2025-11-24",
  "source": "czds",
  "results": [
    {"zone": "com", "status": "added", "import_id": 123, "message": "success"}
  ],
  "summary": {"total": 1, "added": 1, "exists": 0, "rejected": 0, "deleted": 0, "errors": 0}
}
```

Notifications are sent immediately after each zone download completes (single-zone batches).

**Status Code Handling:**

- **Success**: 200 OK
- **Retried**: 429, 503, 5XX, network errors (3 attempts, 1s/2s/3s delays)
- **Not Retried**: All other codes (400, 403, 404, other 4XX, 3XX)

## Thread Safety

`BatchPreDownloadCheck()` and `BatchPostDownloadNotify()` are safe for concurrent use after configuration. Configuration methods (`SetHeader`, `SetRetries`, `SetTimeout`, `SetLogger`) should only be called during initialization, not concurrently with requests.

## Programmatic Usage

The webhook package can be used programmatically in any Go application:

```go
import (
    "context"
    "log"
    "time"
    "github.com/lanrat/czds/cmd/webhook"
)

// Create a single webhook client with both URLs
client, err := webhook.New(
    "https://example.com/addzone/check",  // precheck URL
    "https://example.com/addzone",        // notification URL
)
if err != nil {
    log.Fatal(err)
}

// Configure the client
client.SetHeader("X-Zone-Source", "czds")

// Optional: Configure retries and timeout
client.SetRetries(5, 2*time.Second)   // 5 attempts, 2s base delay
client.SetTimeout(60 * time.Second)   // 60s timeout per request

// Optional: Enable logging
client.SetLogger(log.Default())

// Get current date in YYYY-MM-DD format
dateStr := time.Now().Format(time.DateOnly)

// Batch pre-check
zones := []string{"com", "net", "org"}
approved, err := client.BatchPreDownloadCheck(context.Background(), zones, dateStr)
if err != nil {
    log.Printf("Pre-check failed: %v", err)
}

// Download approved zones...
for zone, shouldDownload := range approved {
    if !shouldDownload {
        log.Printf("Skipping %s", zone)
        continue
    }
    // Download the zone...
}

// Send batch notification after downloads
batchZones := []webhook.BatchAddZone{
    {Zone: "com", FilePath: "/zones/com.zone.gz"},
    {Zone: "net", FilePath: "/zones/net.zone.gz"},
}
results, err := client.BatchPostDownloadNotify(context.Background(), batchZones, dateStr)
if err != nil {
    log.Printf("Notification failed: %v", err)
} else {
    for _, r := range results {
        log.Printf("Zone %s: %s (import_id: %d)", r.Zone, r.Status, r.ImportID)
    }
}
```

### Available Methods

**Client Creation:**

- `New(precheckURL, notifyURL) (*Client, error)` - Create webhook client with both URLs (either can be empty)
- `NewFromEnv() (*Client, error)` - Create client from environment variables (returns nil if both vars unset)

**Configuration:**

- `SetHeader(key, value)` - Set custom HTTP headers
- `SetRetries(max, delay)` - Configure retry behavior (default: 3 retries, 1s delay)
- `SetTimeout(duration)` - Set HTTP request timeout (default: 30s)
- `SetLogger(logger)` - Enable logging (compatible with `log.Logger`)
- `PrecheckEnabled() bool` - Check if precheck webhook is configured
- `NotifyEnabled() bool` - Check if notification webhook is configured

**Single-Zone Operations (convenience wrappers):**

- `PreDownloadCheck(ctx, zone, date) (bool, error)` - Check if single zone should be downloaded
- `PostDownloadNotify(ctx, zone, filePath, date) (*BatchAddResult, error)` - Notify after single zone download

**Batch Operations:**

- `BatchPreDownloadCheck(ctx, zones, date) (map[string]bool, error)` - Batch check multiple zones
- `BatchPostDownloadNotify(ctx, zones, date) ([]BatchAddResult, error)` - Batch notify after downloads

## Example Webhook Server

Here's a simple webhook server that implements both batch endpoints and only allows downloading `.com` and `.net` zones:

```go
package main

import (
    "encoding/json"
    "log"
    "net/http"
)

type BatchCheckRequest struct {
    Date  string   `json:"date"`
    Zones []string `json:"zones"`
}

type BatchCheckResult struct {
    Zone           string `json:"zone"`
    ShouldDownload bool   `json:"should_download"`
    Reason         string `json:"reason"`
}

type BatchCheckResponse struct {
    Date    string             `json:"date"`
    Results []BatchCheckResult `json:"results"`
    Summary struct {
        Total          int `json:"total"`
        ShouldDownload int `json:"should_download"`
        Skip           int `json:"skip"`
    } `json:"summary"`
}

type BatchAddZone struct {
    Zone     string `json:"zone"`
    FilePath string `json:"file_path"`
}

type BatchAddRequest struct {
    Date  string         `json:"date"`
    Zones []BatchAddZone `json:"zones"`
}

type BatchAddResult struct {
    Zone     string `json:"zone"`
    Status   string `json:"status"`
    ImportID int    `json:"import_id,omitempty"`
    Message  string `json:"message,omitempty"`
}

type BatchAddResponse struct {
    Date    string           `json:"date"`
    Source  string           `json:"source"`
    Results []BatchAddResult `json:"results"`
    Summary struct {
        Total    int `json:"total"`
        Added    int `json:"added"`
        Exists   int `json:"exists"`
        Rejected int `json:"rejected"`
        Deleted  int `json:"deleted"`
        Errors   int `json:"errors"`
    } `json:"summary"`
}

func main() {
    // Pre-download batch check
    http.HandleFunc("/check", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            w.WriteHeader(http.StatusMethodNotAllowed)
            return
        }

        var req BatchCheckRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            w.WriteHeader(http.StatusBadRequest)
            return
        }

        log.Printf("Pre-download batch check for %d zones (date: %s)", len(req.Zones), req.Date)

        var resp BatchCheckResponse
        resp.Date = req.Date
        for _, zone := range req.Zones {
            result := BatchCheckResult{Zone: zone}
            if zone == "com" || zone == "net" {
                result.ShouldDownload = true
                result.Reason = "zone in allowlist"
                resp.Summary.ShouldDownload++
            } else {
                result.ShouldDownload = false
                result.Reason = "zone not in allowlist"
                resp.Summary.Skip++
            }
            resp.Results = append(resp.Results, result)
        }
        resp.Summary.Total = len(req.Zones)

        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(resp)
    })

    // Post-download batch notification
    http.HandleFunc("/addzone", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            w.WriteHeader(http.StatusMethodNotAllowed)
            return
        }

        var req BatchAddRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            w.WriteHeader(http.StatusBadRequest)
            return
        }

        source := r.Header.Get("X-Zone-Source")
        log.Printf("Post-download batch notification for %d zones (date: %s, source: %s)", len(req.Zones), req.Date, source)

        var resp BatchAddResponse
        resp.Date = req.Date
        resp.Source = source
        importID := 1000
        for _, zone := range req.Zones {
            result := BatchAddResult{
                Zone:     zone.Zone,
                Status:   "added",
                ImportID: importID,
                Message:  "import added successfully",
            }
            resp.Results = append(resp.Results, result)
            resp.Summary.Added++
            importID++
            log.Printf("Received: %s -> %s", zone.Zone, zone.FilePath)
        }
        resp.Summary.Total = len(req.Zones)

        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(resp)
    })

    log.Println("Starting webhook server on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

Run the server:

```bash
go run server.go
```

Use with czds:

```bash
export PRECHECK_WEBHOOK_URL=http://localhost:8080/check
export NOTIFICATION_WEBHOOK_URL=http://localhost:8080/addzone
czds download
```

## Use Cases

- Zone filtering and quota management
- Post-processing triggers and monitoring
- Integration with existing systems (databases, message queues, etc.)

## Retry Logic

Transient errors (network failures, 429, 503, 5XX) are automatically retried up to 3 times with exponential backoff (1s, 2s, 3s). Permanent errors (4XX except 429, 3XX) are not retried. Context cancellation is respected during retries and sleep delays. Customize via `SetRetries(max, delay)`.

### Example: Custom Retry Configuration

```go
wh, _ := webhook.New("https://example.com/webhook")

// More aggressive retries: 10 attempts with 500ms base delay
wh.SetRetries(10, 500*time.Millisecond)

// Or fewer retries for faster failure
wh.SetRetries(2, 1*time.Second)
```

## Timeouts

Default timeout is 30 seconds per request. With 3 retries, maximum time is ~96 seconds (3×30s + 1s+2s+3s delays). Customize with `SetTimeout(duration)`.

## Documentation

For detailed API documentation, see:

```bash
go doc github.com/lanrat/czds/cmd/webhook
```
