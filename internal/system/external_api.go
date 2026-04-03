package system

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"net/http"
	"os/exec"
	"painel-ssh/internal/db"
	"painel-ssh/internal/models"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	externalApiServer   *http.Server
	externalApiPort     = 2020
	externalApiMutex    sync.Mutex
	isExternalApiActive bool
	ExternalApiRequests uint64

	monitorApiServer   *http.Server
	monitorApiPort     = 3030
	monitorApiMutex    sync.Mutex
	isMonitorApiActive bool
	MonitorApiRequests uint64

	registrosApiServer   *http.Server
	registrosApiPort     = 1010
	registrosApiMutex    sync.Mutex
	isRegistrosApiActive bool
	RegistrosApiRequests uint64

	servidorApiServer   *http.Server
	servidorApiPort     = 1030
	servidorApiMutex    sync.Mutex
	isServidorApiActive bool
	ServidorApiRequests uint64
)

// ... (GenerateApiToken e StartExternalAPI permanecem iguais)

// StartMonitorAPI inicia o servidor da API de monitoramento
func StartMonitorAPI() error {
	monitorApiMutex.Lock()
	defer monitorApiMutex.Unlock()

	if isMonitorApiActive {
		return nil
	}

	// Abrir porta no firewall
	exec.Command("iptables", "-A", "INPUT", "-p", "tcp", "--dport", fmt.Sprintf("%d", monitorApiPort), "-j", "ACCEPT").Run()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/monitor-onlines", handleMonitorOnlinesApi)

	monitorApiServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", monitorApiPort),
		Handler: mux,
	}

	isMonitorApiActive = true
	go func() {
		if err := monitorApiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			monitorApiMutex.Lock()
			isMonitorApiActive = false
			monitorApiMutex.Unlock()
		}
	}()

	return nil
}

// StopMonitorAPI para o servidor da API de monitoramento
func StopMonitorAPI() error {
	monitorApiMutex.Lock()
	defer monitorApiMutex.Unlock()

	if !isMonitorApiActive || monitorApiServer == nil {
		return nil
	}

	if err := monitorApiServer.Shutdown(context.Background()); err != nil {
		return err
	}

	isMonitorApiActive = false
	return nil
}

// GetMonitorApiStatus retorna o status da API de monitoramento
func GetMonitorApiStatus() bool {
	monitorApiMutex.Lock()
	defer monitorApiMutex.Unlock()
	return isMonitorApiActive
}

// StartRegistrosAPI inicia o servidor da API de registros
func StartRegistrosAPI() error {
	registrosApiMutex.Lock()
	defer registrosApiMutex.Unlock()

	if isRegistrosApiActive {
		return nil
	}

	// Abrir porta no firewall
	exec.Command("iptables", "-A", "INPUT", "-p", "tcp", "--dport", fmt.Sprintf("%d", registrosApiPort), "-j", "ACCEPT").Run()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/usuarios-ssh", handleRegistrosApi)

	registrosApiServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", registrosApiPort),
		Handler: mux,
	}

	isRegistrosApiActive = true
	go func() {
		if err := registrosApiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			registrosApiMutex.Lock()
			isRegistrosApiActive = false
			registrosApiMutex.Unlock()
		}
	}()

	return nil
}

// StopRegistrosAPI para o servidor da API de registros
func StopRegistrosAPI() error {
	registrosApiMutex.Lock()
	defer registrosApiMutex.Unlock()

	if !isRegistrosApiActive || registrosApiServer == nil {
		return nil
	}

	if err := registrosApiServer.Shutdown(context.Background()); err != nil {
		return err
	}

	isRegistrosApiActive = false
	return nil
}

// GetRegistrosApiStatus retorna o status da API de registros
func GetRegistrosApiStatus() bool {
	registrosApiMutex.Lock()
	defer registrosApiMutex.Unlock()
	return isRegistrosApiActive
}

