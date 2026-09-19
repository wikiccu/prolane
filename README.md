# Prolane

> Prove every software change before it reaches production.

Prolane is being developed as a CLI-first software change verification tool.
Its goal is to help engineers answer: **Is this software change safe to deploy?**

The intended workflow is to capture HTTP traffic, replay equivalent requests
against a baseline and a candidate application, and compare their behavior.
Those comparisons should provide reproducible evidence for deployment decisions.

## Current status

**Implemented:** a Go module and a CLI entrypoint that prints the project name
and tagline. The CLI does not yet implement flags or subcommands.

**Planned:** HTTP recording, replay, baseline/candidate comparison, and
deterministic verification results. Prolane cannot yet verify a software change.

## Run from source

Use Go 1.27.1 or newer. From the repository root:

```sh
go run ./cmd/prolane
```

Output:

```text
Prolane
Prove every software change before it reaches production.
```

To build the executable locally:

```sh
go build ./cmd/prolane
```

This creates `prolane` on Unix-like systems or `prolane.exe` on Windows.

## First usable milestone (planned)

The first usable milestone focuses on HTTP:

- Record traffic through a local reverse proxy with bounded capture and basic
  protection for sensitive data.
- Replay recorded requests against baseline and candidate applications.
- Compare HTTP status, response bodies, and selected headers; report request
  failures and latency measurements.
- Present useful CLI results with tests, documented behavior, and safe defaults.

Database migration verification, container and Kubernetes environments,
distributed workers, and a web dashboard are outside this initial scope.

## Engineering principles

- **Correctness before features.** Add behavior that can be reasoned about and
  tested. Test observable behavior and failure cases; measure performance when
  it matters.
- **Simple, explicit Go.** Start with one module and a modular monolith. Add
  packages and interfaces when behavior requires them, and prefer the standard
  library. Each dependency must provide a concrete benefit.
- **Deterministic decisions.** Equivalent configuration, recordings, baseline,
  and candidate behavior should produce equivalent verification results.
  AI must not participate in the PASS/FAIL decision path.
- **Explicit resource ownership.** Design blocking work for cancellation and
  timeouts. Bound memory and concurrency, release resources, and propagate
  errors with useful context.
- **Privacy and safety by design.** Treat captured traffic as sensitive. Define
  capture limits, redaction, and replay destination controls as the relevant
  behavior is introduced.
- **Small, reviewed increments.** Keep each change focused on one coherent idea,
  validate it, and review it before proceeding. Document implemented behavior
  separately from future plans.
