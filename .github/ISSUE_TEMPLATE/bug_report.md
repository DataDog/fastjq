---
name: Bug report
about: Report incorrect behaviour, a panic, or a regression in fastjq
title: ''
labels: bug
assignees: ''
---

## Description

<!-- A clear description of the bug. -->

## Reproducer

**Query:**

```jq
<!-- e.g. select(.level == "error") -->
```

**Input:**

```json
<!-- the JSON document that triggers the issue -->
```

**Expected output:**

```
<!-- what jq / the docs say should happen -->
```

**Actual output:**

```
<!-- what fastjq returned (or the error/panic) -->
```

A minimal Go reproducer using `fastjq.Compile` + `Run` is even better — paste it
in a code block below if you have one.

## Environment

- fastjq version: `<output of: go list -m github.com/DataDog/fastjq>`
- Go version: `<output of: go version>`
- OS / arch:

## Additional context

<!-- Anything else that might help — related operations, recent changes, etc. -->
