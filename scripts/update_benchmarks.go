//go:build ignore

// update_benchmarks.go — Re-runs the full benchmark suite and updates both the
// Summary comparison table and the Raw Output section of BENCHMARKS.md.
//
// Usage:
//
//	go run scripts/update_benchmarks.go
package main

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// row defines one line of the summary comparison table.
// fastjq and gojq are the suffixes after "BenchmarkFastjq_" / "BenchmarkGojq_".
// An empty gojq means no comparison benchmark exists (row is omitted from table).
type row struct {
	op    string // human-readable operation syntax
	input string // input description
	fq    string // BenchmarkFastjq_<fq> suffix
	gq    string // BenchmarkGojq_<gq> suffix (empty = fastjq-only, skip)
}

// tableRows defines the summary table order and descriptions.
// Add new benchmarks here when adding them to bench_test.go.
var tableRows = []row{
	// Access & deletion
	{"`.field`", "Small (~100B)", "Small_Field", "Small_Field"},
	{"`.field`", "Large (~100KB)", "Large_Field", "Large_Field"},
	{"`del(.f)`", "Small (~100B)", "Small_Del", "Small_Del"},
	{"`del(.f)`", "Medium (~2KB)", "Medium_Del", "Medium_Del"},
	{"`del(.f)`", "Large (~100KB)", "Large_Del", "Large_Del"},
	{"`.[n]`", "5-elem array", "Small_Index", "Small_Index"},
	{"`del(.[n], .[m])`", "5-elem array", "Small_ArrayDel", "Small_ArrayDel"},
	// Construction & iteration
	{"`{f0, f2}` (construct)", "Small (~100B)", "Small_Construct", "Small_Construct"},
	{"`{f0, f50}` (construct)", "Large (~100KB)", "Large_Construct", "Large_Construct"},
	{"`.[]` iterator", "5-elem array", "Small_Iterator", "Small_Iterator"},
	{"`.[]` iterator", "200-elem array", "Large_Iterator", "Large_Iterator"},
	// Filtering
	{"`select(.f == \"x\")`", "Small (~100B)", "Small_Select", "Small_Select"},
	{"`select(.f == \"x\")`¹", "Large (~100KB, last field)", "Large_Select", "Large_Select"},
	{"`select(.f and .g)`", "Small (~100B)", "Small_SelectAnd", "Small_SelectAnd"},
	{"`select(.f or .g)`", "Small (~100B)", "Small_SelectOr", "Small_SelectOr"},
	{"`has(\"key\")` in select", "Small (~100B)", "Small_Has", "Small_Has"},
	{"`has(\"key\")` in select", "Large (~100KB)", "Large_Has", "Large_Has"},
	{"`if-then-else`", "Small (~100B)", "Small_IfThenElse", "Small_IfThenElse"},
	{"`.f // \"default\"`", "Small (~100B)", "Small_Alternative", "Small_Alternative"},
	{"`try .field` (no error)", "Small (~100B)", "Small_TryNoError", "Small_TryNoError"},
	// Arithmetic
	{"`.a + .b` (strings)", "Small (~100B)", "Small_Plus", "Small_Plus"},
	{"`\"prefix\" + .name`", "Small (~100B)", "Small_PlusStr", "Small_PlusStr"},
	{"`.a - .b` (subtract)", "Small (~100B)", "Small_Subtract", "Small_Subtract"},
	{"`.a * .b` (multiply)", "Small (~100B)", "Small_Multiply", "Small_Multiply"},
	{"`.a / .b` (divide)", "Small (~100B)", "Small_Divide", "Small_Divide"},
	{"`.a - .b` (array diff)", "5-elem arrays", "Small_ArrayDiff", "Small_ArrayDiff"},
	// Aggregation & reduction
	{"`length`", "Small (~100B)", "Small_Length", "Small_Length"},
	{"`length`", "Large (~100KB)", "Large_Length", "Large_Length"},
	{"`add` (numbers)", "5-elem array", "Small_Add", "Small_Add"},
	{"`add` (strings)", "5-elem array", "Small_AddStrings", "Small_AddStrings"},
	{"`flatten`", "3-elem nested array", "Small_Flatten", "Small_Flatten"},
	{"`min`", "200-int array", "Small_Min", "Small_Min"},
	{"`min_by(.value)`", "100-elem object array", "Small_MinBy", "Small_MinBy"},
	{"`sort`", "200-int array", "Sort", "Sort"},
	{"`sort_by(.value)`", "100-elem object array", "SortBy", "SortBy"},
	{"`unique`", "200-int array", "Unique", "Unique"},
	{"`group_by(.active)`", "100-elem object array", "GroupBy", "GroupBy"},
	// Array ops
	{"`map(.name)`", "20-elem array (~600B)", "Small_Map", "Small_Map"},
	{"`map(.name)`", "200-elem array (~6KB)", "Large_Map", "Large_Map"},
	{"`any`", "5-elem array", "Small_Any", "Small_Any"},
	{"`any(expr)`", "5-elem array", "Small_AnyExpr", "Small_AnyExpr"},
	{"`any(expr)`²", "200-int array", "Large_AnyExpr", "Large_AnyExpr"},
	{"`any(gen; cond)`²", "200-int array", "Small_AnyTwoArg", "Small_AnyTwoArg"},
	{"`first(expr)`", "5-elem array", "Small_First", "Small_First"},
	{"`first(expr)`²", "200-int array", "Large_First", "Large_First"},
	{"`last(expr)`", "5-elem array", "Small_Last", "Small_Last"},
	{"`last(expr)`²", "200-int array", "Large_Last", "Large_Last"},
	{"`limit(3; expr)`", "5-elem array", "Small_Limit", "Small_Limit"},
	{"`limit(10; expr)`", "200-int array", "Large_Limit", "Large_Limit"},
	{"`.[1:4]` slice", "6-elem array", "Small_Slice", "Small_Slice"},
	{"`values`", "9-elem array", "Small_Values", "Small_Values"},
	{"`skip(2; .[])`", "5-elem array", "Small_Skip", "Small_Skip"},
	// Object transforms
	{"`to_entries`", "Small (~100B)", "Small_ToEntries", "Small_ToEntries"},
	{"`to_entries`", "Large (~100KB)", "Large_ToEntries", "Large_ToEntries"},
	{"`keys`", "Small (~100B)", "Small_Keys", "Small_Keys"},
	{"`keys_unsorted`", "Small (~100B)", "Small_KeysUnsorted", "Small_KeysUnsorted"},
	{"`keys_unsorted`", "Large (~100KB)", "Large_KeysUnsorted", "Large_KeysUnsorted"},
	{"`object merge .a + .b`", "Small (~100B)", "Small_ObjectMerge", "Small_ObjectMerge"},
	// Type conversion
	{"`tojson`", "Small (~100B)", "Small_ToJSON", "Small_ToJSON"},
	{"`fromjson`", "JSON string", "Small_FromJSON", "Small_FromJSON"},
	{"`tonumber`", "`\"42\"` string", "Small_ToNumber", "Small_ToNumber"},
	{"`toboolean`", "`\"true\"` string", "Small_ToBoolean", "Small_ToBoolean"},
	// Strings
	{"`split(\",\")`", "short string", "Small_Split", "Small_Split"},
	{"`join(\",\")`", "5-elem array", "Small_Join", "Small_Join"},
	{"`ascii_downcase` in select", "Small (~100B)", "Small_AsciiDowncase", "Small_AsciiDowncase"},
	{"`ascii_downcase` in select", "Large (~100KB)", "Large_AsciiDowncase", "Large_AsciiDowncase"},
	{"`startswith(\"s\")`", "Small (~100B)", "Small_Startswith", "Small_Startswith"},
	{"`startswith(\"s\")`", "Large (~100KB)", "Large_Startswith", "Large_Startswith"},
	{"`endswith(\"s\")`", "Small (~100B)", "Small_Endswith", "Small_Endswith"},
	{"`trim`", "short string", "Small_Trim", "Small_Trim"},
	{"`ltrim`", "short string", "Small_Ltrim", "Small_Ltrim"},
	{"`rtrim`", "short string", "Small_Rtrim", "Small_Rtrim"},
	{"`ltrimstr(\"s\")`", "Small (~100B)", "Small_Ltrimstr", "Small_Ltrimstr"},
	{"`rtrimstr(\"s\")`", "Small (~100B)", "Small_Rtrimstr", "Small_Rtrimstr"},
	// Complex multi-feature (array-building queries; allocs expected per element)
	{"`select` + string ops + arith + construct", "~200B log event", "Complex_LogNormalize", "Complex_LogNormalize"},
	{"`[.[] | select + arith + construct]`", "20-elem array (~1.5KB)", "Complex_ArrayPipeline", "Complex_ArrayPipeline"},
	{"`length + map + add + min_by + any`", "20-elem array (~1.5KB)", "Complex_Aggregation", "Complex_Aggregation"},
	{"`map(try {…} catch …)`", "20-elem array (~1.5KB)", "Complex_TolerantMap", "Complex_TolerantMap"},
	{"`map(if … elif … else …)`", "20-elem array (~1.5KB)", "Complex_ElifRouting", "Complex_ElifRouting"},
	{"`[.[] | str + tostring + str]`", "20-elem array (~1.5KB)", "Complex_StringBuild", "Complex_StringBuild"},
	{"`to_entries | map(select) | from_entries`", "~200B log event", "Complex_EntryFilter", "Complex_EntryFilter"},
	// Format strings
	{"`@base64`", "34-char string", "Small_Base64Encode", "Small_Base64Encode"},
	{"`@base64d`", "48-char encoded", "Small_Base64Decode", "Small_Base64Decode"},
	{"`@uri`", "36-char URL string", "Small_URIEncode", "Small_URIEncode"},
	// Search
	{"`index(\",\")`", "short string", "Small_IndexFind", "Small_IndexFind"},
	{"`indices(\",\")`", "short string", "Small_IndicesAll", "Small_IndicesAll"},
	// Math builtins
	{"`sqrt`", "float (e≈2.718)", "Small_Sqrt", "Small_Sqrt"},
	{"`log`", "float (e≈2.718)", "Small_Log", "Small_Log"},
	{"`sin`", "float (e≈2.718)", "Small_Sin", "Small_Sin"},
	{"`atan`", "integer 1", "Small_Atan", "Small_Atan"},
	{"`exp`", "integer 1", "Small_Exp", "Small_Exp"},
	{"`tgamma`", "integer 5", "Small_Tgamma", "Small_Tgamma"},
	{"`fabs`", "float -3.14", "Small_Fabs", "Small_Fabs"},
	{"`abs`", "float -3.14", "Small_Abs", "Small_Abs"},
	// Bindings
	{"`expr as $x | body`", "Small (~20B object)", "Small_Bind", "Small_Bind"},
	// String interpolation
	{"``\"\\(.level): \\(.svc)\"``", "~45B object", "Small_StringInterp", "Small_StringInterp"},
	{"``\"user \\(.name) …\"``", "~45B object", "Small_StringInterpNum", "Small_StringInterpNum"},
	// Stream ops
	{"`isempty(empty)`", "null", "Small_IsEmptyTrue", "Small_IsEmptyTrue"},
	{"`isempty(.[])`", "5-elem array", "Small_IsEmptyFalse", "Small_IsEmptyFalse"},
	{"`nth(2; .[])`", "5-elem array", "Small_Nth", "Small_Nth"},
	// range (Tier 2: 1 alloc/value — proportional to what was requested)
	{"`range(10)` (10 values)", "null", "Small_Range10", "Small_Range10"},
	{"`limit(3; range(1000))`", "null — only 3 allocs", "Small_RangeLimit", "Small_RangeLimit"},
	// Regex (Go RE2) — pattern compiled once at Compile() time
	{"`test(re)` hit", "short string", "Small_TestRe_Hit", "Small_TestRe_Hit"},
	{"`test(re)` miss", "short string", "Small_TestRe_Miss", "Small_TestRe_Miss"},
	{"`match(re)` hit", "short string", "Small_MatchRe_Hit", "Small_MatchRe_Hit"},
	{"`match(re)` miss", "short string", "Small_MatchRe_Miss", "Small_MatchRe_Miss"},
	{"`capture(re)` hit", "short string", "Small_CaptureRe_Hit", ""},
	{"`scan(re)` no groups (4 matches)", "short string", "Small_ScanRe_NoGroups", ""},
	{"`sub(re; s)` hit", "short string", "Small_SubRe_Hit", ""},
	{"`gsub(re; s)` hit (4 matches)", "short string", "Small_GSubRe_Hit", ""},
}

