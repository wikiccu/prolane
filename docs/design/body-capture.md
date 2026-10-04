# Bounded body capture design

Status: proposed, not implemented. The current command writes metadata-only
[version 1 recordings](http-recorder.md#recording-model-version-1) and does not
accept the options below. This design covers controlled synthetic traffic.

## Opt-in and privacy

Proposed command options:

| Option | Default | Planned effect |
| --- | --- | --- |
| `--capture-request-body` | `false` | Retain at most the first 16 KiB read from each admitted client request body |
| `--capture-response-body` | `false` | Retain at most the first 16 KiB read from each supported upstream response body |

Select each direction independently. With neither option enabled, continue
emitting version 1 without body data. Enabling either option selects version 2
for every exchange in that session, even when no body bytes are observed. Do
not infer opt-in from a file name, media type, HTTP method, or environment.

Capture raw prefixes of eligible bodies, including binary and compressed data.
There is no automatic body redaction, media-type sanitization, or encryption;
base64 is an encoding, not protection. A short prefix can contain credentials
or personal data. Help and the startup notice must explain this risk before
body capture becomes available. Use only synthetic payloads whose retention the
operator has approved. Header and query exclusion does not sanitize a body.

No header, URL-query, full-URL, or raw-error metadata fields are added. Captured
bodies may themselves contain URLs, credentials, or other sensitive values.
Keep the existing file creation, access-control, log, and retention rules in
the [recorder design](http-recorder.md#capture-and-privacy-boundary). Opt-in does
not make production traffic safe to capture or establish replay readiness.

## Observation and limits

Wrap the existing request body after admission and the upstream response body
in `ModifyResponse` after checking for unsupported protocols. Observe bytes
returned by those readers, not writes to the client response writer. Never
capture the proxy's generated gateway-error body. Rejected requests have no
record; rejected upstream upgrades/event streams retain observed status but
omit the response body, even when response capture was selected.

Forward every byte and the original read/close errors unchanged. Once the
prefix buffer fills, keep forwarding and counting reads without retaining more
payload. Copy retained bytes into owned storage; never keep the caller's read
buffer. Do not limit the forwarded reader, pre-read the body, drain it solely
for capture, or retry a request to complete a recording. Transfer framing and
trailers are not body data; content-encoded bytes stay encoded. Keep automatic
upstream decompression disabled.

| Resource | Proposed bound |
| --- | --- |
| Retained request prefix | 16,384 bytes |
| Retained response prefix | 16,384 bytes |
| Active exchanges | Existing limit of 64 |
| Retained prefixes across active exchanges | At most 2 MiB, excluding HTTP, observer, snapshot, and encoding overhead |
| Version 1 or 2 JSONL record, including LF | Existing limit of 64 KiB |

Keep the existing 8 KiB combined method/path bound and all network deadlines.
Padded base64 expands each 16 KiB prefix to 21,848 characters. Method tokens and
escaped paths are ASCII without JSON quote/control escapes. With the current
JSON v2 encoder, two prefixes plus 8 KiB of method/path and 1 KiB of fixed
metadata fit below 64 KiB; still check the encoded size. Keep fixed limits,
without configuration files or additional limit flags. The current single
writer and synchronous backpressure remain sufficient.

## Proposed version 2 fields

Preserve version 1's exchange metadata and failure codes, with
`schema_version: 2`. Request and observed-response metadata retain required
`body_omitted` booleans and gain an optional `body` object:

- `body_omitted: true`: omit `body`; no byte counts or completeness claims.
- `body_omitted: false`: require `body` and all four fields below, including
  zero counts, an empty string, and `false` booleans when applicable.

| Body field | Contract |
| --- | --- |
| `data_base64` | Standard padded [RFC 4648 base64](https://pkg.go.dev/encoding/base64#StdEncoding) of the retained byte prefix; lossless, with no line breaks |
| `observed_bytes` | Nonnegative signed 64-bit integer counting bytes observed before the snapshot; a full source-body count only when `stream_complete` is true |
| `stream_complete` | Required boolean; true only when the observer saw source EOF before sealing, including bytes returned alongside EOF |
| `truncated` | Required boolean; true exactly when `observed_bytes` exceeds 16,384, independently of completeness |

Decoded prefix length must equal `min(observed_bytes, 16384)`. Treat count
overflow as a recording failure rather than wrapping a counter. At exactly
16,384 bytes, the buffer is full but `truncated` remains false until another
byte is observed. Neither reaching a declared `Content-Length` nor closing a
body proves EOF. Handle bytes returned with an error before classifying the
error, as required by the [Go Reader contract](https://pkg.go.dev/io#Reader).
An absent body represented by `http.NoBody` may be recorded as known empty.

| Observation | Required representation |
| --- | --- |
| Capture disabled or response protocol excluded | `body_omitted: true`, no `body` |
| Known empty source | `body_omitted: false`, empty data, zero observed bytes, complete, not truncated |
| Interrupted before any bytes | `body_omitted: false`, empty data, zero observed bytes, incomplete, not truncated |
| EOF after more than 16 KiB | Captured 16 KiB prefix, complete and truncated |
| Interrupted after more than 16 KiB | Captured 16 KiB prefix, incomplete and truncated |

For example, a proposed version 2 record for a canceled request after observing
three binary bytes (`00 01 02`), without observing EOF:

```json
{
  "schema_version": 2,
  "id": "1",
  "started_at": "2026-10-04T08:30:00Z",
  "duration_ns": 10500000,
  "request": {
    "method": "POST",
    "path": "/items",
    "query_omitted": false,
    "headers_omitted": true,
    "body_omitted": false,
    "body": {
      "data_base64": "AAEC",
      "observed_bytes": 3,
      "stream_complete": false,
      "truncated": false
    }
  },
  "failure": "canceled"
}
```

Body completeness means the source reader reached EOF, not that the target or
client received every byte. It is independent of the exchange failure code:
an EOF-observed body can coexist with a failed transfer. An early upstream
response can also accompany an incompletely observed request body without an
exchange failure. Incomplete or truncated request bodies cannot establish
equivalent replay; headers and query data still need a reconstruction policy.
Never silently substitute the prefix as a complete request.

## Ownership and rollout

The [HTTP transport](https://pkg.go.dev/net/http#RoundTripper) can consume or close
a request body asynchronously. Synchronize observer updates and finalization;
do not hold an observer mutex across a blocking network read or close. At
handler finalization, seal each observer and give the writer an immutable,
bounded snapshot. Later reads/close still reach the original body but do not
change the snapshot or its counts. If EOF was not observed before sealing,
mark the source incomplete. Keep the existing handler join before file close.

Both directions use this contract from the outset; their command support can
arrive in separate increments. Validate required fields, strict base64, count
and prefix consistency, omission rules, and size limits; unknown versions and
fields must not be interpreted as older data. Version 1 stays unchanged.

Next, implement request opt-in, its bounded observer, and version 2 emission as
one increment. Add response opt-in separately. Validate binary/empty bodies,
limits minus one/at/plus one, EOF with bytes, read errors, early responses,
concurrent sealing, cancellation/shutdown, default omission, unchanged forwarded
payloads, and existing persistence failures.

Header selection, query sanitization, body redaction, replay readers, and a
generic capture/configuration framework remain outside these increments.
