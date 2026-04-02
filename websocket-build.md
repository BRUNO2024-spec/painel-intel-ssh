# Deploy do WebSocket→SSH Proxy

## Copiar arquivo para o servidor e compilar

```bash
# No servidor Linux (/opt/painel-ssh):
cd /opt/painel-ssh

# Remover dependência do gorilla (não usada mais)
go mod tidy

# Compilar o binário
go build -o websocket-server ./cmd/websocket/

# Parar processo antigo
pkill -f websocket-server

# Iniciar novo (proxy WS:80 → SSH:22)
./websocket-server -port 80 -ssh-host 127.0.0.1 -ssh-port 22 &
```

## Testar handshake (deve retornar 101)

```bash
curl -i -N \
  -H "Connection: Upgrade" \
  -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" \
  -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  http://localhost:80/
```

## Configuração do app VPN

```
Payload: GET / HTTP/1.1[crlf]Host: ssh-teste.animestreambr.site[lf]Upgrade: websocket[crlf][crlf]
Proxy:   188.114.99.229
Porta:   80
```

## Flags disponíveis

| Flag         | Padrão      | Descrição                        |
|--------------|-------------|----------------------------------|
| `-port`      | `80`        | Porta do servidor WebSocket      |
| `-ssh-host`  | `127.0.0.1` | Host do SSH destino              |
| `-ssh-port`  | `22`        | Porta do SSH destino             |
| `-cert`      | *(vazio)*   | Certificado SSL para WSS         |
| `-key`       | *(vazio)*   | Chave SSL para WSS               |