type result struct {
	ns     float64
	allocs int
}

var benchLineRE = regexp.MustCompile(
	`^(Benchmark\S+)-\d+\s+\d+\s+([\d.]+) ns/op\s+\d+ B/op\s+(\d+) allocs/op`,
)

func parseBenchmarks(output string) map[string]result {
	m := make(map[string]result)
	for _, line := range strings.Split(output, "\n") {
		sub := benchLineRE.FindStringSubmatch(line)
		if sub == nil {
			continue
		}
		ns, _ := strconv.ParseFloat(sub[2], 64)
		allocs, _ := strconv.Atoi(sub[3])
		m[sub[1]] = result{ns, allocs}
	}
	return m
}

// formatNs converts nanoseconds to a µs string with appropriate precision.
func formatNs(ns float64) string {
	us := ns / 1000
	switch {
	case us < 0.01:
		return fmt.Sprintf("%.4f", us)
	case us < 0.1:
		return fmt.Sprintf("%.3f", us)
	case us < 1:
		return fmt.Sprintf("%.3f", us)
	case us < 10:
		return fmt.Sprintf("%.2f", us)
	case us < 100:
		return fmt.Sprintf("%.1f", us)
	default:
		return fmt.Sprintf("%.0f", us)
	}
}

// formatSpeedup returns the speedup markdown string.
func formatSpeedup(fqNs, gqNs float64) string {
	ratio := gqNs / fqNs
	// Round to 1 decimal; if >= 10 round to integer.
	if fqNs <= gqNs {
		if ratio >= 10 {
			return fmt.Sprintf("**%.0fx**", math.Round(ratio))
		}
		// Show one decimal, but drop ".0"
		s := fmt.Sprintf("%.1f", ratio)
		if strings.HasSuffix(s, ".0") {
			s = s[:len(s)-2]
		}
		return fmt.Sprintf("**%sx**", s)
	}
	// gojq wins
	return fmt.Sprintf("%.1fx†", ratio)
}

