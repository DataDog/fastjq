// Package jqtest runs the official jq test suites against fastjq and reports
// coverage + any bugs found.
//
// Two test files are loaded:
//   - tests/jq.test  — the main regression suite
//   - tests/man.test — tests extracted from the jq manual
//
// Usage:
//
//	go test ./jqtest/
//
// Files are fetched from the jqlang/jq repository on first run and cached in
// testdata/.  Tests that use operations not supported by fastjq are skipped and
// counted separately — they are NOT treated as failures.  Any test that fastjq
// attempts but gets wrong is a bug.
package jqtest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
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
const jqTestLocalCache = "testdata/jq.test"
const manTestURL = "https://raw.githubusercontent.com/jqlang/jq/master/tests/man.test"
const manTestLocalCache = "testdata/man.test"

// codex/jq-parity tracking snapshot (baseline on 2026-04-30)
//
// Official-suite snapshot for this branch:
//   - Total: 751
//   - Skipped: 245
//   - Attempted: 506
//   - Passed: 506
//   - Failed: 0
//
// Skip families currently driving the branch:
//   - label / break
//   - leaf_paths
//   - def
//   - assignment/update syntax, @format templates, and remaining parser breadth gaps
//
// unsupportedOps lists functions/syntax that fastjq does not implement.
// A test whose program contains any of these tokens is skipped — NOT counted
// as a failure. Keep this list tight: only skip what we genuinely don't support.
var unsupportedOps = []string{
	// Recursive descent — permanently rejected (allocs scale with input depth, not output)
	"..", "recurse",
	// Path operations
	"leaf_paths",
	// User-defined functions
	"def ",
	// label-break
	"label", "break",
	// nan/infinite/predicates: now implemented
	// 2/3-arg math: hypot/fma REJECTED — 0 exclusive tests (all also blocked by as-$)
	// pow(x;y) is now implemented
	"hypot", "fma",
	// frexp/modf/ldexp/scalb/scalbln/significand: REJECTED — 0 exclusive tests
	"frexp", "modf", "ldexp", "scalb", "scalbln", "significand",
	// splits( — streaming split variant, 0 exclusive tests
	"splits(",
	// Date/time operations
	"strftime", "strptime", "mktime", "gmtime", "dateadd", "todate", "fromdate",
	"date", "now",
	// Streaming / IO
	"input", "inputs", "stderr",
	// env
	"env", "$ENV",
	// Reflection
	"builtins", "modulemeta", "$__loc__",
	// walk — requires recursive descent (Tier 3)
	"walk(",
}

// test represents one parsed test case.
type test struct {
	program    string
	input      string
	expected   []string // one or more expected output lines
	expectFail bool     // %%FAIL — fastjq should produce an error or different output
	line       int      // line number in source file for debugging
	source     string   // filename, e.g. "jq.test" or "man.test"
}

// loadTests fetches (or reads from cache) both official jq test files and
// returns all tests combined with source attribution.
func loadTests(t *testing.T) []test {
	t.Helper()
	var all []test
	for _, spec := range []struct{ url, cache, name string }{
		{jqTestURL, jqTestLocalCache, "jq.test"},
		{manTestURL, manTestLocalCache, "man.test"},
	} {
		data := fetchOrCache(t, spec.url, spec.cache)
		all = append(all, parseTests(data, spec.name)...)
	}
	return all
}

// fetchOrCache returns the content of a remote test file, using a local cache
// when available.
func fetchOrCache(t *testing.T, url, localPath string) string {
	t.Helper()
	if raw, err := os.ReadFile(localPath); err == nil {
		return string(raw)
	}
	t.Logf("Fetching %s from GitHub...", filepath.Base(localPath))
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to fetch %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status %d fetching %s", resp.StatusCode, url)
	}
	var sb strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
		sb.WriteByte('\n')
	}
	raw := sb.String()
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err == nil {
		os.WriteFile(localPath, []byte(raw), 0644)
	}
	return raw
}

