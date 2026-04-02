package system

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// InstallSSLTunnel sets up stunnel4 for SSH encapsulation
func InstallSSLTunnel(port int) error {
	// 1. Install stunnel4
	exec.Command("apt", "update").Run()
	if err := exec.Command("apt", "install", "stunnel4", "openssl", "-y").Run(); err != nil {
		return fmt.Errorf("falha ao instalar stunnel4: %v", err)
	}

	// 2. Create certs directory
	certDir := "/etc/stunnel"
	os.MkdirAll(certDir, 0755)
	certFile := certDir + "/stunnel.pem"

	// 3. Generate self-signed certificate if not exists
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		genCertCmd := exec.Command("openssl", "req", "-x509", "-newkey", "rsa:4096", "-keyout", certFile, "-out", certFile, "-days", "365", "-nodes", "-subj", "/C=BR/ST=SP/L=SP/O=SSHPanel/OU=IT/CN=sshpanel.com")
		if err := genCertCmd.Run(); err != nil {
			return fmt.Errorf("falha ao gerar certificado SSL: %v", err)
		}
	}

	// 4. Create stunnel configuration
	stunnelConf := fmt.Sprintf(`pid = /var/run/stunnel4.pid
cert = %s
key = %s
client = no
socket = a:SO_REUSEADDR=1
socket = l:TCP_NODELAY=1
socket = r:TCP_NODELAY=1

[ssh]
accept = 0.0.0.0:%d
connect = 127.0.0.1:22
`, certFile, certFile, port)

	if err := os.WriteFile("/etc/stunnel/stunnel.conf", []byte(stunnelConf), 0644); err != nil {
		return err
	}

	// 5. Enable stunnel4 in /etc/default/stunnel4
	exec.Command("sed", "-i", "s/ENABLED=0/ENABLED=1/g", "/etc/default/stunnel4").Run()

	// 6. Create systemd override or ensure service is ready
	// stunnel4 on Ubuntu usually comes with a service
	exec.Command("systemctl", "enable", "stunnel4").Run()
	if err := exec.Command("systemctl", "restart", "stunnel4").Run(); err != nil {
		return err
	}

	// 7. Open firewall
	ManageFirewall(port, "allow")

	return nil
}

// GetSSLTunnelStatus returns the status of the stunnel4 service
func GetSSLTunnelStatus() (string, bool) {
	cmd := exec.Command("systemctl", "is-active", "stunnel4")
	output, err := cmd.Output()
	status := strings.TrimSpace(string(output))
	if err != nil || status != "active" {
		return "INATIVO", false
	}
	return "ATIVO", true
}

// DisableSSLTunnel stops and disables stunnel4
func DisableSSLTunnel(port int) error {
	exec.Command("systemctl", "stop", "stunnel4").Run()
	exec.Command("systemctl", "disable", "stunnel4").Run()
	os.Remove("/etc/stunnel/stunnel.conf")
	
	if port > 0 {
		ManageFirewall(port, "deny")
	}
	return nil
}
