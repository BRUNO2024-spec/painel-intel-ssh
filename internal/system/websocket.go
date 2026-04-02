package system

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	WSBinary = "/usr/local/bin/websocket-server"
)

// InstallWebSocket sets up a Go-based WebSocket server
func InstallWebSocket(port int) error {
	// 1. Get Go binary
	goBin := GetGoBin()

	// 2. Compile the Go WebSocket server
	// We'll assume the source code is in the project directory /opt/painel-ssh
	// If not, we'd need to fetch it. For now, let's use the local source.
	projectDir := "/opt/painel-ssh"
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		// Fallback to current working directory if /opt/painel-ssh doesn't exist
		cwd, _ := os.Getwd()
		projectDir = cwd
	}

	cmdBuild := exec.Command(goBin, "build", "-o", WSBinary, "cmd/websocket/main.go")
	cmdBuild.Dir = projectDir
	if output, err := cmdBuild.CombinedOutput(); err != nil {
		return fmt.Errorf("falha ao compilar WebSocket server: %v\nSaída: %s", err, string(output))
	}

	// 3. Create systemd service
	serviceContent := fmt.Sprintf(`[Unit]
Description=WebSocket Service for SSH Panel
After=network.target

[Service]
Type=simple
ExecStart=%s -port %d -path /ws
Restart=always
User=root

[Install]
WantedBy=multi-user.target
`, WSBinary, port)

	if err := os.WriteFile("/etc/systemd/system/websocket.service", []byte(serviceContent), 0644); err != nil {
		return err
	}

	// 4. Reload systemd and start service
	exec.Command("systemctl", "daemon-reload").Run()
	exec.Command("systemctl", "enable", "websocket.service").Run()
	if err := exec.Command("systemctl", "restart", "websocket.service").Run(); err != nil {
		return err
	}

	// 5. Open firewall
	ManageFirewall(port, "allow")

	return nil
}

// GetWebSocketStatus returns the status of the websocket service
func GetWebSocketStatus() (string, bool) {
	cmd := exec.Command("systemctl", "is-active", "websocket.service")
	output, err := cmd.Output()
	status := strings.TrimSpace(string(output))
	if err != nil || status != "active" {
		return "INATIVO", false
	}
	return "ATIVO", true
}

// DisableWebSocket stops and removes the websocket service
func DisableWebSocket(port int) error {
	exec.Command("systemctl", "stop", "websocket.service").Run()
	exec.Command("systemctl", "disable", "websocket.service").Run()
	os.Remove("/etc/systemd/system/websocket.service")
	// Keep binary for other WS services if needed, or remove
	// os.Remove(WSBinary)
	exec.Command("systemctl", "daemon-reload").Run()
	
	if port > 0 {
		ManageFirewall(port, "deny")
	}
	return nil
}