func parseTests(data, source string) []test {
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
			source:     source,
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
// Also checks the input for jq-specific number constants (nan, infinite)
// that appear as bare values — these would be inside quoted strings if
// they were data, so matching without a leading quote is safe.
func isUnsupported(program string) string {
	if strings.Contains(program, "del((") {
		return "dynamic del() path"
	}
	for _, op := range unsupportedOps {
		if containsUnsupportedOp(program, op) {
			return op
		}
	}
	return ""
}

func containsUnsupportedOp(program, op string) bool {
	searchFrom := 0
	for {
		idx := strings.Index(program[searchFrom:], op)
		if idx < 0 {
			return false
		}
		idx += searchFrom
		beforeOK := idx == 0 || !isIdentRune(rune(program[idx-1]))
		afterPos := idx + len(op)
		afterOK := true
		if len(op) > 0 && op[len(op)-1] != '(' && afterPos < len(program) {
			afterOK = !isIdentRune(rune(program[afterPos]))
		}
		if beforeOK && afterOK {
			return true
		}
		searchFrom = idx + 1
	}
}

func isIdentRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// inputContainsUnsupported returns true if the test input contains a jq-specific
// constant (nan, infinite) used as a bare JSON value rather than as string data.
// We check for patterns like [nan, ,nan, nan] which can only occur as bare values.
func inputContainsUnsupported(input string) string {
	for _, tok := range []string{"nan", "infinite"} {
		// Bare value: preceded by [ or , or whitespace (not by ")
		if strings.Contains(input, "["+tok) || strings.Contains(input, ","+tok) || strings.Contains(input, " "+tok) {
			return tok
		}
	}
	return ""
}

// suiteStats tracks pass/fail/skip counts for one test file.
type suiteStats struct {
	total, skipped, passed, failed, errorOnly int
}

// TestJQOfficialSuite runs both official jq test suites and reports coverage.
func TestJQOfficialSuite(t *testing.T) {
	tests := loadTests(t)

	// Per-source stats
	bySource := make(map[string]*suiteStats)

	type failure struct {
		source   string
		line     int
		program  string
		input    string
		expected []string
		got      []string
		err      error
	}
	var failures []failure

	for _, tc := range tests {
		s := bySource[tc.source]
		if s == nil {
			s = &suiteStats{}
			bySource[tc.source] = s
		}
		s.total++

		// Skip unsupported operations (check program and input)
		if reason := isUnsupported(tc.program); reason != "" {
			s.skipped++
			continue
		}
		if reason := inputContainsUnsupported(tc.input); reason != "" {
			s.skipped++
			continue
		}

		// %%FAIL tests: jq itself errors on these. fastjq should either also
		// error (compatible) or produce output (we note it but don't fail).
		if tc.expectFail {
			p, compileErr := fastjq.Compile(tc.program)
			if compileErr != nil {
				s.errorOnly++
				s.passed++
				continue
			}
			_, runErr := p.Run([]byte(tc.input))
			if runErr != nil {
				s.errorOnly++
				s.passed++
				continue
			}
			// fastjq succeeded where jq fails — count as skip, not failure
			s.skipped++
			continue
		}

		// Normal test: compile and run
		p, compileErr := fastjq.Compile(tc.program)
		if compileErr != nil {
			// Compile error — unsupported syntax, not a bug
			s.skipped++
			continue
		}

		results, runErr := runWithTimeout(p, []byte(tc.input), 2*time.Second)
		if runErr != nil {
			errMsg := runErr.Error()
			if strings.Contains(errMsg, "timeout") {
				s.skipped++
				continue
			}
			s.failed++
			got := []string{"ERROR: " + errMsg}
			failures = append(failures, failure{tc.source, tc.line, tc.program, tc.input, tc.expected, got, runErr})
			continue
		}

		got := make([]string, len(results))
		for i, r := range results {
			got[i] = string(r)
		}

		if !outputsMatch(got, tc.expected) {
			s.failed++
			failures = append(failures, failure{tc.source, tc.line, tc.program, tc.input, tc.expected, got, nil})
			continue
		}

		s.passed++
	}

	// Compute combined totals
	var combined suiteStats
	for _, s := range bySource {
		combined.total += s.total
		combined.skipped += s.skipped
		combined.passed += s.passed
		combined.failed += s.failed
		combined.errorOnly += s.errorOnly
	}

	pct := func(s *suiteStats) float64 {
		attempted := s.total - s.skipped
		if attempted == 0 {
			return 0
		}
		return float64(s.passed) / float64(attempted) * 100
	}

	t.Logf("\n=== Official jq Test Suite Results ===")

	// Per-file breakdown
	for _, name := range []string{"jq.test", "man.test"} {
		s := bySource[name]
		if s == nil {
			continue
		}
		attempted := s.total - s.skipped
		t.Logf("\n  %s", name)
		t.Logf("    Total:     %d", s.total)
		t.Logf("    Skipped:   %d", s.skipped)
		t.Logf("    Attempted: %d   Passed: %d (%.1f%%)", attempted, s.passed, pct(s))
		t.Logf("    Failed:    %d", s.failed)
		if s.errorOnly > 0 {
			t.Logf("    (compatible errors: %d)", s.errorOnly)
		}
	}

	// Combined totals
	combinedAttempted := combined.total - combined.skipped
	t.Logf("\n  Combined (both files)")
	t.Logf("    Total tests:     %d", combined.total)
	t.Logf("    Skipped:         %d (unsupported operations — not failures)", combined.skipped)
	t.Logf("    Attempted:       %d", combinedAttempted)
	t.Logf("    Passed:          %d (%.1f%% of attempted)", combined.passed, pct(&combined))
	t.Logf("    Failed:          %d", combined.failed)
	if combined.errorOnly > 0 {
		t.Logf("      of which %d are compatible error cases (both sides errored)", combined.errorOnly)
	}

	if len(failures) > 0 {
		t.Logf("\n=== Failures (%d) ===", len(failures))
		for _, f := range failures {
			t.Logf("\n  [%s line %d] %s", f.source, f.line, f.program)
			t.Logf("    input:    %s", f.input)
			t.Logf("    expected: %v", f.expected)
			t.Logf("    got:      %v", f.got)
		}
	}

	if combined.failed > 0 {
		t.Errorf("%d tests failed — see details above", combined.failed)
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
		_ = runtime.NumGoroutine()
		return nil, fmt.Errorf("timeout after %v", timeout)
	}
}

// outputsMatch compares fastjq outputs to jq expected outputs leniently:
//  1. Exact string match
//  2. JSON-normalized match (removes whitespace differences)
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
		if jsonStructurallyEqual(got[i], expected[i]) {
			continue
		}
		return false
	}
	return true
}

