//go:build darwin

package glance

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func readCPUSample(ctx context.Context) (cpuSample, bool) {
	output, err := exec.CommandContext(ctx, "sysctl", "-n", "kern.cp_time").Output()
	if err != nil {
		return cpuSample{}, false
	}

	fields := strings.Fields(string(output))
	if len(fields) < 5 {
		return cpuSample{}, false
	}

	var total uint64
	values := make([]uint64, 0, len(fields))
	for _, field := range fields {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return cpuSample{}, false
		}
		values = append(values, value)
		total += value
	}

	// New feature: CPU Glance uses Darwin's kern.cp_time counters because they
	// match the shared total/idle delta model and avoid parsing localized UI
	// output from Activity Monitor-like tools.
	return cpuSample{idle: values[4], total: total, valid: true}, true
}

func readMemoryPercent(ctx context.Context) (float64, bool) {
	totalOutput, err := exec.CommandContext(ctx, "sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, false
	}
	totalBytes, err := strconv.ParseUint(strings.TrimSpace(string(totalOutput)), 10, 64)
	if err != nil || totalBytes == 0 {
		return 0, false
	}

	vmStatOutput, err := exec.CommandContext(ctx, "vm_stat").Output()
	if err != nil {
		return 0, false
	}

	pageSize := parseDarwinPageSize(string(vmStatOutput))
	freePages := parseDarwinVMStatPages(string(vmStatOutput), "Pages free")
	speculativePages := parseDarwinVMStatPages(string(vmStatOutput), "Pages speculative")
	if pageSize == 0 {
		return 0, false
	}

	availableBytes := (freePages + speculativePages) * pageSize
	if availableBytes > totalBytes {
		return 0, false
	}

	// New feature: Memory Glance treats free and speculative pages as available
	// so the displayed percentage reflects memory pressure better than a raw
	// "not free" calculation on macOS.
	return 100 * float64(totalBytes-availableBytes) / float64(totalBytes), true
}

func parseDarwinPageSize(output string) uint64 {
	match := regexp.MustCompile(`page size of (\d+) bytes`).FindStringSubmatch(output)
	if len(match) < 2 {
		return 0
	}
	value, err := strconv.ParseUint(match[1], 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func parseDarwinVMStatPages(output string, label string) uint64 {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, label+":") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			return 0
		}
		value := strings.TrimRight(fields[len(fields)-1], ".")
		pages, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return 0
		}
		return pages
	}
	return 0
}
