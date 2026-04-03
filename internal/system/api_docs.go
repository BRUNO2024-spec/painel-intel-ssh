package system

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"painel-ssh/internal/db"
	"sync"
)

var (
	docsServer   *http.Server
	docsPort     = 333
	docsMutex    sync.Mutex
	isDocsActive bool
)

// StartDocsAPI inicia o servidor da página de documentação
func StartDocsAPI() error {
	docsMutex.Lock()
	defer docsMutex.Unlock()

	if isDocsActive {
		return nil
	}

	// Abrir porta no firewall
	exec.Command("iptables", "-A", "INPUT", "-p", "tcp", "--dport", fmt.Sprintf("%d", docsPort), "-j", "ACCEPT").Run()

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleDocsPage)

	docsServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", docsPort),
		Handler: mux,
	}

	isDocsActive = true
	go func() {
		if err := docsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			docsMutex.Lock()
			isDocsActive = false
			docsMutex.Unlock()
		}
	}()

	return nil
}

// StopDocsAPI para o servidor da documentação
func StopDocsAPI() error {
	docsMutex.Lock()
	defer docsMutex.Unlock()

	if !isDocsActive || docsServer == nil {
		return nil
	}

	if err := docsServer.Shutdown(context.Background()); err != nil {
		return err
	}

	isDocsActive = false
	return nil
}

// GetDocsStatus retorna o status da página de documentação
func GetDocsStatus() bool {
	docsMutex.Lock()
	defer docsMutex.Unlock()
	return isDocsActive
}

