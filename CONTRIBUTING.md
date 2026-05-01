# Contributing to fastjq

Thanks for your interest in contributing! fastjq is a small, focused library
with strong invariants — please read this page before opening a non-trivial PR.

## Filing issues

- Bugs: include the query, an input that triggers the issue, the expected and
  actual output, the fastjq version (`go list -m github.com/DataDog/fastjq`),
  and the Go version. A minimal reproducer in a runnable form is ideal.
- Feature requests: link to the relevant jq documentation / behaviour, and
  describe the expected allocation tier (zero allocations on the hot path is a
  hard constraint — see [docs/CONSTRAINTS.md](docs/CONSTRAINTS.md)).
- Security issues: do **not** open a public issue. See [SECURITY.md](SECURITY.md).

## Pull requests

1. Fork the repo and create a feature branch.
2. Make your change.
3. Run the test suite — every change must keep `go test ./... -count=1`
   passing locally.
4. Run benchmarks if your change touches the hot path:
   `go run scripts/update_benchmarks.go`. This regenerates
   [`docs/BENCHMARKS.md`](docs/BENCHMARKS.md) automatically — never edit that
   file by hand.
5. Update documentation. Any change that adds or changes an operation must
   update `docs/DESIGN.md`, `docs/SYNTAX.md`, the supported-ops table in
   `README.md`, and add an entry to `CHANGELOG.md`.
6. Open a PR against `master`. The PR template will prompt you for the
   information reviewers need.

The full developer workflow (test → benchmark → doc updates) is documented in
[`AGENTS.md`](AGENTS.md). Read it before working on a non-trivial change.

## Design constraints

These are non-negotiable. Please discuss in an issue first if you think a
change should violate any of them:

- **Zero allocations on the hot path.** `RunWithBuffer` and `RunFunc` must
  achieve 0 allocs/op at steady state for all supported operations.
- **No marshal/unmarshal.** The library never converts input to `interface{}`,
  `map[string]interface{}`, or any other Go type — only raw `[]byte`.
- **Output is compact JSON only.** No pretty printing.
- **Scope is intentionally limited.** fastjq is not a full jq implementation.
  See `docs/SYNTAX.md` for which operations are out of scope and why.

## Licensing of contributions

fastjq is licensed under the Apache License 2.0. By submitting a contribution,
you agree that your contribution will be licensed under the same terms. Please
make sure your work is yours to contribute (no copy-pasted code from
incompatibly-licensed sources).
