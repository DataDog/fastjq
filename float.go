// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License 2.0.
//
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2025-Present Datadog, Inc.

package fastjq

import (
	"strconv"
	"unsafe"
)

// parseFloatUnsafe parses a float64 from a byte slice without allocating
// a string, using unsafe.String.
func parseFloatUnsafe(b []byte) (float64, error) {
	return strconv.ParseFloat(unsafe.String(unsafe.SliceData(b), len(b)), 64)
}
