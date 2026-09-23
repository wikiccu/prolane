# Security policy

## Project status and supported versions

Prolane is in its foundation stage and has no published releases. The current
CLI prints the project name and tagline; traffic capture, replay, and deployment
verification are not implemented.

During this stage, security fixes target the latest code on `main`. Older commits
do not receive separate security backports.

## Reporting a vulnerability

Do not disclose suspected vulnerability details in public issues, pull requests,
or discussions. Use a private report for issues that could compromise Prolane,
its development workflow, or data it handles.

GitHub private vulnerability reporting is not currently enabled for this
repository, and no dedicated security email address is published.

To arrange a private report, open an [issue](https://github.com/wikiccu/prolane/issues/new)
titled `Request for a private security contact`. Include only a request for a
private reporting channel. Do not include affected components, impact,
reproduction steps, logs, screenshots, or a proof of concept in that public
issue. Wait for the maintainer to provide a private channel, then send the report
through that channel. This follows [GitHub's guidance for repositories without
private reporting](https://docs.github.com/en/code-security/how-tos/report-and-fix-vulnerabilities/report-privately).

In the private report, include only the information needed to investigate:

- The affected commit and relevant operating system and Go version.
- A description of the issue, its potential impact, and any required conditions.
- Minimal reproduction steps or a proof of concept using synthetic data.
- A proposed fix or mitigation, if you have one.

For bugs without security impact and feature proposals, follow the
[contributor workflow](CONTRIBUTING.md).

## Coordinated disclosure

The maintainer will use the private reporting channel to discuss reproduction,
assess the impact, and coordinate a fix or mitigation. Please coordinate public
disclosure with the maintainer so affected users can receive useful guidance.

This policy does not promise a fixed response or remediation time.

## Sensitive information

Even in a private report, do not include production credentials, customer data,
or unredacted traffic. Use synthetic requests and clearly fake credentials in
reproduction cases and test fixtures.

Remove secrets and personal information from URLs, headers, request and response
bodies, logs, and screenshots before sharing them. In particular, check
`Authorization`, `Proxy-Authorization`, `Cookie`, and `Set-Cookie` headers.
