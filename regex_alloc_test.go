package fastjq

// regex_alloc_test.go — benchmarks probing Go's regexp allocation behaviour,
// plus correctness tests and fastjq benchmarks for test(re) and match(re).
//
// We test the operations that would map to jq's:
//   test(re)    → re.Match([]byte)
//   match(re)   → re.FindSubmatch([]byte)  — returns [][]byte
//   capture(re) → re.FindSubmatch + subgroup extraction
//   sub(re;s)   → re.ReplaceAll([]byte, []byte)
//   gsub(re;s)  → same
//
// The regexp is compiled once (at Compile time in a real implementation).
// Only the per-call execution is benchmarked.

import (
	"regexp"
	"strings"
	"testing"
)

var (
	reSimple  = regexp.MustCompile(`error`)
	reComplex = regexp.MustCompile(`(?i)^(error|warn|fatal):\s+(.+)$`)
	reCapture = regexp.MustCompile(`(\w+)@(\w+)\.(\w+)`)

	inputMatch   = []byte(`"error: connection timeout after 30s"`)
	inputNoMatch = []byte(`"info: all systems normal"`)
	inputCapture = []byte(`"user@example.com"`)
	inputLong    = []byte(`"this is a longer string that does not match the pattern and requires scanning the full thing"`)
)

// --- test(re) — re.Match ---

func BenchmarkRegexp_Match_Hit(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		reSimple.Match(inputMatch)
	}
}

func BenchmarkRegexp_Match_Miss(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		reSimple.Match(inputNoMatch)
	}
}

func BenchmarkRegexp_Match_Complex_Hit(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		reComplex.Match(inputMatch)
	}
}

func BenchmarkRegexp_Match_Long_Miss(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		reSimple.Match(inputLong)
	}
}

// MatchString for completeness (string vs []byte)
func BenchmarkRegexp_MatchString(b *testing.B) {
	s := string(inputMatch)
	b.ReportAllocs()
	for b.Loop() {
		reSimple.MatchString(s)
	}
}

// --- match(re) — re.FindSubmatch (returns [][]byte) ---

func BenchmarkRegexp_FindSubmatch_Hit(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		reCapture.FindSubmatch(inputCapture)
	}
}

func BenchmarkRegexp_FindSubmatch_Miss(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		reCapture.FindSubmatch(inputNoMatch)
	}
}

// FindSubmatchIndex returns []int instead of [][]byte — may differ
func BenchmarkRegexp_FindSubmatchIndex_Hit(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		reCapture.FindSubmatchIndex(inputCapture)
	}
}

func BenchmarkRegexp_FindSubmatchIndex_Miss(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		reCapture.FindSubmatchIndex(inputNoMatch)
	}
}

// --- sub(re;s) — re.ReplaceAll ---

func BenchmarkRegexp_ReplaceAll_Hit(b *testing.B) {
	repl := []byte(`REDACTED`)
	b.ReportAllocs()
	for b.Loop() {
		reSimple.ReplaceAll(inputMatch, repl)
	}
}

func BenchmarkRegexp_ReplaceAll_Miss(b *testing.B) {
	repl := []byte(`REDACTED`)
	b.ReportAllocs()
	for b.Loop() {
		reSimple.ReplaceAll(inputNoMatch, repl)
	}
}

// --- Comparison: what a zero-alloc test(re) implementation would look like ---
// If Match is 0 allocs we could store *regexp.Regexp in the AST node and call
// it directly, keeping the rest of execMulti's callback pattern intact.

func BenchmarkRegexp_MatchViaClosure(b *testing.B) {
	// Simulate what execMulti would do: store re in node at compile time,
	// call re.Match(input) at runtime, pass bTrue/bFalse to fn.
	b.ReportAllocs()
	for b.Loop() {
		var result []byte
		if reSimple.Match(inputMatch) {
			result = bTrue
		} else {
			result = bFalse
		}
		_ = result
	}
}

// --- Correctness tests for test(re) ---

func TestRegexTest_Match(t *testing.T) {
	assertQuery(t, `test("error")`, `"error: connection timeout"`, `true`)
	assertQuery(t, `test("error")`, `"info: all good"`, `false`)
	assertQuery(t, `test("^error")`, `"error: boom"`, `true`)
	assertQuery(t, `test("^error")`, `"not error: boom"`, `false`)
}

func TestRegexTest_CaseInsensitive(t *testing.T) {
	assertQuery(t, `test("error"; "i")`, `"ERROR: timeout"`, `true`)
	assertQuery(t, `test("ERROR"; "i")`, `"error: timeout"`, `true`)
}

func TestRegexTest_NonStringInput(t *testing.T) {
	// non-string input returns false, not an error
	assertQuery(t, `test("x")`, `42`, `false`)
	assertQuery(t, `test("x")`, `null`, `false`)
	assertQuery(t, `test("x")`, `{"a":1}`, `false`)
}

func TestRegexTest_InSelect(t *testing.T) {
	// primary use case: filter log lines
	input := `[{"level":"ERROR","msg":"connection timeout"},{"level":"INFO","msg":"started"},{"level":"ERROR","msg":"disk full"}]`
	assertQuery(t, `[.[] | select(.msg | test("timeout|full"))] | length`, input, `2`)
}

