package host

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func readCPUTimes() (cpuTimes, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuTimes{}, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return cpuTimes{}, errors.New("empty /proc/stat")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuTimes{}, fmt.Errorf("unexpected /proc/stat line %q", scanner.Text())
	}
	var times cpuTimes
	for i, field := range fields[1:] {
		v, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return cpuTimes{}, fmt.Errorf("parse /proc/stat: %w", err)
		}
		times.total += v
		if i == 3 || i == 4 { // idle, iowait
			times.idle += v
		}
	}
	return times, nil
}

func memoryPercent() (float32, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()

	values := map[string]uint64{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 {
			if v, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				values[strings.TrimSuffix(fields[0], ":")] = v
			}
		}
	}
	total, available := values["MemTotal"], values["MemAvailable"]
	if total == 0 {
		return 0, errors.New("MemTotal missing from /proc/meminfo")
	}
	return clampPercent(100 * float64(total-available) / float64(total)), nil
}

func diskPercent(path string) (float32, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	if stat.Blocks == 0 {
		return 0, fmt.Errorf("statfs %s: no blocks", path)
	}
	used := stat.Blocks - stat.Bfree
	return clampPercent(100 * float64(used) / float64(used+stat.Bavail)), nil
}