func handleDocsPage(w http.ResponseWriter, r *http.Request) {
	ip := GetPublicIP()
	token, _ := db.GetConfig("api_token")

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html lang="pt-br">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Documentação API - Painel SSH</title>
    <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/css/bootstrap.min.css" rel="stylesheet">
    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.4.0/css/all.min.css">
    <style>
        :root {
            --primary-color: #0d6efd;
            --bg-dark: #121212;
            --card-bg: #1e1e1e;
            --text-light: #e0e0e0;
        }
        body {
            background-color: var(--bg-dark);
            color: var(--text-light);
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
        }
        .navbar {
            background-color: var(--card-bg);
            border-bottom: 1px solid #333;
        }
        .card {
            background-color: var(--card-bg);
            border: 1px solid #333;
            margin-bottom: 2rem;
            color: var(--text-light);
        }
        .code-block {
            background-color: #000;
            color: #00ff00;
            padding: 1rem;
            border-radius: 8px;
            font-family: 'Courier New', Courier, monospace;
            position: relative;
            overflow-x: auto;
        }
        .badge-method {
            font-weight: bold;
            padding: 0.4rem 0.8rem;
            border-radius: 4px;
        }
        .badge-get { background-color: #28a745; }
        .sidebar {
            background-color: var(--card-bg);
            min-height: 100vh;
            padding: 2rem 1rem;
            border-right: 1px solid #333;
        }
        .copy-btn {
            position: absolute;
            top: 10px;
            right: 10px;
            background: #333;
            color: #fff;
            border: none;
            padding: 5px 10px;
            border-radius: 4px;
            cursor: pointer;
        }
        .copy-btn:hover { background: #444; }
        .endpoint-url { color: #00d4ff; font-weight: bold; }
    </style>
</head>
<body>

<div class="container-fluid">
    <div class="row">
        <!-- Sidebar -->
        <nav class="col-md-3 col-lg-2 sidebar d-none d-md-block">
            <h4 class="text-center mb-4"><i class="fas fa-server me-2"></i>API SSH</h4>
            <ul class="nav flex-column">
                <li class="nav-item mb-2">
                    <a class="nav-link text-light" href="#introducao"><i class="fas fa-book me-2"></i>Introdução</a>
                </li>
                <li class="nav-item mb-2">
                    <a class="nav-link text-light" href="#autenticacao"><i class="fas fa-lock me-2"></i>Autenticação</a>
                </li>
                <li class="nav-item mb-2">
                    <a class="nav-link text-light" href="#criar-usuario"><i class="fas fa-user-plus me-2"></i>Criar Usuário</a>
                </li>
                <li class="nav-item mb-2">
                    <a class="nav-link text-light" href="#monitorar"><i class="fas fa-chart-line me-2"></i>Monitorar Onlines</a>
                </li>
                <li class="nav-item mb-2">
                    <a class="nav-link text-light" href="#registros"><i class="fas fa-list me-2"></i>API Registros</a>
                </li>
                <li class="nav-item mb-2">
                    <a class="nav-link text-light" href="#servidor"><i class="fas fa-server me-2"></i>API Servidor</a>
                </li>
            </ul>
        </nav>

        <!-- Main Content -->
        <main class="col-md-9 ms-sm-auto col-lg-10 px-md-4 py-4">
            <div id="introducao" class="mb-5">
                <h1 class="display-5">Documentação da API</h1>
                <p class="lead">Bem-vindo à documentação oficial do Painel SSH. Integre seu site ou bot com facilidade.</p>
                <div class="alert alert-info bg-dark text-light border-info">
                    <i class="fas fa-info-circle me-2"></i>Seu IP Público: <strong>%s</strong>
                </div>
            </div>

            <div id="autenticacao" class="card shadow">
                <div class="card-body">
                    <h3 class="card-title"><i class="fas fa-lock me-2"></i>Autenticação</h3>
                    <p>Todas as chamadas requerem o Token de Acesso gerado no painel.</p>
                    <p>Você pode enviar o token de duas formas:</p>
                    <ul>
                        <li><strong>Header:</strong> <code>Authorization: Bearer %s</code></li>
                        <li><strong>URL Query:</strong> <code>?token=%s</code></li>
                    </ul>
                    <div class="code-block">
                        curl -H "Authorization: Bearer %s" "http://%s:2020/v1/criar-usuario"
                    </div>
                </div>
            </div>

            <div id="criar-usuario" class="card shadow">
                <div class="card-body">
                    <h3 class="card-title text-success"><i class="fas fa-user-plus me-2"></i>Criar Usuário (Automático)</h3>
                    <p>Cria um usuário SSH e UUID Xray automaticamente com 8 caracteres aleatórios.</p>
                    <div class="mb-3">
                        <span class="badge badge-method badge-get">GET</span>
                        <span class="endpoint-url">http://%s:2020/v1/criar-usuario</span>
                    </div>
                    
                    <h5>Resposta de Sucesso (JSON):</h5>
                    <div class="code-block">
{
  "status": "success",
  "usuario": "AbCdEfGh",
  "senha": "123#AbCd",
  "validade": "30/04/2026",
  "dias_validos": 30,
  "limite": 1,
  "xray_uuid": "f47ac10b-...",
  "dominio": "dns.meusite.com"
}
                    </div>

                    <h5 class="mt-4">Descrição dos Campos:</h5>
                    <table class="table table-dark table-hover mt-3">
                        <thead>
                            <tr>
                                <th>Campo</th>
                                <th>Tipo</th>
                                <th>Descrição</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td><code>status</code></td>
                                <td>string</td>
                                <td>Status da operação (success/error)</td>
                            </tr>
                            <tr>
                                <td><code>usuario</code></td>
                                <td>string</td>
                                <td>Nome do usuário gerado</td>
                            </tr>
                            <tr>
                                <td><code>senha</code></td>
                                <td>string</td>
                                <td>Senha gerada para o login SSH</td>
                            </tr>
                            <tr>
                                <td><code>validade</code></td>
                                <td>string</td>
                                <td>Data de expiração formatada (DD/MM/AAAA)</td>
                            </tr>
                            <tr>
                                <td><code>dias_validos</code></td>
                                <td>int</td>
                                <td>Quantidade total de dias de validade</td>
                            </tr>
                            <tr>
                                <td><code>limite</code></td>
                                <td>int</td>
                                <td>Limite de conexões simultâneas</td>
                            </tr>
                            <tr>
                                <td><code>xray_uuid</code></td>
                                <td>string</td>
                                <td>UUID gerado para conexões V2Ray/Xray</td>
                            </tr>
                            <tr>
                                <td><code>dominio</code></td>
                                <td>string</td>
                                <td>Domínio configurado para o servidor</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <div id="monitorar" class="card shadow">
                <div class="card-body">
                    <h3 class="card-title text-info"><i class="fas fa-chart-line me-2"></i>Monitorar Onlines</h3>
                    <p>Retorna o total de usuários conectados em tempo real.</p>
                    <div class="mb-3">
                        <span class="badge badge-method badge-get">GET</span>
                        <span class="endpoint-url">http://%s:3030/v1/monitor-onlines</span>
                    </div>

                    <h5>Resposta (JSON):</h5>
                    <div class="code-block">
{
  "status": "success",
  "ssh_onlines": 10,
  "xray_onlines": 5,
  "total_onlines": 15,
  "timestamp": "2026-03-30T22:15:00Z"
}
                    </div>

                    <h5 class="mt-4">Descrição dos Campos:</h5>
                    <table class="table table-dark table-hover mt-3">
                        <thead>
                            <tr>
                                <th>Campo</th>
                                <th>Tipo</th>
                                <th>Descrição</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td><code>status</code></td>
                                <td>string</td>
                                <td>Status da operação</td>
                            </tr>
                            <tr>
                                <td><code>ssh_onlines</code></td>
                                <td>int</td>
                                <td>Total de conexões SSH ativas</td>
                            </tr>
                            <tr>
                                <td><code>xray_onlines</code></td>
                                <td>int</td>
                                <td>Total de conexões Xray ativas</td>
                            </tr>
                            <tr>
                                <td><code>total_onlines</code></td>
                                <td>int</td>
                                <td>Soma total de todas as conexões</td>
                            </tr>
                            <tr>
                                <td><code>timestamp</code></td>
                                <td>string</td>
                                <td>Horário da consulta (ISO 8601)</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <div id="registros" class="card shadow">
                <div class="card-body">
                    <h3 class="card-title text-warning"><i class="fas fa-list me-2"></i>API Registros</h3>
                    <p>Retorna dados estatísticos de todos os usuários criados.</p>
                    <div class="mb-3">
                        <span class="badge badge-method badge-get">GET</span>
                        <span class="endpoint-url">http://%s:1010/v1/usuarios-ssh</span>
                    </div>

                    <h5>Resposta (JSON):</h5>
                    <div class="code-block">
{
  "status": "success",
  "total_usuarios": 100,
  "usuarios_ativos": 85,
  "usuarios_exp": 15,
  "timestamp": "2026-03-30T22:15:00Z"
}
                    </div>

                    <h5 class="mt-4">Descrição dos Campos:</h5>
                    <table class="table table-dark table-hover mt-3">
                        <thead>
                            <tr>
                                <th>Campo</th>
                                <th>Tipo</th>
                                <th>Descrição</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td><code>status</code></td>
                                <td>string</td>
                                <td>Status da operação</td>
                            </tr>
                            <tr>
                                <td><code>total_usuarios</code></td>
                                <td>int</td>
                                <td>Total de usuários registrados no banco</td>
                            </tr>
                            <tr>
                                <td><code>usuarios_ativos</code></td>
                                <td>int</td>
                                <td>Total de usuários com validade ativa</td>
                            </tr>
                            <tr>
                                <td><code>usuarios_exp</code></td>
                                <td>int</td>
                                <td>Total de usuários com validade expirada</td>
                            </tr>
                            <tr>
                                <td><code>timestamp</code></td>
                                <td>string</td>
                                <td>Horário da consulta (ISO 8601)</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <div id="servidor" class="card shadow">
                <div class="card-body">
                    <h3 class="card-title text-danger"><i class="fas fa-server me-2"></i>API Servidor</h3>
                    <p>Retorna o status atual do servidor (Online/Offline).</p>
                    <div class="mb-3">
                        <span class="badge badge-method badge-get">GET</span>
                        <span class="endpoint-url">http://%s:1030/v1/servidor-status</span>
                    </div>

                    <h5>Resposta (JSON):</h5>
                    <div class="code-block">
{
  "status": "success",
  "servidor": "ONLINE",
  "timestamp": "2026-03-30T22:15:00Z"
}
                    </div>

                    <h5 class="mt-4">Descrição dos Campos:</h5>
                    <table class="table table-dark table-hover mt-3">
                        <thead>
                            <tr>
                                <th>Campo</th>
                                <th>Tipo</th>
                                <th>Descrição</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td><code>status</code></td>
                                <td>string</td>
                                <td>Status da operação</td>
                            </tr>
                            <tr>
                                <td><code>servidor</code></td>
                                <td>string</td>
                                <td>Status do servidor (ONLINE)</td>
                            </tr>
                            <tr>
                                <td><code>timestamp</code></td>
                                <td>string</td>
                                <td>Horário da consulta (ISO 8601)</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <footer class="text-center mt-5 text-secondary">
                <p>&copy; 2026 Painel SSH - Todos os direitos reservados.</p>
            </footer>
        </main>
    </div>
</div>

<script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/js/bootstrap.bundle.min.js"></script>
</body>
</html>
	`, ip, token, token, token, ip, ip, ip, ip, ip)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
