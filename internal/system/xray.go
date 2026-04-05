package system

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	XrayConfigPath = "/usr/local/etc/xray/config.json"
	XrayService    = "xray.service"
	XrayCertFile   = "/etc/xray/xray.crt"
	XrayKeyFile    = "/etc/xray/xray.key"
	XrayCertDir    = "/etc/xray"
)

// ── Structs da configuração Xray ─────────────────────────────────────────────

type XrayConfig struct {
	Log struct {
		Access   string `json:"access,omitempty"`
		Error    string `json:"error,omitempty"`
		LogLevel string `json:"loglevel"`
	} `json:"log"`
	DNS struct {
		Servers []string `json:"servers"`
	} `json:"dns"`
	Inbounds  []XrayInbound  `json:"inbounds"`
	Outbounds []XrayOutbound `json:"outbounds"`
	Routing   struct {
		DomainStrategy string `json:"domainStrategy"`
		Rules          []struct {
			Type        string   `json:"type"`
			OutboundTag string   `json:"outboundTag"`
			IP          []string `json:"ip,omitempty"`
			Domain      []string `json:"domain,omitempty"`
			Protocol    []string `json:"protocol,omitempty"`
		} `json:"rules"`
	} `json:"routing"`
}

type XrayOutbound struct {
	Protocol string      `json:"protocol"`
	Settings interface{} `json:"settings,omitempty"`
	Tag      string      `json:"tag"`
}

