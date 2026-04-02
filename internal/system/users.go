package system

import (
	"fmt"
	"math/rand"
	"os/exec"
	"painel-ssh/internal/db"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GenerateRandomUser gera um nome de usuário aleatório para teste
func GenerateRandomUser() string {
	prefixes := []string{"teste", "trial", "user", "ssh"}
	rand.Seed(time.Now().UnixNano())
	prefix := prefixes[rand.Intn(len(prefixes))]
	num := rand.Intn(900) + 100 // 100-999
	return fmt.Sprintf("%s%d", prefix, num)
}

// GenerateRandomPassword gera uma senha aleatória segura
func GenerateRandomPassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	rand.Seed(time.Now().UnixNano())
	length := 8
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// GetUserType retorna o tipo do usuário (premium/teste)
func GetUserType(username string) string {
	user, err := db.GetUserByUsername(username)
	if err != nil || user == nil {
		return "premium"
	}
	return user.Type
}

// CreateSSHUser creates a new user in the Linux system
func CreateSSHUser(username, password string, expirationDate time.Time) error {
	// 1. Check if user already exists in Linux system to avoid exit status 9
	checkCmd := exec.Command("id", username)
	if err := checkCmd.Run(); err == nil {
		// User already exists in system, just update password and expiration
		// This is safer than failing with exit status 9
	} else {
		// 2. Create user with no shell (for SSH only or as requested)
		cmd := exec.Command("useradd", "-m", "-s", "/bin/bash", username)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create user: %v", err)
		}
	}

	// 3. Set password
	cmd := exec.Command("chpasswd")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("%s:%s", username, password))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set password: %v", err)
	}

	// 4. Set expiration and aging parameters
	// Reset password aging to prevent "expired password" or "must change password"
	exec.Command("chage", "-m", "0", "-M", "99999", "-I", "-1", username).Run()

	if !expirationDate.IsZero() {
		// No Linux, 'chage -E YYYY-MM-DD' expira o usuário no INÍCIO do dia (00:00).
		// Se criarmos um teste de 2 horas hoje, ele seria bloqueado IMEDIATAMENTE.
		// Para usuários com vencimento curto (menos de 24h), definimos 'Never' no SO
		// e deixamos o monitor do painel (que roda a cada minuto) fazer a limpeza exata.
		if time.Until(expirationDate) < 24*time.Hour {
			// Define como 'Nunca' no SO para não bloquear prematuramente no mesmo dia
			cmd = exec.Command("chage", "-E", "-1", username)
		} else {
			// Para premium/longo prazo, define o dia seguinte como margem de segurança
			expStr := expirationDate.AddDate(0, 0, 1).Format("2006-01-02")
			cmd = exec.Command("chage", "-E", expStr, username)
		}

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to set expiration: %v", err)
		}
	}

	return nil
}

// RemoveSSHUser removes a user from the Linux system
func RemoveSSHUser(username string) error {
	cmd := exec.Command("userdel", "-r", "-f", username)
	return cmd.Run()
}

// SetUserPassword changes the password for an existing user
func SetUserPassword(username, password string) error {
	cmd := exec.Command("chpasswd")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("%s:%s", username, password))
	return cmd.Run()
}

// SetUserExpiration updates the expiration date for a user
func SetUserExpiration(username string, expirationDate time.Time) error {
	// Garantir que o envelhecimento da senha não bloqueie o usuário
	exec.Command("chage", "-m", "0", "-M", "99999", "-I", "-1", username).Run()

	// Se a expiração for menor que 24h (teste ou renovação curta), removemos a expiração do SO
	// e deixamos o monitor do painel cuidar da limpeza exata.
	if time.Until(expirationDate) < 24*time.Hour {
		cmd := exec.Command("chage", "-E", "-1", username)
		return cmd.Run()
	}
	// Para prazos maiores, definimos o dia seguinte como margem de segurança no SO
	expStr := expirationDate.AddDate(0, 0, 1).Format("2006-01-02")
	cmd := exec.Command("chage", "-E", expStr, username)
	return cmd.Run()
}

// SetConnectionLimit sets the maximum number of simultaneous connections
// This implementation appends/updates /etc/security/limits.conf
func SetConnectionLimit(username string, limit int) error {
	// A more robust way would be to use a separate file in /etc/security/limits.d/
	filename := fmt.Sprintf("/etc/security/limits.d/%s.conf", username)
	content := fmt.Sprintf("%s hard maxlogins %d\n%s soft maxlogins %d\n", username, limit, username, limit)

	cmd := exec.Command("bash", "-c", fmt.Sprintf("echo '%s' > %s", content, filename))
	return cmd.Run()
}

