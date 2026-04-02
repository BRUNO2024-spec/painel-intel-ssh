package system

import (
	"fmt"
	"os"
	"os/exec"
	"painel-ssh/internal/db"
	"strconv"
	"strings"
)

const (
	BadVPNBinary   = "/usr/bin/badvpn-udpgw"
	BadVPNTemplate = "/etc/systemd/system/badvpn@.service"
)

// InstallBadVPNPro prepares the system for BadVPN with template support
func InstallBadVPNPro() error {
	// 1. Download if not exists
	if _, err := os.Stat(BadVPNBinary); os.IsNotExist(err) {
		fmt.Println("⏳ Baixando badvpn-udpgw...")
		exec.Command("wget", "-O", BadVPNBinary, "https://raw.githubusercontent.com/kiritosshxd/SSHPLUS/master/Install/badvpn-udpgw").Run()
		exec.Command("chmod", "+x", BadVPNBinary).Run()
	}

	// 2. Create systemd template service
	template := `[Unit]
Description=BadVPN UDPGW Service on Port %I
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/badvpn-udpgw --listen-addr 127.0.0.1:%i --max-clients 1000 --max-connections-for-client 10
Restart=always
User=root

[Install]
WantedBy=multi-user.target
`
	if err := os.WriteFile(BadVPNTemplate, []byte(template), 0644); err != nil {
		return fmt.Errorf("erro ao criar template badvpn: %v", err)
	}

	exec.Command("systemctl", "daemon-reload").Run()
	return nil
}

// AddBadVPNPort starts a new BadVPN instance on specific port
func AddBadVPNPort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("porta inválida")
	}

	portStr := strconv.Itoa(port)
	serviceName := fmt.Sprintf("badvpn@%s", portStr)

	// Start and Enable
	exec.Command("systemctl", "enable", serviceName).Run()
	if err := exec.Command("systemctl", "start", serviceName).Run(); err != nil {
		return fmt.Errorf("erro ao iniciar porta %d: %v", port, err)
	}

	// Firewall
	exec.Command("iptables", "-I", "INPUT", "-p", "tcp", "--dport", portStr, "-j", "ACCEPT").Run()
	exec.Command("iptables", "-I", "INPUT", "-p", "udp", "--dport", portStr, "-j", "ACCEPT").Run()

	// Save in DB (using a comma-separated list)
	ports, _ := db.GetConfig("badvpn_ports")
	if ports == "" {
		db.SetConfig("badvpn_ports", portStr)
	} else if !strings.Contains(ports, portStr) {
		db.SetConfig("badvpn_ports", ports+","+portStr)
	}

	return nil
}

// RemoveBadVPNPort stops and disables a BadVPN instance
func RemoveBadVPNPort(port int) error {
	portStr := strconv.Itoa(port)
	serviceName := fmt.Sprintf("badvpn@%s", portStr)

	exec.Command("systemctl", "stop", serviceName).Run()
	exec.Command("systemctl", "disable", serviceName).Run()

	// Remove Firewall
	exec.Command("iptables", "-D", "INPUT", "-p", "tcp", "--dport", portStr, "-j", "ACCEPT").Run()
	exec.Command("iptables", "-D", "INPUT", "-p", "udp", "--dport", portStr, "-j", "ACCEPT").Run()

	// Update DB
	ports, _ := db.GetConfig("badvpn_ports")
	list := strings.Split(ports, ",")
	var newList []string
	for _, p := range list {
		if p != portStr && p != "" {
			newList = append(newList, p)
		}
	}
	db.SetConfig("badvpn_ports", strings.Join(newList, ","))

	return nil
}

// ListActiveBadVPNPorts returns all active ports from systemd
func ListActiveBadVPNPorts() []string {
	cmd := "systemctl list-units --type=service --state=running | grep 'badvpn@' | awk '{print $1}' | cut -d'@' -f2 | cut -d'.' -f1"
	out, _ := exec.Command("bash", "-c", cmd).Output()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var ports []string
	for _, l := range lines {
		if l != "" {
			ports = append(ports, l)
		}
	}
	return ports
}

// GetBadVPNUsage returns user and connection count for BadVPN (UDP)
func GetBadVPNUsage() map[string]int {
	usage := make(map[string]int)

	// Utiliza netstat para ver conexões UDP estabelecidas (via badvpn-udpgw)
	// Como badvpn-udpgw escuta em 127.0.0.1, precisamos ver quem está conectado a ele
	// Uma forma é ver conexões UDP locais
	cmd := "netstat -unp | grep badvpn-udpgw | awk '{print $5}' | cut -d: -f1 | sort | uniq -c"
	out, _ := exec.Command("bash", "-c", cmd).Output()

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			count, _ := strconv.Atoi(fields[0])
			ip := fields[1]
			user := GetUserByIP(ip)
			if user == "" {
				user = "Desconhecido (" + ip + ")"
			}
			usage[user] += count
		}
	}
	return usage
}

// StopAllBadVPN stops all instances and removes firewall rules
func StopAllBadVPN() {
	ports := ListActiveBadVPNPorts()
	for _, p := range ports {
		portInt, _ := strconv.Atoi(p)
		RemoveBadVPNPort(portInt)
	}
	exec.Command("systemctl", "stop", "badvpn*").Run()
	db.SetConfig("badvpn_ports", "")
}
