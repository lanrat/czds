package czds

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Filters for RequestsFilter.Status
// Statuses for RequestStatus.Status
const (
	RequestAll       = ""
	RequestSubmitted = "Submitted"
	RequestPending   = "Pending"
	RequestApproved  = "Approved"
	RequestDenied    = "Denied"
	RequestRevoked   = "Revoked"
	RequestExpired   = "Expired"
	RequestCanceled  = "Canceled"
)

// Filters for RequestsSort.Direction
const (
	SortAsc  = "asc"
	SortDesc = "desc"
)

// Filters for RequestsSort.Field
const (
	SortByTLD         = "tld"
	SortByStatus      = "status"
	SortByLastUpdated = "last_updated"
	SortByExpiration  = "expired"
	SortByCreated     = "created"
	SortByAutoRenew   = "auto_renew"
)

// Status from TLDStatus.CurrentStatus and RequestsInfo.Status
const (
	StatusAvailable = "available"
	StatusSubmitted = "submitted"
	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusDenied    = "denied"
	StatusExpired   = "expired"
	StatusCanceled  = "canceled"
	StatusRevoked   = "revoked" // TODO unverified
)

// number of days into the future to check zones for expiration extensions.
// 0 disables the check
const expiryDateThreshold = 120

// used in RequestExtension
var emptyStruct, _ = json.Marshal(make(map[int]int))

// RequestsFilter is used to set what results should be returned by GetRequests
type RequestsFilter struct {
	Status     string             `json:"status"` // should be set to one of the Request* constants
	Filter     string             `json:"filter"` // zone name search
	Pagination RequestsPagination `json:"pagination"`
	Sort       RequestsSort       `json:"sort"`
}

// RequestsSort sets which field and direction the results for the RequestsFilter request should be returned with
type RequestsSort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

// RequestsPagination sets the page size and offset for paginated results for RequestsFilter
type RequestsPagination struct {
	Size int `json:"size"`
	Page int `json:"page"`
}

// Request holds information about a request in RequestsResponse from GetRequests()
type Request struct {
	RequestID   string    `json:"requestId"`
	TLD         string    `json:"tld"`
	ULabel      string    `json:"ulable"` // ULabel contains UTF-8 decoded punycode (API appears to have a typo in the field name)
	Status      string    `json:"status"` // Status should be set to one of the Request* constants
	Created     time.Time `json:"created"`
	LastUpdated time.Time `json:"last_updated"`
	Expired     time.Time `json:"expired"` // Expired time; epoch 0 means no expiration set
	SFTP        bool      `json:"sftp"`
	AutoRenew   bool      `json:"auto_renew"`
}

// RequestsResponse holds Requests from GetRequests() and total number of requests that match the query but may not be returned due to pagination
type RequestsResponse struct {
	Requests      []Request `json:"requests"`
	TotalRequests int64     `json:"totalRequests"`
}

// TLDStatus is information about a particular TLD returned from GetTLDStatus() or included in RequestsInfo
type TLDStatus struct {
	TLD           string `json:"tld"`
	ULabel        string `json:"ulable"`        // ULabel contains UTF-8 decoded punycode (API appears to have a typo in the field name)
	CurrentStatus string `json:"currentStatus"` // CurrentStatus should be set to one of the Status* constants
	SFTP          bool   `json:"sftp"`
}

// HistoryEntry contains a timestamp and description of an action that happened for a RequestsInfo.
// For example: requested, expired, approved, etc.
type HistoryEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	Comment   string    `json:"comment"`
}

// FtpDetails contains FTP information for RequestsInfo.
type FtpDetails struct {
	PrivateDataError bool `json:"privateDataError"`
}

// RequestsInfo contains the detailed information about a particular zone request returned by GetRequestInfo()
type RequestsInfo struct {
	RequestID          string         `json:"requestId"`
	TLD                *TLDStatus     `json:"tld"`
	FtpIps             []string       `json:"ftpips"`
	Status             string         `json:"status"` // should be set to one of the Status* constants
	TcVersion          string         `json:"tcVersion"`
	Created            time.Time      `json:"created"`
	RequestIP          string         `json:"requestIp"`
	Reason             string         `json:"reason"`
	LastUpdated        time.Time      `json:"last_updated"`
	Cancellable        bool           `json:"cancellable"`
	Extensible         bool           `json:"extensible"`
	ExtensionInProcess bool           `json:"extensionInProcess"`
	AutoRenew          bool           `json:"auto_renew"`
	Expired            time.Time      `json:"expired"` // Note: epoch 0 means no expiration set.
	History            []HistoryEntry `json:"history"`
	FtpDetails         *FtpDetails    `json:"ftpDetails"`
	PrivateDataError   bool           `json:"privateDataError"`
}