func TestRegexTest_InvalidPattern(t *testing.T) {
	_, err := Compile(`test("[invalid")`)
	if err == nil {
		t.Error("expected compile error for invalid regexp")
	}
}

func TestRegexTest_Groups(t *testing.T) {
	// groups in pattern don't affect boolean result
	assertQuery(t, `test("(foo)(bar)")`, `"foobar"`, `true`)
	assertQuery(t, `test("(foo)(bar)")`, `"foobaz"`, `false`)
}

// --- Correctness tests for match(re) ---

func TestRegexMatch_Basic(t *testing.T) {
	// In a raw string literal, \w is already the two bytes that regexp needs.
	p, _ := Compile(`match("(\w+)@(\w+)")`)
	got, err := p.Run([]byte(`"user@example"`))
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, `"offset":0`) {
		t.Errorf("missing offset: %s", s)
	}
	if !strings.Contains(s, `"string":"user@example"`) {
		t.Errorf("missing string: %s", s)
	}
	if !strings.Contains(s, `"captures":[`) {
		t.Errorf("missing captures: %s", s)
	}
}

func TestRegexMatch_NoMatch(t *testing.T) {
	assertQuery(t, `match("xyz")`, `"hello world"`, `null`)
}

func TestRegexMatch_NonString(t *testing.T) {
	assertQuery(t, `match("x")`, `42`, `null`)
}

func TestRegexMatch_NamedCapture(t *testing.T) {
	p, _ := Compile(`match("(?P<user>\w+)@(?P<domain>\w+)")`)
	got, err := p.Run([]byte(`"alice@example"`))
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, `"name":"user"`) {
		t.Errorf("missing named capture 'user': %s", s)
	}
	if !strings.Contains(s, `"name":"domain"`) {
		t.Errorf("missing named capture 'domain': %s", s)
	}
}

// --- fastjq benchmarks: test(re) and match(re) ---

func BenchmarkFastjq_Small_TestRe_Hit(b *testing.B) {
	benchFastjqObj(b, `select(test("error"))`, []byte(`"error: connection timeout after 30s"`))
}

func BenchmarkFastjq_Small_TestRe_Miss(b *testing.B) {
	benchFastjqObj(b, `select(test("error"))`, []byte(`"info: all systems normal, nothing to report"`))
}

func BenchmarkFastjq_Small_TestRe_InPipeline(b *testing.B) {
	benchFastjqObj(b, `select(.level == "error" and (.msg | test("timeout|refused")))`, complexLogEvent)
}

func BenchmarkFastjq_Small_MatchRe_Hit(b *testing.B) {
	benchFastjqObj(b, `match("(\w+)@(\w+\.\w+)")`, []byte(`"user@example.com"`))
}

func BenchmarkFastjq_Small_MatchRe_Miss(b *testing.B) {
	benchFastjqObj(b, `match("(\w+)@(\w+\.\w+)")`, []byte(`"not an email address"`))
}

// --- Correctness tests for capture(re) ---

func TestRegexCapture_Named(t *testing.T) {
	assertQuery(t, `capture("(?P<user>\w+)@(?P<domain>[\w.]+)")`,
		`"alice@example.com"`, `{"user":"alice","domain":"example.com"}`)
}

func TestRegexCapture_NoMatch(t *testing.T) {
	assertQuery(t, `capture("(?P<x>\d+)")`, `"hello"`, `null`)
}

func TestRegexCapture_UnnamedIgnored(t *testing.T) {
	// unnamed group at position 2 is ignored; only named group 'word' is returned.
	// Use [a-z]+ vs [0-9]+ so the groups don't overlap.
	assertQuery(t, `capture("(?P<word>[a-z]+)([0-9]+)")`, `"foo123"`, `{"word":"foo"}`)
}

func TestRegexCapture_NonString(t *testing.T) {
	assertQuery(t, `capture("(?P<x>\d+)")`, `42`, `null`)
	assertQuery(t, `capture("(?P<x>\d+)")`, `null`, `null`)
}

func TestRegexCapture_AllNamedGroups(t *testing.T) {
	assertQuery(t, `capture("(?P<y>\d{4})-(?P<m>\d{2})-(?P<d>\d{2})")`,
		`"2024-01-15"`, `{"y":"2024","m":"01","d":"15"}`)
}

// --- Correctness tests for scan(re) ---

func TestRegexScan_NoGroups(t *testing.T) {
	p, _ := Compile(`[scan("[0-9]+")]`)
	got, _ := p.Run([]byte(`"foo123bar456"`))
	if string(got) != `["123","456"]` {
		t.Errorf("got %s", got)
	}
}

func TestRegexScan_WithGroups(t *testing.T) {
	p, _ := Compile(`[scan("([a-z]+)([0-9]+)")]`)
	got, _ := p.Run([]byte(`"foo123bar456"`))
	// each match is an array of captured groups
	if string(got) != `[["foo","123"],["bar","456"]]` {
		t.Errorf("got %s", got)
	}
}

