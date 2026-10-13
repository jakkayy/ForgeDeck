//go:build !linux

package host

import "errors"

var errUnsupported = errors.New("host metrics are only collected on Linux")

func readCPUTimes() (cpuTimes, error)     { return cpuTimes{}, errUnsupported }
func memoryPercent() (float32, error)     { return 0, errUnsupported }
func diskPercent(string) (float32, error) { return 0, errUnsupported }
