# HTTP recorder design

Status: forwarding implemented, including command validation, fixed-target
routing, limits, cancellation, and shutdown. Capture, privacy filtering for
persisted data, and persistence below remain proposed. This document scopes the
first recorder increments, not a production traffic capture guarantee.

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
forwarded payload. The example currently forwards traffic until interrupted;
`--output` remains required but no recording is written. Only loopback listener IPs
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

Before the first persistence increment, implement an explicit header allowlist.
Do not persist `Authorization`, `Proxy-Authorization`, `Cookie`, or `Set-Cookie`.
Unknown headers stay excluded because custom headers can carry secrets. Keep
multiple values for allowed headers; do not flatten them into one string.

Initially omit query values and bodies from recordings. Introduce their capture
only through a reviewed policy and explicit opt-in; bounded body capture must
not land ahead of that policy. Paths and even allowed metadata can contain
private data, so the first recorder is suitable only for controlled synthetic
traffic. Header filtering is not a claim of general traffic sanitization.

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

## Proposed recording model

Start with JSONL: one completed exchange per line, encoded as JSON. This is
easy to inspect and process incrementally without an external dependency.
A binary format is deferred until measurements justify it. Exact field names,
body encoding, and compatibility rules belong to the separate versioned-model
increment; this document does not establish a file schema.

The model should cover:

- An explicit schema version and a session-local exchange identifier.
- Request method, target-relative path, allowed headers, and capture state for
  query values and bodies.
- Upstream response status, allowed headers, and body capture state, or an
  explicit transport/cancellation failure when no response exists.
- Start timestamp and elapsed forwarding time, with the timing boundary defined
  separately from file writing.
- Omission, redaction, truncation, and interruption metadata.

Represent arbitrary captured body bytes losslessly; do not assume UTF-8.
Lines are written as handlers finish; their order is not a replay schedule.
Never silently reinterpret a schema version.
A future reader must reject unknown versions and incomplete lines. Missing or
modified request data must be distinguishable from an empty value; future replay
must reject insufficient recordings unless an explicit substitution policy
makes them usable. Such recordings cannot establish equivalent replay by default.

## Persistence and failures

Use one file writer with a mutex around each complete encoded record. Bound
record size as well as active exchanges; synchronous writes provide backpressure
without another queue or worker. This serializes disk writes, which is an
acceptable initial throughput ceiling; revisit it only after measurement.

Check encoding, write, short-write, and close errors. A write failure stops new
admission and triggers shutdown with a nonzero exit. Requests already forwarded
may have changed the application, and responses already sent cannot be undone.
Report the recording as incomplete; do not retry application requests to repair
the recording. A crash can leave a partial final line, and successful writes
alone do not guarantee survival of a power failure.

Invalid configuration, output creation failure, or listener bind failure must
fail startup before accepting traffic. Upstream connection failure returns a
generic gateway error and records a failure rather than inventing a response.
If response forwarding fails after headers were sent, do not claim the client
received a complete response. Client cancellation and shutdown interruption
remain distinct from a successful exchange.

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
   cancellation, and shutdown. Recording remains explicitly unimplemented.
3. Review the versioned model and implement privacy exclusions before adding
   metadata persistence and its failure handling.
4. Add opt-in bounded request/response capture with explicit completeness state.

Privacy exclusions move ahead of persistence so no intermediate increment
casually stores credentials. Replay and comparison get their own designs after
the recorder produces a reviewed, versioned artifact.
