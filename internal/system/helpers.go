package system

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// GetLocation retorna o fuso horário de Brasília (America/Sao_Paulo)
func GetLocation() *time.Location {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		// Fallback se o zoneinfo não estiver instalado (comum em sistemas mínimos)
		return time.FixedZone("BRT", -3*60*60)
	}
	return loc
}

// GetNowBrasilia retorna o horário atual em Brasília
func GetNowBrasilia() time.Time {
	return time.Now().In(GetLocation())
}

// FormatDuration formata uma duração em HH:MM:SS
func FormatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// GetGoBin attempts to find the Go binary
func GetGoBin() string {
	if _, err := os.Stat("/usr/local/go/bin/go"); err == nil {
		return "/usr/local/go/bin/go"
	}
	if path, err := exec.LookPath("go"); err == nil {
		return path
	}
	return "go"
}

// IsPortAvailable checks if a port is currently in use
func IsPortAvailable(port int) bool {
	// Verifica netstat
	cmd := exec.Command("bash", "-c", fmt.Sprintf("netstat -tuln | grep :%d", port))
	err := cmd.Run()
	if err == nil {
		return false // Porta em uso
	}

	// Verifica se o Nginx está rodando na porta 80/443 se for o caso
	if port == 80 || port == 443 {
		cmdCheck := exec.Command("systemctl", "is-active", "nginx")
		if err := cmdCheck.Run(); err == nil {
			return false // Nginx ativo e pode causar conflito/301
		}
		cmdCheckApache := exec.Command("systemctl", "is-active", "apache2")
		if err := cmdCheckApache.Run(); err == nil {
			return false // Apache ativo
		}
	}

	return true // Porta provavelmente livre
}

// ManageFirewall handles ufw/iptables for a specific port
func ManageFirewall(port int, action string) {
	portStr := strconv.Itoa(port)
	if action == "allow" {
		exec.Command("ufw", "allow", portStr+"/tcp").Run()
		exec.Command("iptables", "-A", "INPUT", "-p", "tcp", "--dport", portStr, "-j", "ACCEPT").Run()
	} else {
		exec.Command("ufw", "delete", "allow", portStr+"/tcp").Run()
		exec.Command("iptables", "-D", "INPUT", "-p", "tcp", "--dport", portStr, "-j", "ACCEPT").Run()
	}
}

// IsRoot checks if the current process is running as root
func IsRoot() bool {
	return os.Getuid() == 0
}

// CopyToClipboard tenta copiar um texto para o clipboard usando xclip ou xsel.
// Retorna true se teve sucesso.
func CopyToClipboard(text string) bool {
	// Tenta xclip
	cmd := exec.Command("xclip", "-selection", "clipboard")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err == nil {
		return true
	}

	// Tenta xsel
	cmd = exec.Command("xsel", "--clipboard", "--input")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err == nil {
		return true
	}

	return false
}

// EnsureClipboardTools garante que as ferramentas de clipboard estejam instaladas.
func EnsureClipboardTools() {
	if _, err := exec.LookPath("xclip"); err != nil {
		exec.Command("apt-get", "install", "-y", "xclip").Run()
	}
}

// EnsurePainelService cria e inicia o serviço do painel para rodar em background.
func EnsurePainelService() error {
	const servicePath = "/etc/systemd/system/painel-api.service"
	
	// Tenta encontrar o binário atual
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("erro ao obter caminho do executável: %v", err)
	}

	serviceContent := fmt.Sprintf(`[Unit]
Description=Painel SSH - Background APIs
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/etc/painel-ssh
ExecStart=%s --run-apis
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, exePath)

	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("erro ao criar arquivo de serviço: %v", err)
	}

	exec.Command("systemctl", "daemon-reload").Run()
	exec.Command("systemctl", "enable", "painel-api.service").Run()
	exec.Command("systemctl", "start", "painel-api.service").Run()

	return nil
}
