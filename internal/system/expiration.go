package system

import (
	"fmt"
	"os/exec"
	"painel-ssh/internal/db"
	"strings"
	"time"
)

// CheckAllExpirations verifica todos os usuários (SSH e Xray) e bloqueia os que expiraram.
func CheckAllExpirations() {
	now := time.Now().UTC()
	// 1. Verificar usuários SSH
	users, err := db.GetUsers()
	if err == nil {
		for _, u := range users {
			if !u.ExpirationDate.IsZero() && now.After(u.ExpirationDate.UTC()) {
				// Usuário expirou
				if u.Type == "teste" {
					fmt.Printf("🗑️ Removendo usuário TESTE expirado: %s (Venceu em: %s)\n",
						u.Username, u.ExpirationDate.UTC().Format("02/01/2006 15:04"))
					RemoveSSHUser(u.Username)
					_ = db.DeleteUser(u.Username)
					// Também remover do Xray se existir
					_ = SuspendXrayUser(u.Username)
					_ = db.DeleteXrayUser(u.Username)
				} else if IsUserActive(u.Username) {
					fmt.Printf("🔒 Bloqueando usuário PREMIUM expirado: %s (Venceu em: %s)\n",
						u.Username, u.ExpirationDate.UTC().Format("02/01/2006"))
					BlockUser(u.Username)
				}
			}
		}
	}

	// 2. Verificar usuários Xray (UUID)
	xrayUsers, err := db.GetXrayUsers()
	if err == nil {
		for _, u := range xrayUsers {
			if !u.ExpiresAt.IsZero() && now.After(u.ExpiresAt.UTC()) {
				// Usuário expirou
				if u.Status == "active" {
					fmt.Printf("🔒 Suspendendo usuário Xray expirado: %s (Venceu em: %s)\n",
						u.Username, u.ExpiresAt.UTC().Format("02/01/2006"))
					EnforceXrayExpiration(u.Username)
				}
			}
		}
	}

	// 3. Verificar usuários OpenVPN (se implementado com expiração no futuro)
	// Atualmente OpenVPN usa os mesmos usuários do sistema ou perfis .ovpn.
	// Se for via perfil .ovpn no /root, a remoção é manual, mas se usarmos
	// os mesmos usuários do sistema, o bloqueio do SSH já impede o login se houver auth.
}

// IsUserActive verifica se a conta Linux está ativa (não bloqueada).
func IsUserActive(username string) bool {
	// shadow file: ! ou * no início do campo de senha indica conta bloqueada
	cmd := exec.Command("bash", "-c", fmt.Sprintf("grep '^%s:' /etc/shadow | cut -d: -f2", username))
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	passField := strings.TrimSpace(string(out))
	if strings.HasPrefix(passField, "!") || strings.HasPrefix(passField, "*") {
		return false
	}
	return true
}

// BlockUser bloqueia a conta Linux e mata as conexões.
func BlockUser(username string) {
	// Bloquear conta (passwd -l)
	exec.Command("passwd", "-l", username).Run()

	// Matar todos os processos do usuário (SSH, etc)
	exec.Command("pkill", "-u", username).Run()
}

// UnblockUser desbloqueia a conta Linux.
func UnblockUser(username string) {
	// Desbloquear conta (passwd -u)
	exec.Command("passwd", "-u", username).Run()
}

// EnforceXrayExpiration desativa um usuário no config.json do Xray e atualiza o banco.
func EnforceXrayExpiration(username string) {
	// 1. Remover do config.json e reiniciar
	_ = SuspendXrayUser(username)

	// 2. Atualizar status no banco para 'expired'
	_ = db.UpdateXrayUserStatus(username, "expired")
}