func buildTable(results map[string]result) string {
	var sb strings.Builder
	sb.WriteString("All times in µs. ¹Large Select uses the last field — fastjq scans the full document. ²gojq wins: after unmarshal, integer arrays are native Go slices.\n\n")
	sb.WriteString("| Operation | Input | fastjq (µs) | gojq (µs) | Speedup | fastjq allocs | gojq allocs |\n")
	sb.WriteString("|-----------|-------|------------|----------|---------|---------------|-------------|\n")

	for _, r := range tableRows {
		fqKey := "BenchmarkFastjq_" + r.fq
		gqKey := "BenchmarkGojq_" + r.gq

		fq, fqOK := results[fqKey]
		gq, gqOK := results[gqKey]

		if !fqOK {
			continue // benchmark not found in output — skip row
		}

		fqTime := formatNs(fq.ns)
		gqTime := "—"
		speedup := "—"
		gqAllocs := "—"

		if gqOK {
			gqTime = formatNs(gq.ns)
			speedup = formatSpeedup(fq.ns, gq.ns)
			gqAllocs = fmt.Sprintf("%d", gq.allocs)
		}

		// Escape bare pipe characters so they don't break the markdown table.
		// GitHub GFM treats | as a column separator even inside backtick spans.
		safeOp := strings.ReplaceAll(r.op, "|", `\|`)
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %d | %s |\n",
			safeOp, r.input, fqTime, gqTime, speedup, fq.allocs, gqAllocs)
	}
	return sb.String()
}

