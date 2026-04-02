package system

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	HysteriaBinary = "/usr/local/bin/hysteria"
	HysteriaConfig = "/etc/hysteria/config.yaml"
	HysteriaCert   = "/etc/hysteria/server.crt"
	HysteriaKey    = "/etc/hysteria/server.key"
)

// GetHysteriaStatus retorna o status do serviço Hysteria 2
func GetHysteriaStatus() (string, bool) {
	cmd := exec.Command("systemctl", "is-active", "hysteria.service")
	output, _ := cmd.Output()
	status := strings.TrimSpace(string(output))
	if status == "active" {
		return "ATIVO", true
	}
	return "INATIVO", false
}

// InstallHysteria baixa e instala o Hysteria 2
func InstallHysteria(port int, password string) error {
	// 1. Criar diretórios
	os.MkdirAll("/etc/hysteria", 0755)

	// 2. Baixar binário (versão v2 mais recente)
	// Para simplificar, usamos o script oficial de instalação via curl se possível, 
	// ou baixamos o binário direto do GitHub.
	fmt.Println("⏳ Baixando binário do Hysteria 2...")
	dlCmd := "bash -c \"curl -fsSL https://get.hy2.sh/ | bash\""
	if err := exec.Command("bash", "-c", dlCmd).Run(); err != nil {
		return fmt.Errorf("falha ao instalar Hysteria via script: %v", err)
	}

	// 3. Gerar certificado auto-assinado
	fmt.Println("⏳ Gerando certificados auto-assinados...")
	certCmd := exec.Command("openssl", "req", "-x509", "-nodes", "-newkey", "rsa:2048", 
		"-keyout", HysteriaKey, "-out", HysteriaCert, "-days", "3650", 
		"-subj", "/CN=hysteria-ssh-panel")
	if err := certCmd.Run(); err != nil {
		return fmt.Errorf("falha ao gerar certificados: %v", err)
	}

	// 4. Criar arquivo de configuração (Hysteria 2 usa YAML)
	configContent := fmt.Sprintf(`listen: :%d

tls:
  cert: %s
  key: %s

auth:
  type: password
  password: %s

masquerade:
  type: proxy
  proxy:
    url: https://www.google.com
    rewriteHost: true

quic:
  initStreamReceiveWindow: 8388608
  maxStreamReceiveWindow: 8388608
  initConnReceiveWindow: 20971520
  maxConnReceiveWindow: 20971520
  maxIdleTimeout: 30s
  maxIncomingStreams: 1024
  disablePathMTUDiscovery: false
`, port, HysteriaCert, HysteriaKey, password)

	if err := os.WriteFile(HysteriaConfig, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("falha ao criar config.yaml: %v", err)
	}

	// 5. Criar serviço Systemd
	serviceContent := `[Unit]
Description=Hysteria 2 Service
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/hysteria server --config /etc/hysteria/config.yaml
Restart=always
RestartSec=3
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
`
	if err := os.WriteFile("/etc/systemd/system/hysteria.service", []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("falha ao criar hysteria.service: %v", err)
	}

	// 6. Recarregar systemd e iniciar
	exec.Command("systemctl", "daemon-reload").Run()
	exec.Command("systemctl", "enable", "hysteria.service").Run()
	if err := exec.Command("systemctl", "restart", "hysteria.service").Run(); err != nil {
		return fmt.Errorf("falha ao iniciar hysteria: %v", err)
	}

	return nil
}

// DisableHysteria para e desativa o serviço
func DisableHysteria() error {
	exec.Command("systemctl", "stop", "hysteria.service").Run()
	exec.Command("systemctl", "disable", "hysteria.service").Run()
	return nil
}

// UninstallHysteria remove o Hysteria do sistema
func UninstallHysteria() error {
	exec.Command("systemctl", "stop", "hysteria.service").Run()
	exec.Command("systemctl", "disable", "hysteria.service").Run()
	os.Remove("/etc/systemd/system/hysteria.service")
	exec.Command("systemctl", "daemon-reload").Run()
	os.RemoveAll("/etc/hysteria")
	os.Remove(HysteriaBinary)
	return nil
}

// GenerateHysteriaLink gera o link hy2:// para o cliente
func GenerateHysteriaLink(ip, password string, port int) string {
	// hy2://password@ip:port?insecure=1&sni=google.com
	return fmt.Sprintf("hy2://%s@%s:%d?insecure=1&sni=www.google.com#Hysteria2_SSH_Panel", password, ip, port)
}
