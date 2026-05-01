// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License 2.0.
//
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2025-Present Datadog, Inc.

package compare

import "testing"

func BenchmarkGojq_Small_TestRe_Hit(b *testing.B) {
	benchGojqObj(b, `select(test("error"))`, []byte(`"error: connection timeout after 30s"`))
}

func BenchmarkGojq_Small_TestRe_Miss(b *testing.B) {
	benchGojqObj(b, `select(test("error"))`, []byte(`"info: all systems normal, nothing to report"`))
}

func BenchmarkGojq_Small_MatchRe_Hit(b *testing.B) {
	benchGojqObj(b, `match("(\\w+)@(\\w+\\.\\w+)")`, []byte(`"user@example.com"`))
}

func BenchmarkGojq_Small_MatchRe_Miss(b *testing.B) {
	benchGojqObj(b, `match("(\\w+)@(\\w+\\.\\w+)")`, []byte(`"not an email address"`))
}

func BenchmarkGojq_Small_CaptureRe_Hit(b *testing.B) {
	benchGojqObj(b, `capture("(?P<user>\\w+)@(?P<domain>[\\w.]+)")`, []byte(`"alice@example.com"`))
}

func BenchmarkGojq_Small_CaptureRe_Miss(b *testing.B) {
	benchGojqObj(b, `capture("(?P<user>\\w+)@(?P<domain>[\\w.]+)")`, []byte(`"not an email address"`))
}

func BenchmarkGojq_Small_ScanRe_NoGroups(b *testing.B) {
	benchGojqObj(b, `[scan("[0-9]+")]`, []byte(`"foo123bar456baz789"`))
}

func BenchmarkGojq_Small_SubRe_Hit(b *testing.B) {
	benchGojqObj(b, `sub("error"; "warning")`, []byte(`"error: connection timeout after 30s"`))
}

func BenchmarkGojq_Small_GSubRe_Hit(b *testing.B) {
	benchGojqObj(b, `gsub("[0-9]+"; "NUM")`, []byte(`"user 12345 logged in at 09:30:00"`))
}
