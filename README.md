# Painel SSH Intel - Sistema de Gerenciamento para VPS Ubuntu 22.04

Este é um sistema profissional e interativo para gerenciamento de usuários SSH em servidores Linux (Ubuntu 22.04), desenvolvido em Go (Golang).

## Funcionalidades

- Interface CLI Interativa e Colorida (ANSI)
- Monitoramento de Recursos em Tempo Real (CPU, RAM, Uptime)
- Persistência de Dados com SQLite
- Criação de Usuários com Limites de Conexões e Expiração
- Remoção e Alteração de Senhas
- Monitoramento de Usuários Online (SSH sessions)
- Controle de Conexões Simultâneas (/etc/security/limits.conf)

## Requisitos

- Sistema Operacional: Ubuntu 22.04 (ou similar)
- Privilégios de ROOT
- **Go 1.21+** (Obrigatório para compilação do painel e do DNSTT)

## Instalação e Compilação

Siga os passos abaixo para compilar o sistema diretamente no seu servidor:

1. Instale o Go 1.22 (Obrigatório):
   ```bash
   sudo apt update
   sudo apt remove golang-go -y # Remove versão antiga se houver
   wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
   sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
   
   # Adiciona ao PATH permanentemente
   echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
   export PATH=$PATH:/usr/local/go/bin
   
   go version # Deve mostrar 1.22.0
   ```

2. Compile o sistema:
   ```bash
   go mod tidy
   go build -o painel-ssh cmd/painel/main.go
   ```

   chmod +x install.sh
   sudo ./install.sh

3. Dê permissão de execução (se necessário):
   ```bash
   chmod +x painel-ssh
   ```

## Como Executar

O sistema **deve** ser executado como root para gerenciar os usuários do sistema operacional:

```bash
sudo ./painel-ssh
```

## Estrutura do Projeto

- `cmd/painel/main.go`: Ponto de entrada e interface do usuário.
- `internal/db/sqlite.go`: Gerenciamento do banco de dados SQLite.
- `internal/system/`: Wrappers para comandos do sistema Linux (useradd, chage, etc.).
- `internal/ui/`: Utilitários para interface terminal (cores, inputs, limpeza de tela).
- `internal/models/`: Definições de estruturas de dados.

## Notas Técnicas

- O sistema utiliza o driver `ncruces/go-sqlite3` que é um driver pure-Go para SQLite, facilitando a compilação cruzada.
- O controle de limites de conexão é feito através da criação de arquivos em `/etc/security/limits.d/`.
- A expiração de conta utiliza o comando `chage -E`.
