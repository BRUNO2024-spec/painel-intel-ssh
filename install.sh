#!/bin/bash

# ── PAINEL SSH INTEL - INSTALADOR AUTOMÁTICO ──────────────────────────────────
# Repositório: https://github.com/BRUNO2024-spec/painel-intel-ssh.git
# ──────────────────────────────────────────────────────────────────────────────

# Cores para o terminal
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

clear

echo -e "${CYAN}${BOLD}====================================================${RESET}"
echo -e "${CYAN}${BOLD}          PAINEL SSH INTEL - INSTALAÇÃO             ${RESET}"
echo -e "${CYAN}${BOLD}====================================================${RESET}"

# Verificar se é root
if [ "$EUID" -ne 0 ]; then
  echo -e "${RED}Erro: Você precisa rodar este script como ROOT.${RESET}"
  exit 1
fi

# 1. Instalar dependências básicas
echo -e "\n${YELLOW}[⏳] Instalando dependências básicas...${RESET}"
apt update -y
apt install -y git wget curl zip unzip screen socat lsof

# 2. Instalar Go 1.22 (Obrigatório)
echo -e "${YELLOW}[⏳] Configurando ambiente Go 1.22...${RESET}"
if ! command -v go &> /dev/null || [[ "$(go version | awk '{print $3}')" < "go1.22" ]]; then
    echo -e "${YELLOW}[⏳] Baixando e instalando Go 1.22...${RESET}"
    wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz -q
    rm -rf /usr/local/go
    tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
    rm go1.22.0.linux-amd64.tar.gz
    
    # Configurar PATH permanentemente
    if ! grep -q "/usr/local/go/bin" ~/.bashrc; then
        echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    fi
    export PATH=$PATH:/usr/local/go/bin
else
    echo -e "${GREEN}[✔] Go já está instalado na versão correta.${RESET}"
fi

# 3. Clonar o Repositório
echo -e "${YELLOW}[⏳] Clonando repositório do projeto...${RESET}"
INSTALL_DIR="/root/painel-intel-ssh"
if [ -d "$INSTALL_DIR" ]; then
    echo -e "${YELLOW}[⚠] Pasta já existe. Atualizando repositório...${RESET}"
    cd "$INSTALL_DIR"
    git pull
else
    git clone https://github.com/BRUNO2024-spec/painel-intel-ssh.git "$INSTALL_DIR"
    cd "$INSTALL_DIR"
fi

# 4. Compilar o Projeto
echo -e "${YELLOW}[⏳] Compilando o Painel SSH...${RESET}"
go mod tidy
go build -o painel-ssh cmd/painel/main.go
chmod +x painel-ssh

# 5. Criar link simbólico para acesso fácil
ln -sf "$INSTALL_DIR/painel-ssh" /usr/local/bin/painel
ln -sf "$INSTALL_DIR/painel-ssh" /usr/local/bin/p

# 6. Configurar Diretórios e Permissões
echo -e "${YELLOW}[⏳] Ajustando permissões e diretórios...${RESET}"
mkdir -p /etc/painel-ssh
chmod 755 /etc/painel-ssh

# 7. Iniciar o Serviço de Background (APIs)
echo -e "${YELLOW}[⏳] Iniciando serviços de background...${RESET}"
# O próprio painel cria o serviço systemd ao rodar pela primeira vez no modo normal.
# Mas vamos rodar uma vez com --run-apis para garantir que o serviço systemd seja criado.
./painel-ssh --run-apis &
sleep 2
pkill -f painel-ssh

echo -e "${CYAN}${BOLD}====================================================${RESET}"
echo -e "${GREEN}${BOLD}       INSTALAÇÃO CONCLUÍDA COM SUCESSO!            ${RESET}"
echo -e "${CYAN}${BOLD}====================================================${RESET}"
echo -e "${YELLOW}Comandos rápidos:${RESET}"
echo -e "${BOLD}painel${RESET} - Abre o menu interativo"
echo -e "${BOLD}p${RESET}      - Abre o menu interativo (atalho)"
echo -e ""
echo -e "${CYAN}As APIs e serviços de background já estão rodando.${RESET}"
echo -e "${CYAN}Acesse a documentação na porta 333 do seu IP.${RESET}"
echo -e "${CYAN}${BOLD}====================================================${RESET}"

# Iniciar o painel automaticamente
echo -e "\n${GREEN}Iniciando o painel em 3 segundos...${RESET}"
sleep 3
./painel-ssh