// RequestSubmission contains the information required to submit a new request with SubmitRequest()
type RequestSubmission struct {
	AllTLDs          bool     `json:"allTlds"`
	TLDNames         []string `json:"tldNames"`
	Reason           string   `json:"reason"`
	TcVersion        string   `json:"tcVersion"` // terms and conditions revision version
	AdditionalFTPIps []string `json:"additionalFtfIps,omitempty"`
}

// Terms holds the terms and conditions details from GetTerms()
type Terms struct {
	Version    string    `json:"version"`
	Content    string    `json:"content"`
	ContentURL string    `json:"contentUrl"`
	Created    time.Time `json:"created"`
}

// CancelRequestSubmission contains request cancellation arguments passed to CancelRequest()
type CancelRequestSubmission struct {
	RequestID string `json:"integrationId"` // This is effectively 'requestId'
	TLDName   string `json:"tldName"`
}

// GetRequests searches for the status of zone requests as seen on the
// CZDS dashboard page "https://czds.icann.org/zone-requests/all".
// This function uses a background context.
//
// Deprecated: Use GetRequestsWithContext for context cancellation support.
func (c *Client) GetRequests(filter *RequestsFilter) (*RequestsResponse, error) {
	return c.GetRequestsWithContext(context.Background(), filter)
}

// GetRequestsWithContext retrieves zone access requests based on the provided filter criteria.
// It supports pagination and filtering by status. The operation can be cancelled using the provided context.
func (c *Client) GetRequestsWithContext(ctx context.Context, filter *RequestsFilter) (*RequestsResponse, error) {
	c.v("GetRequests filter: %+v", filter)
	requests := new(RequestsResponse)
	err := c.jsonAPI(ctx, http.MethodPost, "/czds/requests/all", filter, requests)
	return requests, err
}

// GetRequestInfo gets detailed information about a particular request and its timeline
// as seen on the CZDS dashboard page "https://czds.icann.org/zone-requests/{ID}"
//
// Deprecated: Use GetRequestInfoWithContext
func (c *Client) GetRequestInfo(requestID string) (*RequestsInfo, error) {
	return c.GetRequestInfoWithContext(context.Background(), requestID)
}

// GetRequestInfoWithContext retrieves detailed information about a specific zone access request,
// including its status timeline and history. The operation can be cancelled using the provided context.
func (c *Client) GetRequestInfoWithContext(ctx context.Context, requestID string) (*RequestsInfo, error) {
	c.v("GetRequestInfo request ID: %s", requestID)
	request := new(RequestsInfo)
	err := c.jsonAPI(ctx, http.MethodGet, "/czds/requests/"+requestID, nil, request)
	return request, err
}

// GetTLDStatus gets the current status of all TLDs and their ability to be requested
//
// Deprecated: Use GetTLDStatusWithContext
func (c *Client) GetTLDStatus() ([]TLDStatus, error) {
	return c.GetTLDStatusWithContext(context.Background())
}

func (c *Client) GetTLDStatusWithContext(ctx context.Context) ([]TLDStatus, error) {
	c.v("GetTLDStatus")
	requests := make([]TLDStatus, 0, 20)
	err := c.jsonAPI(ctx, http.MethodGet, "/czds/tlds", nil, &requests)
	return requests, err
}

// GetTerms gets the current terms and conditions from the CZDS portal
// page "https://czds.icann.org/terms-and-conditions"
// this is required to accept the terms and conditions when submitting a new request
//
// Deprecated: Use GetTermsWithContext
func (c *Client) GetTerms() (*Terms, error) {
	return c.GetTermsWithContext(context.Background())
}

func (c *Client) GetTermsWithContext(ctx context.Context) (*Terms, error) {
	c.v("GetTerms")
	terms := new(Terms)
	// this does not appear to need auth, but we auth regardless
	err := c.jsonAPI(ctx, http.MethodGet, "/czds/terms/condition", nil, terms)
	return terms, err
}

// SubmitRequest submits a new request for access to new zones
//
// Deprecated: Use SubmitRequestWithContext
func (c *Client) SubmitRequest(request *RequestSubmission) error {
	return c.SubmitRequestWithContext(context.Background(), request)
}

func (c *Client) SubmitRequestWithContext(ctx context.Context, request *RequestSubmission) error {
	c.v("SubmitRequest request: %+v", request)
	err := c.jsonAPI(ctx, http.MethodPost, "/czds/requests/create", request, nil)
	return err
}

// CancelRequest cancels a pre-existing request.
// Can only cancel pending requests.
//
// Deprecated: Use CancelRequestWithContext
func (c *Client) CancelRequest(cancel *CancelRequestSubmission) (*RequestsInfo, error) {
	return c.CancelRequestWithContext(context.Background(), cancel)
}

