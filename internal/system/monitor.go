package system

import (
	"os/exec"
	"painel-ssh/internal/db"
	"strings"
	"time"
)

// Global control for monitor loop
var monitorActive = false

// StartTorrentMonitor starts the background monitoring loop
func StartTorrentMonitor() {
	if monitorActive {
		return
	}
	monitorActive = true

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			if !monitorActive {
				return
			}

			isBlocked := GetTorrentStatus()
			if !isBlocked {
				continue
			}

			level, _ := db.GetConfig("torrent_level")

			// Detect Suspicious Activity
			ips := detectSuspiciousActivity()
			for _, ip := range ips {
				user := GetUserByIP(ip)
				if user != "" {
					// Handle based on level
					if level == LevelAutoBan {
						ApplyBan(user, ip)
					} else {
						// Just kill session for BASIC/PRO levels if detected
						exec.Command("pkill", "-u", user).Run()
					}
				}
			}
		}
	}()
}

// StopTorrentMonitor stops the background loop
func StopTorrentMonitor() {
	monitorActive = false
}

// detectSuspiciousActivity identifies IPs with high connection counts or specific patterns
func detectSuspiciousActivity() []string {
	var ips []string

	// Use ss to find established connections with high count (potential P2P)
	cmd := "ss -ntu state established | awk '{print $5}' | cut -d: -f1 | sort | uniq -c | awk '$1 > 40 {print $2}'"
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, ip := range lines {
			if ip != "" && ip != "127.0.0.1" && ip != "Local" && ip != "Address" {
				ips = append(ips, ip)
			}
		}
	}

	return ips
}

// GetMonitorStatus returns if the background thread is running
func GetMonitorStatus() bool {
	return monitorActive
}
