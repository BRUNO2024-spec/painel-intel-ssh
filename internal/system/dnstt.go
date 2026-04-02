package system

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	DNSTTDir    = "/etc/dnstt"
	DNSTTBinary = "/usr/local/bin/dnstt-server"
)

// InstallDNSTT installs and configures DNSTT / Slow DNS
func InstallDNSTT(ns string) (string, error) {
	// 0. Disable systemd-resolved to free port 53
	fmt.Println("Liberando porta 53 (desativando systemd-resolved)...")
	exec.Command("systemctl", "stop", "systemd-resolved").Run()
	exec.Command("systemctl", "disable", "systemd-resolved").Run()
	exec.Command("rm", "/etc/resolv.conf").Run()
	os.WriteFile("/etc/resolv.conf", []byte("nameserver 8.8.8.8\nnameserver 8.8.4.4\n"), 0644)

	// 1. Install dependencies
	exec.Command("apt", "update").Run()
	if err := exec.Command("apt", "install", "golang", "git", "build-essential", "-y").Run(); err != nil {
		return "", fmt.Errorf("falha ao instalar dependências: %v", err)
	}

	// 2. Download and compile DNSTT
	tmpDir := "/tmp/dnstt-build"
	os.RemoveAll(tmpDir)
	if err := exec.Command("git", "clone", "https://www.bamsoftware.com/git/dnstt.git", tmpDir).Run(); err != nil {
		return "", fmt.Errorf("falha ao clonar DNSTT: %v", err)
	}

	// Use helper to find go binary
	goBin := GetGoBin()

	// Log Go version for diagnostics
	goVer, _ := exec.Command(goBin, "version").Output()
	fmt.Printf("Usando Go: %s", string(goVer))

	cmdBuild := exec.Command(goBin, "build", "-o", DNSTTBinary)
	cmdBuild.Dir = tmpDir + "/dnstt-server"
	// Ensure we use the latest modules and download dependencies
	exec.Command("bash", "-c", "cd "+tmpDir+"/dnstt-server && "+goBin+" mod tidy && "+goBin+" mod download").Run()

	if output, err := cmdBuild.CombinedOutput(); err != nil {
		return "", fmt.Errorf("falha ao compilar DNSTT: %v\n\nIMPORTANTE: O DNSTT requer Go 1.21 ou superior.\nSua versão atual: %s\n\nSaída do erro: %s", err, string(goVer), string(output))
	}

	// 3. Create config directory and generate keys
	os.MkdirAll(DNSTTDir, 0755)
	keyFile := DNSTTDir + "/server.key"
	pubFile := DNSTTDir + "/server.pub"

	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		genKeyCmd := exec.Command(DNSTTBinary, "-gen-key", "-privkey-file", keyFile, "-pubkey-file", pubFile)
		if err := genKeyCmd.Run(); err != nil {
			return "", fmt.Errorf("falha ao gerar chaves DNSTT: %v", err)
		}
	}

	// Read public key
	pubKeyBytes, err := os.ReadFile(pubFile)
	if err != nil {
		return "", fmt.Errorf("falha ao ler chave pública: %v", err)
	}
	pubKey := strings.TrimSpace(string(pubKeyBytes))

	// 4. Create systemd service with optimization
	// Otimizações: Ajuste de buffers e conexões
	serviceContent := fmt.Sprintf(`[Unit]
Description=DNSTT Slow DNS Server
After=network.target

[Service]
Type=simple
ExecStart=%s -udp :53 -privkey-file %s %s 127.0.0.1:22
Restart=always
User=root
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
`, DNSTTBinary, keyFile, ns)

	if err := os.WriteFile("/etc/systemd/system/dnstt.service", []byte(serviceContent), 0644); err != nil {
		return "", err
	}

	// 5. Reload systemd and start
	exec.Command("systemctl", "daemon-reload").Run()
	exec.Command("systemctl", "enable", "dnstt.service").Run()
	if err := exec.Command("systemctl", "restart", "dnstt.service").Run(); err != nil {
		return "", err
	}

	// 6. Firewall
	exec.Command("ufw", "allow", "53/udp").Run()
	exec.Command("iptables", "-A", "INPUT", "-p", "udp", "--dport", "53", "-j", "ACCEPT").Run()

	return pubKey, nil
}

// GetDNSTTStatus returns the status of the dnstt service
func GetDNSTTStatus() (string, bool) {
	cmd := exec.Command("systemctl", "is-active", "dnstt.service")
	output, err := cmd.Output()
	status := strings.TrimSpace(string(output))
	if err != nil || status != "active" {
		return "INATIVO", false
	}
	return "ATIVO", true
}

// DisableDNSTT stops and disables dnstt service
func DisableDNSTT() error {
	exec.Command("systemctl", "stop", "dnstt.service").Run()
	return exec.Command("systemctl", "disable", "dnstt.service").Run()
}

// UninstallDNSTT removes all dnstt files and services
func UninstallDNSTT() error {
	exec.Command("systemctl", "stop", "dnstt.service").Run()
	exec.Command("systemctl", "disable", "dnstt.service").Run()
	os.Remove("/etc/systemd/system/dnstt.service")
	os.Remove(DNSTTBinary)
	os.RemoveAll(DNSTTDir)
	exec.Command("systemctl", "daemon-reload").Run()
	
	// Close firewall
	exec.Command("ufw", "delete", "allow", "53/udp").Run()
	exec.Command("iptables", "-D", "INPUT", "-p", "udp", "--dport", "53", "-j", "ACCEPT").Run()
	
	return nil
}
