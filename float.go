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
