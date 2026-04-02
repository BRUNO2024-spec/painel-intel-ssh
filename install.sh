#!/bin/bash

# ==============================================================================
# Script de Instalação do Painel SSH
# Desenvolvido para sistemas Debian/Ubuntu
# ==============================================================================

# Cores para o terminal
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Funções de Log
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[OK]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[AVISO]${NC} $1"; }
log_error() { echo -e "${RED}[ERRO]${NC} $1"; exit 1; }

# 1. Verificar permissões de root
if [ "$EUID" -ne 0 ]; then
    log_error "Este script deve ser executado como ROOT (sudo su)."
fi

# 2. Atualizar sistema e instalar dependências básicas
log_info "Atualizando o sistema e instalando dependências básicas..."
apt-get update -y || log_error "Falha ao atualizar o sistema (apt-get update)."
apt-get install -y wget curl git build-essential unzip net-tools ufw tar || log_error "Falha ao instalar dependências básicas."
log_success "Dependências básicas instaladas com sucesso."

# 3. Instalar o Go (1.21.x)
GO_VERSION="1.21.8"
log_info "Verificando instalação do Go (versão desejada: $GO_VERSION)..."

if command -v go &>/dev/null; then
    CURRENT_GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    log_info "Versão atual do Go detectada: $CURRENT_GO_VERSION"
else
    CURRENT_GO_VERSION="0.0.0"
fi

if [[ "$CURRENT_GO_VERSION" < "$GO_VERSION" ]]; then
    log_info "Instalando Go $GO_VERSION..."
    ARCH=$(uname -m)
    case $ARCH in
        x86_64) GO_ARCH="amd64" ;;
        aarch64) GO_ARCH="arm64" ;;
        *) log_error "Arquitetura não suportada: $ARCH" ;;
    esac

    GO_TAR="go$GO_VERSION.linux-$GO_ARCH.tar.gz"
    wget "https://go.dev/dl/$GO_TAR" -O /tmp/go.tar.gz || log_error "Falha ao baixar o binário do Go."
    
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz || log_error "Falha ao extrair o Go."
    rm /tmp/go.tar.gz
    
    log_success "Go $GO_VERSION instalado com sucesso em /usr/local/go."
else
    log_success "Go já está instalado em uma versão compatível."
fi

# 4. Configurar variáveis de ambiente do Go
log_info "Configurando variáveis de ambiente..."
if ! grep -q "/usr/local/go/bin" /etc/profile; then
    echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
    echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> /etc/profile
fi
export PATH=$PATH:/usr/local/go/bin

# 5. Preparar o diretório do projeto e dependências
INSTALL_DIR="/opt/painel-ssh"
log_info "Preparando o diretório de instalação: $INSTALL_DIR"

if [ ! -d "$INSTALL_DIR" ]; then
    mkdir -p "$INSTALL_DIR"
fi

# Se estivermos rodando de dentro da pasta clonada, apenas copiamos.
# Caso contrário, assumimos que o script deve clonar o repositório (opcional).
# Como estamos operando no ambiente local do usuário, vamos copiar os arquivos atuais.
cp -r . "$INSTALL_DIR" || log_warning "Não foi possível copiar os arquivos. Continuando do diretório atual..."
cd "$INSTALL_DIR" || cd - &>/dev/null

log_info "Baixando dependências do projeto (go mod tidy)..."
go mod tidy || log_error "Falha ao baixar dependências do Go (go mod tidy)."

# 6. Compilar o binário do painel e websocket
log_info "Compilando o binário do Painel..."
go build -o painel ./cmd/painel/main.go || log_error "Falha ao compilar o Painel (cmd/painel/main.go)."
log_success "Painel compilado com sucesso."

log_info "Compilando o binário do WebSocket Proxy..."
go build -o websocket-server ./cmd/websocket/main.go || log_error "Falha ao compilar o WebSocket Proxy (cmd/websocket/main.go)."
log_success "WebSocket Proxy compilado com sucesso."

# 7. Criar atalhos no sistema
log_info "Criar comando de execução 'painel'..."
cat <<EOF > /usr/local/bin/painel
#!/bin/bash
cd $INSTALL_DIR
./painel
EOF
chmod +x /usr/local/bin/painel
log_success "Atalho 'painel' criado com sucesso."

# 8. Finalização e Execução
log_info "Instalação concluída com sucesso!"
echo -e "${GREEN}================================================================${NC}"
echo -e "${GREEN} O Painel SSH foi instalado com sucesso!                       ${NC}"
echo -e "${GREEN} Para iniciar o painel a qualquer momento, digite: ${YELLOW}painel${NC}"
echo -e "${GREEN}================================================================${NC}"

# Perguntar se o usuário quer rodar agora
read -p "Deseja iniciar o painel agora? (s/n): " RUN_NOW
if [[ "$RUN_NOW" == "s" || "$RUN_NOW" == "S" ]]; then
    painel
fi
