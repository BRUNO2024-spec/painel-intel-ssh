package system

import (
	"fmt"
)

// InjectXrayResponse modifica o config.json do Xray para injetar informações nos headers HTTP.
// Note: O Xray não suporta injeção dinâmica de headers baseada no usuário logado no inbound XHTTP
// de forma nativa sem um backend intermediário.
// No entanto, podemos injetar uma mensagem global ou configurar o Xray para encaminhar para um
// script que faça a autenticação e retorne os headers.
// Para esta implementação, focaremos na configuração que permite exibir informações.

func SetupXrayLogInjection(enabled bool) error {
	return updateXrayConfigField(func(cfg *XrayConfig) {
		for i := range cfg.Inbounds {
			if cfg.Inbounds[i].StreamSettings.Network == "xhttp" {
				// Ativamos logs detalhados para o checkeruser capturar
				cfg.Log.LogLevel = "info"
				if enabled {
					// Podemos adicionar um header customizado na resposta XHTTP
					// mas o Xray-core nativo tem limitações para headers dinâmicos por usuário.
					// A melhor forma de exibição em apps VPN para Xray é via log de conexão (access.log).
				}
			}
		}
	})
}

// GetXrayUserDataResponse retorna uma string formatada para ser injetada em respostas (se houver proxy reverso)
func GetXrayUserDataResponse(username string) string {
	data, err := GetUserData(username)
	if err != nil || data == nil {
		return ""
	}

	resp := "HTTP/1.1 200 OK\r\n"
	resp += fmt.Sprintf("X-SSH-User: %s\r\n", data.Usuario)
	resp += fmt.Sprintf("X-SSH-Pass: %s\r\n", data.Senha)
	resp += fmt.Sprintf("X-SSH-Days: %d\r\n", data.ExpiraEmDias)
	resp += fmt.Sprintf("X-SSH-Limit: %d\r\n", data.LimiteConexoes)
	resp += "\r\n"

	return resp
}
