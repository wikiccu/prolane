# HTTP recorder design

Status: forwarding implemented, including command validation, fixed-target
routing, limits, cancellation, and shutdown. Metadata-only version 1 JSONL
persistence is implemented; headers, query data, and bodies remain excluded.
Richer capture remains unimplemented. This document scopes the first recorder
increments, not a production traffic capture guarantee.

## Goal and workflow

Forward ordinary HTTP requests to one operator-selected application and produce
bounded, inspectable evidence for later replay. Start with local applications
and synthetic traffic whose contents the operator controls.

The intended command is:

```sh
prolane record --listen 127.0.0.1:8080 --target http://127.0.0.1:3000 --output ./traffic.jsonl
```

Clients send requests to the listener; Prolane forwards them to the fixed target
and returns the upstream response. Recording must not silently change the
forwarded payload. The example forwards traffic until interrupted and writes
metadata to a new `--output` file. Only loopback listener IPs
are accepted; remote exposure needs a separate security review before it becomes
supported behavior.

Replay, comparison, verification verdicts, databases, distributed workers,
configuration files, and a dashboard are outside this design. CONNECT tunnels,
asterisk-form requests, protocol upgrades, WebSockets, and long-lived streaming exchanges are also
outside the first slice. Reject unsupported tunnels and upgrades before
contacting the target; streaming protocols need a separate timeout and capture
design.

## Forwarding and ownership

Use the standard library: `flag` for the small command, `net/http` for the
listener and transport, and `net/http/httputil.ReverseProxy` for forwarding.
Keep command orchestration separate from forwarding and persistence as those
behaviors arrive; do not create empty packages or generic interfaces now.

