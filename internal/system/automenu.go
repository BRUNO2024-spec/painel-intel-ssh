package system

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

const (
	BashrcPath = "/root/.bashrc"
	AutoMenuLine = "[[ -f /usr/local/bin/painel ]] && /usr/local/bin/painel"
)

// GetAutoMenuStatus verifica se a linha de inicialização está no .bashrc
func GetAutoMenuStatus() bool {
	content, err := ioutil.ReadFile(BashrcPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(content), AutoMenuLine)
}

// EnableAutoMenu adiciona a linha de inicialização no .bashrc
func EnableAutoMenu() error {
	if GetAutoMenuStatus() {
		return nil // Já está habilitado
	}

	f, err := os.OpenFile(BashrcPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("erro ao abrir .bashrc: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString("\n" + AutoMenuLine + "\n"); err != nil {
		return fmt.Errorf("erro ao escrever no .bashrc: %v", err)
	}

	return nil
}

// DisableAutoMenu remove a linha de inicialização do .bashrc
func DisableAutoMenu() error {
	content, err := ioutil.ReadFile(BashrcPath)
	if err != nil {
		return fmt.Errorf("erro ao ler .bashrc: %v", err)
	}

	newContent := strings.ReplaceAll(string(content), "\n"+AutoMenuLine+"\n", "\n")
	newContent = strings.ReplaceAll(newContent, AutoMenuLine, "")
	newContent = strings.TrimSpace(newContent) + "\n"

	if err := ioutil.WriteFile(BashrcPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("erro ao escrever no .bashrc: %v", err)
	}

	return nil
}
