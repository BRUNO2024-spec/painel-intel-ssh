package main

import (
	"crypto/sha1"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

var (
	port    = flag.Int("port", 80, "Porta para escutar (WebSocket)")
	cert    = flag.String("cert", "", "Caminho do certificado SSL (.crt)")
	key     = flag.String("key", "", "Caminho da chave SSL (.key)")
	sshHost = flag.String("ssh-host", "127.0.0.1", "Host do servidor SSH para proxy")
	sshPort = flag.Int("ssh-port", 22, "Porta do servidor SSH para proxy")
	_       = flag.String("path", "/ws", "Endpoint WebSocket (aceito em qualquer rota)")
)

// websocketGUID é a magic string do RFC 6455
const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// computeAccept calcula o Sec-WebSocket-Accept dado o Sec-WebSocket-Key
func computeAccept(key string) string {
	h := sha1.New()
	h.Write([]byte(strings.TrimSpace(key) + websocketGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// upgradeHandler faz o hijack manual da conexão TCP e responde com 101.
// Funciona mesmo que o cliente não envie Sec-WebSocket-Key (apps VPN/inject).
func upgradeHandler(w http.ResponseWriter, r *http.Request) {
	// Hijack: assume controle direto do TCP conn
	hj, ok := w.(http.Hijacker)
	if !ok {
		log.Printf("[!] Servidor não suporta Hijack")
		http.Error(w, "Hijack não suportado", http.StatusInternalServerError)
		return
	}

	clientConn, bufrw, err := hj.Hijack()
	if err != nil {
		log.Printf("[!] Erro no Hijack: %v", err)
		return
	}
	defer clientConn.Close()

	log.Printf("[→] WS recebido de: %s | Path: %s | Host: %s",
		r.RemoteAddr, r.URL.Path, r.Host)

	// ── Montar e enviar resposta 101 ──────────────────────────────────────────
	wsKey := r.Header.Get("Sec-Websocket-Key")
	if wsKey == "" {
		wsKey = r.Header.Get("Sec-WebSocket-Key")
	}

	var response string
	if wsKey != "" {
		// Cliente RFC-compliant: calcular Accept key
		accept := computeAccept(wsKey)
		response = "HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + accept + "\r\n" +
			"\r\n"
	} else {
		// Cliente VPN/inject sem Sec-WebSocket-Key: mandar 101 simples
		response = "HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"\r\n"
	}

	if _, err := bufrw.WriteString(response); err != nil {
		log.Printf("[!] Erro ao escrever 101: %v", err)
		return
	}
	if err := bufrw.Flush(); err != nil {
		log.Printf("[!] Erro ao flush 101: %v", err)
		return
	}

	log.Printf("[✓] 101 Switching Protocols enviado para: %s", r.RemoteAddr)

	// ── Conectar ao servidor SSH local ────────────────────────────────────────
	sshAddr := fmt.Sprintf("%s:%d", *sshHost, *sshPort)
	sshConn, err := net.Dial("tcp", sshAddr)
	if err != nil {
		log.Printf("[!] Falha ao conectar ao SSH (%s): %v", sshAddr, err)
		return
	}
	defer sshConn.Close()

	log.Printf("[✓] Túnel estabelecido: %s ↔ SSH(%s)", r.RemoteAddr, sshAddr)

	// ── Proxy bidirecional: cliente ↔ SSH ─────────────────────────────────────
	// O bufio.ReadWriter pode ter dados já lidos no buffer do HTTP parser;
	// usamos o Reader do bufrw para o lado do cliente para não perder esses bytes.
	done := make(chan struct{}, 2)

	// Cliente → SSH
	go func() {
		_, err := io.Copy(sshConn, bufrw)
		if err != nil && !isClosedErr(err) {
			log.Printf("[!] Cliente→SSH erro: %v", err)
		}
		sshConn.(*net.TCPConn).CloseWrite()
		done <- struct{}{}
	}()

	// SSH → Cliente
	go func() {
		_, err := io.Copy(clientConn, sshConn)
		if err != nil && !isClosedErr(err) {
			log.Printf("[!] SSH→Cliente erro: %v", err)
		}
		clientConn.(*net.TCPConn).CloseWrite()
		done <- struct{}{}
	}()

	// Aguardar um dos lados fechar
	<-done
	log.Printf("[✓] Túnel encerrado: %s", r.RemoteAddr)
}

// isClosedErr ignora erros de "use of closed network connection"
func isClosedErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "use of closed network connection") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "connection reset by peer") ||
		err == io.EOF
}

func main() {
	flag.Parse()

	sshTarget := fmt.Sprintf("%s:%d", *sshHost, *sshPort)

	// Handler universal: aceita qualquer rota com header Upgrade: websocket
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		upgrade := strings.ToLower(r.Header.Get("Upgrade"))

		log.Printf("[←] %s %s | Upgrade: %q | Key: %q | Host: %s",
			r.Method, r.URL.Path,
			r.Header.Get("Upgrade"),
			r.Header.Get("Sec-Websocket-Key"),
			r.Host,
		)

		if upgrade == "websocket" {
			upgradeHandler(w, r)
			return
		}

		// Requisição HTTP normal — mostrar status
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "WebSocket→SSH Proxy ativo.\nProxy destino: %s\nPath solicitado: %s\n",
			sshTarget, r.URL.Path)
	})

	addr := fmt.Sprintf("0.0.0.0:%d", *port)
	log.Printf("[✓] WebSocket→SSH Proxy em %s → SSH em %s", addr, sshTarget)

	if *cert != "" && *key != "" {
		log.Printf("[✓] Usando SSL (WSS)")
		if err := http.ListenAndServeTLS(addr, *cert, *key, nil); err != nil {
			log.Fatalf("[!] Erro no ListenAndServeTLS: %v", err)
		}
	} else {
		log.Printf("[✓] Usando modo normal (WS)")
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatalf("[!] Erro no ListenAndServe: %v", err)
		}
	}
}