// SSHCache armazena o cache das conexões para evitar chamadas excessivas ao SO
var (
	sshCache      []string
	sshCacheTime  time.Time
	sshCacheMutex sync.Mutex
)

// OnlineUser representa uma conexão ativa com IP, duração e terminal
type OnlineUser struct {
	IP       string
	Duration time.Duration
	Terminal string
}

// GetSSHDListeningPorts retorna as portas que o sshd está ouvindo.
func GetSSHDListeningPorts() map[string]struct{} {
	ports := make(map[string]struct{})
	// ss -lnpt | grep sshd
	cmd := exec.Command("bash", "-c", "ss -lnpt | grep sshd | awk '{print $4}' | awk -F: '{print $NF}' | sort -u")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, p := range lines {
			p = strings.TrimSpace(p)
			if p != "" && p != "*" {
				ports[p] = struct{}{}
			}
		}
	}
	// Fallback comum se ss falhar
	if len(ports) == 0 {
		ports["22"] = struct{}{}
	}
	return ports
}

// InstallSSHMonitor configura o monitor PAM para registrar logins/logouts
func InstallSSHMonitor() error {
	scriptPath := "/usr/local/bin/ssh-monitor.sh"
	logPath := "/var/log/ssh-monitor.log"
	pamFile := "/etc/pam.d/sshd"

	scriptContent := `#!/bin/bash
LOG_FILE="/var/log/ssh-monitor.log"
echo "$(date) USER=$PAM_USER IP=$PAM_RHOST EVENT=$PAM_TYPE" >> "$LOG_FILE"
`
	// 1. Criar script
	exec.Command("bash", "-c", fmt.Sprintf("echo '%s' > %s", scriptContent, scriptPath)).Run()
	exec.Command("chmod", "+x", scriptPath).Run()

	// 2. Criar log com permissão de escrita para o PAM
	exec.Command("touch", logPath).Run()
	exec.Command("chmod", "666", logPath).Run()

	// 3. Integrar com PAM se não existir
	checkCmd := fmt.Sprintf("grep -q '%s' %s", scriptPath, pamFile)
	if err := exec.Command("bash", "-c", checkCmd).Run(); err != nil {
		addCmd := fmt.Sprintf("echo 'session optional pam_exec.so %s' >> %s", scriptPath, pamFile)
		exec.Command("bash", "-c", addCmd).Run()
	}

	return nil
}

// EnforceSSHLimit verifica e encerra conexões excedentes de um usuário (Lógica SSHPLUS)
func EnforceSSHLimit(username string, limit int) {
	if limit <= 0 {
		return
	}

	// Conta processos sshd atuais
	count, _ := GetSSHOnlineUsers(username)

	if count > limit {
		// Se ultrapassar, derruba o usuário (pkill -u $user)
		// Isso é o que o script SSHPLUS faz para forçar o limite
		exec.Command("pkill", "-u", username).Run()
	}
}

// GetSSHOnlineUsers retorna a contagem e os detalhes das conexões de um usuário.
// Replica a lógica do painel SSHPLUS combinada com requisitos de tempo real.
func GetSSHOnlineUsers(username string) (int, []OnlineUser) {
	if username == "" {
		return 0, nil
	}

	// 1. Lógica SSHPLUS: ps -u $user | grep sshd | wc -l
	cmdPS := exec.Command("bash", "-c", fmt.Sprintf("ps -u %s | grep sshd | wc -l", username))
	outPS, _ := cmdPS.Output()
	count, _ := strconv.Atoi(strings.TrimSpace(string(outPS)))

	if count == 0 {
		return 0, nil
	}

	// 2. Lógica para Duração e IPs (usando 'who' como requisitado e para detalhes extras)
	sshCacheMutex.Lock()
	if time.Since(sshCacheTime) > 2*time.Second {
		// 'who' mostra o login time
		cmdWho := exec.Command("who")
		outWho, _ := cmdWho.Output()
		sshCache = strings.Split(strings.TrimSpace(string(outWho)), "\n")
		sshCacheTime = time.Now()
	}
	lines := sshCache
	sshCacheMutex.Unlock()

	var details []OnlineUser
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		// Formato do 'who': username terminal data hora (IP)
		if len(fields) >= 4 && fields[0] == username {
			// Tenta extrair a duração
			// Exemplo de campos: fields[2] = 2026-03-30, fields[3] = 22:10
			loginTimeStr := fmt.Sprintf("%s %s", fields[2], fields[3])

			// Layouts comuns do 'who'
			layouts := []string{"2006-01-02 15:04", "Jan 2 15:04"}
			var loginTime time.Time
			var err error

			for _, layout := range layouts {
				loginTime, err = time.ParseInLocation(layout, loginTimeStr, time.Local)
				if err == nil {
					// Se o layout não tem ano, usa o ano atual
					if !strings.Contains(layout, "2006") {
						loginTime = loginTime.AddDate(time.Now().Year(), 0, 0)
					}
					break
				}
			}

			duration := time.Duration(0)
			if err == nil {
				duration = time.Since(loginTime)
			}

			// Tenta extrair o IP entre parênteses no final da linha
			ip := ""
			lastField := fields[len(fields)-1]
			if strings.HasPrefix(lastField, "(") && strings.HasSuffix(lastField, ")") {
				ip = strings.Trim(lastField, "()")
			}

			details = append(details, OnlineUser{
				IP:       ip,
				Duration: duration,
				Terminal: fields[1],
			})
		}
	}

	// Se o PS contou mas o WHO não achou detalhes (comum em túneis sem TTY),
	// garantimos que a contagem retorne pelo menos objetos vazios para a UI
	if len(details) == 0 && count > 0 {
		for i := 0; i < count; i++ {
			details = append(details, OnlineUser{})
		}
	}

	return count, details
}