func handleRegistrosApi(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&RegistrosApiRequests, 1)
	// Validar Token (mesmo token da API de criação)
	token, _ := db.GetConfig("api_token")
	if r.Header.Get("Authorization") != "Bearer "+token && r.URL.Query().Get("token") != token {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Token de autenticação inválido"}`))
		return
	}

	// Coletar dados de usuários
	totalUsers, _ := db.GetTotalUsers()
	expiredUsers, _ := db.GetExpiredUsersCount()
	activeUsers := totalUsers - expiredUsers

	// Resposta
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"status":          "success",
		"total_usuarios":  totalUsers,
		"usuarios_ativos": activeUsers,
		"usuarios_exp":    expiredUsers,
		"timestamp":       time.Now().Format(time.RFC3339),
	}
	json.NewEncoder(w).Encode(response)
}

// StartServidorAPI inicia o servidor da API de status do servidor
func StartServidorAPI() error {
	servidorApiMutex.Lock()
	defer servidorApiMutex.Unlock()

	if isServidorApiActive {
		return nil
	}

	// Abrir porta no firewall
	exec.Command("iptables", "-A", "INPUT", "-p", "tcp", "--dport", fmt.Sprintf("%d", servidorApiPort), "-j", "ACCEPT").Run()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/servidor-status", handleServidorApi)

	servidorApiServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", servidorApiPort),
		Handler: mux,
	}

	isServidorApiActive = true
	go func() {
		if err := servidorApiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			servidorApiMutex.Lock()
			isServidorApiActive = false
			servidorApiMutex.Unlock()
		}
	}()

	return nil
}

// StopServidorAPI para o servidor da API de status do servidor
func StopServidorAPI() error {
	servidorApiMutex.Lock()
	defer servidorApiMutex.Unlock()

	if !isServidorApiActive || servidorApiServer == nil {
		return nil
	}

	if err := servidorApiServer.Shutdown(context.Background()); err != nil {
		return err
	}

	isServidorApiActive = false
	return nil
}

// GetServidorApiStatus retorna o status da API de status do servidor
func GetServidorApiStatus() bool {
	servidorApiMutex.Lock()
	defer servidorApiMutex.Unlock()
	return isServidorApiActive
}

func handleServidorApi(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&ServidorApiRequests, 1)
	// Validar Token (mesmo token da API de criação)
	token, _ := db.GetConfig("api_token")
	if r.Header.Get("Authorization") != "Bearer "+token && r.URL.Query().Get("token") != token {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Token de autenticação inválido"}`))
		return
	}

	// Resposta
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"status":    "success",
		"servidor":  "ONLINE",
		"timestamp": time.Now().Format(time.RFC3339),
	}
	json.NewEncoder(w).Encode(response)
}

func handleMonitorOnlinesApi(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&MonitorApiRequests, 1)
	// 1. Validar Token (mesmo token da API de criação)
	token, _ := db.GetConfig("api_token")
	if r.Header.Get("Authorization") != "Bearer "+token && r.URL.Query().Get("token") != token {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Token de autenticação inválido"}`))
		return
	}

	// 2. Coletar dados de onlines
	sshOnlines, _ := GetSSHOnlineCount()
	// Se houver suporte a Xray onlines, somamos aqui
	xrayOnlines := 0
	if host, _ := db.GetConfig("xray_host"); host != "" {
		// Implementação simplificada ou chamada de função existente
		// xrayOnlines = GetXrayOnlineCount()
	}

	totalOnlines := sshOnlines + xrayOnlines

	// 3. Resposta
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"status":        "success",
		"ssh_onlines":   sshOnlines,
		"xray_onlines":  xrayOnlines,
		"total_onlines": totalOnlines,
		"timestamp":     time.Now().Format(time.RFC3339),
	}
	json.NewEncoder(w).Encode(response)
}

// GenerateApiToken gera um token aleatório seguro
func GenerateApiToken() string {
	b := make([]byte, 16)
	if _, err := crand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// StartExternalAPI inicia o servidor da API externa
func StartExternalAPI() error {
	externalApiMutex.Lock()
	defer externalApiMutex.Unlock()

	if isExternalApiActive {
		return nil
	}

	// Abrir porta no firewall
	exec.Command("iptables", "-A", "INPUT", "-p", "tcp", "--dport", fmt.Sprintf("%d", externalApiPort), "-j", "ACCEPT").Run()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/criar-usuario", handleCreateUserApi)

	externalApiServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", externalApiPort),
		Handler: mux,
	}

	isExternalApiActive = true
	go func() {
		if err := externalApiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			externalApiMutex.Lock()
			isExternalApiActive = false
			externalApiMutex.Unlock()
		}
	}()

	return nil
}

