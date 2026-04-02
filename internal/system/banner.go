package system

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	BannerConfigPath = "/etc/banner_config.json"
	NewBannerPath    = "/etc/banner"
	SSHDPath         = "/etc/ssh/sshd_config"
)

type BannerConfig struct {
	Status  string `json:"status"` // "active" or "inactive"
	Message string `json:"message"`
	Color   string `json:"color"`
	Style   string `json:"style"` // "normal", "bold", "ascii"
}

func LoadBannerConfig() (*BannerConfig, error) {
	if _, err := os.Stat(BannerConfigPath); os.IsNotExist(err) {
		return &BannerConfig{Status: "inactive", Message: "Bem-vindo ao servidor SSH", Color: "0", Style: "normal"}, nil
	}
	data, err := os.ReadFile(BannerConfigPath)
	if err != nil {
		return nil, err
	}
	var config BannerConfig
	err = json.Unmarshal(data, &config)
	return &config, err
}

func SaveBannerConfig(config *BannerConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(BannerConfigPath, data, 0644)
}

func EnableBanner() error {
	// 1. Limpeza RADICAL de resíduos do sistema antigo que podem estar travando o banner
	exec.Command("rm", "-f", "/etc/update-motd.d/99-panel-banner").Run()
	exec.Command("rm", "-f", "/etc/profile.d/panel_banner.sh").Run()

	// Limpar bash.bashrc de QUALQUER injeção antiga
	bashrcPath := "/etc/bash.bashrc"
	if input, err := os.ReadFile(bashrcPath); err == nil {
		lines := strings.Split(string(input), "\n")
		var newLines []string
		for _, line := range lines {
			if !strings.Contains(line, "99-panel-banner") && !strings.Contains(line, "panel_banner.sh") {
				newLines = append(newLines, line)
			}
		}
		os.WriteFile(bashrcPath, []byte(strings.Join(newLines, "\n")), 0644)
	}

	config, _ := LoadBannerConfig()
	config.Status = "active"
	if err := SaveBannerConfig(config); err != nil {
		return err
	}
	return ApplyBanner()
}

func DisableBanner() error {
	config, _ := LoadBannerConfig()
	config.Status = "inactive"
	if err := SaveBannerConfig(config); err != nil {
		return err
	}

	// 1. Limpeza total de resíduos do sistema antigo
	os.Remove("/etc/update-motd.d/99-panel-banner")
	os.Remove("/etc/profile.d/panel_banner.sh")
	os.Remove("/etc/ssh/banner") // Remove o antigo se existir

	// Limpar injeção no bash.bashrc
	bashrcPath := "/etc/bash.bashrc"
	if input, err := os.ReadFile(bashrcPath); err == nil {
		content := string(input)
		// Remove referências antigas
		lines := strings.Split(content, "\n")
		var newLines []string
		for _, line := range lines {
			if !strings.Contains(line, "99-panel-banner") && !strings.Contains(line, "panel_banner.sh") {
				newLines = append(newLines, line)
			}
		}
		os.WriteFile(bashrcPath, []byte(strings.Join(newLines, "\n")), 0644)
	}

	// 2. Remover do sshd_config
	input, err := os.ReadFile(SSHDPath)
	if err == nil {
		lines := strings.Split(string(input), "\n")
		var newLines []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			// Remove QUALQUER linha de Banner para resetar
			if !strings.HasPrefix(trimmed, "Banner") {
				newLines = append(newLines, line)
			}
		}
		os.WriteFile(SSHDPath, []byte(strings.Join(newLines, "\n")), 0644)
	}

	os.Remove(NewBannerPath)
	exec.Command("systemctl", "restart", "ssh").Run()
	return nil
}

func SetBannerMessage(msg string) error {
	config, _ := LoadBannerConfig()
	config.Message = msg
	if err := SaveBannerConfig(config); err != nil {
		return err
	}
	return ApplyBanner()
}

func SetBannerColor(color string) error {
	config, _ := LoadBannerConfig()
	config.Color = color
	if err := SaveBannerConfig(config); err != nil {
		return err
	}
	return ApplyBanner()
}