func (c *Client) CancelRequestWithContext(ctx context.Context, cancel *CancelRequestSubmission) (*RequestsInfo, error) {
	c.v("CancelRequest request: %+v", cancel)
	request := new(RequestsInfo)
	err := c.jsonAPI(ctx, http.MethodPost, "/czds/requests/cancel", cancel, request)
	return request, err
}

// RequestExtension submits a request to have the access extended.
// Can only request extensions for requests expiring within 30 days.
//
// Deprecated: Use RequestExtensionWithContext
func (c *Client) RequestExtension(requestID string) (*RequestsInfo, error) {
	return c.RequestExtensionWithContext(context.Background(), requestID)
}

func (c *Client) RequestExtensionWithContext(ctx context.Context, requestID string) (*RequestsInfo, error) {
	c.v("RequestExtension request ID: %s", requestID)
	request := new(RequestsInfo)
	err := c.jsonAPI(ctx, http.MethodPost, "/czds/requests/extension/"+requestID, emptyStruct, request)
	return request, err
}

// DownloadAllRequests outputs the contents of the CSV file downloaded by
// the "Download All Requests" button on the CZDS portal to the provided output
//
// Deprecated: Use DownloadAllRequestsWithContext
func (c *Client) DownloadAllRequests(output io.Writer) error {
	return c.DownloadAllRequestsWithContext(context.Background(), output)
}

func (c *Client) DownloadAllRequestsWithContext(ctx context.Context, output io.Writer) error {
	c.v("DownloadAllRequests")
	url := c.BaseURL + "/czds/requests/report"
	resp, err := c.apiRequest(ctx, true, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.v("Error closing response body: %v", err)
		}
	}()

	n, err := io.Copy(output, resp.Body)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%s was empty", url)
	}

	return nil
}

// RequestTLDs is a helper function that requests access to the provided TLDs with the provided reason
// TLDs provided should be marked as able to request from GetTLDStatus()
//
// Deprecated: Use RequestTLDsWithContext
func (c *Client) RequestTLDs(tlds []string, reason string) error {
	return c.RequestTLDsWithContext(context.Background(), tlds, reason)
}

func (c *Client) RequestTLDsWithContext(ctx context.Context, tlds []string, reason string) error {
	c.v("RequestTLDs TLDS: %+v", tlds)
	// get terms
	terms, err := c.GetTermsWithContext(ctx)
	if err != nil {
		return err
	}

	// submit request
	request := &RequestSubmission{
		TLDNames:  tlds,
		Reason:    reason,
		TcVersion: terms.Version,
	}
	err = c.SubmitRequestWithContext(ctx, request)
	return err
}

// RequestAllTLDs is a helper function to request access to all available TLDs with the provided reason
//
// Deprecated: Use RequestAllTLDsExceptWithContext
func (c *Client) RequestAllTLDs(reason string) ([]string, error) {
	return c.RequestAllTLDsExceptWithContext(context.Background(), reason, nil)
}

func (c *Client) RequestAllTLDsWithContext(ctx context.Context, reason string) ([]string, error) {
	return c.RequestAllTLDsExceptWithContext(ctx, reason, nil)
}

// RequestAllTLDsExcept is a helper function to request access to all available TLDs with the provided reason,
// excluding the TLDs listed in the except parameter.
//
// Deprecated: Use RequestAllTLDsExceptWithContext
func (c *Client) RequestAllTLDsExcept(reason string, except []string) ([]string, error) {
	return c.RequestAllTLDsExceptWithContext(context.Background(), reason, except)
}

func (c *Client) RequestAllTLDsExceptWithContext(ctx context.Context, reason string, except []string) ([]string, error) {
	c.v("RequestAllTLDs")
	exceptMap := slice2LowerMap(except)
	// get available to request
	status, err := c.GetTLDStatusWithContext(ctx)
	if err != nil {
		return nil, err
	}
	// check to see if any available to request
	requestTLDs := make([]string, 0, 10)
	for _, tld := range status {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if exceptMap[tld.TLD] {
			// skip over excluded TLDs
			continue
		}
		switch tld.CurrentStatus {
		case StatusAvailable, StatusCanceled, StatusDenied, StatusExpired, StatusRevoked:
			requestTLDs = append(requestTLDs, tld.TLD)
		}
	}
	// if none, return now
	if len(requestTLDs) == 0 {
		c.v("no TLDs to request")
		return requestTLDs, nil
	}

	// get terms
	terms, err := c.GetTermsWithContext(ctx)
	if err != nil {
		return nil, err
	}

	// submit request
	request := &RequestSubmission{
		AllTLDs:   true,
		TLDNames:  requestTLDs,
		Reason:    reason,
		TcVersion: terms.Version,
	}
	c.v("Requesting %d TLDs %+v", len(requestTLDs), requestTLDs)
	err = c.SubmitRequestWithContext(ctx, request)
	return requestTLDs, err
}

