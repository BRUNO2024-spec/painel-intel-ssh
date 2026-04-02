package system

import (
	"fmt"
	"os/exec"
	"strings"

	"painel-ssh/internal/db"
)

// Protection Levels
const (
	LevelBasic   = "BASIC"
	LevelPro     = "PRO"
	LevelAutoBan = "AUTO-BAN"
)

// EnableTorrentProtection applies rules based on current level
func EnableTorrentProtection() error {
	level, _ := db.GetConfig("torrent_level")
	if level == "" {
		level = LevelPro
		db.SetConfig("torrent_level", LevelPro)
	}

	// 0. Ensure ipset
	exec.Command("apt-get", "install", "-y", "ipset").Run()
	exec.Command("ipset", "create", "torrent", "hash:ip").Run()

	var rules []string

	// BASIC Rules (Ports)
	rules = append(rules,
		"OUTPUT -p tcp --dport 6881:6999 -j DROP",
		"OUTPUT -p udp --dport 6881:6999 -j DROP",
	)

	// PRO Rules (Strings + IPSET + QUIC)
	if level == LevelPro || level == LevelAutoBan {
		rules = append(rules,
			"OUTPUT -m string --string \"BitTorrent\" --algo bm -j DROP",
			"OUTPUT -m string --string \"peer_id=\" --algo bm -j DROP",
			"OUTPUT -m string --string \"info_hash\" --algo bm -j DROP",
			"OUTPUT -m string --string \"announce\" --algo bm -j DROP",
			"OUTPUT -m string --string \".torrent\" --algo bm -j DROP",
			"OUTPUT -m string --string \"announce.php?passkey=\" --algo bm -j DROP",
			"OUTPUT -m set --match-set torrent dst -j DROP",
			"OUTPUT -p udp --dport 443 -j DROP", // Block QUIC
			"OUTPUT -p tcp --syn -m connlimit --connlimit-above 20 -j REJECT",
		)
		blockTorrentDomains()
	}

	for _, rule := range rules {
		checkCmd := exec.Command("iptables", append([]string{"-C"}, strings.Fields(rule)...)...)
		if err := checkCmd.Run(); err != nil {
			addCmd := exec.Command("iptables", append([]string{"-A"}, strings.Fields(rule)...)...)
			addCmd.Run()
		}
	}

	exec.Command("iptables-save").Run()
	db.SetConfig("torrent_blocked", "true")
	return nil
}

// DisableTorrentProtection removes all torrent rules
func DisableTorrentProtection() error {
	rules := []string{
		"OUTPUT -p tcp --dport 6881:6999 -j DROP",
		"OUTPUT -p udp --dport 6881:6999 -j DROP",
		"OUTPUT -m string --string \"BitTorrent\" --algo bm -j DROP",
		"OUTPUT -m string --string \"peer_id=\" --algo bm -j DROP",
		"OUTPUT -m string --string \"info_hash\" --algo bm -j DROP",
		"OUTPUT -m string --string \"announce\" --algo bm -j DROP",
		"OUTPUT -m string --string \".torrent\" --algo bm -j DROP",
		"OUTPUT -m string --string \"announce.php?passkey=\" --algo bm -j DROP",
		"OUTPUT -m set --match-set torrent dst -j DROP",
		"OUTPUT -p udp --dport 443 -j DROP",
		"OUTPUT -p tcp --syn -m connlimit --connlimit-above 20 -j REJECT",
	}

	for _, rule := range rules {
		checkCmd := exec.Command("iptables", append([]string{"-C"}, strings.Fields(rule)...)...)
		if err := checkCmd.Run(); err == nil {
			delCmd := exec.Command("iptables", append([]string{"-D"}, strings.Fields(rule)...)...)
			delCmd.Run()
		}
	}

	exec.Command("ipset", "destroy", "torrent").Run()
	unblockTorrentDomains()
	exec.Command("iptables-save").Run()
	db.SetConfig("torrent_blocked", "false")
	return nil
}

// GetDetailedTorrentStatus returns active rules info
func GetDetailedTorrentStatus() map[string]bool {
	status := make(map[string]bool)

	checkRule := func(rule string) bool {
		return exec.Command("iptables", append([]string{"-C"}, strings.Fields(rule)...)...).Run() == nil
	}

	status["PORT_BLOCK"] = checkRule("OUTPUT -p tcp --dport 6881:6999 -j DROP")
	status["STRING_BLOCK"] = checkRule("OUTPUT -m string --string \"BitTorrent\" --algo bm -j DROP")
	status["QUIC_BLOCK"] = checkRule("OUTPUT -p udp --dport 443 -j DROP")
	status["CONNLIMIT"] = checkRule("OUTPUT -p tcp --syn -m connlimit --connlimit-above 20 -j REJECT")

	ipsetCmd := exec.Command("ipset", "list", "torrent")
	status["IPSET"] = ipsetCmd.Run() == nil

	return status
}

// GetTorrentStatus returns if protection is enabled in DB
func GetTorrentStatus() bool {
	status, _ := db.GetConfig("torrent_blocked")
	return status == "true"
}

// blockTorrentDomains adds common trackers to /etc/hosts
func blockTorrentDomains() {
	domains := []string{
		"tracker.openwebtorrent.com",
		"tracker.publicbt.com",
		"tracker.istole.it",
		"open.demonii.com",
		"tracker.coppersurfer.tk",
		"tracker.leechers-paradise.org",
		"ipv4.tracker.harry.lu",
	}

	for _, domain := range domains {
		cmd := fmt.Sprintf("echo '127.0.0.1 %s' >> /etc/hosts", domain)
		checkCmd := fmt.Sprintf("grep -q '%s' /etc/hosts", domain)
		if err := exec.Command("bash", "-c", checkCmd).Run(); err != nil {
			exec.Command("bash", "-c", cmd).Run()
		}
	}
}

// unblockTorrentDomains removes trackers from /etc/hosts
func unblockTorrentDomains() {
	domains := []string{
		"tracker.openwebtorrent.com",
		"tracker.publicbt.com",
		"tracker.istole.it",
		"open.demonii.com",
		"tracker.coppersurfer.tk",
		"tracker.leechers-paradise.org",
		"ipv4.tracker.harry.lu",
	}

	for _, domain := range domains {
		cmd := fmt.Sprintf("sed -i '/%s/d' /etc/hosts", domain)
		exec.Command("bash", "-c", cmd).Run()
	}
}
