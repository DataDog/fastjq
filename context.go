// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License 2.0.
//
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2026-Present Datadog, Inc.

package fastjq

import (
	"strconv"
)

// execContext threads runtime state (variables, function table, label
// sentinels, recursion depth) through the executor. A nil *execContext is a
// valid value meaning "empty context" — every helper on this type and every
// caller in the executor must be nil-safe. Phase 1+ work extends these
// structures; the surface here is the Phase 0 minimum.
type execContext struct {
	env    *envFrame           // linked-list scope frames for $-variables
	funcs  *funcTable          // user-defined function lookup chain
	labels map[string]struct{} // active label set for label/break
	depth  int                 // recursion guard, incremented on user calls
}

// envFrame is one node in the variable-binding chain. Bound values are
// materialized into a fresh []byte owned by the frame so later mutation of
// the input buffer cannot disturb a captured variable.
type envFrame struct {
	name   string
	value  []byte
	parent *envFrame
}

// lookupVar walks the env frame chain returning the first match.
// Returns (nil, false) when ctx is nil or the name is unbound.
func (ctx *execContext) lookupVar(name string) ([]byte, bool) {
	if ctx == nil {
		return nil, false
	}
	for f := ctx.env; f != nil; f = f.parent {
		if f.name == name {
			return f.value, true
		}
	}
	return nil, false
}

// bindVar returns a new *execContext with `name` bound to a copy of `value`.
// The returned context shares the rest of the receiver's state. The receiver
// is unmodified, so binds in nested scopes naturally pop on return.
// A nil receiver is allowed and is treated as an empty context.
func (ctx *execContext) bindVar(name string, value []byte) *execContext {
	frame := &envFrame{
		name:  name,
		value: append([]byte(nil), value...),
	}
	if ctx == nil {
		return &execContext{env: frame}
	}
	frame.parent = ctx.env
	out := *ctx
	out.env = frame
	return &out
}

// funcTable holds compiled user-defined functions. Functions are keyed by
// "name/arity"; lookup walks the parent chain so inner def's can shadow outer.
type funcTable struct {
	defs   map[string]*funcDef
	parent *funcTable
}

// funcDef describes a single user-defined function.
//
// Parameter naming convention follows jq: a parameter written as `$x` is a
// value parameter (the argument is evaluated to a single value before bind),
// while `x` is a filter parameter (the argument is bound by name as a closure
// expression). For Phase 0, we just record the raw parameter strings.
type funcDef struct {
	name          string
	params        []string
	valueParams   []bool
	body          *op
	capturedEnv   *envFrame
	capturedFuncs *funcTable
}

func funcKey(name string, arity int) string {
	return name + "/" + strconv.Itoa(arity)
}

// lookup returns the funcDef registered under "name/arity", walking parents.
// Returns nil when ctx (or its funcs) is nil or the function is unknown.
func (ctx *execContext) lookupFunc(key string) *funcDef {
	if ctx == nil {
		return nil
	}
	for tbl := ctx.funcs; tbl != nil; tbl = tbl.parent {
		if tbl.defs != nil {
			if def, ok := tbl.defs[key]; ok {
				return def
			}
		}
	}
	return nil
}

// errBreakLabel is the sentinel error type that unwinds an active
// `label $L | …` scope. opLabel matches its own label and absorbs the
// sentinel; non-matching labels (or absence of any matching label scope)
// must propagate up so try/catch and the top-level Run can observe them.
type errBreakLabel struct {
	label string
}

func (e *errBreakLabel) Error() string {
	return "break $" + e.label
}

type indexScopeFrame struct {
	value  []byte
	parent *indexScopeFrame
}

// execState holds per-execution dynamic state that used to live in package
// globals keyed by goroutine id. It is threaded explicitly through the
// executor so concurrent Program.Run calls stay isolated without mutexes.
type execState struct {
	ctx           *execContext
	tryDepth      int
	optionalDepth int
	indexScope    *indexScopeFrame
}

func (st execState) currentExecContext() *execContext {
	return st.ctx
}

func (st execState) withExecContext(ctx *execContext, fn func(execState) error) error {
	st.ctx = ctx
	return fn(st)
}

func (st execState) tryScopeActive() bool {
	return st.tryDepth > 0
}

func (st execState) withTryScope(fn func(execState) error) error {
	st.tryDepth++
	return fn(st)
}

func (st execState) optionalScopeActive() bool {
	return st.optionalDepth > 0
}

func (st execState) withOptionalScope(fn func(execState) error) error {
	st.optionalDepth++
	return fn(st)
}

func (st execState) currentIndexScope() []byte {
	if st.indexScope == nil {
		return nil
	}
	return st.indexScope.value
}

func (st execState) chainedIndexScope(input []byte) []byte {
	if scope := st.currentIndexScope(); scope != nil {
		return scope
	}
	return input
}

func (st execState) withIndexScope(scope []byte, fn func(execState) error) error {
	frame := indexScopeFrame{value: scope, parent: st.indexScope}
	st.indexScope = &frame
	return fn(st)
}
