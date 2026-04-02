package system

import (
	"math"
	"painel-ssh/internal/db"
	"time"
)

// UserData representa o esquema de resposta para a consulta de usuário
type UserData struct {
	Usuario        string `json:"usuario"`
	Senha          string `json:"senha"`
	Tipo           string `json:"tipo"` // "premium" ou "teste"
	ExpiraEmDias   int    `json:"expira_em_dias"`
	ExpiraEmHoras  int    `json:"expira_em_horas"`
	ConexoesAtivas int    `json:"conexoes_ativas"`
	LimiteConexoes int    `json:"limite_conexoes"`
	Status         string `json:"status"`
}

// GetUserData coleta os dados de um usuário SSH em tempo real
func GetUserData(username string) (*UserData, error) {
	// 1. Buscar usuário no banco
	user, err := db.GetUserByUsername(username)
	if err != nil || user == nil {
		return nil, err
	}

	// 2. Calcular tempo restante
	daysRemaining := 0
	hoursRemaining := 0
	if !user.ExpirationDate.IsZero() {
		diff := time.Until(user.ExpirationDate)
		daysRemaining = int(math.Ceil(diff.Hours() / 24))
		hoursRemaining = int(math.Ceil(diff.Hours()))
		if daysRemaining < 0 {
			daysRemaining = 0
		}
		if hoursRemaining < 0 {
			hoursRemaining = 0
		}
	} else {
		daysRemaining = 999  // Sem expiração
		hoursRemaining = 999 // Sem expiração
	}

	// 3. Obter conexões ativas
	conns, _ := GetSSHOnlineUsers(user.Username)

	// 4. Definir status
	status := "offline"
	if conns > 0 {
		status = "online"
	}

	userType := user.Type
	if userType == "" {
		userType = "premium"
	}

	return &UserData{
		Usuario:        user.Username,
		Senha:          user.Password,
		Tipo:           userType,
		ExpiraEmDias:   daysRemaining,
		ExpiraEmHoras:  hoursRemaining,
		ConexoesAtivas: conns,
		LimiteConexoes: user.Limit,
		Status:         status,
	}, nil
}
