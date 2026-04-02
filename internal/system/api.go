package system

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"sync/atomic"
)

var (
	checkerServer    *http.Server
	checkerPort      = 5757
	requestCount     uint64
	checkerMutex     sync.Mutex
	isCheckerRunning bool
)

// StartCheckerAPI inicia o servidor HTTP da API em background
func StartCheckerAPI() error {
	checkerMutex.Lock()
	defer checkerMutex.Unlock()

	if isCheckerRunning {
		return nil
	}

	// Abrir porta no firewall
	exec.Command("iptables", "-A", "INPUT", "-p", "tcp", "--dport", fmt.Sprintf("%d", checkerPort), "-j", "ACCEPT").Run()

	mux := http.NewServeMux()
	mux.HandleFunc("/v3/checkeruser", checkerHandler)

	checkerServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", checkerPort),
		Handler: mux,
	}

	isCheckerRunning = true
	go func() {
		if err := checkerServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			checkerMutex.Lock()
			isCheckerRunning = false
			checkerMutex.Unlock()
		}
	}()

	return nil
}

// StopCheckerAPI para o servidor HTTP
func StopCheckerAPI() error {
	checkerMutex.Lock()
	defer checkerMutex.Unlock()

	if !isCheckerRunning || checkerServer == nil {
		return nil
	}

	if err := checkerServer.Shutdown(context.Background()); err != nil {
		return err
	}

	isCheckerRunning = false
	return nil
}

// GetCheckerStatus retorna se a API está rodando
func GetCheckerStatus() bool {
	checkerMutex.Lock()
	defer checkerMutex.Unlock()
	return isCheckerRunning
}

// GetCheckerRequests retorna o total de requisições processadas
func GetCheckerRequests() uint64 {
	return atomic.LoadUint64(&requestCount)
}

func checkerHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCount, 1)

	username := r.URL.Query().Get("user")
	if username == "" {
		http.Error(w, `{"error": "usuário não informado"}`, http.StatusBadRequest)
		return
	}

	data, err := GetUserData(username)
	if err != nil || data == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "usuário não encontrado"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
