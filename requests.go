package czds

import (
	"encoding/json"
	"fmt"
	"io"
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
	StatusRevoked   = "revoked" // unverified
)

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
	ULabel      string    `json:"ulable"` // UTF-8 decoded punycode, looks like API has a typo
	Status      string    `json:"status"` // should be set to one of the Request* constants
	Created     time.Time `json:"created"`
	LastUpdated time.Time `json:"last_updated"`
	Expired     time.Time `json:"expired"` // Note: epoch 0 means no expiration set
	SFTP        bool      `json:"sftp"`
}

// RequestsResponse holds Requests from from GetRequests() and total number of requests that match the query but may not be returned due to pagination
type RequestsResponse struct {
	Requests      []Request `json:"requests"`
	TotalRequests int64     `json:"totalRequests"`
}

// TLDStatus is information about a particular TLD returned from GetTLDStatus() or included in RequestsInfo
type TLDStatus struct {
	TLD           string `json:"tld"`
	ULabel        string `json:"ulable"`        // UTF-8 decoded punycode, looks like API has a typo
	CurrentStatus string `json:"currentStatus"` // should be set to one of the Status* constants
	SFTP          bool   `json:"sftp"`
}

// HistoryEntry contains a timestamp and description of action that happened for a RequestsInfo
// For example: requested, expired, approved, etc..
type HistoryEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	Comment   string    `json:"comment"`
}

// FtpDetails contains FTP information for RequestsInfo
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
	Expired            time.Time      `json:"expired"` // Note: epoch 0 means no expiration set
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

// CancelRequestSubmission Request canceletion arguments passed to CancelRequest()
type CancelRequestSubmission struct {
	RequestID string `json:"integrationId"` // This is effectivly 'requestId'
	TLDName   string `json:"tldName"`
}

// GetRequests searches for the status of zones requests as seen on the
// CZDS dashboard page "https://czds.icann.org/zone-requests/all"
func (c *Client) GetRequests(filter *RequestsFilter) (*RequestsResponse, error) {
	requests := new(RequestsResponse)
	err := c.jsonAPI("POST", "/czds/requests/all", filter, requests)
	return requests, err
}

// GetRequestInfo gets detailed information about a particular request and its timeline
// as seen on the CZDS dashboard page "https://czds.icann.org/zone-requests/{ID}"
func (c *Client) GetRequestInfo(requestID string) (*RequestsInfo, error) {
	request := new(RequestsInfo)
	err := c.jsonAPI("GET", "/czds/requests/"+requestID, nil, request)
	return request, err
}

// GetTLDStatus gets the current status of all TLDs and their ability to be requested
func (c *Client) GetTLDStatus() ([]TLDStatus, error) {
	requests := make([]TLDStatus, 0, 20)
	err := c.jsonAPI("GET", "/czds/tlds", nil, &requests)
	return requests, err
}

// GetTerms gets the current terms and conditions from the CZDS portal
// page "https://czds.icann.org/terms-and-conditions"
// this is required to accept the terms and conditions when submitting a new request
func (c *Client) GetTerms() (*Terms, error) {
	terms := new(Terms)
	// this does not appear to need auth, but we auth regardless
	err := c.jsonAPI("GET", "/czds/terms/condition", nil, terms)
	return terms, err
}

// SubmitRequest submits a new request for access to new zones
func (c *Client) SubmitRequest(request *RequestSubmission) error {
	err := c.jsonAPI("POST", "/czds/requests/create", request, nil)
	return err
}

// CancelRequest cancels a pre-existing request.
// Can only cancel pending requests.
func (c *Client) CancelRequest(cancel *CancelRequestSubmission) (*RequestsInfo, error) {
	request := new(RequestsInfo)
	err := c.jsonAPI("POST", "/czds/requests/cancel", cancel, request)
	return request, err
}

// RequestExtension submits a request to have the access extended.
// Can only request extensions for requests expiering within 30 days.
func (c *Client) RequestExtension(requestID string) (*RequestsInfo, error) {
	request := new(RequestsInfo)
	err := c.jsonAPI("POST", "/czds/requests/extension/"+requestID, emptyStruct, request)
	return request, err
}

// DownloadAllRequests outputs the contents of the csv file downloaded by
// the "Download All Requests" button on the CZDS portal to the provided output
func (c *Client) DownloadAllRequests(output io.Writer) error {
	url := c.BaseURL + "/czds/requests/report"
	resp, err := c.apiRequest(true, "GET", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	n, err := io.Copy(output, resp.Body)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%s was empty", url)
	}

	return nil
}

// RequestTLDs is a helper function that requests access to the provided tlds with the provided reason
// TLDs provided should be marked as able to request from GetTLDStatus()
func (c *Client) RequestTLDs(tlds []string, reason string) error {
	// get terms
	terms, err := c.GetTerms()
	if err != nil {
		return err
	}

	// submit request
	request := &RequestSubmission{
		TLDNames:  tlds,
		Reason:    reason,
		TcVersion: terms.Version,
	}
	err = c.SubmitRequest(request)
	return err
}

// RequestAllTLDs is a helper function to request access to all available TLDs with the provided reason
func (c *Client) RequestAllTLDs(reason string) ([]string, error) {
	// get available to request
	status, err := c.GetTLDStatus()
	if err != nil {
		return nil, err
	}
	// check to see if any available to request
	requestTLDs := make([]string, 0, 10)
	for _, tld := range status {
		switch tld.CurrentStatus {
		case StatusAvailable, StatusExpired, StatusDenied, StatusRevoked:
			requestTLDs = append(requestTLDs, tld.TLD)
		}
	}
	// if none, return now
	if len(requestTLDs) == 0 {
		return requestTLDs, nil
	}

	// get terms
	terms, err := c.GetTerms()
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
	err = c.SubmitRequest(request)
	return requestTLDs, err
}

// ExtendTLD is a helper function that requests extensions to the provided tld
// TLDs provided should be marked as Extensible from GetRequestInfo()
func (c *Client) ExtendTLD(tld string) error {

	zoneID, err := c.GetZoneRequestID(tld)
	if err != nil {
		return fmt.Errorf("error GetZoneRequestID(%q): %w", tld, err)
	}

	info, err := c.RequestExtension(zoneID)
	if err != nil {
		return fmt.Errorf("RequestExtension(%q): %w", tld, err)
	}

	if !info.ExtensionInProcess {
		return fmt.Errorf("error, zone request %q, %q: extension already in progress", tld, zoneID)
	}

	return nil
}

// ExtendAllTLDs is a helper function to request extensions to all TLDs that are extendable
func (c *Client) ExtendAllTLDs() ([]string, error) {
	tlds := make([]string, 0, 10)
	toExtend := make([]Request, 0, 10)

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
		info, err := c.GetRequestInfo(id)
		return info.Extensible, err
	}

	// since requests are sorted by expiration date once we find one that is non extensible, we can exit
	loopExtensible := true
	for loopExtensible {
		req, err := c.GetRequests(&filter)
		if err != nil {
			return tlds, err
		}
		for _, r := range req.Requests {
			if !loopExtensible {
				break
			}
			ext, err := isExtensible(r.RequestID)
			if err != nil {
				return tlds, err
			}
			if ext {
				toExtend = append(toExtend, r)
			} else {
				loopExtensible = false
			}
		}
		filter.Pagination.Page++
		if len(req.Requests) == 0 {
			break
		}
	}

	// perform extend
	for _, r := range toExtend {
		_, err := c.RequestExtension(r.RequestID)
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
