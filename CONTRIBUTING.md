# Contributing to fastjq

Thanks for your interest! A few notes before you open a PR:

- Run `go test ./... -count=1` and make sure it passes.
- If you touch the hot path (scanner / executor / query), regenerate
  benchmarks with `go run scripts/update_benchmarks.go` — never edit
  `docs/BENCHMARKS.md` by hand.
- Update the relevant docs (`docs/SYNTAX.md`, `docs/DESIGN.md`,
  `README.md`) and add a `CHANGELOG.md` entry for any user-visible change.
- The full developer workflow is in [`AGENTS.md`](AGENTS.md).

## Non-negotiable design constraints

- **Zero allocations on the hot path.** `RunWithBuffer` and `RunFunc` must
  hit 0 allocs/op at steady state.
- **No marshal/unmarshal.** The library only ever operates on raw `[]byte`.
- **Output is compact JSON.** No pretty-printing.

## Licensing

fastjq is licensed under the Apache License 2.0. By submitting a
contribution you agree it will be licensed under the same terms.
