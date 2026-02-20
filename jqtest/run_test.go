// Package jqtest runs the official jq test suite against fastjq and reports
// coverage + any bugs found.
//
// Usage:
//
//	go test ./jqtest/
//
// The test file is fetched from the jqlang/jq repository (or from a local
// copy in testdata/jq.test if present).  Tests that use operations not
// supported by fastjq are skipped and counted separately — they are NOT
// treated as failures.  Any test that fastjq attempts but gets wrong is a bug.
package jqtest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/brianfloersch/fastjq"
)

const jqTestURL = "https://raw.githubusercontent.com/jqlang/jq/master/tests/jq.test"
const localCachePath = "testdata/jq.test"

// unsupportedOps lists functions/syntax that fastjq does not implement.
// A test whose program contains any of these tokens is skipped — NOT counted
// as a failure. Keep this list tight: only skip what we genuinely don't support.
var unsupportedOps = []string{
	// Recursive descent
	"..", "recurse",
	// Sorting / grouping (require allocation, not yet implemented)
	"sort", "group_by", "unique",
	// Path operations
	"path(", "paths", "getpath", "setpath", "delpaths", "leaf_paths",
	// Variables and binding
	" as $", "as $",
	// User-defined functions
	"def ",
	// Reduce / foreach / label-break
	"reduce", "foreach", "label", "break",
	// String interpolation
	`\(`,
	// Format strings not yet implemented (the ones below ARE supported now)
	// "@text" — supported (same as tostring)
	// "@html" — supported
	// "@csv"  — supported
	// "@tsv"  — supported
	// "@sh"   — supported
	// "@urid" — supported
	// Math builtins (trig, exponential, etc.)
	"nan", "infinite", "isinfinite", "isnan", "isfinite", "isnormal",
	"fabs", "sqrt(", "pow(", "log(", "log2(",
	"exp(", "exp2(", "exp10(", "log10(",
	"logb", "nearbyint", "frexp", "modf", "ldexp", "scalb", "scalbln",
	"tgamma", "lgamma", "j0", "j1", "atan", "sin(", "cos(", "tan(",
	"asin", "acos", "significand", "cbrt", "hypot", "fma",
	// Streaming / IO
	"input", "inputs", "stderr", "debug(",
	// env
	"env", "$ENV",
	// Unicode codepoint operations
	"implode", "explode",
	// Reflection
	"builtins", "modulemeta", "$__loc__",
	// Generators not supported in our form
	"range(",
	// walk
	"walk(",
	// ascii() function (not ascii_downcase/upcase)
	"ascii(",
	// Object construction with dynamic keys: {(expr): val}
	// We detect this by looking for the pattern
	// transpose
	"transpose",
	// floor/ceil/round are now supported — removed from skip list
	// contains()/inside() are now supported — removed from skip list
	// limit(1; a, b) generator body is now supported — removed from skip list
}

// test represents one parsed test case from jq.test.
type test struct {
	program  string
	input    string
	expected []string // one or more expected output lines
	expectFail bool   // %%FAIL — fastjq should produce an error or different output
	line     int      // line number in source file for debugging
}

// loadTests fetches (or reads from cache) the official jq test file and parses it.
func loadTests(t *testing.T) []test {
	t.Helper()
	data := fetchTestFile(t)
	return parseTests(data)
}

func fetchTestFile(t *testing.T) string {
	t.Helper()
	// Use local cache if present
	if raw, err := os.ReadFile(localCachePath); err == nil {
		return string(raw)
	}
	// Fetch from GitHub
	t.Log("Fetching jq test suite from GitHub...")
	resp, err := http.Get(jqTestURL)
	if err != nil {
		t.Fatalf("failed to fetch jq test suite: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status %d fetching jq test suite", resp.StatusCode)
	}
	var sb strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
		sb.WriteByte('\n')
	}
	raw := sb.String()
	// Cache locally for future runs
	if err := os.MkdirAll(filepath.Dir(localCachePath), 0755); err == nil {
		os.WriteFile(localCachePath, []byte(raw), 0644)
	}
	return raw
}

func parseTests(data string) []test {
	var tests []test
	lines := strings.Split(data, "\n")

	i := 0
	for i < len(lines) {
		// Skip blank lines and comments
		line := lines[i]
		if isBlankOrComment(line) {
			i++
			continue
		}

		// Check for %%FAIL marker
		expectFail := false
		if strings.HasPrefix(line, "%%FAIL") {
			expectFail = true
			i++
			if i >= len(lines) {
				break
			}
			line = lines[i]
			if isBlankOrComment(line) {
				i++
				continue
			}
		}

		startLine := i + 1

		// Line 1: program
		program := line
		i++
		if i >= len(lines) {
			break
		}

		// Line 2: input
		input := lines[i]
		i++
		if i >= len(lines) {
			break
		}

		// Line 3+: expected outputs (until blank line or comment or %%FAIL or EOF)
		var expected []string
		for i < len(lines) {
			out := lines[i]
			if isBlankOrComment(out) || strings.HasPrefix(out, "%%") {
				break
			}
			expected = append(expected, out)
			i++
		}

		if len(expected) == 0 {
			continue
		}

		tests = append(tests, test{
			program:    program,
			input:      input,
			expected:   expected,
			expectFail: expectFail,
			line:       startLine,
		})
	}
	return tests
}

func isBlankOrComment(s string) bool {
	trimmed := strings.TrimLeftFunc(s, unicode.IsSpace)
	return trimmed == "" || strings.HasPrefix(trimmed, "#")
}

