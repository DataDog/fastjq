// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License 2.0.
//
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2025-Present Datadog, Inc.

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/DataDog/fastjq"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: fastjq-bench 'QUERY'\n")
		os.Exit(1)
	}
	p, err := fastjq.Compile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "compile error: %v\n", err)
		os.Exit(1)
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1MB max line
	w := bufio.NewWriter(os.Stdout)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if !json.Valid(line) {
			fmt.Fprintf(os.Stderr, "invalid JSON: %s\n", line)
			os.Exit(1)
		}
		p.RunFunc(line, func(result []byte) error {
			w.Write(result)
			w.WriteByte('\n')
			return nil
		})
	}
	w.Flush()
}
