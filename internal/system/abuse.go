package system

import (
	"fmt"
	"os/exec"
	"painel-ssh/internal/db"
	"strings"
	"time"
)

// MonitorTorrentPro runs a background monitor for torrent activity
func MonitorTorrentPro() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !GetTorrentStatus() {
			continue
		}

		// 1. Detect Torrent Activity via conntrack/iptables/netstat
		// Procuramos por conexões ESTABLISHED em portas comuns ou tráfego suspeito
		ips := detectTorrentActivity()
		for _, ip := range ips {
			username := GetUserByIP(ip)
			if username != "" {
				ApplyBan(username, ip)
			}
		}
	}
}

// detectTorrentActivity returns a list of IPs suspected of torrent usage
func detectTorrentActivity() []string {
	var suspiciousIPs []string

	// Check conntrack for high connection count per IP (suspicious for P2P)
	// ss -ntu state established | awk '{print $5}' | cut -d: -f1 | sort | uniq -c | awk '$1 > 50 {print $2}'
	cmd := exec.Command("bash", "-c", "ss -ntu state established | awk '{print $5}' | cut -d: -f1 | sort | uniq -c | awk '$1 > 50 {print $2}'")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, ip := range lines {
			if ip != "" && ip != "127.0.0.1" && ip != "Local" && ip != "Address" {
				suspiciousIPs = append(suspiciousIPs, ip)
			}
		}
	}

	return suspiciousIPs
}

// GetUserByIP maps a connection IP to a local SSH user
func GetUserByIP(ip string) string {
	if ip == "" {
		return ""
	}

	// 1. Check who for SSH users
	// we use grep -w to match the exact IP and head -n 1 to get only one user
	cmd := exec.Command("bash", "-c", fmt.Sprintf("who | grep -w '%s' | awk '{print $1}' | head -n 1", ip))
	out, err := cmd.Output()
	if err == nil {
		user := strings.TrimSpace(string(out))
		if user != "" {
			return user
		}
	}

	// 2. Check Xray logs for UUID (if possible)
	// To implement this, we could parse /var/log/xray/access.log searching for the IP
	return ""
}

// ApplyBan handles the progressive punishment for abuse
func ApplyBan(username, ip string) {
	attempts, status, err := db.GetUserAbuseStatus(username)
	if err != nil || status == "banned" {
		return
	}
	_ = attempts // attempts já é verificado após AddAbuseAttempt

	newAttempts, _ := db.AddAbuseAttempt(username, ip)

	// Log the alert
	logMsg := fmt.Sprintf("[ALERTA] %s tentou usar torrent (IP: %s) - Tentativa %d", username, ip, newAttempts)
	fmt.Println(logMsg)
	exec.Command("bash", "-c", fmt.Sprintf("echo '%s [%s]' >> /var/log/torrent-block.log", time.Now().Format("2006-01-02 15:04:05"), logMsg)).Run()

	if newAttempts >= 3 {
		// PERMANENT BAN
		db.BanUser(username)

		// 1. Lock SSH account
		exec.Command("usermod", "-L", username).Run()

		// 2. Kill all processes from user
		exec.Command("pkill", "-u", username).Run()

		// 3. Block IP in firewall
		exec.Command("iptables", "-A", "INPUT", "-s", ip, "-j", "DROP").Run()

		banMsg := fmt.Sprintf("[BAN] Usuário %s bloqueado permanentemente por abuso de Torrent.", username)
		fmt.Println(banMsg)
		exec.Command("bash", "-c", fmt.Sprintf("echo '%s [%s]' >> /var/log/torrent-block.log", time.Now().Format("2006-01-02 15:04:05"), banMsg)).Run()
	} else {
		// TEMPORARY KICK
		exec.Command("pkill", "-u", username).Run()
	}
}
