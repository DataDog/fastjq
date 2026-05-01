# Security Policy

## Reporting a vulnerability

If you discover a security issue in fastjq, **please do not open a public
GitHub issue**.

Instead, report it privately to <security@datadoghq.com>. Include:

- A description of the vulnerability and its impact.
- Steps to reproduce, ideally with a minimal Go program or a JSON input + jq
  query that triggers the issue.
- The fastjq version (`go list -m github.com/DataDog/fastjq`) and Go version.

We will acknowledge your report and work with you on remediation. Once a fix
is released we will credit you in the release notes (unless you prefer to
remain anonymous).

## Supported versions

While fastjq has not yet cut a tagged release, security fixes will be applied
to the `master` branch. Once tagged releases exist this section will be
updated with the supported version range.

## Threat model

fastjq is a JSON-bytes-in, JSON-bytes-out library. It does **not** validate
its input — callers are expected to pass valid JSON (use `json.Valid` if you
cannot guarantee that). Behaviour on malformed input is undefined but is
covered by `go test -fuzz` to ensure the library never panics. Reports of
crashes, infinite loops, or excessive memory use on any byte input are in
scope.

Out of scope: behaviour when the caller misuses the API (e.g. mutates input
during execution, shares a buffer between concurrent goroutines).