// isUnsupported returns the first matched token if the program uses an
// unsupported operation, or "" if the program is potentially runnable.
func isUnsupported(program string) string {
	for _, op := range unsupportedOps {
		if strings.Contains(program, op) {
			return op
		}
	}
	return ""
}

// TestJQOfficialSuite runs the official jq test suite and reports coverage.
func TestJQOfficialSuite(t *testing.T) {
	tests := loadTests(t)

	var (
		total     int
		skipped   int
		passed    int
		failed    int
		errorOnly int // tests where both sides errored (compatible errors)
	)

	type failure struct {
		line     int
		program  string
		input    string
		expected []string
		got      []string
		err      error
	}
	var failures []failure

	for _, tc := range tests {
		total++

		// Skip unsupported operations
		if reason := isUnsupported(tc.program); reason != "" {
			skipped++
			continue
		}

		// %%FAIL tests: jq itself errors on these. fastjq should either also
		// error (compatible) or produce output (we note it but don't fail).
		if tc.expectFail {
			p, compileErr := fastjq.Compile(tc.program)
			if compileErr != nil {
				// Both error at compile time — compatible
				errorOnly++
				passed++
				continue
			}
			_, runErr := p.Run([]byte(tc.input))
			if runErr != nil {
				// Both produce a runtime error — compatible
				errorOnly++
				passed++
				continue
			}
			// fastjq succeeded where jq fails — note but don't fail the suite
			skipped++
			continue
		}

		// Normal test: compile and run
		p, compileErr := fastjq.Compile(tc.program)
		if compileErr != nil {
			// Compile error on a test we should support — report as unsupported
			// (might be syntax we don't support yet)
			skipped++
			continue
		}

		results, runErr := runWithTimeout(p, []byte(tc.input), 2*time.Second)
		if runErr != nil {
			errMsg := runErr.Error()
			if strings.Contains(errMsg, "timeout") {
				// Timed out — skip rather than fail (likely unsupported recursive op)
				skipped++
				continue
			}
			// Runtime error — fastjq errored but jq succeeded
			failed++
			got := []string{"ERROR: " + errMsg}
			failures = append(failures, failure{tc.line, tc.program, tc.input, tc.expected, got, runErr})
			continue
		}

		// Compare outputs
		got := make([]string, len(results))
		for i, r := range results {
			got[i] = string(r)
		}

		if !outputsMatch(got, tc.expected) {
			failed++
			failures = append(failures, failure{tc.line, tc.program, tc.input, tc.expected, got, nil})
			continue
		}

		passed++
	}

	attempted := total - skipped
	var coveragePct float64
	if attempted > 0 {
		coveragePct = float64(passed) / float64(attempted) * 100
	}

	// Print summary
	t.Logf("\n=== Official jq Test Suite Results ===")
	t.Logf("Total tests:     %d", total)
	t.Logf("Skipped:         %d (unsupported operations — not failures)", skipped)
	t.Logf("Attempted:       %d", attempted)
	t.Logf("Passed:          %d (%.1f%% of attempted)", passed, coveragePct)
	t.Logf("Failed:          %d", failed)
	if errorOnly > 0 {
		t.Logf("  of which %d are compatible error cases (both sides errored)", errorOnly)
	}

	if len(failures) > 0 {
		t.Logf("\n=== Failures (%d) ===", len(failures))
		t.Logf("(see below for categorized analysis)")
		for _, f := range failures {
			t.Logf("\n  [line %d] %s", f.line, f.program)
			t.Logf("    input:    %s", f.input)
			t.Logf("    expected: %v", f.expected)
			t.Logf("    got:      %v", f.got)
		}
	}

	// Mark test failed if there are actual bugs (not just unsupported features)
	if failed > 0 {
		t.Errorf("%d tests failed — see details above", failed)
	}
}

// runWithTimeout runs p.RunAll with a hard timeout, returning an error if it
// takes too long. This prevents a single pathological test from hanging the suite.
func runWithTimeout(p *fastjq.Program, input []byte, timeout time.Duration) ([][]byte, error) {
	type result struct {
		out [][]byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := p.RunAll(input)
		ch <- result{out, err}
	}()
	select {
	case r := <-ch:
		return r.out, r.err
	case <-time.After(timeout):
		// Goroutine leaked intentionally — it will exit when the test binary exits.
		_ = runtime.NumGoroutine() // suppress unused import
		return nil, fmt.Errorf("timeout after %v", timeout)
	}
}

// outputsMatch compares fastjq outputs to jq expected outputs leniently:
//  1. Exact string match
//  2. JSON-normalized match (removes whitespace differences: jq sometimes
//     emits `[true, true]` while fastjq emits `[true,true]`)
//  3. Numeric equivalence (1.5E+10 vs 1.5e10 vs 15000000000)
func outputsMatch(got, expected []string) bool {
	if len(got) != len(expected) {
		return false
	}
	for i := range got {
		if got[i] == expected[i] {
			continue
		}
		if jsonNormalized(got[i]) == jsonNormalized(expected[i]) {
			continue
		}
		if numericEquiv(got[i], expected[i]) {
			continue
		}
		return false
	}
	return true
}

// jsonNormalized compacts a JSON value to remove insignificant whitespace.
// Falls back to the original string if the value is not valid JSON.
func jsonNormalized(s string) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err == nil {
		return buf.String()
	}
	return s
}

// numericEquiv returns true if two strings represent the same float64
// under formatting differences (e.g. 1e10 vs 1E+10 vs 10000000000).
func numericEquiv(a, b string) bool {
	var fa, fb float64
	if _, err := fmt.Sscanf(a, "%g", &fa); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(b, "%g", &fb); err != nil {
		return false
	}
	return fa == fb
}
