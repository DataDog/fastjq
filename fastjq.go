// Package fastjq provides a fast, zero-allocation JQ engine for Go.
// It operates directly on raw []byte using slice offsets — no interface{},
// no map[string]interface{}, no marshal/unmarshal cycle.
package fastjq

// Program is a compiled jq query, safe for concurrent use.
// Use Compile to create one.
type Program struct {
	root *op
}

// Compile parses and compiles a jq query string into a reusable Program.
// The returned Program can be used concurrently from multiple goroutines.
func Compile(query string) (*Program, error) {
	root, err := parse(query)
	if err != nil {
		return nil, err
	}
	return &Program{root: root}, nil
}

// stripBOM removes a UTF-8 BOM (EF BB BF) prefix if present.
// jq silently strips the BOM before parsing; we do the same.
func stripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

// Run executes the compiled query against the input JSON bytes.
// Returns the first result as a new byte slice. For multi-output queries
// (e.g. .[]), only the first result is returned; use RunAll or RunFunc
// for all results.
func (p *Program) Run(input []byte) ([]byte, error) {
	input = stripBOM(input)
	buf := make([]byte, 0, len(input))
	return exec(p.root, input, buf)
}

// RunWithBuffer executes the compiled query, reusing the provided buffer
// to avoid allocations. The buffer is reset (buf[:0]) before use.
// Callers can pass the same buffer across calls for zero steady-state allocations.
// For multi-output queries, only the first result is returned.
func (p *Program) RunWithBuffer(input []byte, buf []byte) ([]byte, error) {
	buf = buf[:0]
	return exec(p.root, stripBOM(input), buf)
}

// RunAll executes the compiled query and collects all results.
// For single-output queries this returns a slice of length 1.
// For iterators (.[] etc) this returns one result per element.
func (p *Program) RunAll(input []byte) ([][]byte, error) {
	input = stripBOM(input)
	var results [][]byte
	err := execMulti(p.root, input, nil, func(result []byte) error {
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
func (p *Program) RunFunc(input []byte, fn func(result []byte) error) error {
	return execMulti(p.root, stripBOM(input), nil, fn)
}