func SetBannerStyle(style string) error {
	config, _ := LoadBannerConfig()
	config.Style = style
	if err := SaveBannerConfig(config); err != nil {
		return err
	}
	return ApplyBanner()
}

func ApplyBanner() error {
	config, err := LoadBannerConfig()
	if err != nil {
		return err
	}

	if config.Status != "active" {
		return nil
	}

	// 1. Limpeza RADICAL de resíduos do sistema antigo ANTES de aplicar o novo
	exec.Command("rm", "-f", "/etc/update-motd.d/99-panel-banner").Run()
	exec.Command("rm", "-f", "/etc/profile.d/panel_banner.sh").Run()

	// Limpar bash.bashrc de QUALQUER injeção antiga
	bashrcPath := "/etc/bash.bashrc"
	if input, err := os.ReadFile(bashrcPath); err == nil {
		lines := strings.Split(string(input), "\n")
		var newLines []string
		for _, line := range lines {
			if !strings.Contains(line, "99-panel-banner") && !strings.Contains(line, "panel_banner.sh") {
				newLines = append(newLines, line)
			}
		}
		os.WriteFile(bashrcPath, []byte(strings.Join(newLines, "\n")), 0644)
	}

	var bannerText string
	message := config.Message

	// Cores ANSI compatíveis com SSH (Escape Real)
	colorCode := ""
	switch config.Color {
	case "1": // Verde
		colorCode = "\033[1;32m"
	case "2": // Amarelo
		colorCode = "\033[1;33m"
	case "3": // Azul
		colorCode = "\033[1;34m"
	case "4": // Vermelho
		colorCode = "\033[1;31m"
	default:
		colorCode = "\033[1;37m" // Branco Negrito
	}

	// Estilo ASCII (FIGLET) ou Normal
	if config.Style == "ascii" {
		out, err := exec.Command("figlet", "-f", "standard", message).Output()
		if err == nil {
			bannerText = string(out)
		} else {
			bannerText = message
		}
	} else {
		bannerText = message
	}

	// Montagem do Banner Estilo SSHPLUS (Centralizado e Colorido)
	finalBanner := "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"

	// Aplicar cor em cada linha do banner para garantir exibição
	lines := strings.Split(bannerText, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Centralização básica (aproximada para 60 colunas)
		padding := (60 - len(line)) / 2
		if padding < 0 {
			padding = 0
		}
		finalBanner += fmt.Sprintf("%s%s%s%s\n", strings.Repeat(" ", padding), colorCode, line, "\033[0m")
	}

	finalBanner += "\033[0m━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
	finalBanner += fmt.Sprintf("%s       BEM-VINDO AO SERVIDOR SSH       %s\n", colorCode, "\033[0m")
	finalBanner += "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"

	// 2. Salvar o arquivo de banner estático para o SSHD
	if err := os.WriteFile(NewBannerPath, []byte(finalBanner), 0644); err != nil {
		return err
	}

	// 3. Aplicar no sshd_config
	input, err := os.ReadFile(SSHDPath)
	if err != nil {
		return err
	}

	configLines := strings.Split(string(input), "\n")
	var newConfigLines []string
	found := false
	for _, line := range configLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Banner") {
			newConfigLines = append(newConfigLines, "Banner "+NewBannerPath)
			found = true
		} else {
			newConfigLines = append(newConfigLines, line)
		}
	}

	if !found {
		newConfigLines = append(newConfigLines, "Banner "+NewBannerPath)
	}

	if err := os.WriteFile(SSHDPath, []byte(strings.Join(newConfigLines, "\n")), 0644); err != nil {
		return err
	}

	// 4. Reiniciar SSH para aplicar
	exec.Command("systemctl", "restart", "ssh").Run()
	return nil
}

// Old functions removed or kept for compatibility if needed
// For this task, we'll replace the old SetupSSHBanner logic in the menu.
func GetNewBannerStatus() string {
	config, _ := LoadBannerConfig()
	if config.Status == "active" {
		return "\033[32mATIVO\033[0m"
	}
	return "\033[31mINATIVO\033[0m"
}