func TestRegexScan_NoMatch(t *testing.T) {
	p, _ := Compile(`[scan("x+")]`)
	got, _ := p.Run([]byte(`"hello"`))
	if string(got) != `[]` {
		t.Errorf("got %s", got)
	}
}

func TestRegexScan_NonString(t *testing.T) {
	// non-string input produces no outputs
	p, _ := Compile(`[scan("[0-9]+")]`)
	got, _ := p.Run([]byte(`42`))
	if string(got) != `[]` {
		t.Errorf("expected [], got %s", got)
	}
}

func TestRegexScan_SingleChar(t *testing.T) {
	p, _ := Compile(`[scan("[aeiou]")]`)
	got, _ := p.Run([]byte(`"hello world"`))
	if string(got) != `["e","o","o"]` {
		t.Errorf("got %s", got)
	}
}

// --- Correctness tests for sub(re; ...) ---

func TestRegexSub_FirstOnly(t *testing.T) {
	assertQuery(t, `sub("foo"; "baz")`, `"foo bar foo"`, `"baz bar foo"`)
}

func TestRegexSub_NoMatch(t *testing.T) {
	assertQuery(t, `sub("xyz"; "nope")`, `"hello world"`, `"hello world"`)
}

func TestRegexSub_NonString(t *testing.T) {
	// non-string input is returned unchanged
	assertQuery(t, `sub("x"; "y")`, `42`, `42`)
	assertQuery(t, `sub("x"; "y")`, `null`, `null`)
}

func TestRegexSub_Pattern(t *testing.T) {
	assertQuery(t, `sub("[0-9]+"; "NUM")`, `"foo 123 bar 456"`, `"foo NUM bar 456"`)
}

func TestRegexSub_EmptyReplacement(t *testing.T) {
	assertQuery(t, `sub("foo"; "")`, `"foobar"`, `"bar"`)
}

// --- Correctness tests for gsub(re; ...) ---

func TestRegexGSub_All(t *testing.T) {
	assertQuery(t, `gsub("foo"; "baz")`, `"foo bar foo"`, `"baz bar baz"`)
}

func TestRegexGSub_NoMatch(t *testing.T) {
	assertQuery(t, `gsub("xyz"; "nope")`, `"hello world"`, `"hello world"`)
}

func TestRegexGSub_NonString(t *testing.T) {
	// non-string input is returned unchanged
	assertQuery(t, `gsub("x"; "y")`, `42`, `42`)
	assertQuery(t, `gsub("x"; "y")`, `null`, `null`)
}

func TestRegexGSub_Redaction(t *testing.T) {
	// realistic use: redact tokens from log messages
	assertQuery(t, `gsub("[0-9]+"; "REDACTED")`,
		`"user 12345 logged in at 09:30"`,
		`"user REDACTED logged in at REDACTED:REDACTED"`)
}

func TestRegexGSub_AllOccurrences(t *testing.T) {
	assertQuery(t, `gsub("o"; "0")`, `"foo"`, `"f00"`)
}

func TestRegexGSub_EmptyReplacement(t *testing.T) {
	assertQuery(t, `gsub("o"; "")`, `"foobar"`, `"fbar"`)
}

// --- fastjq benchmarks: capture, scan, sub, gsub ---

func BenchmarkFastjq_Small_CaptureRe_Hit(b *testing.B) {
	benchFastjqObj(b, `capture("(?P<user>\w+)@(?P<domain>[\w.]+)")`, []byte(`"alice@example.com"`))
}

func BenchmarkFastjq_Small_CaptureRe_Miss(b *testing.B) {
	benchFastjqObj(b, `capture("(?P<user>\w+)@(?P<domain>[\w.]+)")`, []byte(`"not an email address"`))
}

func BenchmarkFastjq_Small_ScanRe_NoGroups(b *testing.B) {
	benchFastjqObj(b, `[scan("[0-9]+")]`, []byte(`"foo123bar456baz789"`))
}

func BenchmarkFastjq_Small_ScanRe_WithGroups(b *testing.B) {
	benchFastjqObj(b, `[scan("([a-z]+)([0-9]+)")]`, []byte(`"foo123bar456baz789"`))
}

func BenchmarkFastjq_Small_SubRe_Hit(b *testing.B) {
	benchFastjqObj(b, `sub("error"; "warning")`, []byte(`"error: connection timeout after 30s"`))
}

func BenchmarkFastjq_Small_SubRe_Miss(b *testing.B) {
	benchFastjqObj(b, `sub("error"; "warning")`, []byte(`"info: all systems normal, nothing to report"`))
}

func BenchmarkFastjq_Small_GSubRe_Hit(b *testing.B) {
	benchFastjqObj(b, `gsub("[0-9]+"; "NUM")`, []byte(`"user 12345 logged in at 09:30:00"`))
}

func BenchmarkFastjq_Small_GSubRe_Miss(b *testing.B) {
	benchFastjqObj(b, `gsub("[0-9]+"; "NUM")`, []byte(`"info: all systems normal"`))
}