// GetUserOnlineDuration retorna o tempo de conexão do processo mais antigo (Lógica SSHPLUS)
func GetUserOnlineDuration(username string) string {
	// ps -o etime $(ps -u $user | grep sshd | awk 'NR==1 {print $1}')
	cmdPID := exec.Command("bash", "-c", fmt.Sprintf("ps -u %s | grep sshd | awk 'NR==1 {print $1}'", username))
	outPID, _ := cmdPID.Output()
	pid := strings.TrimSpace(string(outPID))

	if pid == "" {
		return "00:00:00"
	}

	cmdTime := exec.Command("ps", "-o", "etime=", "-p", pid)
	outTime, _ := cmdTime.Output()
	t := strings.TrimSpace(string(outTime))

	// Formatos do etime: MM:SS, HH:MM:SS ou DD-HH:MM:SS
	// Vamos normalizar para HH:MM:SS
	if len(t) == 5 { // MM:SS
		return "00:" + t
	}
	return t
}

// GetLastLogin retorna o último registro de login do usuário (Integração PAM)
func GetLastLogin(username string) string {
	const logFile = "/var/log/ssh-monitor.log"
	// Lógica robusta de busca no log
	cmd := exec.Command("bash", "-c", fmt.Sprintf("grep 'USER=%s' %s 2>/dev/null | tail -n 1", username, logFile))
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return "Nenhum registro"
	}

	line := string(out)
	// Exemplo: Mon Mar 30 22:10:00 BRT 2026 USER=bruno ...
	parts := strings.Split(line, " USER=")
	if len(parts) > 0 {
		return strings.TrimSpace(parts[0])
	}
	return "Desconhecido"
}

// GetSSHConnections é um alias para GetSSHOnlineUsers (compatibilidade)
func GetSSHConnections(username string) (int, []OnlineUser) {
	return GetSSHOnlineUsers(username)
}

// GetSSHOnlineCount retorna o total de usuários SSH online no servidor
func GetSSHOnlineCount() (int, error) {
	// ps aux | grep sshd | grep -v grep | grep -v "root" | awk '{print $1}' | sort | uniq | wc -l
	cmd := exec.Command("bash", "-c", "ps aux | grep sshd | grep -v grep | grep -v 'root' | awk '{print $1}' | sort | uniq | wc -l")
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return count, nil
}

// GetSSHUserOnlineDetails retorna detalhes usando a nova lógica baseada no 'who'
func GetSSHUserOnlineDetails(username string) []OnlineUser {
	count, details := GetSSHOnlineUsers(username)
	if count == 0 {
		return nil
	}
	return details
}

// IsUserOnline verifica se o usuário está online via SSH
func IsUserOnline(username string) bool {
	count, _ := GetSSHConnections(username)
	return count > 0
}

// GetOnlineUsersCount retorna o número de conexões SSH ativas para um usuário
func GetOnlineUsersCount(username string) int {
	count, _ := GetSSHConnections(username)
	return count
}

// GetAllOnlineSessionsCount retorna o total de conexões ativas (SSH via 'w' + Xray)
func GetAllOnlineSessionsCount() int {
	sshCacheMutex.Lock()
	if time.Since(sshCacheTime) > 2*time.Second {
		cmd := exec.Command("w", "-h")
		out, _ := cmd.Output()
		sshCache = strings.Split(strings.TrimSpace(string(out)), "\n")
		sshCacheTime = time.Now()
	}
	lines := sshCache
	sshCacheMutex.Unlock()

	sshCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			sshCount++
		}
	}

	return sshCount + GetXrayTotalConnections()
}
