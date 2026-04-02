package system

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type CPUStats struct {
	User      uint64
	Nice      uint64
	System    uint64
	Idle      uint64
	IOWait    uint64
	IRQ       uint64
	SoftIRQ   uint64
	Steal     uint64
	Guest     uint64
	GuestNice uint64
}

func (c CPUStats) Total() uint64 {
	return c.User + c.Nice + c.System + c.Idle + c.IOWait + c.IRQ + c.SoftIRQ + c.Steal + c.Guest + c.GuestNice
}

func getCPUStats() (CPUStats, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return CPUStats{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 5 {
				return CPUStats{}, fmt.Errorf("invalid cpu line in /proc/stat")
			}
			var stats CPUStats
			stats.User, _ = strconv.ParseUint(fields[1], 10, 64)
			stats.Nice, _ = strconv.ParseUint(fields[2], 10, 64)
			stats.System, _ = strconv.ParseUint(fields[3], 10, 64)
			stats.Idle, _ = strconv.ParseUint(fields[4], 10, 64)
			if len(fields) > 5 {
				stats.IOWait, _ = strconv.ParseUint(fields[5], 10, 64)
			}
			if len(fields) > 6 {
				stats.IRQ, _ = strconv.ParseUint(fields[6], 10, 64)
			}
			if len(fields) > 7 {
				stats.SoftIRQ, _ = strconv.ParseUint(fields[7], 10, 64)
			}
			if len(fields) > 8 {
				stats.Steal, _ = strconv.ParseUint(fields[8], 10, 64)
			}
			return stats, nil
		}
	}
	return CPUStats{}, fmt.Errorf("cpu line not found in /proc/stat")
}

// GetCPUUsage calculates current CPU usage percentage
func GetCPUUsage() float64 {
	s1, err := getCPUStats()
	if err != nil {
		return 0.0
	}
	time.Sleep(500 * time.Millisecond) // Sample for 500ms
	s2, err := getCPUStats()
	if err != nil {
		return 0.0
	}

	idleTicks := s2.Idle - s1.Idle
	totalTicks := s2.Total() - s1.Total()

	if totalTicks == 0 {
		return 0.0
	}

	usage := float64(totalTicks-idleTicks) / float64(totalTicks) * 100
	return usage
}

// GetRAMUsage returns percentage, used GB, and total GB
func GetRAMUsage() (percent float64, used string, total string) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, "0", "0"
	}
	defer file.Close()

	var memTotal, memAvailable, memFree uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, _ := strconv.ParseUint(fields[1], 10, 64)
		if fields[0] == "MemTotal:" {
			memTotal = val
		} else if fields[0] == "MemAvailable:" {
			memAvailable = val
		} else if fields[0] == "MemFree:" {
			memFree = val
		}
	}

	if memTotal == 0 {
		return 0, "0", "0"
	}

	memUsed := memTotal - memAvailable
	if memAvailable == 0 {
		memUsed = memTotal - memFree
	}
	percent = (float64(memUsed) / float64(memTotal)) * 100
	usedGB := float64(memUsed) / 1024 / 1024
	totalGB := float64(memTotal) / 1024 / 1024

	return percent, fmt.Sprintf("%.2f GB", usedGB), fmt.Sprintf("%.2f GB", totalGB)
}

// GetDiskUsage returns percent, used, total, and free space
func GetDiskUsage() (percent float64, used string, total string, free string) {
	cmd := exec.Command("df", "/", "-B1")
	out, err := cmd.Output()
	if err != nil {
		return 0, "0", "0", "0"
	}

	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		return 0, "0", "0", "0"
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return 0, "0", "0", "0"
	}

	totalBytes, _ := strconv.ParseUint(fields[1], 10, 64)
	usedBytes, _ := strconv.ParseUint(fields[2], 10, 64)
	freeBytes, _ := strconv.ParseUint(fields[3], 10, 64)

	if totalBytes == 0 {
		return 0, "0", "0", "0"
	}

	percent = (float64(usedBytes) / float64(totalBytes)) * 100
	totalStr := formatBytes(totalBytes)
	usedStr := formatBytes(usedBytes)
	freeStr := formatBytes(freeBytes)

	return percent, usedStr, totalStr, freeStr
}

var lastNetTime time.Time
var lastRX, lastTX uint64

// GetNetworkSpeed calculates download and upload speed
func GetNetworkSpeed() (download string, upload string) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return "0 KB/s", "0 KB/s"
	}
	defer file.Close()

	var rx, tx uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, ":") {
			fields := strings.Fields(line)
			if len(fields) < 10 {
				continue
			}
			// Skip loopback
			if strings.Contains(fields[0], "lo:") {
				continue
			}

			r, _ := strconv.ParseUint(fields[1], 10, 64)
			t, _ := strconv.ParseUint(fields[9], 10, 64)
			rx += r
			tx += t
		}
	}

	now := time.Now()
	if lastNetTime.IsZero() {
		lastNetTime = now
		lastRX = rx
		lastTX = tx
		return "0 KB/s", "0 KB/s"
	}

	duration := now.Sub(lastNetTime).Seconds()
	if duration == 0 {
		return "0 KB/s", "0 KB/s"
	}

	dlSpeed := float64(rx-lastRX) / duration
	ulSpeed := float64(tx-lastTX) / duration

	lastNetTime = now
	lastRX = rx
	lastTX = tx

	return formatSpeed(dlSpeed), formatSpeed(ulSpeed)
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatSpeed(s float64) string {
	if s < 1024 {
		return fmt.Sprintf("%.0f B/s", s)
	} else if s < 1024*1024 {
		return fmt.Sprintf("%.1f KB/s", s/1024)
	}
	return fmt.Sprintf("%.1f MB/s", s/(1024*1024))
}
