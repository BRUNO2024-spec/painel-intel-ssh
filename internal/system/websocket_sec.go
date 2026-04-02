package system

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// InstallWebSocketSec sets up a secure WebSocket server (websocket-sec.service)
func InstallWebSocketSec(port int) error {
	// 1. Get Go binary
	goBin := GetGoBin()

	// 2. Compile (should already be compiled by InstallWebSocket, but let's ensure)
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

	// 3. Create systemd service (websocket-sec.service)
	serviceContent := fmt.Sprintf(`[Unit]
Description=WebSocket Security Service for SSH Panel
After=network.target

[Service]
Type=simple
ExecStart=%s -port %d -path /ws
Restart=always
User=root

[Install]
WantedBy=multi-user.target
`, WSBinary, port)

	if err := os.WriteFile("/etc/systemd/system/websocket-sec.service", []byte(serviceContent), 0644); err != nil {
		return err
	}

	// 4. Reload systemd and start service
	exec.Command("systemctl", "daemon-reload").Run()
	exec.Command("systemctl", "enable", "websocket-sec.service").Run()
	if err := exec.Command("systemctl", "restart", "websocket-sec.service").Run(); err != nil {
		return err
	}

	// 5. Open firewall
	ManageFirewall(port, "allow")

	return nil
}

// GetWebSocketSecStatus returns the status of the websocket-sec service
func GetWebSocketSecStatus() (string, bool) {
	cmd := exec.Command("systemctl", "is-active", "websocket-sec.service")
	output, err := cmd.Output()
	status := strings.TrimSpace(string(output))
	if err != nil || status != "active" {
		return "INATIVO", false
	}
	return "ATIVO", true
}

// DisableWebSocketSec stops and removes the websocket-sec service
func DisableWebSocketSec(port int) error {
	exec.Command("systemctl", "stop", "websocket-sec.service").Run()
	exec.Command("systemctl", "disable", "websocket-sec.service").Run()
	os.Remove("/etc/systemd/system/websocket-sec.service")
	exec.Command("systemctl", "daemon-reload").Run()
	
	if port > 0 {
		ManageFirewall(port, "deny")
	}
	return nil
}
