package system

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

const (
	V2RayConfigPath = "/etc/v2ray/config.json"
	V2RayService    = "v2ray.service"
)

type V2RayConfig struct {
	Log struct {
		LogLevel string `json:"loglevel"`
		Access   string `json:"access"`
		Error    string `json:"error"`
	} `json:"log"`
	Inbounds []V2RayInbound `json:"inbounds"`
	Outbounds []struct {
		Protocol string `json:"protocol"`
		Settings struct{} `json:"settings"`
	} `json:"outbounds"`
}

type V2RayInbound struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Settings struct {
		Clients []V2RayClient `json:"clients"`
	} `json:"settings"`
	StreamSettings struct {
		Network  string `json:"network"`
		Security string `json:"security"`
	} `json:"streamSettings"`
}

type V2RayClient struct {
	ID    string `json:"id"`
	Level int    `json:"level"`
	AlterID int  `json:"alterId"`
	Email string `json:"email"`
}

// GetV2RayStatus retorna o status do serviço V2Ray
func GetV2RayStatus() (string, bool) {
	cmd := exec.Command("systemctl", "is-active", V2RayService)
	output, _ := cmd.Output()
	status := strings.TrimSpace(string(output))
	if status == "active" {
		return "ATIVO", true
	}
	return "INATIVO", false
}

// InstallV2Ray instala o V2Ray via script oficial
func InstallV2Ray(port int) error {
	// 1. Instalação via script oficial
	fmt.Println("⏳ Instalando V2Ray via script oficial...")
	installCmd := "curl -Ls https://install.direct/go.sh | bash"
	if err := exec.Command("bash", "-c", installCmd).Run(); err != nil {
		return fmt.Errorf("falha ao instalar V2Ray: %v", err)
	}

	// 2. Criar configuração inicial básica (VMess)
	config := V2RayConfig{}
	config.Log.LogLevel = "warning"
	config.Log.Access = "/var/log/v2ray/access.log"
	config.Log.Error = "/var/log/v2ray/error.log"

	inbound := V2RayInbound{
		Port:     port,
		Protocol: "vmess",
	}
	inbound.StreamSettings.Network = "tcp"
	inbound.StreamSettings.Security = "none"
	
	config.Inbounds = append(config.Inbounds, inbound)
	
	outbound := struct {
		Protocol string `json:"protocol"`
		Settings struct{} `json:"settings"`
	}{Protocol: "freedom"}
	config.Outbounds = append(config.Outbounds, outbound)

	return SaveV2RayConfig(&config)
}

// SaveV2RayConfig salva a struct no config.json
func SaveV2RayConfig(cfg *V2RayConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	os.MkdirAll("/etc/v2ray", 0755)
	if err := ioutil.WriteFile(V2RayConfigPath, data, 0644); err != nil {
		return err
	}
	return RestartV2Ray()
}

// LoadV2RayConfig carrega do config.json
func LoadV2RayConfig() (*V2RayConfig, error) {
	data, err := ioutil.ReadFile(V2RayConfigPath)
	if err != nil {
		return nil, err
	}
	var cfg V2RayConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// AddV2RayUser adiciona um cliente VMess
func AddV2RayUser(email, uuid string) error {
	cfg, err := LoadV2RayConfig()
	if err != nil {
		return err
	}

	for i := range cfg.Inbounds {
		if cfg.Inbounds[i].Protocol == "vmess" {
			cfg.Inbounds[i].Settings.Clients = append(cfg.Inbounds[i].Settings.Clients, V2RayClient{
				ID:      uuid,
				Level:   1,
				AlterID: 0,
				Email:   email,
			})
		}
	}

	return SaveV2RayConfig(cfg)
}

// RemoveV2RayUser remove um cliente pelo email
func RemoveV2RayUser(email string) error {
	cfg, err := LoadV2RayConfig()
	if err != nil {
		return err
	}

	for i := range cfg.Inbounds {
		var kept []V2RayClient
		for _, c := range cfg.Inbounds[i].Settings.Clients {
			if c.Email != email {
				kept = append(kept, c)
			}
		}
		cfg.Inbounds[i].Settings.Clients = kept
	}

	return SaveV2RayConfig(cfg)
}

// RestartV2Ray reinicia o serviço
func RestartV2Ray() error {
	return exec.Command("systemctl", "restart", V2RayService).Run()
}

// GenerateVMessLink gera o link vmess://
func GenerateVMessLink(email, uuid, ip string, port int) string {
	// JSON do link VMess
	linkMap := map[string]interface{}{
		"v":    "2",
		"ps":   "V2Ray_" + email,
		"add":  ip,
		"port": port,
		"id":   uuid,
		"aid":  "0",
		"net":  "tcp",
		"type": "none",
		"host": "",
		"path": "",
		"tls":  "",
	}
	
	data, _ := json.Marshal(linkMap)
	encoded := base64.StdEncoding.EncodeToString(data)
	return "vmess://" + encoded
}
