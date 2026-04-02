package system

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// InstallWebSocketTLS sets up a secure WebSocket server with TLS (WSS)
func InstallWebSocketTLS(port int) error {
	// 1. Get Go binary
	goBin := GetGoBin()

	// 2. Install dependencies (openssl)
	exec.Command("apt", "update").Run()
	exec.Command("apt", "install", "openssl", "-y").Run()

	// 3. Create directory for certificates
	certDir := "/etc/painel-ssh/certs"
	os.MkdirAll(certDir, 0755)

	certFile := certDir + "/websocket.crt"
	keyFile := certDir + "/websocket.key"

	// 4. Generate self-signed certificate if not exists
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		genCertCmd := exec.Command("openssl", "req", "-x509", "-newkey", "rsa:4096", "-keyout", keyFile, "-out", certFile, "-days", "365", "-nodes", "-subj", "/C=BR/ST=SP/L=SP/O=SSHPanel/OU=IT/CN=sshpanel.com")
		if err := genCertCmd.Run(); err != nil {
			return fmt.Errorf("falha ao gerar certificado SSL: %v", err)
		}
	}

	// 5. Compile WebSocket server
	projectDir := "/opt/painel-ssh"
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		cwd, _ := os.Getwd()
		projectDir = cwd
	}

	cmdBuild := exec.Command(goBin, "build", "-o", WSBinary, "cmd/websocket/main.go")
	cmdBuild.Dir = projectDir
	if output, err := cmdBuild.CombinedOutput(); err != nil {
		return fmt.Errorf("falha ao compilar WebSocket server: %v\nSaída: %s", err, string(output))
	}

	// 6. Create systemd service (websocket-tls.service)
	serviceContent := fmt.Sprintf(`[Unit]
Description=WebSocket TLS Service for SSH Panel
After=network.target

[Service]
Type=simple
ExecStart=%s -port %d -path /ws -cert %s -key %s
Restart=always
User=root

[Install]
WantedBy=multi-user.target
`, WSBinary, port, certFile, keyFile)

	if err := os.WriteFile("/etc/systemd/system/websocket-tls.service", []byte(serviceContent), 0644); err != nil {
		return err
	}

	// 7. Reload systemd and start service
	exec.Command("systemctl", "daemon-reload").Run()
	exec.Command("systemctl", "enable", "websocket-tls.service").Run()
	if err := exec.Command("systemctl", "restart", "websocket-tls.service").Run(); err != nil {
		return err
	}

	// 8. Open firewall
	ManageFirewall(port, "allow")

	return nil
}

// GetWebSocketTLSStatus returns the status of the websocket-tls service
func GetWebSocketTLSStatus() (string, bool) {
	cmd := exec.Command("systemctl", "is-active", "websocket-tls.service")
	output, err := cmd.Output()
	status := strings.TrimSpace(string(output))
	if err != nil || status != "active" {
		return "INATIVO", false
	}
	return "ATIVO", true
}

// DisableWebSocketTLS stops and removes the websocket-tls service
func DisableWebSocketTLS(port int) error {
	exec.Command("systemctl", "stop", "websocket-tls.service").Run()
	exec.Command("systemctl", "disable", "websocket-tls.service").Run()
	os.Remove("/etc/systemd/system/websocket-tls.service")
	exec.Command("systemctl", "daemon-reload").Run()
	
	if port > 0 {
		ManageFirewall(port, "deny")
	}
	return nil
}