type XrayInbound struct {
	Tag      string `json:"tag,omitempty"`
	Listen   string `json:"listen"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Settings struct {
		Clients    []XrayClient `json:"clients"`
		Decryption string       `json:"decryption,omitempty"` // Obrigatório no VLESS: "none"
	} `json:"settings"`
	StreamSettings XrayStreamSettings `json:"streamSettings"`
	Sniffing       struct {
		Enabled      bool     `json:"enabled"`
		DestOverride []string `json:"destOverride"`
	} `json:"sniffing"`
}

// XrayStreamSettings agrupa todas as configurações de transporte.
type XrayStreamSettings struct {
	Network  string            `json:"network"`
	Security string            `json:"security"`
	TLS      XrayTLSSettings   `json:"tlsSettings"`
	XHTTP    XrayXHTTPSettings `json:"xhttpSettings"`
}

// XrayTLSSettings contém o serverName (SNI) e os certificados.
type XrayTLSSettings struct {
	ServerName   string     `json:"serverName"` // SNI — ex: www.tim.com.br
	Certificates []XrayCert `json:"certificates"`
}

// XrayXHTTPSettings define path e host do transporte XHTTP.
// O campo Host é o Bug Host (CDN/host mascarado) enviado no header.
type XrayXHTTPSettings struct {
	Path string `json:"path"`
	Host string `json:"host,omitempty"` // Bug Host — ex: mraqhhblnuy.map.azionedge.net
}

type XrayClient struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type XrayCert struct {
	CertificateFile string `json:"certificateFile"`
	KeyFile         string `json:"keyFile"`
}

// CheckPort443InUse retorna true se a porta 443 está em uso por outro processo.
func CheckPort443InUse() (bool, string) {
	out, err := exec.Command("bash", "-c", "lsof -i :443 -sTCP:LISTEN -n -P 2>/dev/null").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return false, ""
	}
	return true, strings.TrimSpace(string(out))
}

// CheckCertsExist verifica se os arquivos de certificado existem.
func CheckCertsExist() error {
	if _, err := os.Stat(XrayCertFile); os.IsNotExist(err) {
		return fmt.Errorf("certificado não encontrado: %s", XrayCertFile)
	}
	if _, err := os.Stat(XrayKeyFile); os.IsNotExist(err) {
		return fmt.Errorf("chave privada não encontrada: %s", XrayKeyFile)
	}
	return nil
}

// ── Geração de link VLESS ─────────────────────────────────────────────────────

// GenerateVlessLink monta o link VLESS no formato padrão de cliente.
// Se insecure for true, adiciona allowInsecure=1 e insecure=1.
func GenerateVlessLink(uuid, host, sni, bugHost string, insecure bool) string {
	q := url.Values{}
	q.Set("path", "/")
	q.Set("security", "tls")
	q.Set("encryption", "none")
	q.Set("host", bugHost)
	q.Set("fp", "chrome")
	q.Set("type", "xhttp")
	q.Set("sni", sni)

	if insecure {
		q.Set("insecure", "1")
		q.Set("allowInsecure", "1")
	}

	return fmt.Sprintf("vless://%s@%s:443?%s#", uuid, host, q.Encode())
}

// ── Instalação ────────────────────────────────────────────────────────────────

// InstallXray instala o Xray-core, gera certificados e configura o serviço.
func InstallXray(host, sni, bugHost, uuid string) error {
	return InstallXrayExtended(host, sni, bugHost, "", "", "", uuid)
}

// InstallXrayExtended instala o Xray com suporte opcional a uma segunda configuração.
func InstallXrayExtended(host, sni, bugHost, host2, sni2, bugHost2, uuid string) error {
	xlog("[⏳] Atualizando pacotes e instalando dependências...")
	exec.Command("apt", "update").Run()
	exec.Command("apt", "install", "-y", "curl", "unzip", "socat", "openssl", "lsof").Run()

	// ── Verificar conflito de porta 443 ──────────────────────────────────────
	xlog("[⏳] Verificando conflito na porta 443...")
	if err := killPort443Conflicts(); err != nil {
		xlog(fmt.Sprintf("[⚠] Aviso ao limpar porta 443: %v", err))
	}

	// ── Instalar Xray-core via script oficial ────────────────────────────────
	xlog("[⏳] Instalando Xray-core...")
	installCmd := exec.Command("bash", "-c",
		"bash <(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)")
	if out, err := installCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("falha ao instalar Xray-core: %v\n%s", err, string(out))
	}

	// ── Criar diretórios necessários ───────────────────────────────────────
	os.MkdirAll("/var/log/xray", 0755)
	exec.Command("chown", "-R", "root:root", "/var/log/xray").Run()
	exec.Command("touch", "/var/log/xray/access.log").Run()
	exec.Command("chmod", "664", "/var/log/xray/access.log").Run()
	exec.Command("touch", "/var/log/xray/error.log").Run()
	exec.Command("chmod", "664", "/var/log/xray/error.log").Run()

	// ── Gerar certificado SSL auto-assinado ──────────────────────────────────
	xlog("[⏳] Gerando certificados SSL...")
	os.MkdirAll(XrayCertDir, 0755)

	// Gerar certificado para o HOST 1
	genCert := exec.Command("openssl", "req", "-x509",
		"-newkey", "rsa:4096",
		"-keyout", XrayKeyFile,
		"-out", XrayCertFile,
		"-days", "365",
		"-nodes",
		"-subj", "/CN="+host)
	if out, err := genCert.CombinedOutput(); err != nil {
		return fmt.Errorf("falha ao gerar certificado SSL: %v\n%s", err, string(out))
	}

	// ── Corrigir permissões dos certificados ─────────────────────────────────
	xlog("[⏳] Ajustando permissões dos certificados...")
	if err := fixCertPermissions(); err != nil {
		return fmt.Errorf("falha ao ajustar permissões: %v", err)
	}

	// ── Reescrever unit do systemd para rodar como root ───────────────────────
	xlog("[⏳] Configurando unit do systemd (User=root)...")
	if err := writeXrayServiceFile(); err != nil {
		return fmt.Errorf("falha ao escrever xray.service: %v", err)
	}

	// ── Escrever config.json ─────────────────────────────────────────────────
	xlog("[⏳] Escrevendo configuração do Xray...")
	config := createDefaultXrayConfigExtended(host, sni, bugHost, host2, sni2, bugHost2, uuid, XrayCertFile, XrayKeyFile)
	configBytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("falha ao serializar config.json: %v", err)
	}

	os.MkdirAll("/usr/local/etc/xray", 0755)
	if err := ioutil.WriteFile(XrayConfigPath, configBytes, 0644); err != nil {
		return fmt.Errorf("falha ao salvar config.json: %v", err)
	}

	// ── Validar configuração antes de iniciar ────────────────────────────────
	xlog("[⏳] Validando configuração do Xray...")
	if err := ValidateXrayConfig(); err != nil {
		return fmt.Errorf("configuração inválida: %v", err)
	}

	ManageFirewall(443, "allow")

	xlog("[⏳] Iniciando serviço Xray...")
	if err := reloadAndRestartXray(); err != nil {
		return fmt.Errorf("falha ao iniciar serviço Xray: %v", err)
	}

	xlog("[⏳] Aguardando serviço estabilizar...")
	time.Sleep(2 * time.Second)

	status := GetXrayStatus()
	if status != "active" {
		logs := GetXrayLogs(20)
		return fmt.Errorf("serviço iniciado mas STATUS=%s\n\n%s", status, logs)
	}

	xlog("[✔] Xray instalado e rodando com sucesso!")
	return nil
}

// createDefaultXrayConfigExtended monta a configuração com suporte a duas hosts.
func createDefaultXrayConfigExtended(host, sni, bugHost, host2, sni2, bugHost2, uuid, cert, key string) XrayConfig {
	if sni == "" {
		sni = host
	}

	var config XrayConfig
	config.Log.LogLevel = "warning"
	config.Log.Access = "/var/log/xray/access.log"
	config.Log.Error = "/var/log/xray/error.log"
	config.DNS.Servers = []string{"1.1.1.1", "8.8.8.8", "localhost"}

	tlsSettings := XrayTLSSettings{
		ServerName:   sni,
		Certificates: []XrayCert{{CertificateFile: cert, KeyFile: key}},
	}

	// Inbound VLESS 1 (Porta 443)
	vless1 := XrayInbound{
		Tag:      "VLESS_1",
		Listen:   "0.0.0.0",
		Port:     443,
		Protocol: "vless",
	}
	vless1.Settings.Clients = []XrayClient{{ID: uuid, Email: "admin@vless1"}}
	vless1.Settings.Decryption = "none"
	vless1.StreamSettings = XrayStreamSettings{
		Network:  "xhttp",
		Security: "tls",
		TLS:      tlsSettings,
		XHTTP: XrayXHTTPSettings{
			Path: "/",
			Host: bugHost,
		},
	}
	vless1.Sniffing.Enabled = true
	vless1.Sniffing.DestOverride = []string{"http", "tls"}

	config.Inbounds = []XrayInbound{vless1}

	// Se houver uma segunda host, adicionamos um inbound secundário.
	// Como não podemos ter dois inbounds na mesma porta 443 com o mesmo transporte sem fallbacks complexos,
	// vamos usar uma porta diferente internamente e você pode redirecionar se necessário,
	// OU, se o objetivo for apenas gerar o link (como parece ser o caso de CDNs),
	// o Xray pode aceitar a conexão se o SNI bater.
	// No entanto, para que o Xray realmente funcione com a segunda host se ela apontar direto pro IP,
	// precisaríamos de SNI routing.
	if host2 != "" {
		if sni2 == "" {
			sni2 = host2
		}
		// Inbound VLESS 2 (Porta 444 como secundária para a segunda host)
		vless2 := XrayInbound{
			Tag:      "VLESS_2",
			Listen:   "0.0.0.0",
			Port:     444, // Usamos 444 para a segunda host
			Protocol: "vless",
		}
		vless2.Settings.Clients = []XrayClient{{ID: uuid, Email: "admin@vless2"}}
		vless2.Settings.Decryption = "none"
		vless2.StreamSettings = XrayStreamSettings{
			Network:  "xhttp",
			Security: "tls",
			TLS: XrayTLSSettings{
				ServerName:   sni2,
				Certificates: []XrayCert{{CertificateFile: cert, KeyFile: key}},
			},
			XHTTP: XrayXHTTPSettings{
				Path: "/",
				Host: bugHost2,
			},
		}
		vless2.Sniffing.Enabled = true
		vless2.Sniffing.DestOverride = []string{"http", "tls"}
		config.Inbounds = append(config.Inbounds, vless2)
		
		// Liberar porta 444
		ManageFirewall(444, "allow")
	}

	// VMESS na 8443
	vmess := XrayInbound{
		Tag:      "VMESS_TLS",
		Listen:   "0.0.0.0",
		Port:     8443,
		Protocol: "vmess",
	}
	vmess.Settings.Clients = []XrayClient{{ID: uuid, Email: "admin@vmess"}}
	vmess.StreamSettings = XrayStreamSettings{
		Network:  "xhttp",
		Security: "tls",
		TLS:      tlsSettings,
		XHTTP: XrayXHTTPSettings{
			Path: "/",
			Host: bugHost,
		},
	}
	vmess.Sniffing.Enabled = true
	vmess.Sniffing.DestOverride = []string{"http", "tls"}
	config.Inbounds = append(config.Inbounds, vmess)

	config.Outbounds = []XrayOutbound{
		{Protocol: "freedom", Tag: "direct"},
		{Protocol: "blackhole", Tag: "blocked"},
	}

	config.Routing.DomainStrategy = "AsIs"
	config.Routing.Rules = []struct {
		Type        string   `json:"type"`
		OutboundTag string   `json:"outboundTag"`
		IP          []string `json:"ip,omitempty"`
		Domain      []string `json:"domain,omitempty"`
		Protocol    []string `json:"protocol,omitempty"`
	}{
		{Type: "field", OutboundTag: "blocked", Protocol: []string{"bittorrent"}},
		{Type: "field", OutboundTag: "direct", IP: []string{"1.1.1.1", "8.8.8.8"}},
	}

	return config
}

// createDefaultXrayConfig é mantido por compatibilidade
func createDefaultXrayConfig(host, sni, bugHost, uuid, cert, key string) XrayConfig {
	return createDefaultXrayConfigExtended(host, sni, bugHost, "", "", "", uuid, cert, key)
}

// ── Funções de diagnóstico e validação ───────────────────────────────────────

// GetXrayStatus retorna o status do serviço via `systemctl is-active`.
func GetXrayStatus() string {
	out, _ := exec.Command("systemctl", "is-active", XrayService).Output()
	return strings.TrimSpace(string(out))
}

// GetXrayStatusBool retorna o status e um bool indicando se está ativo.
func GetXrayStatusBool() (string, bool) {
	status := GetXrayStatus()
	return status, status == "active"
}

// GetXrayLogs retorna as últimas n linhas do journal do serviço xray.
func GetXrayLogs(lines int) string {
	out, err := exec.Command(
		"journalctl", "-u", XrayService,
		"--no-pager", "-n", fmt.Sprintf("%d", lines),
	).Output()
	if err != nil {
		return fmt.Sprintf("(não foi possível obter logs: %v)", err)
	}
	return strings.TrimSpace(string(out))
}

// ValidateXrayConfig executa `xray -test -config` e retorna erro se inválido.
func ValidateXrayConfig() error {
	out, err := exec.Command("xray", "-test", "-config", XrayConfigPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// fixCertPermissions ajusta permissões e ownership dos certificados.
func fixCertPermissions() error {
	exec.Command("chown", "-R", "root:root", XrayCertDir).Run()

	if err := os.Chmod(XrayCertFile, 0644); err != nil {
		return fmt.Errorf("chmod cert: %v", err)
	}
	if err := os.Chmod(XrayKeyFile, 0600); err != nil {
		return fmt.Errorf("chmod key: %v", err)
	}
	return nil
}

func writeXrayServiceFile() error {
	const unit = `[Unit]
Description=Xray Service
Documentation=https://github.com/xtls
After=network.target nss-lookup.target

[Service]
User=root
Group=root
Type=simple
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
NoNewPrivileges=false
ExecStart=/usr/local/bin/xray run -config /usr/local/etc/xray/config.json
Restart=on-failure
RestartPreventExitStatus=23
LimitNPROC=10000
LimitNOFILE=1000000

[Install]
WantedBy=multi-user.target
`
	return os.WriteFile("/etc/systemd/system/xray.service", []byte(unit), 0644)
}

// killPort443Conflicts verifica e encerra processos que estejam usando a porta 443.
func killPort443Conflicts() error {
	exec.Command("systemctl", "stop", "nginx").Run()
	exec.Command("systemctl", "stop", "apache2").Run()

	out, _ := exec.Command("bash", "-c",
		"ps aux | grep '[x]ray' | grep -v 'systemd' | awk '{print $2}'").Output()
	pids := strings.Fields(string(out))
	for _, pid := range pids {
		exec.Command("kill", "-9", pid).Run()
	}

	return nil
}

// reloadAndRestartXray executa a sequência completa de reload e restart do systemd.
func reloadAndRestartXray() error {
	// Remover unit instalada pelo script oficial em /lib/ se houver conflito
	os.Remove("/lib/systemd/system/xray.service")

	steps := [][]string{
		{"systemctl", "daemon-reexec"},
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", XrayService},
		{"systemctl", "restart", XrayService},
	}

	for _, args := range steps {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("erro em '%s': %v\n%s",
				strings.Join(args, " "), err, string(out))
		}
	}
	return nil
}

// ── Desativar ─────────────────────────────────────────────────────────────────

// DisableXray para e desabilita o serviço Xray.
func DisableXray() error {
	exec.Command("systemctl", "stop", XrayService).Run()
	return exec.Command("systemctl", "disable", XrayService).Run()
}

// ── Funções de edição ─────────────────────────────────────────────────────────

// UpdateXrayHost atualiza o host, regenera o certificado e reinicia o serviço.
func UpdateXrayHost(host string) error {
	os.MkdirAll(XrayCertDir, 0755)

	genCert := exec.Command("openssl", "req", "-x509",
		"-newkey", "rsa:4096",
		"-keyout", XrayKeyFile,
		"-out", XrayCertFile,
		"-days", "365",
		"-nodes",
		"-subj", "/CN="+host)
	if out, err := genCert.CombinedOutput(); err != nil {
		return fmt.Errorf("erro ao regenerar certificado: %v\n%s", err, string(out))
	}

	if err := fixCertPermissions(); err != nil {
		return err
	}

	if err := updateXrayConfigField(func(cfg *XrayConfig) {
		for i := range cfg.Inbounds {
			cfg.Inbounds[i].StreamSettings.TLS.Certificates = []XrayCert{
				{CertificateFile: XrayCertFile, KeyFile: XrayKeyFile},
			}
		}
	}); err != nil {
		return err
	}

	return restartXray()
}

// UpdateXraySNI atualiza o campo serverName (SNI) em todos os inbounds e reinicia.
func UpdateXraySNI(sni string) error {
	if sni == "" {
		return fmt.Errorf("SNI não pode ser vazio")
	}

	if err := updateXrayConfigField(func(cfg *XrayConfig) {
		for i := range cfg.Inbounds {
			cfg.Inbounds[i].StreamSettings.TLS.ServerName = sni
		}
	}); err != nil {
		return fmt.Errorf("erro ao atualizar SNI no config.json: %v", err)
	}

	return restartXray()
}

// UpdateXrayBugHost atualiza o host XHTTP (Bug Host / CDN) em todos os inbounds e reinicia.
func UpdateXrayBugHost(bugHost string) error {
	if bugHost == "" {
		return fmt.Errorf("Bug Host não pode ser vazio")
	}

	if err := updateXrayConfigField(func(cfg *XrayConfig) {
		for i := range cfg.Inbounds {
			cfg.Inbounds[i].StreamSettings.XHTTP.Host = bugHost
		}
	}); err != nil {
		return fmt.Errorf("erro ao atualizar Bug Host no config.json: %v", err)
	}

	return restartXray()
}

// ── Gerenciamento de usuários ─────────────────────────────────────────────────

// GenerateUUID gera um novo UUID v4 usando crypto/rand.
// Isso evita dependência do binário 'xray' que pode não estar instalado.
func GenerateUUID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "00000000-0000-0000-0000-000000000000"
	}

	// UUID v4: b[6] deve ser 0100xxxx e b[8] deve ser 10xxxxxx
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// AddXrayUser adiciona um novo cliente a todos os inbounds da configuração.
func AddXrayUser(username, uuid string) error {
	if err := updateXrayConfigField(func(cfg *XrayConfig) {
		for i := range cfg.Inbounds {
			cfg.Inbounds[i].Settings.Clients = append(
				cfg.Inbounds[i].Settings.Clients,
				XrayClient{ID: uuid, Email: username},
			)
		}
	}); err != nil {
		return err
	}
	return restartXray()
}

// ── Helpers internos ──────────────────────────────────────────────────────────

// EnsureXrayLogConfig verifica se o config.json tem os caminhos de log corretos.
// Se houver mudanças, reinicia o serviço.
func EnsureXrayLogConfig() error {
	changed := false
	err := updateXrayConfigField(func(cfg *XrayConfig) {
		if cfg.Log.Access != "/var/log/xray/access.log" || cfg.Log.Error != "/var/log/xray/error.log" {
			cfg.Log.Access = "/var/log/xray/access.log"
			cfg.Log.Error = "/var/log/xray/error.log"
			changed = true
		}
		if cfg.Log.LogLevel == "" {
			cfg.Log.LogLevel = "warning"
			changed = true
		}
	})
	if err != nil {
		return err
	}
	if changed {
		return restartXray()
	}
	return nil
}

// updateXrayConfigField lê o config.json, aplica a função mutadora e salva.
func updateXrayConfigField(mutate func(*XrayConfig)) error {
	data, err := ioutil.ReadFile(XrayConfigPath)
	if err != nil {
		return fmt.Errorf("erro ao ler %s: %v", XrayConfigPath, err)
	}

	var config XrayConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("erro ao parsear config.json: %v", err)
	}

	mutate(&config)

	newData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar config.json: %v", err)
	}

	return ioutil.WriteFile(XrayConfigPath, newData, 0644)
}

// restartXray valida a config e reinicia o serviço via systemctl.
func restartXray() error {
	if err := ValidateXrayConfig(); err != nil {
		return fmt.Errorf("config inválida, serviço não reiniciado: %v", err)
	}
	if err := exec.Command("systemctl", "restart", XrayService).Run(); err != nil {
		return fmt.Errorf("erro ao reiniciar %s: %v", XrayService, err)
	}
	return nil
}

// xlog é um helper de log simples para as funções do pacote system.
func xlog(msg string) {
	fmt.Println(msg)
}

// ── Gerenciamento de clientes (suspend / activate / delete) ───────────────────

// RemoveXrayUser remove o cliente do config.json pelo email (username).
func RemoveXrayUser(username string) error {
	return updateXrayConfigField(func(cfg *XrayConfig) {
		for i := range cfg.Inbounds {
			var kept []XrayClient
			for _, c := range cfg.Inbounds[i].Settings.Clients {
				if c.Email != username {
					kept = append(kept, c)
				}
			}
			cfg.Inbounds[i].Settings.Clients = kept
		}
	})
}

// SuspendXrayUser remove o cliente do config.json e reinicia o XRAY.
func SuspendXrayUser(username string) error {
	if err := RemoveXrayUser(username); err != nil {
		return fmt.Errorf("falha ao remover cliente do config: %v", err)
	}
	return RestartXrayService()
}

// ActivateXrayUser reinsere o cliente no config.json e reinicia o XRAY.
// Remove antes qualquer entrada duplicada para garantir idempotência.
func ActivateXrayUser(username, uuid string) error {
	// Remove duplicatas antes de inserir
	_ = RemoveXrayUser(username)

	if err := updateXrayConfigField(func(cfg *XrayConfig) {
		for i := range cfg.Inbounds {
			cfg.Inbounds[i].Settings.Clients = append(
				cfg.Inbounds[i].Settings.Clients,
				XrayClient{ID: uuid, Email: username},
			)
		}
	}); err != nil {
		return fmt.Errorf("falha ao adicionar usuário ao config: %v", err)
	}
	return RestartXrayService()
}

func RestartXrayService() error {
	return restartXray()
}

// ── Monitoramento de conexões ─────────────────────────────────────────────────

// CheckAndFixXrayLogs garante que os arquivos de log existam e tenham permissões corretas.
// Retorna erro se não conseguir criar/ajustar os arquivos.
func CheckAndFixXrayLogs() error {
	const logDir = "/var/log/xray"
	const accessLog = "/var/log/xray/access.log"
	const errorLog = "/var/log/xray/error.log"

	// 1. Garantir diretório
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("erro ao criar diretório de logs: %v", err)
	}

	// 2. Garantir arquivos e permissões
	files := []string{accessLog, errorLog}
	for _, f := range files {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			if err := ioutil.WriteFile(f, []byte(""), 0644); err != nil {
				return fmt.Errorf("erro ao criar arquivo %s: %v", f, err)
			}
		}
		if err := os.Chmod(f, 0644); err != nil {
			return fmt.Errorf("erro ao ajustar permissões de %s: %v", f, err)
		}
	}

	// 3. Garantir que o owner seja root (visto que o serviço roda como root agora)
	exec.Command("chown", "-R", "root:root", logDir).Run()

	return nil
}

// EnforceXrayLimit verifica e tenta derrubar conexões excedentes do Xray (via iptables)
func EnforceXrayLimit(username string, uuid string, limit int) {
	if limit <= 0 {
		return
	}

	details := GetXrayUserOnlineDetails(username, uuid)
	if len(details) > limit {
		// Se ultrapassou o limite de IPs, bloqueamos os IPs excedentes temporariamente
		// ou apenas derrubamos as conexões no kernel (TCP reset).
		// Uma forma simples é usar o tcpkill ou apenas o ss -K (se disponível no kernel)
		for i := limit; i < len(details); i++ {
			ip := details[i].IP
			// Tenta derrubar a conexão no kernel (requer kernel >= 4.9 e ss moderno)
			exec.Command("bash", "-c", fmt.Sprintf("ss -K dst %s", ip)).Run()
		}
	}
}

// GetXrayUserOnlineDetails retorna detalhes das conexões ativas do Xray.
func GetXrayUserOnlineDetails(identifiers ...string) []OnlineUser {
	if len(identifiers) == 0 {
		return nil
	}

	_ = CheckAndFixXrayLogs()

	// Tenta ler as últimas 3000 linhas
	cmd := exec.Command("bash", "-c", "tail -n 3000 /var/log/xray/access.log 2>/dev/null")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil // Sem logs, sem detecção detalhada por enquanto
	}

	lines := strings.Split(string(out), "\n")
	now := time.Now()

	// Map para guardar a conexão mais recente de cada IP encontrado no log para o usuário
	latestLogPerIP := make(map[string]time.Time)

	for _, line := range lines {
		if line == "" || !strings.Contains(line, "accepted") {
			continue
		}

		found := false
		for _, id := range identifiers {
			if id != "" && strings.Contains(line, id) {
				found = true
				break
			}
		}
		if !found {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		// Timestamp do log
		logTimeStr := parts[0] + " " + parts[1]
		logTime, err := time.ParseInLocation("2006/01/02 15:04:05", logTimeStr, time.Local)
		if err != nil {
			logTime, err = time.ParseInLocation("2006-01-02 15:04:05", logTimeStr, time.Local)
		}
		if err != nil {
			continue
		}

		// Extrair IP:Porta da origem
		var fullAddr string
		for _, p := range parts {
			p = strings.Trim(p, "\",")
			if strings.Contains(p, ":") && (strings.Count(p, ".") == 3 || strings.Contains(p, "[")) {
				fullAddr = p
				break
			}
		}

		if fullAddr != "" {
			latestLogPerIP[fullAddr] = logTime
		}
	}

	var details []OnlineUser
	for fullAddr, startTime := range latestLogPerIP {
		// 1. Extrair IP para busca no ss
		ipOnly := strings.Split(fullAddr, ":")[0]
		ipOnly = strings.Trim(ipOnly, "[]")

		// 2. VERIFICAÇÃO EM TEMPO REAL: o socket ainda está no kernel?
		// Procuramos pelo IP de origem nas conexões ESTABLISHED na porta 443.
		// O formato no ss pode ser IPv4 ou IPv6 mapeado.
		cmdCheck := exec.Command("bash", "-c", fmt.Sprintf("ss -tnp 'sport = :443' state established 2>/dev/null | grep '%s'", ipOnly))
		if err := cmdCheck.Run(); err == nil {
			// Socket ainda vivo!
			details = append(details, OnlineUser{
				IP:       ipOnly,
				Duration: now.Sub(startTime),
			})
		}
	}

	return details
}

// GetXrayUserOnlineIPs retorna uma lista de IPs únicos conectados recentemente.
func GetXrayUserOnlineIPs(identifiers ...string) []string {
	details := GetXrayUserOnlineDetails(identifiers...)
	var ips []string
	for _, d := range details {
		ips = append(ips, d.IP)
	}
	return ips
}

// getXrayUserOnlineIPsFallback usa journalctl quando o access.log falha.
func getXrayUserOnlineIPsFallback(identifiers ...string) []string {
	// A lógica detalhada acima já é bem resiliente.
	// Fallback simples para compatibilidade de assinatura se necessário.
	return nil
}

// GetXrayUserOnlineCount conta conexões reais em tempo real.
func GetXrayUserOnlineCount(identifiers ...string) int {
	return len(GetXrayUserOnlineDetails(identifiers...))
}

// GetXrayTotalConnections retorna o total de conexões reais no kernel na porta 443.
func GetXrayTotalConnections() int {
	// Buscamos conexões ESTABLISHED na porta 443 (Xray)
	cmd := exec.Command("bash", "-c", "ss -tnp 'sport = :443' state established 2>/dev/null | grep -v 'Local Address' | awk '{print $5}' | cut -d: -f1 | sort -u | wc -l")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n
}
