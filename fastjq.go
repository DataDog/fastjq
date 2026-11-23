// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License 2.0.
//
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2026-Present Datadog, Inc.

// Package fastjq provides a fast, zero-allocation JQ engine for Go.
// It operates directly on raw []byte using slice offsets — no interface{},
// no map[string]interface{}, no marshal/unmarshal cycle.
package fastjq

import (
	"bytes"
	"fmt"
	"strings"
)

// Program is a compiled jq query, safe for concurrent use.
// Use Compile to create one.
type Program struct {
	root *op
	// vars holds the names declared with WithVariables, without the leading
	// '$', in declaration order. Run methods take their values in that order.
	vars []string
	host *hostNames
}

// CompileOption configures Compile.
type CompileOption func(*compileConfig) error

type compileConfig struct {
	vars []string
}

// WithVariables declares host variables for the query. Each name is written
// with its leading '$', for example WithVariables("$api_key", "$env"), and the
// query can then refer to it as $api_key. The values are supplied positionally,
// in declaration order, as trailing arguments to Run, RunWithBuffer, RunAll or
// RunFunc, each one a JSON document.
//
// Passing data this way keeps it out of the query text, so it cannot change
// how the query parses and it never shows up in a compile error. Names must be
// jq identifiers, must be unique, and may not be one of the built-in variables
// $__loc__ or $ENV. A variable bound inside the query with `as $name` shadows
// a host variable of the same name.
func WithVariables(names ...string) CompileOption {
	return func(c *compileConfig) error {
		for _, name := range names {
			bare, err := validateHostVarName(name)
			if err != nil {
				return err
			}
			for _, have := range c.vars {
				if have == bare {
					return fmt.Errorf("variable %s declared more than once", name)
				}
			}
			c.vars = append(c.vars, bare)
		}
		return nil
	}
}

func validateHostVarName(name string) (string, error) {
	if !strings.HasPrefix(name, "$") {
		return "", fmt.Errorf("variable name %q must start with '$'", name)
	}
	bare := name[1:]
	if bare == "" || !isIdentStart(bare[0]) {
		return "", fmt.Errorf("invalid variable name %q", name)
	}
	if rest, _ := readIdentifier(bare); len(rest) != len(bare) {
		return "", fmt.Errorf("invalid variable name %q", name)
	}
	if bare == "__loc__" || bare == "ENV" {
		return "", fmt.Errorf("variable name %q is reserved", name)
	}
	return bare, nil
}

// Compile parses and compiles a jq query string into a reusable Program.
// The returned Program can be used concurrently from multiple goroutines.
func Compile(query string, opts ...CompileOption) (*Program, error) {
	var cfg compileConfig
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	root, err := parseWithVars(query, cfg.vars)
	if err != nil {
		return nil, err
	}
	p := &Program{root: root, vars: cfg.vars}
	if len(cfg.vars) > 0 {
		p.host = &hostNames{names: cfg.vars}
	}
	return p, nil
}

// errVarCount reports a mismatch between declared variables and supplied values.
func (p *Program) errVarCount(got int) error {
	return fmt.Errorf("query declares %d variable(s) but %d value(s) were supplied", len(p.vars), got)
}

// state checks the value count and returns the initial executor state for a
// run. An empty value is an error rather than silently producing invalid
// output.
func (p *Program) state(vals [][]byte) (execState, error) {
	if len(vals) != len(p.vars) {
		return execState{}, p.errVarCount(len(vals))
	}
	for i, v := range vals {
		if len(bytes.TrimSpace(v)) == 0 {
			return execState{}, fmt.Errorf("value for $%s is empty", p.vars[i])
		}
	}
	return execState{host: p.host, hostVals: vals}, nil
}

// stripBOM removes a UTF-8 BOM (EF BB BF) prefix if present.
// jq silently strips the BOM before parsing; we do the same.
func stripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

// normalizeNaNInf converts top-level NaN/infinite sentinel bytes to null,
// maintaining valid JSON output. Values nested inside arrays/objects are
// converted by the array/object construction code paths.
// "NaN" → "null", "infinite" → "null", "-infinite" → "null"
func normalizeNaNInf(v []byte) []byte {
	if len(v) >= 3 && v[0] == 'N' && v[1] == 'a' && v[2] == 'N' {
		return []byte("null")
	}
	if len(v) >= 8 && v[0] == 'i' && v[1] == 'n' && v[2] == 'f' {
		return []byte("null")
	}
	if len(v) >= 9 && v[0] == '-' && v[1] == 'i' && v[2] == 'n' {
		return []byte("null")
	}
	return v
}

// Run executes the compiled query against the input JSON bytes.
// Returns the first result as a new byte slice. For multi-output queries
// (e.g. .[]), only the first result is returned; use RunAll or RunFunc
// for all results.
//
// vals supplies one JSON value per variable declared with WithVariables, in
// declaration order. The count must match exactly.
func (p *Program) Run(input []byte, vals ...[]byte) ([]byte, error) {
	st, err := p.state(vals)
	if err != nil {
		return nil, err
	}
	input = stripBOM(input)
	buf := make([]byte, 0, len(input))
	result, err := exec(st, p.root, input, buf)
	if err != nil {
		return nil, err
	}
	return normalizeNaNInf(result), nil
}

// RunWithBuffer executes the compiled query, reusing the provided buffer
// to avoid allocations. The buffer is reset (buf[:0]) before use.
// Callers can pass the same buffer across calls for zero steady-state allocations.
// For multi-output queries, only the first result is returned. vals binds
// host variables as in Run.
func (p *Program) RunWithBuffer(input []byte, buf []byte, vals ...[]byte) ([]byte, error) {
	st, err := p.state(vals)
	if err != nil {
		return nil, err
	}
	buf = buf[:0]
	result, err := exec(st, p.root, stripBOM(input), buf)
	if err != nil {
		return nil, err
	}
	return normalizeNaNInf(result), nil
}

// RunAll executes the compiled query and collects all results.
// For single-output queries this returns a slice of length 1.
// For iterators (.[] etc) this returns one result per element.
// vals binds host variables as in Run.
func (p *Program) RunAll(input []byte, vals ...[]byte) ([][]byte, error) {
	st, err := p.state(vals)
	if err != nil {
		return nil, err
	}
	input = stripBOM(input)
	var results [][]byte
	err = execMulti(st, p.root, input, nil, func(result []byte) error {
		result = normalizeNaNInf(result)
		// Copy result since buf may be reused
		cp := make([]byte, len(result))
		copy(cp, result)
		results = append(results, cp)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// RunFunc executes the compiled query and calls fn for each result.
// This is the most efficient multi-output API — it avoids allocating
// result slices. The result bytes passed to fn are only valid for the
// duration of the callback.
//
// Returning a non-nil error from fn stops iteration; RunFunc returns that error
// unchanged. A jq `try` does not catch it. vals binds host variables as in Run.
func (p *Program) RunFunc(input []byte, fn func(result []byte) error, vals ...[]byte) error {
	st, err := p.state(vals)
	if err != nil {
		return err
	}
	err = execMulti(st, p.root, stripBOM(input), nil, func(result []byte) error {
		if err := fn(normalizeNaNInf(result)); err != nil {
			return &callbackError{err: err}
		}
		return nil
	})
	// Hand the caller's own error back, unwrapping whatever opTry layers it
	// picked up on the way out.
	if cause, ok := callbackErrorCause(err); ok {
		return cause
	}
	return err
}
