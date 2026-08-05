//go:build !darwin || !cgo

package main

import "fmt"

func extractSystemTerminology() ([]byte, error) {
	return nil, fmt.Errorf("OSAGetSysTerminology is available only on macOS with cgo enabled")
}