// StopExternalAPI para o servidor da API externa
func StopExternalAPI() error {
	externalApiMutex.Lock()
	defer externalApiMutex.Unlock()

	if !isExternalApiActive || externalApiServer == nil {
		return nil
	}

	if err := externalApiServer.Shutdown(context.Background()); err != nil {
		return err
	}

	isExternalApiActive = false
	return nil
}

// GetExternalApiStatus retorna o status da API
func GetExternalApiStatus() bool {
	externalApiMutex.Lock()
	defer externalApiMutex.Unlock()
	return isExternalApiActive
}

// GenerateSecureUsername gera um nome de usuário de até 8 dígitos (letras maiúsculas e minúsculas)
func GenerateSecureUsername() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	mrand.Seed(time.Now().UnixNano())
	b := make([]byte, 8)
	for i := range b {
		b[i] = charset[mrand.Intn(len(charset))]
	}
	return string(b)
}

// GenerateSecurePassword gera uma senha de até 8 dígitos (números, letras, caracteres especiais)
func GenerateSecurePassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	mrand.Seed(time.Now().UnixNano())
	b := make([]byte, 8)
	for i := range b {
		b[i] = charset[mrand.Intn(len(charset))]
	}
	return string(b)
}

func handleCreateUserApi(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&ExternalApiRequests, 1)
	// 1. Validar Token
	token, _ := db.GetConfig("api_token")
	if r.Header.Get("Authorization") != "Bearer "+token && r.URL.Query().Get("token") != token {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Token de autenticação inválido"}`))
		return
	}

	// 2. Validar Referer (Domínios permitidos)
	allowedReferer, _ := db.GetConfig("api_referer")
	if allowedReferer != "" {
		referer := r.Header.Get("Referer")
		if referer == "" || !strings.Contains(referer, allowedReferer) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error": "Referer não permitido"}`))
			return
		}
	}

	// 3. Gerar Usuário e Senha Automáticos
	var username, password string
	for {
		username = GenerateSecureUsername()
		// Verificar se já existe no sistema/banco
		existing, _ := db.GetUserByUsername(username)
		if existing == nil {
			break
		}
	}
	password = GenerateSecurePassword()

	// 4. Coletar parâmetros de configuração padrão
	daysStr, _ := db.GetConfig("api_days")
	days, _ := strconv.Atoi(daysStr)
	if days <= 0 {
		days = 30
	}

	limitStr, _ := db.GetConfig("api_limit")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 1
	}

	expirationDate := time.Now().AddDate(0, 0, days)

	// 5. Criar Usuário
	// Criar no sistema
	err := CreateSSHUser(username, password, expirationDate)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf(`{"error": "Erro ao criar usuário no sistema: %v"}`, err)))
		return
	}

	_ = SetConnectionLimit(username, limit)

	// Xray se habilitado
	xrayUUID := ""
	if host, _ := db.GetConfig("xray_host"); host != "" {
		xrayUUID = GenerateUUID()
		_ = AddXrayUser(username, xrayUUID)
		xusr := &models.XrayUser{
			Username:        username,
			UUID:            xrayUUID,
			ExpiresAt:       expirationDate,
			ConnectionLimit: limit,
			Status:          "active",
			CreatedAt:       time.Now(),
		}
		_ = db.SaveXrayUser(xusr)
	}

	// Salvar no Banco
	user := &models.User{
		Username:       username,
		Password:       password,
		Limit:          limit,
		ExpirationDate: expirationDate,
		XrayUUID:       xrayUUID,
		Type:           "premium",
		CreatedAt:      time.Now(),
	}
	_ = db.SaveUser(user)

	// 5. Resposta
	domain, _ := db.GetConfig("api_domain")
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"status":       "success",
		"usuario":      username,
		"senha":        password,
		"validade":     expirationDate.Format("02/01/2006"),
		"dias_validos": days,
		"limite":       limit,
		"xray_uuid":    xrayUUID,
		"dominio":      domain,
	}
	json.NewEncoder(w).Encode(response)
}
