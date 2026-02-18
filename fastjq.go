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

// Run executes the compiled query against the input JSON bytes.
// Returns the result as a new byte slice.
func (p *Program) Run(input []byte) ([]byte, error) {
	buf := make([]byte, 0, len(input))
	return exec(p.root, input, buf)
}

// RunWithBuffer executes the compiled query, reusing the provided buffer
// to avoid allocations. The buffer is reset (buf[:0]) before use.
// Callers can pass the same buffer across calls for zero steady-state allocations.
func (p *Program) RunWithBuffer(input []byte, buf []byte) ([]byte, error) {
	buf = buf[:0]
	return exec(p.root, input, buf)
}