Use `ReverseProxy.Rewrite` and `ProxyRequest.SetURL` to select the target and
its outbound Host. SetURL joins the target's base path with the incoming path.
Let the proxy handle hop-by-hop headers. Rewrite removes inbound forwarding
headers before the callback; initially do not add client-address forwarding
headers. These semantics are defined by the
[ReverseProxy API](https://pkg.go.dev/net/http/httputil#ReverseProxy).

Validate the listen address, target, and output before serving. Accept only an
absolute HTTP or HTTPS target with a host; reject embedded credentials, query
strings, and fragments in the target. A target base path is allowed. The
incoming Host or absolute request URI must never select another destination.
Use a dedicated transport with environment proxy discovery disabled and normal
TLS certificate verification enabled. Return upstream redirects to the client
without following them.

The command owns the server, transport, and output file. Request cancellation
must reach the upstream exchange. Give header reads, request/response transfer,
dialing, response-header waits, idle connections, and shutdown finite limits.
Current values are documented in the [README](../../README.md#recorder-command-in-progress):
64 active exchanges, 64 KiB header limits, a 30-second exchange context and
client transfer timeouts, a 10-second response-header timeout, 5-second
header/dial/TLS and shutdown limits, and 30-second idle timeouts. Excess active
work is rejected with HTTP 503. Request bodies and responses stream without
whole-body buffering. Malformed query parameters are rejected before forwarding
so the standard proxy does not silently discard them.
Credentialed or opaque request URIs are also rejected. `OPTIONS *` receives
HTTP 501 explicitly instead of the HTTP server's implicit local response.
Request method and escaped inbound path are limited to 8 KiB combined; excess
metadata receives HTTP 414 before forwarding. Requests rejected before admission
do not consume an exchange ID or produce a recording line.

On interruption, stop accepting requests, allow active exchanges to finish up
to a deadline, then cancel and close remaining exchanges. Wait for
[Server.Shutdown](https://pkg.go.dev/net/http#Server.Shutdown) and for handlers to
release the writer before closing the recording file; Shutdown can return at
its deadline while handlers are still active. Close idle upstream connections.
An exceeded deadline must produce an incomplete-session error, not a success
message.

## Capture and privacy boundary

Forwarding and persistence use separate views of the exchange. Excluding a
field from the recording must not remove it from traffic sent to the target
or the response returned to the client.

Version 1 excludes all headers from its model; its initial allowlist is empty.
Before adding header capture, define an explicit allowlist. Do not persist
`Authorization`, `Proxy-Authorization`, `Cookie`, or `Set-Cookie`. Unknown headers
stay excluded because custom headers can carry secrets. Keep multiple values
for any headers later allowed; do not flatten them into one string.

Initially omit query values and bodies from recordings. Introduce their capture
only through a reviewed policy and explicit opt-in; bounded body capture must
not land ahead of that policy. Paths and even allowed metadata can contain
private data, so the first recorder is suitable only for controlled synthetic
traffic. Header filtering is not a claim of general traffic sanitization.

The [bounded body capture design](body-capture.md) now defines proposed opt-in,
version 2 fields, limits, and observation semantics. Body capture and its new
options remain unimplemented; version 1 stays metadata-only.

When body capture arrives, copy a bounded prefix while the proxy streams the
complete body. Cap request and response captures independently before storing
bytes in memory. Mark omitted, redacted, truncated, and interrupted data
explicitly. Report a full byte count only when the complete stream was observed.
Capture limits must not truncate what the application or client receives.
Disable automatic upstream decompression so captured response bytes and headers
describe the same representation; do not decompress bodies for storage.

Recordings are sensitive local artifacts. Refuse to overwrite or append to an
existing file: create the output with
[`os.OpenFile` and `O_CREATE|O_EXCL`](https://pkg.go.dev/os#OpenFile). Request
owner-only permissions on systems supporting Unix modes; Windows protection
depends on the destination directory's access controls. Do not create missing
directories or write recordings to logs. Before capturing real traffic, revisit
path/query sanitization, body rules, retention, and file access controls.

## Recording model: version 1

The [Go model](../../internal/record/model.go) defines metadata for one completed
forwarding attempt. It contains no raw headers, query values, bodies, full URLs,
or raw error messages. The proxy populates the model and a single file writer
persists it after each admitted forwarding attempt finishes.
Paths can still contain sensitive data; this remains a controlled synthetic
traffic design, not general sanitization.

Use UTF-8 JSONL: one compact JSON object followed by LF per exchange, with no
array wrapper. Keep zero-valued required fields explicit. Omit `response` when
no upstream response headers were observed, and omit `failure` on success;
neither field is encoded as `null`. The model uses `omitzero` tags supported by
Go's [standard JSON encoding](https://pkg.go.dev/encoding/json/v2).

| Field | Version 1 contract |
| --- | --- |
| `schema_version` | Required integer `1` |
| `id` | Required string of positive decimal digits, without leading zeros; unique within one recording, assigned from `"1"` as requests are admitted |
| `started_at` | Required nonzero UTC timestamp, when the request is admitted for forwarding; RFC 3339 with up to nanosecond precision, ending in `Z` |
| `duration_ns` | Required nonnegative signed 64-bit integer; elapsed forwarding time in nanoseconds, excluding encoding and file writes |
| `request.method` | Required HTTP method as received |
| `request.path` | Required escaped inbound path, before joining the configured target base path; no query, fragment, or authority; an empty path remains explicit |
| `request.query_omitted` | Required boolean; `true` if the inbound URL contained a query or a bare `?`, otherwise `false`; no query keys or values are stored |
| `request.headers_omitted`, `request.body_omitted` | Required booleans, both `true`; omission does not mean empty headers or an empty body |
| `response.status_code` | Required when `response` exists; observed upstream status from `100` through `999`, never the proxy's generated gateway status |
| `response.headers_omitted`, `response.body_omitted` | Required when `response` exists, both `true` |
| `failure` | Optional fixed code: `upstream_error`, `timeout`, `canceled`, or `incomplete_response`; an empty code is omitted |

IDs are strings so readers do not need floating-point JSON numbers to preserve
their precision. IDs reflect admission order; lines are written as handlers
finish, and neither establishes a replay schedule. Store the start timestamp in
UTC and measure duration from the original monotonic clock reading before
converting the timestamp for storage. Go's
[time JSON encoding](https://pkg.go.dev/time#Time.MarshalJSON) supports the chosen
timestamp representation.

A successful exchange has `response` and no `failure`. An unsuccessful exchange
has `failure`; it also has `response` if upstream headers were observed before
the failure. `upstream_error` covers connection/TLS failures and unsupported
upstream protocols. `incomplete_response` covers interrupted response transfer
when it is not classified as a timeout or cancellation. A status alone is not
proof that the client received the complete response. Interim informational
responses are not separate exchanges.
Success describes the proxy handler's observed completion. The HTTP server can
still fail a final buffered client flush after the handler returns; records do
not confirm end-client receipt.

For illustration, shown indented here rather than as a single JSONL line:

```json
{
  "schema_version": 1,
  "id": "1",
  "started_at": "2026-10-03T08:30:00Z",
  "duration_ns": 10500000,
  "request": {
    "method": "GET",
    "path": "/items%2Fexample",
    "query_omitted": true,
    "headers_omitted": true,
    "body_omitted": true
  },
  "response": {
    "status_code": 200,
    "headers_omitted": true,
    "body_omitted": true
  }
}
```

A timeout before upstream headers arrive has no response object:

```json
{
  "schema_version": 1,
  "id": "2",
  "started_at": "2026-10-03T08:30:01Z",
  "duration_ns": 10000000000,
  "request": {
    "method": "POST",
    "path": "/items",
    "query_omitted": false,
    "headers_omitted": true,
    "body_omitted": true
  },
  "failure": "timeout"
}
```

The Go types describe the shape, not a general-purpose validator. The handler
constructs required fields and omission flags from admitted HTTP exchanges; the
writer bounds the encoded record and checks encoding and write errors. There is
no public API for writing arbitrary model values. A future reader must reject
unknown versions, invalid/missing fields, unknown fields or failure codes,
duplicate object keys/IDs, and incomplete lines. Do not reinterpret excluded data as empty
values or assume version 1 records support equivalent replay. Replay would need
an explicit reconstruction policy before using them for verification.

Changing field meaning, shape, or capture policy requires a reviewed schema
version change; do not silently add captured data to version 1. Header selection,
query sanitization, lossless binary body encoding, and truncation metadata are
deferred until their capture increment. No decoder, migration, or generic
serialization framework is introduced.

## Persistence and failures

One file writer holds a mutex around encoding and writing each complete record.
Encoded JSONL records, including LF, are capped at 64 KiB. Bounding method/path
metadata before forwarding also bounds the encoding allocation. Synchronous
writes provide backpressure without another queue or worker. This serializes
disk writes, which is an acceptable initial throughput ceiling; revisit it only
after measurement.

Encoding, write, short-write, and close errors are checked. A recording failure
stops new admission and triggers shutdown with a nonzero exit. Requests already
forwarded may have changed the application, and responses already sent cannot be undone.
Report the recording as incomplete; do not retry application requests to repair
the recording. A crash can leave a partial final line, and successful writes
alone do not guarantee survival of a power failure. There is no explicit fsync
or crash-durability promise. Blocking regular-file I/O is governed by the
filesystem; the network shutdown deadline cannot interrupt it.

Invalid configuration, output creation failure, or listener bind failure must
fail startup before accepting traffic. Upstream connection failure returns a
generic gateway error and records a failure rather than inventing a response.
If response forwarding fails after headers were sent, do not claim the client
received a complete response. Client cancellation and shutdown interruption
remain distinct from a successful exchange.
Bind the listener before exclusive output creation, then start serving only
after the file is open and verified to be regular. A creation failure closes
the listener without forwarding traffic. Keep created artifacts, including
empty or partial files, instead of automatically deleting them on an error.
On forced shutdown, cancel and close network exchanges, join admitted handlers,
then close the file. The writer's admission mutex prevents new handler
registrations from racing with that final join.

Send lifecycle messages, counts, and sanitized errors to stderr. Do not log
request URLs, headers, bodies, credentials, or raw upstream errors. Configure
the proxy's error handling and logging to preserve this rule. No metrics stack
or tracing dependency is needed for this slice.

## Validation and implementation order

As each behavior is implemented, validate it with local `httptest` servers and
temporary outputs: target selection, base/escaped paths, query handling, Host,
repeated and hop-by-hop headers, bodies, redirects, cancellations, upstream
failures, timeout/shutdown behavior, capture limits, sensitive-field omission,
output failures, and complete JSONL lines under concurrent requests. Use explicit
synchronization for lifecycle checks rather than arbitrary sleeps.

Implement in separate reviewable increments:

1. Implemented: `record` command parsing, help, and argument validation.
2. Implemented: forwarding with fixed-target routing, resource limits,
   cancellation, and shutdown.
3. Implemented: metadata-only version 1 model with explicit omissions and fixed
   failure codes.
4. Implemented: metadata construction, capture exclusions, exclusive JSONL
   persistence, recording-error shutdown, and handler joining before file close.
5. Designed, not implemented: [opt-in bounded body capture](body-capture.md),
   with version 2 fields and independent completeness/truncation state.
6. Implement request body opt-in and version 2 emission together, then response
   body opt-in in a separate increment.

Privacy exclusions move ahead of persistence so no intermediate increment
casually stores credentials. Replay and comparison get their own designs after
the recorder produces a reviewed, versioned artifact.