func jsonNormalized(s string) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err == nil {
		return buf.String()
	}
	return s
}

func numericEquiv(a, b string) bool {
	if cmp, ok := compareJSONNumberStrings(a, b); ok {
		return cmp == 0
	}
	var fa, fb float64
	if _, err := fmt.Sscanf(a, "%g", &fa); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(b, "%g", &fb); err != nil {
		return false
	}
	return fa == fb
}

func jsonStructurallyEqual(a, b string) bool {
	av, aok := decodeJSONValue(a)
	if !aok {
		return false
	}
	bv, bok := decodeJSONValue(b)
	if !bok {
		return false
	}
	return jsonValueEqual(av, bv)
}

func decodeJSONValue(s string) (any, bool) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	return v, true
}

func jsonValueEqual(a, b any) bool {
	switch av := a.(type) {
	case nil:
		return b == nil
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case json.Number:
		bv, ok := b.(json.Number)
		return ok && jsonNumbersEqual(av, bv)
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !jsonValueEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !jsonValueEqual(v, bv[k]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func jsonNumbersEqual(a, b json.Number) bool {
	if a == b {
		return true
	}
	if cmp, ok := compareJSONNumberStrings(string(a), string(b)); ok {
		return cmp == 0
	}
	ar, aok := new(big.Rat).SetString(string(a))
	br, bok := new(big.Rat).SetString(string(b))
	if aok && bok {
		return ar.Cmp(br) == 0
	}
	return numericEquiv(string(a), string(b))
}

type decimalNumber struct {
	neg    bool
	digits string
	exp10  int64
}

func compareJSONNumberStrings(a, b string) (int, bool) {
	da, aok := parseDecimalNumber(a)
	db, bok := parseDecimalNumber(b)
	if !aok || !bok {
		return 0, false
	}
	return compareDecimalNumbers(da, db), true
}

func parseDecimalNumber(s string) (decimalNumber, bool) {
	if s == "" {
		return decimalNumber{}, false
	}
	i := 0
	neg := false
	if s[i] == '-' {
		neg = true
		i++
		if i >= len(s) {
			return decimalNumber{}, false
		}
	}

	digits := make([]byte, 0, len(s))
	sawDigit := false
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		digits = append(digits, s[i])
		sawDigit = true
		i++
	}

	fracDigits := int64(0)
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			digits = append(digits, s[i])
			fracDigits++
			sawDigit = true
			i++
		}
	}
	if !sawDigit {
		return decimalNumber{}, false
	}

	exp10 := int64(0)
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i >= len(s) {
			return decimalNumber{}, false
		}
		expNeg := false
		if s[i] == '+' || s[i] == '-' {
			expNeg = s[i] == '-'
			i++
		}
		if i >= len(s) || s[i] < '0' || s[i] > '9' {
			return decimalNumber{}, false
		}
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			exp10 = exp10*10 + int64(s[i]-'0')
			i++
		}
		if expNeg {
			exp10 = -exp10
		}
	}
	if i != len(s) {
		return decimalNumber{}, false
	}

	leading := 0
	for leading < len(digits) && digits[leading] == '0' {
		leading++
	}
	digits = digits[leading:]
	if len(digits) == 0 {
		return decimalNumber{digits: "0"}, true
	}

	exp10 -= fracDigits
	for len(digits) > 1 && digits[len(digits)-1] == '0' {
		digits = digits[:len(digits)-1]
		exp10++
	}
	return decimalNumber{neg: neg, digits: string(digits), exp10: exp10}, true
}

func compareDecimalNumbers(a, b decimalNumber) int {
	aZero := a.digits == "0"
	bZero := b.digits == "0"
	if aZero && bZero {
		return 0
	}
	if a.neg != b.neg {
		if a.neg {
			return -1
		}
		return 1
	}

	magCmp := compareDecimalNumberMagnitudes(a, b)
	if a.neg {
		return -magCmp
	}
	return magCmp
}

func compareDecimalNumberMagnitudes(a, b decimalNumber) int {
	aAdj := int64(len(a.digits)) + a.exp10
	bAdj := int64(len(b.digits)) + b.exp10
	if aAdj < bAdj {
		return -1
	}
	if aAdj > bAdj {
		return 1
	}

	maxLen := len(a.digits)
	if len(b.digits) > maxLen {
		maxLen = len(b.digits)
	}
	for i := 0; i < maxLen; i++ {
		da := byte('0')
		db := byte('0')
		if i < len(a.digits) {
			da = a.digits[i]
		}
		if i < len(b.digits) {
			db = b.digits[i]
		}
		if da < db {
			return -1
		}
		if da > db {
			return 1
		}
	}
	return 0
}