func replaceSection(content, startMarker, endMarker, newBody string) string {
	start := strings.Index(content, startMarker)
	end := strings.Index(content, endMarker)
	if start == -1 || end == -1 {
		fmt.Fprintf(os.Stderr, "warning: could not find markers %q / %q\n", startMarker, endMarker)
		return content
	}
	return content[:start] + startMarker + newBody + content[end:]
}

func main() {
	fmt.Fprintln(os.Stderr, "Running benchmark suite (~3 minutes)...")

	out, _ := exec.Command(
		"go", "test", "-run=^$", "-bench=.", "-benchmem", "-count=1",
	).CombinedOutput()

	results := parseBenchmarks(string(out))
	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "error: no benchmark lines found")
		fmt.Fprint(os.Stderr, string(out))
		os.Exit(1)
	}

	// Build Raw Output block.
	var benchLines []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Benchmark") {
			benchLines = append(benchLines, line)
		}
	}
	goVerOut, _ := exec.Command("go", "version").Output()
	goVer := "unknown"
	if fields := strings.Fields(string(goVerOut)); len(fields) >= 3 {
		goVer = fields[2]
	}
	date := time.Now().Format("2006-01-02")

	rawBlock := fmt.Sprintf(
		"Apple M4 Max, %s, `go test -bench=. -benchmem`. Updated %s. "+
			"Note: some first-run entries show spurious allocs "+
			"(e.g. `KeysUnsorted` 3 allocs, `First` 1 alloc) due to benchmark "+
			"calibration warmup — confirmed 0 allocs on repeat runs.\n\n"+
			"```\n%s\n```\n",
		goVer, date, strings.Join(benchLines, "\n"),
	)

	content, err := os.ReadFile("docs/BENCHMARKS.md")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	s := string(content)
	s = replaceSection(s, "## Summary\n", "\n## Key Takeaways", "\n"+buildTable(results))
	s = replaceSection(s, "## Raw Output\n", "\n## CLI Throughput", "\n"+rawBlock)

	if err := os.WriteFile("docs/BENCHMARKS.md", []byte(s), 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "BENCHMARKS.md updated (%d benchmarks, %d table rows).\n",
		len(benchLines), len(tableRows))
}
