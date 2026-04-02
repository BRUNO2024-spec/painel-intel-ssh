package system

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GetOpenVPNStatus returns the current status of the OpenVPN service
func GetOpenVPNStatus() (string, bool) {
	cmd := exec.Command("systemctl", "is-active", "openvpn-server@server.service")
	output, _ := cmd.Output()
	status := strings.TrimSpace(string(output))
	if status == "active" {
		return "ATIVO", true
	}
	
	// Check legacy name
	cmd = exec.Command("systemctl", "is-active", "openvpn.service")
	output, _ = cmd.Output()
	status = strings.TrimSpace(string(output))
	if status == "active" {
		return "ATIVO", true
	}

	return "INATIVO", false
}

// InstallOpenVPN downloads and installs OpenVPN using the Nyr script
func InstallOpenVPN() error {
	// 1. Download the script if it doesn't exist
	scriptPath := "/usr/local/bin/openvpn-install.sh"
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		cmdDl := exec.Command("wget", "https://git.io/vpn", "-O", scriptPath)
		if err := cmdDl.Run(); err != nil {
			return fmt.Errorf("falha ao baixar script OpenVPN: %v", err)
		}
		os.Chmod(scriptPath, 0755)
	}

	// 2. Run the script in non-interactive mode
	// AUTO_INSTALL=y uses defaults: UDP, Port 1194, DNS 1 (Google), Client name "client"
	cmdInstall := exec.Command("bash", scriptPath)
	cmdInstall.Env = append(os.Environ(), "AUTO_INSTALL=y")
	if output, err := cmdInstall.CombinedOutput(); err != nil {
		return fmt.Errorf("falha ao instalar OpenVPN: %v\nSaída: %s", err, string(output))
	}

	return nil
}

// AddOpenVPNClient adds a new client and returns the path to the .ovpn file
func AddOpenVPNClient(clientName string) (string, error) {
	scriptPath := "/usr/local/bin/openvpn-install.sh"
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return "", fmt.Errorf("script de instalação não encontrado")
	}

	// MENU_OPTION=1: Add a new client
	// CLIENT=clientName: Name of the client
	// PASS=1: No password for the client certificate
	cmdAdd := exec.Command("bash", scriptPath)
	cmdAdd.Env = append(os.Environ(), 
		"MENU_OPTION=1", 
		"CLIENT="+clientName, 
		"PASS=1")
	
	if output, err := cmdAdd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("falha ao criar cliente OpenVPN: %v\nSaída: %s", err, string(output))
	}

	ovpnPath := fmt.Sprintf("/root/%s.ovpn", clientName)
	if _, err := os.Stat(ovpnPath); os.IsNotExist(err) {
		// Try alternative path (home directory of the user running the script)
		ovpnPath = fmt.Sprintf("/home/root/%s.ovpn", clientName)
		if _, err := os.Stat(ovpnPath); os.IsNotExist(err) {
			return "", fmt.Errorf("arquivo .ovpn não foi gerado em /root/%s.ovpn", clientName)
		}
	}

	return ovpnPath, nil
}

// RemoveOpenVPNClient removes a client
func RemoveOpenVPNClient(clientName string) error {
	scriptPath := "/usr/local/bin/openvpn-install.sh"
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("script de instalação não encontrado")
	}

	// MENU_OPTION=2: Remove an existing client
	// CLIENT=clientName: Name of the client to remove
	cmdRemove := exec.Command("bash", scriptPath)
	cmdRemove.Env = append(os.Environ(), 
		"MENU_OPTION=2", 
		"CLIENT="+clientName)
	
	if output, err := cmdRemove.CombinedOutput(); err != nil {
		return fmt.Errorf("falha ao remover cliente OpenVPN: %v\nSaída: %s", err, string(output))
	}

	return nil
}

// ListOpenVPNClients returns a list of existing clients
func ListOpenVPNClients() ([]string, error) {
	// Clients are listed in /etc/openvpn/server/easy-rsa/pki/index.txt or similar
	// But a simpler way is to look for .ovpn files in /root or list the certificates
	
	// Actually, the script stores client names in a specific way.
	// Let's try to list files in /etc/openvpn/client/ or check index.txt
	
	var clients []string
	files, err := os.ReadDir("/root")
	if err == nil {
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".ovpn") {
				clients = append(clients, strings.TrimSuffix(f.Name(), ".ovpn"))
			}
		}
	}
	
	return clients, nil
}

// UninstallOpenVPN removes OpenVPN from the system
func UninstallOpenVPN() error {
	scriptPath := "/usr/local/bin/openvpn-install.sh"
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("script de instalação não encontrado")
	}

	// MENU_OPTION=3: Uninstall OpenVPN
	cmdUninstall := exec.Command("bash", scriptPath)
	cmdUninstall.Env = append(os.Environ(), "MENU_OPTION=3")
	
	// Need to confirm uninstallation if script asks, but typically MENU_OPTION=3 handles it
	if output, err := cmdUninstall.CombinedOutput(); err != nil {
		return fmt.Errorf("falha ao desinstalar OpenVPN: %v\nSaída: %s", err, string(output))
	}

	return nil
}
