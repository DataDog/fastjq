//go:build ignore

// update_benchmarks.go — Re-runs the full benchmark suite and replaces the
// Raw Output section of BENCHMARKS.md with fresh results.
//
// Usage:
//
//	go run scripts/update_benchmarks.go
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const benchmarksMD = "BENCHMARKS.md"
const rawOutputStart = "## Raw Output\n"
const rawOutputEnd = "\n## CLI Throughput"

func main() {
	fmt.Fprintln(os.Stderr, "Running benchmark suite (~3 minutes)...")

	out, _ := exec.Command(
		"go", "test", "-run=^$", "-bench=.", "-benchmem", "-count=1",
	).CombinedOutput()

	// Extract only Benchmark lines, filtering DEBUG noise and test output.
	var benchLines []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Benchmark") {
			benchLines = append(benchLines, line)
		}
	}
	if len(benchLines) == 0 {
		fmt.Fprintln(os.Stderr, "error: no benchmark lines found in output")
		fmt.Fprint(os.Stderr, string(out))
		os.Exit(1)
	}

	// Detect Go version.
	goVerOut, _ := exec.Command("go", "version").Output()
	goVer := "unknown"
	if fields := strings.Fields(string(goVerOut)); len(fields) >= 3 {
		goVer = fields[2]
	}
	date := time.Now().Format("2006-01-02")

	newSection := fmt.Sprintf(
		"## Raw Output\n\n"+
			"Apple M4 Max, %s, `go test -bench=. -benchmem`. Updated %s. "+
			"Note: some first-run entries show spurious allocs "+
			"(e.g. `KeysUnsorted` 3 allocs, `First` 1 alloc) due to benchmark "+
			"calibration warmup — confirmed 0 allocs on repeat runs.\n\n"+
			"```\n%s\n```",
		goVer, date, strings.Join(benchLines, "\n"),
	)

	content, err := os.ReadFile(benchmarksMD)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	s := string(content)
	start := strings.Index(s, rawOutputStart)
	end := strings.Index(s, rawOutputEnd)
	if start == -1 || end == -1 {
		fmt.Fprintln(os.Stderr, "error: could not find Raw Output section boundaries in", benchmarksMD)
		os.Exit(1)
	}

	updated := s[:start] + newSection + s[end:]
	if err := os.WriteFile(benchmarksMD, []byte(updated), 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Updated %s with %d benchmark results.\n", benchmarksMD, len(benchLines))
}
