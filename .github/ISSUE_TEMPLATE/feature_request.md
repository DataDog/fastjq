---
name: Feature request
about: Propose support for a new jq operation or library API
title: ''
labels: enhancement
assignees: ''
---

## Operation / feature

<!--
Which jq operation or fastjq API are you proposing?
e.g. `walk`, `paths`, a new option on `RunFunc`, etc.
-->

## Reference

<!-- Link to the relevant jq manual section or upstream tests. -->

## Expected allocation tier

fastjq has a strict allocation budget — please pick one:

- [ ] **Zero allocations on the hot path** (must be implementable on raw bytes
      without buffering structured data; see `docs/CONSTRAINTS.md`).
- [ ] **Allocates proportional to result size only** (e.g. produces a new
      string / array — like `@base64`, `match`).
- [ ] **Allocates per element of input** (acceptable only with explicit
      tradeoff discussion — see `docs/CONSTRAINTS.md` and `docs/SYNTAX.md`).
- [ ] Not sure — happy to discuss.

## Motivation

<!-- What problem does this solve? Real-world use case is most useful. -->

## Additional context

<!-- Edge cases, alternatives considered, etc. -->