// ExtendTLD is a helper function that requests extensions to the provided TLD
// TLDs provided should be marked as Extensible from GetRequestInfo()
//
// Deprecated: Use ExtendTLDWithContext
func (c *Client) ExtendTLD(tld string) error {
	return c.ExtendTLDWithContext(context.Background(), tld)
}

func (c *Client) ExtendTLDWithContext(ctx context.Context, tld string) error {
	c.v("ExtendTLD: %q", tld)
	requestID, err := c.GetZoneRequestIDWithContext(ctx, tld)
	if err != nil {
		return fmt.Errorf("error GetZoneRequestID(%q): %w", tld, err)
	}
	c.v("ExtendTLD: tld: %q requestID: %q", tld, requestID)

	info, err := c.RequestExtensionWithContext(ctx, requestID)
	if err != nil {
		return fmt.Errorf("RequestExtension(%q): %w", tld, err)
	}

	if !info.ExtensionInProcess {
		return fmt.Errorf("error, zone request %q, %q: extension already in progress", tld, requestID)
	}

	return nil
}

// ExtendAllTLDs is a helper function to request extensions to all TLDs that are extendable
//
// Deprecated: Use ExtendAllTLDsExceptWithContext
func (c *Client) ExtendAllTLDs() ([]string, error) {
	return c.ExtendAllTLDsExceptWithContext(context.Background(), nil)
}

func (c *Client) ExtendAllTLDsWithContext(ctx context.Context) ([]string, error) {
	return c.ExtendAllTLDsExceptWithContext(ctx, nil)
}

// ExtendAllTLDsExcept is a helper function to request extensions to all TLDs that are extendable,
// excluding any TLDs listed in the except parameter.
//
// Deprecated: Use ExtendAllTLDsExceptWithContext
func (c *Client) ExtendAllTLDsExcept(except []string) ([]string, error) {
	return c.ExtendAllTLDsExceptWithContext(context.Background(), except)
}

func (c *Client) ExtendAllTLDsExceptWithContext(ctx context.Context, except []string) ([]string, error) {
	c.v("ExtendAllTLDs")
	tlds := make([]string, 0, 10)
	toExtend := make([]Request, 0, 10)
	exceptMap := slice2LowerMap(except)

	// get all TLDs to extend
	const pageSize = 100
	filter := RequestsFilter{
		Status: RequestApproved,
		Filter: "",
		Pagination: RequestsPagination{
			Size: pageSize,
			Page: 0,
		},
		Sort: RequestsSort{
			Field:     SortByExpiration,
			Direction: SortAsc,
		},
	}

	// test if a request is extensible
	isExtensible := func(id string) (bool, error) {
		info, err := c.GetRequestInfoWithContext(ctx, id)
		return info.Extensible, err
	}

	// get all pages of requests and check which ones are extendable
	morePages := true
	for morePages {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		c.v("ExtendAllTLDs requesting %d requests on page %d", filter.Pagination.Size, filter.Pagination.Page)
		req, err := c.GetRequestsWithContext(ctx, &filter)
		if err != nil {
			return tlds, err
		}

		now := time.Now()

		for _, r := range req.Requests {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			// check for break early
			if expiryDateThreshold > 0 && r.Expired.After(now.AddDate(0, 0, expiryDateThreshold)) {
				c.v("request %q: %q expires on %s, > %d days threshold, looking no further", r.TLD, r.RequestID, r.Expired.Format(time.ANSIC), expiryDateThreshold)
				morePages = false
				break
			}

			// get request info
			ext, err := isExtensible(r.RequestID)
			if err != nil {
				return tlds, err
			}
			if ext {
				toExtend = append(toExtend, r)
			}
		}
		filter.Pagination.Page++
		if len(req.Requests) == 0 {
			morePages = false
		}
	}

	// perform extend
	c.v("requesting extensions for %d tlds: %+v", len(toExtend), toExtend)
	for _, r := range toExtend {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if exceptMap[r.TLD] {
			// skip over excluded TLDs
			continue
		}
		_, err := c.RequestExtensionWithContext(ctx, r.RequestID)
		if err != nil {
			return tlds, err
		}
		tlds = append(tlds, r.TLD)
	}

	if len(tlds) != len(toExtend) {
		return tlds, fmt.Errorf("expected to extend %d TLDs but only extended %d", len(toExtend), len(tlds))
	}

	return tlds, nil
}
