<!-- Thanks for contributing to fastjq! Please fill out the sections below. -->

## Summary

<!-- What does this PR change, in 1–3 sentences? -->

## Motivation

<!-- Why is this change needed? Link issues with `Fixes #N` if applicable. -->

## Test plan

- [ ] `go test ./... -count=1` passes locally
- [ ] Added unit tests for new behaviour (or explained why none are needed)
- [ ] Fuzz tests still pass (if you touched the parser, scanner, or executor)

## Benchmark impact

<!--
Required for any change that touches the hot path (scanner, executor,
exec helpers, query/parser internals).

- [ ] Ran `go run scripts/update_benchmarks.go`
- [ ] `docs/BENCHMARKS.md` regenerated
- [ ] Allocation counts unchanged (or regression explicitly justified below)

If allocs/op changed for any benchmark, paste the relevant before/after numbers.
-->

## Documentation

For new operations or public-API changes, please confirm the following are updated:

- [ ] `README.md` — supported operations table (if applicable)
- [ ] `docs/DESIGN.md` — supported operations list / file structure
- [ ] `docs/CONSTRAINTS.md` — scope or design constraints
- [ ] `docs/SYNTAX.md` — operation reference
- [ ] `docs/BENCHMARKS.md` — regenerated via the script
- [ ] `bench_test.go` — fastjq + matching gojq benchmark for any new op
- [ ] `CHANGELOG.md` — entry under `[Unreleased]`
