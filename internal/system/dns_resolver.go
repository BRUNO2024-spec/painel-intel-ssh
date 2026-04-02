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
	UnboundConfigPath = "/etc/unbound/unbound.conf"
	UnboundService    = "unbound.service"
)

// InstallDNSResolver instala e configura o Unbound como um DNS Resolver otimizado.
func InstallDNSResolver(port int) error {
	// 1. Instalar Unbound
	fmt.Println("⏳ Instalando Unbound DNS Resolver...")
	exec.Command("apt", "update").Run()
	if err := exec.Command("apt", "install", "unbound", "-y").Run(); err != nil {
		return fmt.Errorf("falha ao instalar unbound: %v", err)
	}

	// 2. Liberar porta se necessário (systemd-resolved)
	if port == 53 {
		fmt.Println("⏳ Liberando porta 53 (desativando systemd-resolved)...")
		exec.Command("systemctl", "stop", "systemd-resolved").Run()
		exec.Command("systemctl", "disable", "systemd-resolved").Run()
		// Garantir resolv.conf básico para o servidor não ficar offline
		os.WriteFile("/etc/resolv.conf", []byte("nameserver 8.8.8.8\nnameserver 8.8.4.4\n"), 0644)
	}

	// 3. Criar configuração otimizada
	if err := ConfigureDNSResolver(port); err != nil {
		return err
	}

	// 4. Iniciar serviço
	exec.Command("systemctl", "enable", UnboundService).Run()
	if err := exec.Command("systemctl", "restart", UnboundService).Run(); err != nil {
		return fmt.Errorf("falha ao iniciar unbound: %v", err)
	}

	// 5. Firewall
	portStr := strconv.Itoa(port)
	exec.Command("ufw", "allow", portStr+"/udp").Run()
	exec.Command("iptables", "-I", "INPUT", "-p", "udp", "--dport", portStr, "-j", "ACCEPT").Run()

	// Salvar no banco
	db.SetConfig("dns_resolver_port", portStr)
	db.SetConfig("dns_resolver_active", "true")

	return nil
}

// ConfigureDNSResolver gera o arquivo de configuração do Unbound.
func ConfigureDNSResolver(port int) error {
	config := fmt.Sprintf(`server:
    interface: 0.0.0.0
    port: %d
    access-control: 0.0.0.0/0 allow
    
    # Otimizações de Performance
    num-threads: 2
    msg-cache-size: 64m
    rrset-cache-size: 128m
    cache-min-ttl: 3600
    cache-max-ttl: 86400
    prefetch: yes
    prefetch-key: yes
    
    # Privacidade e Segurança
    hide-identity: yes
    hide-version: yes
    use-caps-for-id: yes
    
    # Encaminhamento para DNS rápidos (Google e Cloudflare)
forward-zone:
    name: "."
    forward-addr: 8.8.8.8
    forward-addr: 1.1.1.1
    forward-addr: 8.8.4.4
    forward-addr: 1.0.0.1
`, port)

	return os.WriteFile(UnboundConfigPath, []byte(config), 0644)
}

// StopDNSResolver para o serviço do DNS Resolver.
func StopDNSResolver() error {
	exec.Command("systemctl", "stop", UnboundService).Run()
	exec.Command("systemctl", "disable", UnboundService).Run()
	db.SetConfig("dns_resolver_active", "false")
	return nil
}

// GetDNSResolverStatus retorna o status do serviço.
func GetDNSResolverStatus() (string, bool) {
	active, _ := db.GetConfig("dns_resolver_active")
	if active != "true" {
		return "INATIVO", false
	}

	cmd := exec.Command("systemctl", "is-active", UnboundService)
	out, _ := cmd.Output()
	status := strings.TrimSpace(string(out))
	
	if status == "active" {
		return "ATIVO", true
	}
	return "ERRO (" + status + ")", false
}

// UpdateDNSResolverPort altera a porta do DNS Resolver.
func UpdateDNSResolverPort(newPort int) error {
	oldPortStr, _ := db.GetConfig("dns_resolver_port")
	
	// Parar firewall da porta antiga
	if oldPortStr != "" {
		exec.Command("ufw", "delete", "allow", oldPortStr+"/udp").Run()
		exec.Command("iptables", "-D", "INPUT", "-p", "udp", "--dport", oldPortStr, "-j", "ACCEPT").Run()
	}

	// Reconfigurar e reiniciar
	if err := ConfigureDNSResolver(newPort); err != nil {
		return err
	}

	if err := exec.Command("systemctl", "restart", UnboundService).Run(); err != nil {
		return err
	}

	// Novo Firewall
	portStr := strconv.Itoa(newPort)
	exec.Command("ufw", "allow", portStr+"/udp").Run()
	exec.Command("iptables", "-I", "INPUT", "-p", "udp", "--dport", portStr, "-j", "ACCEPT").Run()

	db.SetConfig("dns_resolver_port", portStr)
	return nil
}