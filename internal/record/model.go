package record

import "time"

// SchemaVersion identifies the metadata-only recording format.
const SchemaVersion = 1

// Exchange describes one completed forwarding attempt. Its zero value is not
// a valid record; a future writer must enforce the documented version and field
// invariants before encoding it. No live traffic is captured by these types.
type Exchange struct {
	SchemaVersion int               `json:"schema_version"`
	ID            string            `json:"id"`
	StartedAt     time.Time         `json:"started_at"`
	DurationNS    int64             `json:"duration_ns"`
	Request       RequestMetadata   `json:"request"`
	Response      *ResponseMetadata `json:"response,omitzero"`
	Failure       FailureCode       `json:"failure,omitzero"`
}

// RequestMetadata deliberately excludes raw headers, query values, and bodies.
// Path is the escaped inbound path before the target's base path is joined.
// HeadersOmitted and BodyOmitted must be true in schema version 1.
type RequestMetadata struct {
	Method         string `json:"method"`
	Path           string `json:"path"`
	QueryOmitted   bool   `json:"query_omitted"`
	HeadersOmitted bool   `json:"headers_omitted"`
	BodyOmitted    bool   `json:"body_omitted"`
}

// ResponseMetadata describes observed upstream headers, not a proxy-generated
// error response or proof that the client received a complete body.
// HeadersOmitted and BodyOmitted must be true in schema version 1.
type ResponseMetadata struct {
	StatusCode     int  `json:"status_code"`
	HeadersOmitted bool `json:"headers_omitted"`
	BodyOmitted    bool `json:"body_omitted"`
}

// FailureCode contains a fixed classification, never a raw error message.
// An empty value is omitted when forwarding succeeds.
type FailureCode string

const (
	FailureUpstream           FailureCode = "upstream_error"
	FailureTimeout            FailureCode = "timeout"
	FailureCanceled           FailureCode = "canceled"
	FailureIncompleteResponse FailureCode = "incomplete_response"
)
