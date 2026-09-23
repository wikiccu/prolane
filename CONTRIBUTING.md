# Contributing to Prolane

Prolane is in its foundation stage. Read the [current status and planned
scope](README.md) before proposing work. Contributions should move the project
forward through small, reviewable changes.

## Discuss the change

Search existing issues before reporting a bug or proposing a feature. For a bug,
include reproduction steps, expected and actual behavior, your Go version and
operating system, and relevant output with sensitive information removed.

Small fixes and documentation corrections can go directly to a pull request.
For larger features or architectural changes, open an issue first. Describe the
problem, intended user workflow, scope, and alternatives. Agree on a short design
covering failure cases, testing, and relevant privacy or performance concerns
before implementing a large change.

Do not post suspected vulnerability details, credentials, or sensitive traffic
in public issues or pull requests. Follow the [security policy](SECURITY.md) for
vulnerability reporting and sensitive information handling.

## Set up development

Use the Go version declared in [go.mod](go.mod) to match CI. Install Git, GNU
Make, and the golangci-lint version listed in the [development
instructions](README.md#development-commands). That section includes Windows
setup notes and equivalent commands for working without Make.

Clone the repository, or your fork if you do not have write access. Create a
branch from the current `main` branch for your change. Run development commands
from the repository root.

## Keep the change focused

- Follow the [engineering principles](README.md#engineering-principles). Prefer
  explicit Go, standard library functionality, and concrete implementations.
  Explain the need for any new dependency or abstraction.
- Keep each commit focused on one coherent change. Leave unrelated cleanup and
  future features for separate reviews.
- Add or update tests when behavior changes. Use Go's standard testing tools and
  cover observable behavior and meaningful failure cases. Avoid arbitrary sleeps
  where explicit synchronization is possible.
- Update affected documentation and examples. Clearly distinguish implemented
  behavior from planned capabilities.

## Validate your work

Discover the available commands, format Go code, and run the checks:

```sh
make help
make fmt
make verify
```

`make fmt` rewrites Go source formatting. `make verify` checks formatting, runs
tests, vet, and lint, and builds the CLI. The CLI smoke test builds a temporary
executable and checks its output and exit status.

Review the diff after validation. For documentation changes, check links and
examples against the repository. Run the relevant checks for any code, tooling,
or documented commands you change. State which checks you actually ran and any
limitations; do not describe an unexecuted check as passing.

## Submit for review

Use Conventional Commit messages in the form `type(scope): summary`, with an
optional scope. For example: `test(cli): cover root invocation behavior`.

Open a pull request against `main` that explains the problem, resulting behavior,
and validation performed. Link a related issue when one exists. Call out changes
to CLI behavior, output, dependencies, or other user-facing contracts, and explain
any relevant compatibility, performance, or privacy tradeoffs.

The [Quality workflow](.github/workflows/quality.yml) is configured to run
`make verify` on Ubuntu. Address check failures and review feedback before the
change is merged. Keep follow-up ideas separate from the current change.
