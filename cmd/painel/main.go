package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"painel-ssh/internal/db"
	"painel-ssh/internal/models"
	"painel-ssh/internal/system"
	"painel-ssh/internal/ui"
	"strconv"
	"strings"
	"time"
)

var (
	isMainMenuActive  bool
	currentServerInfo *system.ServerInfo
)

func main() {
	// ── Suporte para flags CLI ─────────────────────────────────────────────
	printBanner := flag.String("print-banner", "", "Imprime o banner para o usuário informado")
	runApis := flag.Bool("run-apis", false, "Inicia apenas as APIs em background")
	flag.Parse()

	if *printBanner != "" {
		// O sistema de banner dinâmico foi substituído pelo novo modelo SSHPLUS.
		// Se necessário no futuro, este campo pode ser re-implementado.
		fmt.Print("Bem-vindo ao servidor SSH!\n")
		return
	}

	// Ensure running as root
	if !system.IsRoot() {
		ui.PrintError("Este sistema deve ser executado como ROOT!")
		os.Exit(1)
	}

	// Ensure directory exists
	_ = os.MkdirAll("/etc/painel-ssh", 0755)

	// Initialize database with absolute path
	if err := db.InitDB("/etc/painel-ssh/ssh_panel.db"); err != nil {
		log.Fatalf("Erro ao inicializar banco de dados: %v", err)
	}

	// Se a flag --run-apis estiver presente, roda apenas os serviços e fica em loop
	if *runApis {
		startBackgroundServices()
		// Loop infinito para manter o serviço systemd rodando
		select {}
	}

	// Modo Normal (Terminal Interativo)
	system.EnsureClipboardTools()
	
	// Tenta garantir que o serviço de background esteja rodando
	_ = system.EnsurePainelService()

	// Instalar monitor PAM silenciosamente
	_ = system.InstallSSHMonitor()

	// Iniciar atualização de estatísticas em tempo real
	go startStatsMonitor()

	for {
		showMainMenu()
	}
}

// startBackgroundServices inicia todos os processos que devem persistir
func startBackgroundServices() {
	// 1. Monitor PAM
	_ = system.InstallSSHMonitor()

	// 2. Limitador de Conexões
	go runConnectionLimiter()

	// 3. Monitor de Torrent
	if system.GetTorrentStatus() {
		_ = system.EnableTorrentProtection()
		go system.StartTorrentMonitor()
	}

	// 4. Monitor de Expiração
	go runExpirationMonitor()

	// 5. APIs e Docs (Iniciam se estiverem configuradas para tal no DB ou por padrão)
	// Como o usuário quer que elas persistam, vamos garantir que iniciem aqui
	_ = system.StartExternalAPI()
	_ = system.StartMonitorAPI()
	_ = system.StartRegistrosAPI()
	_ = system.StartServidorAPI()
	_ = system.StartCheckerAPI()
	_ = system.StartDocsAPI()
}

func runExpirationMonitor() {
	for {
		system.CheckAllExpirations()
		time.Sleep(1 * time.Minute) // Verifica a cada 1 minuto
	}
}

func runConnectionLimiter() {
	for {
		// 1. Limitar Usuários SSH
		users, _ := db.GetUsers()
		for _, u := range users {
			if u.Limit > 0 {
				system.EnforceSSHLimit(u.Username, u.Limit)
			}
		}

		// 2. Limitar Usuários Xray
		xrayUsers, _ := db.GetXrayUsers()
		for _, u := range xrayUsers {
			if u.ConnectionLimit > 0 {
				system.EnforceXrayLimit(u.Username, u.UUID, u.ConnectionLimit)
			}
		}

		time.Sleep(5 * time.Second) // Verifica a cada 5 segundos
	}
}

func startStatsMonitor() {
	for {
		if isMainMenuActive {
			// Update dynamic parts
			cpu := system.GetCPUUsage()
			percent, used, total := system.GetRAMUsage()
			diskPercent, diskUsed, diskTotal, diskFree := system.GetDiskUsage()
			download, upload := system.GetNetworkSpeed()

			if currentServerInfo != nil {
				currentServerInfo.CPUUsage = cpu
				currentServerInfo.RAMPercent = percent
				currentServerInfo.RAMUsed = used
				currentServerInfo.RAMTotal = total
				currentServerInfo.DiskPercent = diskPercent
				currentServerInfo.DiskUsed = diskUsed
				currentServerInfo.DiskTotal = diskTotal
				currentServerInfo.DiskFree = diskFree
				currentServerInfo.NetDownload = download
				currentServerInfo.NetUpload = upload

				// Se o menu principal estiver ativo, atualiza o topo da tela
				if isMainMenuActive {
					refreshHeaderStats()
				}
			}
		}
		time.Sleep(1 * time.Second)
	}
}

func refreshHeaderStats() {
	if currentServerInfo == nil {
		return
	}

	// Salva posição do cursor
	fmt.Print("\033[s")

	// Move para a linha da HORA (linha 7)
	fmt.Print("\033[7;1H")
	ui.FormatLine(fmt.Sprintf("%sHORA:%s %s", ui.Bold, ui.Reset, system.GetNowBrasilia().Format("15:04:05")))

	// CPU e DISCO (linha 9)
	fmt.Print("\033[9;1H")
	cpuColor := ui.Green
	if currentServerInfo.CPUUsage > 80 {
		cpuColor = ui.Red
	} else if currentServerInfo.CPUUsage > 50 {
		cpuColor = ui.Yellow
	}
	diskColor := ui.Green
	if currentServerInfo.DiskPercent > 80 {
		diskColor = ui.Red
	} else if currentServerInfo.DiskPercent > 50 {
		diskColor = ui.Yellow
	}
	cpuStr := fmt.Sprintf("%.0f%%", currentServerInfo.CPUUsage)
	diskStr := fmt.Sprintf("%.0f%%", currentServerInfo.DiskPercent)
	ui.FormatLine(fmt.Sprintf("== %sCPU:%s %s%-7s%s ==   == %sDISCO:%s %s%-7s%s ==",
		ui.Bold, ui.Reset, cpuColor, cpuStr, ui.Reset,
		ui.Bold, ui.Reset, diskColor, diskStr, ui.Reset))

	// RAM e LIVRE (linha 10)
	fmt.Print("\033[10;1H")
	ramColor := ui.Green
	if currentServerInfo.RAMPercent > 80 {
		ramColor = ui.Red
	} else if currentServerInfo.RAMPercent > 50 {
		ramColor = ui.Yellow
	}
	ramStr := fmt.Sprintf("%.0f%%", currentServerInfo.RAMPercent)
	freeStr := fmt.Sprintf("%.0f%%", 100-currentServerInfo.DiskPercent)
	ui.FormatLine(fmt.Sprintf("== %sRAM:%s %s%-7s%s ==   == %sLIVRE:%s %s%-7s%s ==",
		ui.Bold, ui.Reset, ramColor, ramStr, ui.Reset,
		ui.Bold, ui.Reset, ui.Green, freeStr, ui.Reset))

	// REDE (linha 12)
	fmt.Print("\033[12;1H")
	ui.FormatLine(fmt.Sprintf("%sREDE ↓:%s %-15s %sREDE ↑:%s %-15s",
		ui.Bold, ui.Reset, currentServerInfo.NetDownload,
		ui.Bold, ui.Reset, currentServerInfo.NetUpload))

	// Restaura posição do cursor
	fmt.Print("\033[u")
}

func showMainMenu() {
	isMainMenuActive = true
	ui.ClearScreen()

	// Server Info in Header
	info, _ := system.GetServerInfo()
	currentServerInfo = info

	ui.DrawLine()
	ui.FormatLine(ui.CenterText("PAINEL SSH INTEL", ui.PanelWidth-4))
	ui.DrawLine()
	ui.FormatLine(fmt.Sprintf("%sSISTEMA:%s %s", ui.Bold, ui.Reset, info.OS))
	ui.FormatLine(fmt.Sprintf("%sPROCESSADOR:%s %s", ui.Bold, ui.Reset, info.CPUModel))
	ui.FormatLine(fmt.Sprintf("%sNÚCLEOS:%s %d", ui.Bold, ui.Reset, info.VCPUs))
	ui.FormatLine(fmt.Sprintf("%sHORA:%s %s", ui.Bold, ui.Reset, system.GetNowBrasilia().Format("15:04:05")))
	ui.FormatLine("")

	// Layout em duas colunas (Render inicial)
	cpuColor := ui.Green
	if info.CPUUsage > 80 {
		cpuColor = ui.Red
	} else if info.CPUUsage > 50 {
		cpuColor = ui.Yellow
	}
	diskColor := ui.Green
	if info.DiskPercent > 80 {
		diskColor = ui.Red
	} else if info.DiskPercent > 50 {
		diskColor = ui.Yellow
	}
	cpuStr := fmt.Sprintf("%.0f%%", info.CPUUsage)
	diskStr := fmt.Sprintf("%.0f%%", info.DiskPercent)
	ui.FormatLine(fmt.Sprintf("== %sCPU:%s %s%-7s%s ==   == %sDISCO:%s %s%-7s%s ==",
		ui.Bold, ui.Reset, cpuColor, cpuStr, ui.Reset,
		ui.Bold, ui.Reset, diskColor, diskStr, ui.Reset))

	ramColor := ui.Green
	if info.RAMPercent > 80 {
		ramColor = ui.Red
	} else if info.RAMPercent > 50 {
		ramColor = ui.Yellow
	}
	ramStr := fmt.Sprintf("%.0f%%", info.RAMPercent)
	freeStr := fmt.Sprintf("%.0f%%", 100-info.DiskPercent)
	ui.FormatLine(fmt.Sprintf("== %sRAM:%s %s%-7s%s ==   == %sLIVRE:%s %s%-7s%s ==",
		ui.Bold, ui.Reset, ramColor, ramStr, ui.Reset,
		ui.Bold, ui.Reset, ui.Green, freeStr, ui.Reset))
	ui.FormatLine("")

	ui.FormatLine(fmt.Sprintf("%sREDE ↓:%s %-15s %sREDE ↑:%s %-15s",
		ui.Bold, ui.Reset, info.NetDownload,
		ui.Bold, ui.Reset, info.NetUpload))
	ui.DrawLine()

	// STATUS DE USUÁRIOS Section
	onlineCount := getRealOnlineCount()
	totalUsers, _ := db.GetTotalUsers()
	expiredCount, _ := db.GetExpiredUsersCount()

	ui.FormatLine(ui.CenterText("STATUS DE USUÁRIOS", ui.PanelWidth-4))
	ui.DrawLine()
	ui.FormatLine(fmt.Sprintf("%sONLINES:%s %s%d%s", ui.Bold, ui.Reset, ui.Green, onlineCount, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%sEXPIRADOS:%s %s%d%s", ui.Bold, ui.Reset, ui.Red, expiredCount, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%sTOTAL:%s %d", ui.Bold, ui.Reset, totalUsers))
	ui.DrawLine()

	// Novo formato em duas colunas (Reorganizado conforme pedido)
	ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s %-22s %s[ 09 ]%s CONEXÕES", ui.Yellow, ui.Reset, "CRIAR USUÁRIO SSH", ui.Yellow, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s %-22s %s[ 10 ]%s PROTEÇÃO TORRENT", ui.Yellow, ui.Reset, "CRIAR USUÁRIO TESTE", ui.Yellow, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s %-22s %s[ 11 ]%s BAD VPN (UDP)", ui.Yellow, ui.Reset, "LISTAR USUÁRIOS", ui.Yellow, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s %-22s %s[ 12 ]%s STATUS PROTEÇÃO", ui.Yellow, ui.Reset, "REMOVER USUÁRIO", ui.Yellow, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%s[ 05 ]%s %-22s %s[ 13 ]%s LOG DE ABUSO", ui.Yellow, ui.Reset, "ALTERAR SENHA", ui.Yellow, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%s[ 06 ]%s %-22s %s[ 14 ]%s RESETAR BANIMENTO", ui.Yellow, ui.Reset, "LIMITE DE CONEXÕES", ui.Yellow, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%s[ 07 ]%s %-22s %s[ 15 ]%s AUTO MENU", ui.Yellow, ui.Reset, "DATA DE EXPIRAÇÃO", ui.Yellow, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%s[ 08 ]%s %-22s %s[ 16 ]%s CHECKER USER API", ui.Yellow, ui.Reset, "MONITORAR ONLINE", ui.Yellow, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%s[ 17 ]%s %-22s %s[ 18 ]%s INFO DO SERVIDOR", ui.Yellow, ui.Reset, "MAIS OPÇÕES", ui.Yellow, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s %-22s", ui.Red, ui.Reset, "SAIR"))
	ui.DrawLine()

	choice := ui.GetInput("Escolha uma opção")
	isMainMenuActive = false

	switch choice {
	case "01", "1":
		handleCreateUser()
	case "02", "2":
		handleCreateTrialUser()
	case "03", "3":
		handleListUsers()
	case "04", "4":
		handleRemoveUser()
	case "05", "5":
		handleChangePassword()
	case "06", "6":
		handleSetLimit()
	case "07", "7":
		handleSetExpiration()
	case "08", "8":
		handleMonitorOnline()
	case "09", "9":
		handleConnectionsMenu()
	case "10":
		handleTorrentMenu()
	case "11":
		handleBadVPNMenu()
	case "12":
		handleProtectionStatus()
	case "13":
		handleAbuseLogs()
	case "14":
		handleResetBan()
	case "15":
		handleAutoMenu()
	case "16":
		handleCheckerUserMenu()
	case "17":
		handleMoreOptions()
	case "18":
		handleServerInfo(info)
	case "00", "0":
		ui.PrintSuccess("Saindo do sistema...")
		os.Exit(0)
	default:
		ui.PrintError("Opção inválida!")
		time.Sleep(1 * time.Second)
	}
}

func handleConnectionsMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("MENU DE CONEXÕES", ui.PanelWidth-4))
		ui.DrawLine()
		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s WEBSOCKET", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s SSL TUNNEL", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s DNSTT / SLOW-DNS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s XRAY TLS + XHTTP", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 05 ]%s OPEN VPN", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 06 ]%s HYSTERIA 2 (UDP)", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 07 ]%s V2RAY (VMESS)", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			handleWebSocketMenu()
		case "02", "2":
			handleSSLTunnelMenu()
		case "03", "3":
			handleDNSTTMenu()
		case "04", "4":
			handleXrayMenu()
		case "05", "5":
			handleOpenVPNMenu()
		case "06", "6":
			handleHysteriaMenu()
		case "07", "7":
			handleV2RayMenu()
		case "00", "0":
			return
		default:
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
		}
	}
}

func handleXrayMenu() {
	// Garantir configuração de logs e arquivos ao entrar no menu
	_ = system.EnsureXrayLogConfig()
	_ = system.CheckAndFixXrayLogs()

	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("XRAY TLS + XHTTP", ui.PanelWidth-4))
		ui.DrawLine()

		status, isActive := system.GetXrayStatusBool()
		host, _ := db.GetConfig("xray_host")
		sni, _ := db.GetConfig("xray_sni")
		bugHost, _ := db.GetConfig("xray_bughost")
		uuid, _ := db.GetConfig("xray_uuid")

		host2, _ := db.GetConfig("xray_host2")
		sni2, _ := db.GetConfig("xray_sni2")
		bugHost2, _ := db.GetConfig("xray_bughost2")

		// Exibir status com cor e ícone
		var statusLine string
		switch status {
		case "active":
			statusLine = fmt.Sprintf("%sSTATUS:%s   %s✔ %s%s", ui.Bold, ui.Reset, ui.Green, status, ui.Reset)
		case "activating":
			statusLine = fmt.Sprintf("%sSTATUS:%s   %s⏳ %s%s", ui.Bold, ui.Reset, ui.Yellow, status, ui.Reset)
		case "failed":
			statusLine = fmt.Sprintf("%sSTATUS:%s   %s✖ %s%s", ui.Bold, ui.Reset, ui.Red, status, ui.Reset)
		default:
			statusLine = fmt.Sprintf("%sSTATUS:%s   %s%s%s", ui.Bold, ui.Reset, ui.Red, status, ui.Reset)
		}
		_ = isActive
		ui.FormatLine(statusLine)
		ui.FormatLine(fmt.Sprintf("%sUUID:%s     %s", ui.Bold, ui.Reset, uuid))
		ui.FormatLine(fmt.Sprintf("%sPORTA:%s    443", ui.Bold, ui.Reset))

		ui.DrawLine()
		ui.FormatLine(fmt.Sprintf("%s[ CONFIGURAÇÃO 1 ]%s", ui.Bold, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%sHOST:%s     %s", ui.Bold, ui.Reset, host))
		ui.FormatLine(fmt.Sprintf("%sSNI:%s      %s%s%s", ui.Bold, ui.Reset, ui.Cyan, sni, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%sBUG HOST:%s %s%s%s", ui.Bold, ui.Reset, ui.Yellow, bugHost, ui.Reset))

		if host2 != "" {
			ui.DrawLine()
			ui.FormatLine(fmt.Sprintf("%s[ CONFIGURAÇÃO 2 ]%s", ui.Bold, ui.Reset))
			ui.FormatLine(fmt.Sprintf("%sHOST:%s     %s", ui.Bold, ui.Reset, host2))
			ui.FormatLine(fmt.Sprintf("%sSNI:%s      %s%s%s", ui.Bold, ui.Reset, ui.Cyan, sni2, ui.Reset))
			ui.FormatLine(fmt.Sprintf("%sBUG HOST:%s %s%s%s", ui.Bold, ui.Reset, ui.Yellow, bugHost2, ui.Reset))
		}
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s INSTALAR XRAY", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s STATUS DO SERVIÇO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s EXIBIR CONFIGURAÇÃO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s EDITAR HOST", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 05 ]%s DESATIVAR XRAY", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 06 ]%s EDITAR SNI", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 07 ]%s GERAR LINK VLESS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 08 ]%s VER USUÁRIOS UUID", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			handleInstallXray()
		case "02", "2":
			handleXrayStatus()
		case "03", "3":
			handleShowXrayConfig()
		case "04", "4":
			handleEditXrayHost()
		case "05", "5":
			handleDisableXray()
		case "06", "6":
			handleEditXraySNI()
		case "07", "7":
			handleGenerateVlessLink()
		case "08", "8":
			handleXrayUsersMenu()
		case "00", "0":
			return
		default:
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
		}
	}
}

func handleInstallXray() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("INSTALAR XRAY TLS + XHTTP", ui.PanelWidth-4))
	ui.DrawLine()

	// ── Configuração 1 ──────────────────────────────────────────────────────
	ui.FormatLine(ui.Yellow + "--- CONFIGURAÇÃO 1 ---" + ui.Reset)
	host := ui.GetInput("Digite o HOST 1 (ex: m.ofertas.tim.com.br)")
	if host == "" {
		ui.PrintError("HOST 1 não pode ser vazio!")
		pause()
		return
	}

	sni := ui.GetInput("Digite o SNI 1 (ex: www.tim.com.br)")
	if sni == "" {
		ui.PrintError("SNI 1 não pode ser vazio!")
		pause()
		return
	}

	bugHost := ui.GetInput("Digite o BUG HOST 1 / CDN (ex: mraqhhblnuy.map.azionedge.net)")
	if bugHost == "" {
		ui.PrintError("Bug Host 1 não pode ser vazio!")
		pause()
		return
	}

	// ── Configuração 2 (Opcional) ───────────────────────────────────────────
	ui.DrawLine()
	ui.FormatLine(ui.Yellow + "--- CONFIGURAÇÃO 2 (OPCIONAL) ---" + ui.Reset)
	ui.FormatLine("Deixe em branco se não quiser configurar a segunda Host.")
	host2 := ui.GetInput("Digite o HOST 2 (ex: webportals.cachefly.net)")
	sni2 := ""
	bugHost2 := ""

	if host2 != "" {
		sni2 = ui.GetInput("Digite o SNI 2 (ex: webportals.cachefly.net)")
		bugHost2 = ui.GetInput("Digite o BUG HOST 2 / CDN (ex: cdnipv6sisu.cachefly.net)")
	}

	// ── Verificar porta 443 antes de instalar ─────────────────────────────────
	if inUse, info := system.CheckPort443InUse(); inUse {
		ui.PrintWarning("⚠ Porta 443 em uso — será liberada automaticamente:")
		for _, line := range strings.Split(info, "\n") {
			if line != "" {
				ui.FormatLine(line)
			}
		}
	}

	// ── Instalar ──────────────────────────────────────────────────────────────
	ui.PrintWarning("⏳ Instalando XRAY-core e dependências... Aguarde.")
	uuid := system.GenerateUUID()
	err := system.InstallXrayExtended(host, sni, bugHost, host2, sni2, bugHost2, uuid)
	if err != nil {
		ui.PrintError(fmt.Sprintf("✖ Erro na instalação: %v", err))
		// Exibir logs de erro para diagnóstico
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("LOG XRAY", ui.PanelWidth-4))
		ui.DrawLine()
		logs := system.GetXrayLogs(20)
		for _, line := range strings.Split(logs, "\n") {
			if line != "" {
				ui.FormatLine(line)
			}
		}
		ui.DrawLine()
		pause()
		return
	}

	// ── Persistir no banco ────────────────────────────────────────────────────
	db.SetConfig("xray_host", host)
	db.SetConfig("xray_sni", sni)
	db.SetConfig("xray_bughost", bugHost)
	db.SetConfig("xray_uuid", uuid)

	if host2 != "" {
		db.SetConfig("xray_host2", host2)
		db.SetConfig("xray_sni2", sni2)
		db.SetConfig("xray_bughost2", bugHost2)
	} else {
		db.SetConfig("xray_host2", "")
		db.SetConfig("xray_sni2", "")
		db.SetConfig("xray_bughost2", "")
	}

	// ── Gerar e exibir links VLESS ─────────────────────────────────────────────
	link1 := system.GenerateVlessLink(uuid, host, sni, bugHost, false)

	ui.PrintSuccess("XRAY instalado com sucesso!")
	ui.DrawLine()
	ui.FormatLine(fmt.Sprintf("%s[ CONFIGURAÇÃO 1 ]%s", ui.Bold, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%sHOST:%s     %s", ui.Bold, ui.Reset, host))
	ui.FormatLine(fmt.Sprintf("%sSNI:%s      %s", ui.Bold, ui.Reset, sni))
	ui.FormatLine(fmt.Sprintf("%sBUG HOST:%s %s", ui.Bold, ui.Reset, bugHost))
	ui.FormatLine(ui.Green + link1 + ui.Reset)

	var link2 string
	if host2 != "" {
		ui.DrawLine()
		link2 = system.GenerateVlessLink(uuid, host2, sni2, bugHost2, true)
		ui.FormatLine(fmt.Sprintf("%s[ CONFIGURAÇÃO 2 ]%s", ui.Bold, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%sHOST:%s     %s", ui.Bold, ui.Reset, host2))
		ui.FormatLine(fmt.Sprintf("%sSNI:%s      %s", ui.Bold, ui.Reset, sni2))
		ui.FormatLine(fmt.Sprintf("%sBUG HOST:%s %s", ui.Bold, ui.Reset, bugHost2))
		ui.FormatLine(ui.Green + link2 + ui.Reset)
	}

	ui.DrawLine()
	ui.FormatLine(fmt.Sprintf("%sPORTA:%s    443", ui.Bold, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%sUUID:%s     %s", ui.Bold, ui.Reset, uuid))
	ui.FormatLine(fmt.Sprintf("%sTLS:%s      ATIVO", ui.Bold, ui.Reset))
	ui.DrawLine()

	ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s COPIAR LINK 1", ui.Yellow, ui.Reset))
	if host2 != "" {
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s COPIAR LINK 2", ui.Yellow, ui.Reset))
	}
	ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
	ui.DrawLine()

	choice := ui.GetInput("Escolha uma opção")
	switch choice {
	case "1", "01":
		if system.CopyToClipboard(link1) {
			ui.PrintSuccess("Link 1 copiado!")
		} else {
			ui.PrintWarning("Copie manualmente o link 1 acima.")
		}
		time.Sleep(2 * time.Second)
	case "2", "02":
		if host2 != "" {
			if system.CopyToClipboard(link2) {
				ui.PrintSuccess("Link 2 copiado!")
			} else {
				ui.PrintWarning("Copie manualmente o link 2 acima.")
			}
			time.Sleep(2 * time.Second)
		}
	}
}

func handleV2RayMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("MENU V2RAY (VMESS)", ui.PanelWidth-4))
		ui.DrawLine()

		status, isActive := system.GetV2RayStatus()
		port, _ := db.GetConfig("v2ray_port")

		if isActive {
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Green, status, ui.Reset))
			ui.FormatLine(fmt.Sprintf("%sPORTA:%s  %s", ui.Bold, ui.Reset, port))
		} else {
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Red, status, ui.Reset))
		}
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s INSTALAR V2RAY", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s ADICIONAR USUÁRIO VMESS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s LISTAR USUÁRIOS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s REMOVER USUÁRIO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 05 ]%s DESINSTALAR V2RAY", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			handleInstallV2Ray()
		case "02", "2":
			handleAddV2RayUser()
		case "03", "3":
			handleListV2RayUsers()
		case "04", "4":
			handleRemoveV2RayUser()
		case "05", "5":
			handleUninstallV2Ray()
		case "00", "0":
			return
		default:
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
		}
	}
}

func handleInstallV2Ray() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("INSTALAR V2RAY", ui.PanelWidth-4))
	ui.DrawLine()

	portStr := ui.GetInput("Digite a porta para o V2Ray (ex: 8080)")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		ui.PrintError("Porta inválida!")
		pause()
		return
	}

	ui.PrintWarning("⏳ Instalando V2Ray... Isso pode levar alguns minutos.")
	if err := system.InstallV2Ray(port); err != nil {
		ui.PrintError(fmt.Sprintf("Erro na instalação: %v", err))
	} else {
		db.SetConfig("v2ray_port", portStr)
		ui.PrintSuccess("V2Ray instalado e iniciado com sucesso!")
	}
	ui.DrawLine()
	pause()
}

func handleAddV2RayUser() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("ADICIONAR USUÁRIO V2RAY", ui.PanelWidth-4))
	ui.DrawLine()

	email := ui.GetInput("Digite o nome do usuário (email)")
	if email == "" {
		return
	}

	uuid := system.GenerateUUID()
	ui.PrintWarning(fmt.Sprintf("⏳ Gerando UUID para %s...", email))

	if err := system.AddV2RayUser(email, uuid); err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao adicionar usuário: %v", err))
	} else {
		ip := system.GetPublicIP()
		portStr, _ := db.GetConfig("v2ray_port")
		port, _ := strconv.Atoi(portStr)
		link := system.GenerateVMessLink(email, uuid, ip, port)

		ui.PrintSuccess("Usuário V2Ray adicionado!")
		ui.FormatLine(fmt.Sprintf("Email: %s", email))
		ui.FormatLine(fmt.Sprintf("UUID:  %s", uuid))
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("LINK VMESS", ui.PanelWidth-4))
		ui.DrawLine()
		ui.FormatLine(ui.Green + link + ui.Reset)
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s COPIAR LINK", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		if choice == "1" || choice == "01" {
			if system.CopyToClipboard(link) {
				ui.PrintSuccess("Link copiado para o clipboard!")
			} else {
				ui.PrintWarning("Não foi possível copiar automaticamente. Copie manualmente o link acima.")
			}
			time.Sleep(2 * time.Second)
		}
	}
	ui.DrawLine()
}

func handleListV2RayUsers() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("USUÁRIOS V2RAY", ui.PanelWidth-4))
	ui.DrawLine()

	cfg, err := system.LoadV2RayConfig()
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao carregar config: %v", err))
		pause()
		return
	}

	found := false
	for _, in := range cfg.Inbounds {
		for _, client := range in.Settings.Clients {
			ui.FormatLine(fmt.Sprintf("%s• %-15s%s | UUID: %s", ui.Yellow, client.Email, ui.Reset, client.ID))
			found = true
		}
	}

	if !found {
		ui.FormatLine("Nenhum usuário cadastrado.")
	}

	ui.DrawLine()
	pause()
}

func handleRemoveV2RayUser() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("REMOVER USUÁRIO V2RAY", ui.PanelWidth-4))
	ui.DrawLine()

	email := ui.GetInput("Digite o email do usuário para remover")
	if email == "" {
		return
	}

	if err := system.RemoveV2RayUser(email); err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao remover: %v", err))
	} else {
		ui.PrintSuccess("Usuário removido do V2Ray.")
	}
	ui.DrawLine()
	pause()
}

func handleUninstallV2Ray() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("DESINSTALAR V2RAY", ui.PanelWidth-4))
	ui.DrawLine()

	confirm := ui.GetInput("Deseja realmente remover o V2Ray do sistema? (s/N)")
	if strings.ToLower(confirm) != "s" {
		return
	}

	// Script oficial de remoção
	ui.PrintWarning("⏳ Removendo V2Ray...")
	exec.Command("systemctl", "stop", "v2ray").Run()
	exec.Command("systemctl", "disable", "v2ray").Run()
	os.Remove("/etc/systemd/system/v2ray.service")
	os.RemoveAll("/etc/v2ray")
	os.Remove("/usr/bin/v2ray/v2ray")

	db.SetConfig("v2ray_port", "")
	ui.PrintSuccess("V2Ray desinstalado completamente!")
	ui.DrawLine()
	pause()
}

func handleAutoMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("AUTO MENU (INICIALIZAÇÃO)", ui.PanelWidth-4))
		ui.DrawLine()

		status := system.GetAutoMenuStatus()
		statusStr := ui.Red + "DESATIVADO" + ui.Reset
		if status {
			statusStr = ui.Green + "ATIVADO" + ui.Reset
		}

		ui.FormatLine(fmt.Sprintf("%sSTATUS ATUAL:%s %s", ui.Bold, ui.Reset, statusStr))
		ui.FormatLine("")
		ui.FormatLine("Quando ativado, o painel abrirá")
		ui.FormatLine("automaticamente ao logar via SSH.")
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s ATIVAR AUTO MENU", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s DESATIVAR AUTO MENU", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			if err := system.EnableAutoMenu(); err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao ativar: %v", err))
			} else {
				ui.PrintSuccess("AUTO MENU ativado com sucesso!")
			}
			pause()
		case "02", "2":
			if err := system.DisableAutoMenu(); err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao desativar: %v", err))
			} else {
				ui.PrintSuccess("AUTO MENU desativado com sucesso!")
			}
			pause()
		case "00", "0":
			return
		}
	}
}

func handleCheckerUserMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("CHECKER USER API", ui.PanelWidth-4))
		ui.DrawLine()

		isRunning := system.GetCheckerStatus()
		statusStr := ui.Red + "INATIVO" + ui.Reset
		if isRunning {
			statusStr = ui.Green + "ATIVO" + ui.Reset
		}

		ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s", ui.Bold, ui.Reset, statusStr))
		ui.FormatLine(fmt.Sprintf("%sPORTA:%s  5757", ui.Bold, ui.Reset))
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s ATIVAR CHECKER", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s DESATIVAR CHECKER", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s VER URL", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s STATUS DETALHADO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			if err := system.StartCheckerAPI(); err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao ativar: %v", err))
			} else {
				ui.PrintSuccess("API ATIVADA COM SUCESSO!")
				ip := system.GetPublicIP()
				ui.FormatLine(fmt.Sprintf("URL: http://%s:5757/v3/checkeruser", ip))
			}
			pause()
		case "02", "2":
			if err := system.StopCheckerAPI(); err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao desativar: %v", err))
			} else {
				ui.PrintSuccess("API DESATIVADA!")
			}
			pause()
		case "03", "3":
			ip := system.GetPublicIP()
			ui.FormatLine(fmt.Sprintf("URL: http://%s:5757/v3/checkeruser", ip))
			ui.FormatLine("Exemplo: ?user=nome_usuario")
			pause()
		case "04", "4":
			reqs := system.GetCheckerRequests()
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s", ui.Bold, ui.Reset, statusStr))
			ui.FormatLine(fmt.Sprintf("%sPORTA:%s  5757", ui.Bold, ui.Reset))
			ui.FormatLine(fmt.Sprintf("%sREQUISIÇÕES:%s %d", ui.Bold, ui.Reset, reqs))
			pause()
		case "00", "0":
			return
		}
	}
}

func handleBadVPNMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("MENU BAD VPN (UDPGW)", ui.PanelWidth-4))
		ui.DrawLine()

		activePorts := system.ListActiveBadVPNPorts()
		statusStr := ui.Red + "INATIVO" + ui.Reset
		if len(activePorts) > 0 {
			statusStr = ui.Green + "ATIVO" + ui.Reset
		}

		ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s", ui.Bold, ui.Reset, statusStr))
		if len(activePorts) > 0 {
			ui.FormatLine(fmt.Sprintf("%sPORTAS ATIVAS:%s %s", ui.Bold, ui.Reset, strings.Join(activePorts, ", ")))
		}
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s ATIVAR BADVPN (PORTAS PADRÃO)", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s GERENCIAR PORTAS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s STATUS DETALHADO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s MONITORAR USO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 05 ]%s DESATIVAR TUDO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			handleActivateBadVPN()
		case "02", "2":
			handleManageBadVPNPorts()
		case "03", "3":
			handleDetailedBadVPNStatus()
		case "04", "4":
			handleMonitorBadVPN()
		case "05", "5":
			handleDeactivateBadVPN()
		case "00", "0":
			return
		default:
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
		}
	}
}

func handleActivateBadVPN() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("ATIVAR BAD VPN", ui.PanelWidth-4))
	ui.DrawLine()
	ui.FormatLine(fmt.Sprintf("%s⏳ Configurando ambiente e portas padrão (7300, 7200, 7100)...%s", ui.Yellow, ui.Reset))

	if err := system.InstallBadVPNPro(); err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao instalar: %v", err))
		ui.DrawLine()
		pause()
		return
	}

	defaultPorts := []int{7300, 7200, 7100}
	for _, p := range defaultPorts {
		system.AddBadVPNPort(p)
	}

	ui.PrintSuccess("BADVPN ativado com sucesso em múltiplas portas!")
	ui.DrawLine()
	pause()
}

func handleManageBadVPNPorts() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("GERENCIAR PORTAS BADVPN", ui.PanelWidth-4))
		ui.DrawLine()
		active := system.ListActiveBadVPNPorts()
		ui.FormatLine(fmt.Sprintf("%sPORTAS ATIVAS:%s %s", ui.Bold, ui.Reset, strings.Join(active, ", ")))
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s ADICIONAR PORTA", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s REMOVER PORTA", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			pStr := ui.GetInput("Digite a porta (1-65535)")
			p, _ := strconv.Atoi(pStr)
			if err := system.AddBadVPNPort(p); err != nil {
				ui.PrintError(fmt.Sprintf("Erro: %v", err))
			} else {
				ui.PrintSuccess("Porta adicionada!")
			}
			time.Sleep(1 * time.Second)
		case "02", "2":
			pStr := ui.GetInput("Digite a porta para remover")
			p, _ := strconv.Atoi(pStr)
			if err := system.RemoveBadVPNPort(p); err != nil {
				ui.PrintError(fmt.Sprintf("Erro: %v", err))
			} else {
				ui.PrintSuccess("Porta removida!")
			}
			time.Sleep(1 * time.Second)
		case "00", "0":
			return
		}
	}
}

func handleDetailedBadVPNStatus() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("STATUS DETALHADO BAD VPN", ui.PanelWidth-4))
	ui.DrawLine()
	active := system.ListActiveBadVPNPorts()

	if len(active) == 0 {
		ui.FormatLine(ui.Yellow + "Nenhuma porta ativa." + ui.Reset)
	} else {
		ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %sATIVO%s", ui.Bold, ui.Reset, ui.Green, ui.Reset))
		ui.FormatLine("")
		ui.FormatLine("Serviços rodando:")
		for _, p := range active {
			ui.FormatLine(fmt.Sprintf(" - badvpn@%s.service: %s✔ OK%s", p, ui.Green, ui.Reset))
		}
	}
	ui.DrawLine()
	pause()
}

func handleMonitorBadVPN() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("MONITORAR USO BAD VPN (UDP)", ui.PanelWidth-4))
	ui.DrawLine()
	usage := system.GetBadVPNUsage()

	if len(usage) == 0 {
		ui.FormatLine("Nenhum tráfego UDP detectado no momento.")
	} else {
		ui.FormatLine(fmt.Sprintf("%-25s | %-10s", "Usuário", "Conexões"))
		ui.DrawLine()
		for user, count := range usage {
			ui.FormatLine(fmt.Sprintf("%-25s | %-10d", user, count))
		}
	}
	ui.DrawLine()
	pause()
}

func handleDeactivateBadVPN() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("DESATIVAR TUDO", ui.PanelWidth-4))
	ui.DrawLine()
	confirm := ui.GetInput("Deseja realmente parar todas as instâncias do BadVPN? (s/N)")
	if strings.ToLower(confirm) == "s" {
		system.StopAllBadVPN()
		ui.PrintSuccess("Todas as instâncias foram desativadas.")
	}
	ui.DrawLine()
	pause()
}

func handleProtectionStatus() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("STATUS PROTEÇÃO (ULTRA)", ui.PanelWidth-4))
	ui.DrawLine()

	isBlocked := system.GetTorrentStatus()
	banned, _ := db.GetTotalBannedUsers()
	attempts, _ := db.GetTotalAbuseAttemptsToday()

	if isBlocked {
		ui.FormatLine(fmt.Sprintf("%sTORRENT BLOCK: %s✔ ATIVO%s", ui.Bold, ui.Green, ui.Reset))
	} else {
		ui.FormatLine(fmt.Sprintf("%sTORRENT BLOCK: %s✖ INATIVO%s", ui.Bold, ui.Red, ui.Reset))
	}

	ui.FormatLine(fmt.Sprintf("%sUSUÁRIOS BANIDOS: %s%d%s", ui.Bold, ui.Red, banned, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%sTENTATIVAS HOJE: %s%d%s", ui.Bold, ui.Yellow, attempts, ui.Reset))
	ui.DrawLine()
	pause()
}

func handleAbuseLogs() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("LOG DE ABUSO", ui.PanelWidth-4))
	ui.DrawLine()

	logs, err := db.GetAbuseLogs()
	if err != nil || len(logs) == 0 {
		ui.FormatLine("Nenhum log de abuso encontrado.")
	} else {
		ui.FormatLine(fmt.Sprintf("%-12s | %-12s | %-4s | %-8s", "Usuário", "IP", "Tent", "Status"))
		ui.DrawLine()
		for _, log := range logs {
			status := log["status"].(string)
			color := ui.Reset
			if status == "banned" {
				color = ui.Red
			}
			line := fmt.Sprintf("%-12s | %-12s | %-4d | %s%-8s%s",
				log["username"], log["ip"], log["attempts"], color, status, ui.Reset)
			ui.FormatLine(line)
		}
	}
	ui.DrawLine()
	pause()
}

func handleResetBan() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("RESETAR BANIMENTO", ui.PanelWidth-4))
	ui.DrawLine()

	username := ui.GetInput("Digite o nome do usuário para resetar")
	if username == "" {
		return
	}

	// 1. Reset no banco
	err := db.ResetUserAbuse(username)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao resetar banco: %v", err))
		ui.DrawLine()
		pause()
		return
	}

	// 2. Desbloquear conta Linux
	exec.Command("usermod", "-U", username).Run()

	ui.PrintSuccess(fmt.Sprintf("Usuário %s resetado com sucesso!", username))
	ui.DrawLine()
	pause()
}

func handleXrayStatus() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("STATUS DO XRAY", ui.PanelWidth-4))
	ui.DrawLine()

	status, isActive := system.GetXrayStatusBool()

	// Ícone e cor por estado
	var statusLine string
	switch status {
	case "active":
		statusLine = fmt.Sprintf("%sSTATUS:%s %s✔ XRAY rodando corretamente (%s)%s", ui.Bold, ui.Reset, ui.Green, status, ui.Reset)
	case "activating":
		statusLine = fmt.Sprintf("%sSTATUS:%s %s⏳ Verificando status... (%s)%s", ui.Bold, ui.Reset, ui.Yellow, status, ui.Reset)
	case "failed":
		statusLine = fmt.Sprintf("%sSTATUS:%s %s✖ XRAY falhou ao iniciar (%s)%s", ui.Bold, ui.Reset, ui.Red, status, ui.Reset)
	default:
		statusLine = fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Red, status, ui.Reset)
	}

	ui.FormatLine(statusLine)
	ui.FormatLine(fmt.Sprintf("%sPORTA:%s   443", ui.Bold, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%sSERVIÇO:%s xray.service", ui.Bold, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%sTLS:%s     ATIVO", ui.Bold, ui.Reset))

	// Diagnóstico adicional
	if inUse, info := system.CheckPort443InUse(); inUse && !isActive {
		ui.DrawLine()
		ui.FormatLine(fmt.Sprintf("%s⚠ PORTA 443 EM USO:%s", ui.Red, ui.Reset))
		ui.FormatLine(info)
	}

	if err := system.CheckCertsExist(); err != nil {
		ui.FormatLine(fmt.Sprintf("⚠ Certificado: %v", err))
	} else {
		ui.FormatLine(fmt.Sprintf("%s✔ Certificados encontrados%s", ui.Green, ui.Reset))
	}

	// Exibir logs se falhou
	if status == "failed" || (!isActive && status != "inactive") {
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("LOG XRAY", ui.PanelWidth-4))
		ui.DrawLine()
		logs := system.GetXrayLogs(10)
		for _, line := range strings.Split(logs, "\n") {
			if line != "" {
				ui.FormatLine(line)
			}
		}
	}

	ui.DrawLine()
	pause()
}

func handleShowXrayConfig() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("CONFIGURAÇÃO XRAY", ui.PanelWidth-4))
	ui.DrawLine()

	host, _ := db.GetConfig("xray_host")
	sni, _ := db.GetConfig("xray_sni")
	bugHost, _ := db.GetConfig("xray_bughost")
	uuid, _ := db.GetConfig("xray_uuid")

	if host == "" || uuid == "" {
		ui.FormatLine(ui.Red + "XRAY não está configurado. Use [ 01 ] INSTALAR XRAY primeiro." + ui.Reset)
	} else {
		ui.FormatLine(fmt.Sprintf("%sHOST:%s     %s", ui.Bold, ui.Reset, host))
		ui.FormatLine(fmt.Sprintf("%sSNI:%s      %s%s%s", ui.Bold, ui.Reset, ui.Cyan, sni, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%sBUG HOST:%s %s%s%s", ui.Bold, ui.Reset, ui.Yellow, bugHost, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%sPORTA:%s    443", ui.Bold, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%sUUID:%s     %s", ui.Bold, ui.Reset, uuid))
		ui.FormatLine(fmt.Sprintf("%sTIPO:%s     XHTTP + TLS", ui.Bold, ui.Reset))

		ui.DrawLine()
		ui.FormatLine(ui.CenterText("LINK VLESS", ui.PanelWidth-4))
		ui.DrawLine()
		link := system.GenerateVlessLink(uuid, host, sni, bugHost, false)
		ui.FormatLine(ui.Green + link + ui.Reset)
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s COPIAR LINK", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		if choice == "1" || choice == "01" {
			if system.CopyToClipboard(link) {
				ui.PrintSuccess("Link copiado para o clipboard!")
			} else {
				ui.PrintWarning("Não foi possível copiar automaticamente. Copie manualmente o link acima.")
			}
			time.Sleep(2 * time.Second)
		} else {
			return
		}
	}
}

func handleEditXrayHost() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("EDITAR HOST XRAY", ui.PanelWidth-4))
	ui.DrawLine()

	currentHost, _ := db.GetConfig("xray_host")
	if currentHost != "" {
		ui.FormatLine(fmt.Sprintf("%sHOST ATUAL:%s %s", ui.Bold, ui.Reset, currentHost))
		ui.DrawLine()
	}

	newHost := ui.GetInput("Digite o novo HOST (ex: m.ofertas.tim.com.br)")
	if newHost == "" {
		ui.PrintError("HOST não pode ser vazio!")
		ui.DrawLine()
		pause()
		return
	}

	ui.PrintWarning("⏳ Atualizando HOST e regenerando certificado...")
	err := system.UpdateXrayHost(newHost)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao atualizar HOST: %v", err))
	} else {
		db.SetConfig("xray_host", newHost)
		ui.PrintSuccess("HOST atualizado e serviço reiniciado!")
		ui.DrawLine()
		ui.FormatLine(fmt.Sprintf("%sHOST ANTERIOR:%s %s", ui.Bold, ui.Reset, currentHost))
		ui.FormatLine(fmt.Sprintf("%sHOST ATUAL:%s    %s%s%s", ui.Bold, ui.Reset, ui.Green, newHost, ui.Reset))
	}
	ui.DrawLine()
	pause()
}

func handleDisableXray() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("DESATIVAR XRAY", ui.PanelWidth-4))
	ui.DrawLine()

	confirm := ui.GetInput("Deseja realmente desativar o XRAY? (s/N)")
	if strings.ToLower(confirm) != "s" {
		return
	}

	err := system.DisableXray()
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao desativar: %v", err))
	} else {
		ui.PrintSuccess("XRAY desativado com sucesso!")
	}
	ui.DrawLine()
	pause()
}

// handleEditXraySNI permite atualizar o campo SNI (serverName no TLS)
// no config.json do Xray sem precisar reinstalar.
func handleEditXraySNI() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("EDITAR SNI DO XRAY", ui.PanelWidth-4))
	ui.DrawLine()

	currentSNI, _ := db.GetConfig("xray_sni")
	if currentSNI == "" {
		currentSNI = "(não configurado)"
	}
	ui.FormatLine(fmt.Sprintf("%sSNI ATUAL:%s %s%s%s", ui.Bold, ui.Reset, ui.Cyan, currentSNI, ui.Reset))
	ui.DrawLine()

	newSNI := ui.GetInput("Digite o novo SNI (ex: www.tim.com.br)")
	if newSNI == "" {
		ui.PrintError("SNI não pode ser vazio!")
		ui.DrawLine()
		pause()
		return
	}

	ui.PrintWarning("⏳ Atualizando SNI e reiniciando Xray...")
	if err := system.UpdateXraySNI(newSNI); err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao atualizar SNI: %v", err))
		ui.DrawLine()
		pause()
		return
	}

	db.SetConfig("xray_sni", newSNI)

	ui.PrintSuccess("SNI atualizado com sucesso!")
	ui.DrawLine()
	ui.FormatLine(fmt.Sprintf("%sSNI ANTERIOR:%s %s", ui.Bold, ui.Reset, currentSNI))
	ui.FormatLine(fmt.Sprintf("%sSNI ATUAL:%s    %s%s%s", ui.Bold, ui.Reset, ui.Green, newSNI, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%sServiço Xray reiniciado automaticamente.%s", ui.Green, ui.Reset))
	ui.DrawLine()
	pause()
}

// handleGenerateVlessLink exibe o link VLESS gerado com os dados atuais
// e oferece opção de simular cópia para clipboard.
func handleGenerateVlessLink() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("LINK VLESS", ui.PanelWidth-4))
		ui.DrawLine()

		host, _ := db.GetConfig("xray_host")
		sni, _ := db.GetConfig("xray_sni")
		bugHost, _ := db.GetConfig("xray_bughost")
		uuid, _ := db.GetConfig("xray_uuid")

		host2, _ := db.GetConfig("xray_host2")
		sni2, _ := db.GetConfig("xray_sni2")
		bugHost2, _ := db.GetConfig("xray_bughost2")

		if host == "" || uuid == "" {
			ui.FormatLine(ui.Red + "XRAY não está configurado. Use [ 01 ] INSTALAR XRAY primeiro." + ui.Reset)
			ui.DrawLine()
			pause()
			return
		}

		link1 := system.GenerateVlessLink(uuid, host, sni, bugHost, false)

		ui.FormatLine(fmt.Sprintf("%s[ LINK 1 ]%s", ui.Bold, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%sHOST:%s     %s", ui.Bold, ui.Reset, host))
		ui.FormatLine(fmt.Sprintf("%sSNI:%s      %s", ui.Bold, ui.Reset, sni))
		ui.FormatLine(fmt.Sprintf("%sBUG HOST:%s %s", ui.Bold, ui.Reset, bugHost))
		ui.FormatLine(ui.Green + link1 + ui.Reset)

		var link2 string
		if host2 != "" {
			ui.DrawLine()
			link2 = system.GenerateVlessLink(uuid, host2, sni2, bugHost2, true)
			ui.FormatLine(fmt.Sprintf("%s[ LINK 2 ]%s", ui.Bold, ui.Reset))
			ui.FormatLine(fmt.Sprintf("%sHOST:%s     %s", ui.Bold, ui.Reset, host2))
			ui.FormatLine(fmt.Sprintf("%sSNI:%s      %s", ui.Bold, ui.Reset, sni2))
			ui.FormatLine(fmt.Sprintf("%sBUG HOST:%s %s", ui.Bold, ui.Reset, bugHost2))
			ui.FormatLine(ui.Green + link2 + ui.Reset)
		}

		ui.DrawLine()
		ui.FormatLine(fmt.Sprintf("%sUUID:%s     %s", ui.Bold, ui.Reset, uuid))
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s COPIAR LINK 1", ui.Yellow, ui.Reset))
		if host2 != "" {
			ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s COPIAR LINK 2", ui.Yellow, ui.Reset))
		}
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			if system.CopyToClipboard(link1) {
				ui.PrintSuccess("Link 1 copiado!")
			} else {
				ui.PrintWarning("Copie manualmente o link 1 acima.")
			}
			pause()
		case "02", "2":
			if host2 != "" {
				if system.CopyToClipboard(link2) {
					ui.PrintSuccess("Link 2 copiado!")
				} else {
					ui.PrintWarning("Copie manualmente o link 2 acima.")
				}
				pause()
			}
		case "00", "0":
			return
		default:
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
		}
	}
}

// handleXrayUsersMenu exibe a lista de todos os usuários XRAY cadastrados,
// com status, conexões ativas e data de expiração.
func handleXrayUsersMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("USUÁRIOS XRAY — UUID", ui.PanelWidth-4))
		ui.DrawLine()

		users, err := db.GetXrayUsers()
		if err != nil {
			ui.FormatLine(fmt.Sprintf("%sErro ao carregar usuários XRAY: %v%s", ui.Red, err, ui.Reset))
			ui.DrawLine()
			pause()
			return
		}

		totalConns := system.GetXrayTotalConnections()
		ui.FormatLine(fmt.Sprintf("%sCONEXÕES TOTAIS:%s %s%d%s", ui.Bold, ui.Reset, ui.Cyan, totalConns, ui.Reset))
		ui.DrawLine()

		if len(users) == 0 {
			ui.FormatLine(ui.Yellow + "Nenhum usuário XRAY cadastrado." + ui.Reset)
			ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
			ui.DrawLine()
			choice := ui.GetInput("Escolha uma opção")
			if choice == "00" || choice == "0" {
				return
			}
			continue
		}

		for i, u := range users {
			details := system.GetXrayUserOnlineDetails(u.Username, u.UUID)
			conns := len(details)

			var statusIcon string
			if u.Status == "active" {
				statusIcon = fmt.Sprintf("%s✔ ativo%s", ui.Green, ui.Reset)
			} else if u.Status == "expired" {
				statusIcon = fmt.Sprintf("%s✖ expirado%s", ui.Red, ui.Reset)
			} else {
				statusIcon = fmt.Sprintf("%s✖ suspenso%s", ui.Red, ui.Reset)
			}

			expStr := "Nunca"
			if !u.ExpiresAt.IsZero() {
				expStr = u.ExpiresAt.Format("02/01/2006")
				if u.IsExpired() {
					expStr = fmt.Sprintf("%s%s (EXPIRADO)%s", ui.Red, expStr, ui.Reset)
				}
			}

			var onlineStr string
			if conns > 0 {
				duration := "00:00:00"
				if len(details) > 0 {
					duration = system.FormatDuration(details[0].Duration)
				}
				onlineStr = fmt.Sprintf("%s✔ %d conexões [%s]%s", ui.Green, conns, duration, ui.Reset)
			} else {
				onlineStr = fmt.Sprintf("%s✖ offline%s", ui.Red, ui.Reset)
			}

			ui.FormatLine(fmt.Sprintf("%s[%02d]%s %s%s%s", ui.Yellow, i+1, ui.Reset, ui.Bold, u.Username, ui.Reset))
			ui.FormatLine(fmt.Sprintf("     STATUS: %s", statusIcon))
			ui.FormatLine(fmt.Sprintf("     ONLINE: %s", onlineStr))
			ui.FormatLine(fmt.Sprintf("     EXPIRA: %s", expStr))
			ui.FormatLine("")
		}

		ui.DrawLine()
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Selecione um usuário (número) ou 00 para voltar")
		if choice == "00" || choice == "0" {
			return
		}

		idx, err := strconv.Atoi(choice)
		if err != nil || idx < 1 || idx > len(users) {
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
			continue
		}

		handleManageXrayUser(users[idx-1])
	}
}

// handleManageXrayUser exibe o menu de gerenciamento de um usuário XRAY específico.
func handleManageXrayUser(user models.XrayUser) {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("GERENCIAR USUÁRIO XRAY", ui.PanelWidth-4))
		ui.DrawLine()

		details := system.GetXrayUserOnlineDetails(user.Username, user.UUID)
		conns := len(details)

		var statusStr string
		if user.Status == "active" {
			statusStr = fmt.Sprintf("%s✔ ativo%s", ui.Green, ui.Reset)
		} else if user.Status == "expired" {
			statusStr = fmt.Sprintf("%s✖ expirado%s", ui.Red, ui.Reset)
		} else {
			statusStr = fmt.Sprintf("%s✖ suspenso%s", ui.Red, ui.Reset)
		}

		expStr := "Nunca"
		if !user.ExpiresAt.IsZero() {
			expStr = user.ExpiresAt.UTC().Format("02/01/2006")
			if time.Now().UTC().After(user.ExpiresAt.UTC()) {
				expStr = fmt.Sprintf("%s (EXPIRADO)", expStr)
			}
		}

		var onlineStr string
		if conns > 0 {
			durationStr := ""
			if len(details) > 0 {
				durationStr = " [" + system.FormatDuration(details[0].Duration) + "]"
			}
			onlineStr = fmt.Sprintf("%s✔ %d conexões%s%s", ui.Green, conns, durationStr, ui.Reset)
		} else {
			onlineStr = fmt.Sprintf("%s✖ offline%s", ui.Red, ui.Reset)
		}

		ui.FormatLine(fmt.Sprintf("%sUSUÁRIO:%s %s%s%s", ui.Bold, ui.Reset, ui.Bold, user.Username, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%sUUID:%s    %s", ui.Bold, ui.Reset, user.UUID))
		ui.FormatLine(fmt.Sprintf("%sSTATUS:%s  %s", ui.Bold, ui.Reset, statusStr))
		ui.FormatLine(fmt.Sprintf("%sONLINE:%s  %s", ui.Bold, ui.Reset, onlineStr))
		ui.FormatLine(fmt.Sprintf("%sEXPIRA:%s  %s", ui.Bold, ui.Reset, expStr))
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s SUSPENDER USUÁRIO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s ATIVAR USUÁRIO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s DELETAR UUID", ui.Red, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s RENOVAR EXPIRAÇÃO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 05 ]%s VER LINK VLESS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {

		case "01", "1": // ── SUSPENDER ──────────────────────────────────────
			if user.Status == "suspended" || user.Status == "expired" {
				ui.PrintWarning("Usuário já está suspenso ou expirado.")
				pause()
				continue
			}
			ui.PrintWarning("⏳ Suspendendo usuário no XRAY...")
			if err := system.SuspendXrayUser(user.Username); err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao suspender: %v", err))
			} else {
				if err := db.UpdateXrayUserStatus(user.Username, "suspended"); err != nil {
					ui.PrintWarning(fmt.Sprintf("Banco não atualizado: %v", err))
				}
				user.Status = "suspended"
				ui.PrintSuccess("✖ Usuário suspenso — XRAY reiniciado.")
			}
			pause()

		case "02", "2": // ── ATIVAR ─────────────────────────────────────────
			if user.Status == "active" {
				ui.PrintWarning("Usuário já está ativo.")
				pause()
				continue
			}
			ui.PrintWarning("⏳ Ativando usuário no XRAY...")
			if err := system.ActivateXrayUser(user.Username, user.UUID); err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao ativar: %v", err))
			} else {
				if err := db.UpdateXrayUserStatus(user.Username, "active"); err != nil {
					ui.PrintWarning(fmt.Sprintf("Banco não atualizado: %v", err))
				}
				user.Status = "active"
				ui.PrintSuccess("✔ Usuário ativado — XRAY reiniciado.")
			}
			pause()

		case "03", "3": // ── DELETAR ────────────────────────────────────────
			confirm := ui.GetInput(fmt.Sprintf("Deletar UUID de %s permanentemente? (s/N)", user.Username))
			if strings.ToLower(confirm) != "s" {
				continue
			}
			ui.PrintWarning("⏳ Removendo UUID do XRAY...")
			if err := system.SuspendXrayUser(user.Username); err != nil {
				ui.PrintWarning(fmt.Sprintf("Aviso ao remover do config: %v", err))
			}
			if err := db.DeleteXrayUser(user.Username); err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao deletar do banco: %v", err))
			} else {
				ui.PrintSuccess("UUID deletado permanentemente.")
			}
			pause()
			return // volta ao menu de listagem

		case "04", "4": // ── RENOVAR ────────────────────────────────────────
			daysStr := ui.GetInput("Quantidade de dias a adicionar (ex: 30)")
			days, err := strconv.Atoi(daysStr)
			if err != nil || days <= 0 {
				ui.PrintError("Quantidade inválida!")
				pause()
				continue
			}
			base := user.ExpiresAt
			if base.IsZero() || base.Before(time.Now()) {
				base = time.Now()
			}
			newExpiry := base.AddDate(0, 0, days)
			if err := db.UpdateXrayUserExpiration(user.Username, newExpiry); err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao renovar: %v", err))
			} else {
				user.ExpiresAt = newExpiry
				ui.PrintSuccess(fmt.Sprintf("✔ Expiração renovada para %s", newExpiry.Format("02/01/2006")))
			}
			pause()

		case "05", "5": // ── LINK VLESS ─────────────────────────────────────
			host, _ := db.GetConfig("xray_host")
			sni, _ := db.GetConfig("xray_sni")
			bugHost, _ := db.GetConfig("xray_bughost")
			if host == "" {
				ui.PrintError("XRAY não configurado. Instale primeiro.")
				pause()
				continue
			}
			link := system.GenerateVlessLink(user.UUID, host, sni, bugHost, false)
			ui.ClearScreen()
			ui.DrawLine()
			ui.FormatLine(ui.CenterText("LINK VLESS — "+user.Username, ui.PanelWidth-4))
			ui.DrawLine()
			ui.FormatLine(fmt.Sprintf("%sUSUÁRIO:%s %s", ui.Bold, ui.Reset, user.Username))
			ui.FormatLine(fmt.Sprintf("%sUUID:%s    %s", ui.Bold, ui.Reset, user.UUID))
			ui.DrawLine()
			ui.FormatLine(ui.Green + link + ui.Reset)
			ui.DrawLine()
			pause()

		case "00", "0":
			return

		default:
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
		}
	}
}

func handleDNSTTMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("DNSTT / SLOW-DNS", ui.PanelWidth-4))
		ui.DrawLine()

		status, isActive := system.GetDNSTTStatus()
		if isActive {
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Green, status, ui.Reset))
		} else {
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Red, status, ui.Reset))
		}
		ui.FormatLine(fmt.Sprintf("%sPORTA:%s 53/UDP", ui.Bold, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%sSERVIÇO:%s dnstt.service", ui.Bold, ui.Reset))
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s INSTALAR E CONFIGURAR", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s STATUS DO SERVIÇO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s MOSTRAR CONFIGURAÇÃO (NS + KEY)", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s DESATIVAR DNSTT", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 05 ]%s DESINSTALAR DNSTT", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 06 ]%s ATIVAR DNS RESOLVER (DNS OTIMIZADO)", ui.Cyan, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			handleInstallDNSTT()
		case "02", "2":
			handleDNSTTStatus()
		case "03", "3":
			handleShowDNSTTConfig()
		case "04", "4":
			handleDisableDNSTT()
		case "05", "5":
			handleUninstallDNSTT()
		case "06", "6":
			handleDNSResolverMenu()
		case "00", "0":
			return
		default:
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
		}
	}
}

func handleDNSResolverMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("DNS RESOLVER (DNS OTIMIZADO)", ui.PanelWidth-4))
		ui.DrawLine()

		status, isActive := system.GetDNSResolverStatus()
		portStr, _ := db.GetConfig("dns_resolver_port")
		ip := system.GetPublicIP()

		if isActive {
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Green, status, ui.Reset))
			ui.FormatLine(fmt.Sprintf("%sPORTA:%s  %s", ui.Bold, ui.Reset, portStr))
			ui.FormatLine(fmt.Sprintf("%sIP:%s     %s", ui.Bold, ui.Reset, ip))
		} else {
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Red, status, ui.Reset))
		}
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s ATIVAR/INSTALAR DNS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s DESATIVAR DNS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s EDITAR PORTA", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s VER DADOS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			handleInstallDNSResolver()
		case "02", "2":
			if err := system.StopDNSResolver(); err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao desativar: %v", err))
			} else {
				ui.PrintSuccess("DNS Resolver desativado com sucesso!")
			}
			pause()
		case "03", "3":
			handleEditDNSResolverPort()
		case "04", "4":
			ui.ClearScreen()
			ui.DrawLine()
			ui.FormatLine(ui.CenterText("DADOS DO DNS RESOLVER", ui.PanelWidth-4))
			ui.DrawLine()
			ui.FormatLine(fmt.Sprintf("%sIP:%s     %s", ui.Bold, ui.Reset, ip))
			ui.FormatLine(fmt.Sprintf("%sPORTA:%s  %s", ui.Bold, ui.Reset, portStr))
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s", ui.Bold, ui.Reset, status))
			ui.DrawLine()
			pause()
		case "00", "0":
			return
		}
	}
}

func handleInstallDNSResolver() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("INSTALAR DNS RESOLVER", ui.PanelWidth-4))
	ui.DrawLine()

	ui.FormatLine("O DNS Resolver melhora a velocidade da internet.")
	ui.FormatLine("Ele pode rodar na porta 53 ou 5353.")
	ui.DrawLine()

	port := 53
	if !system.IsPortAvailable(53) {
		ui.PrintWarning("Porta 53 em uso. Usando porta 5353 como alternativa.")
		port = 5353
	}

	choice := ui.GetInput(fmt.Sprintf("Deseja instalar na porta %d? (s/N/custom)", port))
	if strings.ToLower(choice) == "custom" {
		pStr := ui.GetInput("Digite a porta customizada")
		port, _ = strconv.Atoi(pStr)
	} else if strings.ToLower(choice) != "s" {
		return
	}

	ui.PrintWarning("Instalando e configurando Unbound... Aguarde.")
	if err := system.InstallDNSResolver(port); err != nil {
		ui.PrintError(fmt.Sprintf("Erro na instalação: %v", err))
	} else {
		ui.PrintSuccess("DNS Resolver ativado e otimizado com sucesso!")
	}
	pause()
}

func handleEditDNSResolverPort() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("EDITAR PORTA DNS", ui.PanelWidth-4))
	ui.DrawLine()

	pStr := ui.GetInput("Digite a nova porta (ex: 5353)")
	p, _ := strconv.Atoi(pStr)
	if p <= 0 {
		ui.PrintError("Porta inválida!")
		pause()
		return
	}

	ui.PrintWarning("Atualizando porta e reiniciando DNS Resolver...")
	if err := system.UpdateDNSResolverPort(p); err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao atualizar: %v", err))
	} else {
		ui.PrintSuccess("Porta atualizada com sucesso!")
	}
	pause()
}

func handleInstallDNSTT() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("INSTALAR E CONFIGURAR DNSTT", ui.PanelWidth-4))
	ui.DrawLine()

	ns := ui.GetInput("Digite o NS (ex: ns.seudominio.com)")
	if ns == "" {
		ui.PrintError("NS não pode ser vazio!")
		ui.DrawLine()
		pause()
		return
	}

	ui.PrintWarning("Instalando dependências e compilando DNSTT... Isso pode levar alguns minutos.")
	key, err := system.InstallDNSTT(ns)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro na instalação: %v", err))
		ui.DrawLine()
		pause()
		return
	}

	db.SetConfig("dnstt_ns", ns)
	db.SetConfig("dnstt_key", key)

	ui.PrintSuccess("DNSTT instalado e configurado com sucesso!")
	ui.DrawLine()
	ui.FormatLine(fmt.Sprintf("%sNS:%s %s", ui.Bold, ui.Reset, ns))
	ui.FormatLine(fmt.Sprintf("%sPORTA:%s 53/UDP", ui.Bold, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%sKEY:%s %s", ui.Bold, ui.Reset, key))
	ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %sATIVO%s", ui.Bold, ui.Reset, ui.Green, ui.Reset))
	ui.DrawLine()
	pause()
}

func handleDNSTTStatus() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("STATUS DO DNSTT", ui.PanelWidth-4))
	ui.DrawLine()

	status, isActive := system.GetDNSTTStatus()
	ns, _ := db.GetConfig("dnstt_ns")

	if isActive {
		ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Green, status, ui.Reset))
	} else {
		ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Red, status, ui.Reset))
	}
	ui.FormatLine(fmt.Sprintf("%sNS:%s %s", ui.Bold, ui.Reset, ns))
	ui.FormatLine(fmt.Sprintf("%sPORTA:%s 53/UDP", ui.Bold, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%sSERVIÇO:%s dnstt.service", ui.Bold, ui.Reset))
	ui.DrawLine()
	pause()
}

func handleShowDNSTTConfig() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("CONFIGURAÇÃO DNSTT", ui.PanelWidth-4))
	ui.DrawLine()

	ns, _ := db.GetConfig("dnstt_ns")
	key, _ := db.GetConfig("dnstt_key")

	if ns == "" || key == "" {
		ui.FormatLine(ui.Red + "DNSTT não está configurado." + ui.Reset)
	} else {
		ui.FormatLine(fmt.Sprintf("%sNS:%s %s", ui.Bold, ui.Reset, ns))
		ui.FormatLine(fmt.Sprintf("%sKEY:%s %s", ui.Bold, ui.Reset, key))
		ui.FormatLine(fmt.Sprintf("%sPORTA:%s 53/UDP", ui.Bold, ui.Reset))
	}
	ui.DrawLine()
	pause()
}

func handleDisableDNSTT() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("DESATIVAR DNSTT", ui.PanelWidth-4))
	ui.DrawLine()

	confirm := ui.GetInput("Deseja realmente desativar o DNSTT? (s/N)")
	if strings.ToLower(confirm) != "s" {
		return
	}

	err := system.DisableDNSTT()
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao desativar: %v", err))
	} else {
		ui.PrintSuccess("DNSTT desativado com sucesso!")
	}
	ui.DrawLine()
	pause()
}

func handleUninstallDNSTT() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("DESINSTALAR DNSTT", ui.PanelWidth-4))
	ui.DrawLine()

	confirm := ui.GetInput("Deseja realmente desinstalar o DNSTT? (s/N)")
	if strings.ToLower(confirm) != "s" {
		return
	}

	err := system.UninstallDNSTT()
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao desinstalar: %v", err))
	} else {
		db.SetConfig("dnstt_ns", "")
		db.SetConfig("dnstt_key", "")
		ui.PrintSuccess("DNSTT removido completamente!")
	}
	ui.DrawLine()
	pause()
}

func handleSSLTunnelMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("SSL TUNNEL", ui.PanelWidth-4))
		ui.DrawLine()

		status, isActive := system.GetSSLTunnelStatus()
		port, _ := db.GetConfig("ssl_tunnel_port")

		if isActive {
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Green, status, ui.Reset))
		} else {
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Red, status, ui.Reset))
		}
		ui.FormatLine(fmt.Sprintf("%sPORTA ATIVA:%s %s", ui.Bold, ui.Reset, port))
		ui.FormatLine(fmt.Sprintf("%sPROTOCOLO:%s SSL/TLS", ui.Bold, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%sSERVIÇO:%s ssl-tunnel.service", ui.Bold, ui.Reset))
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s INSTALAR SSL TUNNEL", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s DESATIVAR SSL TUNNEL", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			handleInstallSSLTunnel()
		case "02", "2":
			handleDisableSSLTunnel()
		case "00", "0":
			return
		default:
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
		}
	}
}

func handleInstallSSLTunnel() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("INSTALAR SSL TUNNEL", ui.PanelWidth-4))
	ui.DrawLine()

	portStr := ui.GetInput("Digite a porta para o SSL TUNNEL (ex: 443)")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		ui.PrintError("Porta inválida! Use um número entre 1 e 65535.")
		ui.DrawLine()
		pause()
		return
	}

	if !system.IsPortAvailable(port) {
		ui.PrintError("Porta já está em uso, escolha outra.")
		ui.DrawLine()
		pause()
		return
	}

	ui.PrintWarning("Instalando stunnel4 e configurando túnel SSL... Aguarde.")
	err = system.InstallSSLTunnel(port)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro na instalação: %v", err))
		ui.DrawLine()
		pause()
		return
	}

	db.SetConfig("ssl_tunnel_port", portStr)
	ui.PrintSuccess("SSL Tunnel instalado com sucesso!")
	ui.DrawLine()
	ui.FormatLine("PROTOCOLO: SSL/TLS")
	ui.FormatLine(fmt.Sprintf("PORTA: %d", port))
	ui.FormatLine("REDIRECIONAMENTO: 127.0.0.1:22")
	ui.FormatLine("STATUS: ATIVO")
	ui.DrawLine()
	pause()
}

func handleDisableSSLTunnel() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("DESATIVAR SSL TUNNEL", ui.PanelWidth-4))
	ui.DrawLine()

	confirm := ui.GetInput("Deseja realmente desativar o SSL Tunnel? (s/N)")
	if strings.ToLower(confirm) != "s" {
		return
	}

	portStr, _ := db.GetConfig("ssl_tunnel_port")
	port, _ := strconv.Atoi(portStr)

	err := system.DisableSSLTunnel(port)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao desativar: %v", err))
	} else {
		db.SetConfig("ssl_tunnel_port", "")
		ui.PrintSuccess("SSL Tunnel desativado com sucesso!")
	}
	ui.DrawLine()
	pause()
}

func handleWebSocketMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("WEBSOCKET", ui.PanelWidth-4))
		ui.DrawLine()
		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s INSTALAR WEBSOCKET", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s WEBSOCKET SECURITY", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s WEBSOCKET SECURITY + TLS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s STATUS DO SERVIÇO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 05 ]%s DESABILITAR WEBSOCKET", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			handleInstallWebSocket()
		case "02", "2":
			handleWebSocketSecMenu()
		case "03", "3":
			handleWebSocketTLSMenu()
		case "04", "4":
			handleWebSocketStatus()
		case "05", "5":
			handleDisableWebSocket()
		case "00", "0":
			return
		default:
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
		}
	}
}

func handleWebSocketTLSMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("WEBSOCKET SECURITY + TLS", ui.PanelWidth-4))
		ui.DrawLine()

		status, isActive := system.GetWebSocketTLSStatus()
		port, _ := db.GetConfig("websocket_tls_port")

		if isActive {
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Green, status, ui.Reset))
		} else {
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Red, status, ui.Reset))
		}
		ui.FormatLine(fmt.Sprintf("%sPORTA ATIVA:%s %s", ui.Bold, ui.Reset, port))
		ui.FormatLine(fmt.Sprintf("%sPROTOCOLO:%s WSS", ui.Bold, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%sSERVIÇO:%s websocket-tls.service", ui.Bold, ui.Reset))
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s INSTALAR WEBSOCKET TLS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s DESLIGAR WEBSOCKET TLS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			handleInstallWebSocketTLS()
		case "02", "2":
			handleDisableWebSocketTLS()
		case "00", "0":
			return
		default:
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
		}
	}
}

func handleInstallWebSocketTLS() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("INSTALAR WEBSOCKET SECURITY + TLS", ui.PanelWidth-4))
	ui.DrawLine()

	portStr := ui.GetInput("Digite a porta para o WebSocket TLS (ex: 443)")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		ui.PrintError("Porta inválida! Use um número entre 1 e 65535.")
		ui.DrawLine()
		pause()
		return
	}

	if !system.IsPortAvailable(port) {
		ui.PrintError("Porta já está em uso, escolha outra.")
		ui.DrawLine()
		pause()
		return
	}

	ui.PrintWarning("Instalando dependências, gerando certificados e configurando TLS... Aguarde.")
	err = system.InstallWebSocketTLS(port)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro na instalação: %v", err))
		ui.DrawLine()
		pause()
		return
	}

	db.SetConfig("websocket_tls_port", portStr)
	ui.PrintSuccess("WebSocket TLS instalado com sucesso!")
	ui.DrawLine()
	ui.FormatLine("PROTOCOLO: WSS (Seguro)")
	ui.FormatLine(fmt.Sprintf("PORTA: %d", port))
	ui.FormatLine("STATUS: ATIVO")
	ui.DrawLine()
	pause()
}

func handleDisableWebSocketTLS() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("DESLIGAR WEBSOCKET TLS", ui.PanelWidth-4))
	ui.DrawLine()

	confirm := ui.GetInput("Deseja realmente desligar o WebSocket TLS? (s/N)")
	if strings.ToLower(confirm) != "s" {
		return
	}

	portStr, _ := db.GetConfig("websocket_tls_port")
	port, _ := strconv.Atoi(portStr)

	err := system.DisableWebSocketTLS(port)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao desativar: %v", err))
	} else {
		db.SetConfig("websocket_tls_port", "")
		ui.PrintSuccess("WebSocket TLS desativado com sucesso!")
	}
	ui.DrawLine()
	pause()
}

func handleWebSocketSecMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("WEBSOCKET SECURITY", ui.PanelWidth-4))
		ui.DrawLine()

		status, isActive := system.GetWebSocketSecStatus()
		port, _ := db.GetConfig("websocket_sec_port")

		if isActive {
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Green, status, ui.Reset))
		} else {
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Red, status, ui.Reset))
		}
		ui.FormatLine(fmt.Sprintf("%sPORTA ATIVA:%s %s", ui.Bold, ui.Reset, port))
		ui.FormatLine(fmt.Sprintf("%sSERVIÇO:%s websocket-sec.service", ui.Bold, ui.Reset))
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s INSTALAR WEBSOCKET SECURITY", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s DESLIGAR WEBSOCKET SECURITY", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			handleInstallWebSocketSec()
		case "02", "2":
			handleDisableWebSocketSec()
		case "00", "0":
			return
		default:
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
		}
	}
}

func handleInstallWebSocketSec() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("INSTALAR WEBSOCKET SECURITY", ui.PanelWidth-4))
	ui.DrawLine()

	portStr := ui.GetInput("Digite a porta para o WebSocket Security (ex: 80 ou 8080)")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		ui.PrintError("Porta inválida! Use um número entre 1 e 65535.")
		ui.DrawLine()
		pause()
		return
	}

	if !system.IsPortAvailable(port) {
		ui.PrintError("Porta já está em uso, escolha outra.")
		ui.DrawLine()
		pause()
		return
	}

	ui.PrintWarning("Instalando e configurando WebSocket Security... Aguarde.")
	err = system.InstallWebSocketSec(port)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro na instalação: %v", err))
		ui.DrawLine()
		pause()
		return
	}

	db.SetConfig("websocket_sec_port", portStr)
	ui.PrintSuccess("WebSocket Security instalado com sucesso!")
	ui.DrawLine()
	ui.FormatLine(fmt.Sprintf("PORTA: %d", port))
	ui.FormatLine("STATUS: ATIVO")
	ui.DrawLine()
	pause()
}

func handleDisableWebSocketSec() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("DESLIGAR WEBSOCKET SECURITY", ui.PanelWidth-4))
	ui.DrawLine()

	confirm := ui.GetInput("Deseja realmente desligar o WebSocket Security? (s/N)")
	if strings.ToLower(confirm) != "s" {
		return
	}

	portStr, _ := db.GetConfig("websocket_sec_port")
	port, _ := strconv.Atoi(portStr)

	err := system.DisableWebSocketSec(port)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao desativar: %v", err))
	} else {
		db.SetConfig("websocket_sec_port", "")
		ui.PrintSuccess("WebSocket Security desativado com sucesso!")
	}
	ui.DrawLine()
	pause()
}

func handleInstallWebSocket() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("INSTALAR WEBSOCKET", ui.PanelWidth-4))
	ui.DrawLine()

	portStr := ui.GetInput("Digite a porta que deseja usar para o WebSocket")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		ui.PrintError("Porta inválida! Use um número entre 1 e 65535.")
		ui.DrawLine()
		pause()
		return
	}

	if !system.IsPortAvailable(port) {
		ui.PrintError("Porta já está em uso, escolha outra.")
		ui.DrawLine()
		pause()
		return
	}

	ui.PrintWarning("Instalando dependências e configurando serviço... Aguarde.")
	err = system.InstallWebSocket(port)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro na instalação: %v", err))
		ui.DrawLine()
		pause()
		return
	}

	db.SetConfig("websocket_port", portStr)
	ui.PrintSuccess("WebSocket instalado com sucesso!")
	ui.DrawLine()
	ui.FormatLine(fmt.Sprintf("PORTA: %d", port))
	ui.FormatLine("STATUS: ATIVO")
	ui.DrawLine()
	pause()
}

func handleWebSocketStatus() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("STATUS DO WEBSOCKET", ui.PanelWidth-4))
	ui.DrawLine()

	status, isActive := system.GetWebSocketStatus()
	port, _ := db.GetConfig("websocket_port")

	if isActive {
		ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Green, status, ui.Reset))
	} else {
		ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Red, status, ui.Reset))
	}

	ui.FormatLine(fmt.Sprintf("%sPORTA:%s %s", ui.Bold, ui.Reset, port))
	ui.FormatLine(fmt.Sprintf("%sPROCESSO:%s websocket.service", ui.Bold, ui.Reset))
	ui.DrawLine()
	pause()
}

func handleDisableWebSocket() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("DESABILITAR WEBSOCKET", ui.PanelWidth-4))
	ui.DrawLine()

	confirm := ui.GetInput("Deseja realmente desabilitar o WebSocket? (s/N)")
	if strings.ToLower(confirm) != "s" {
		return
	}

	portStr, _ := db.GetConfig("websocket_port")
	port, _ := strconv.Atoi(portStr)

	err := system.DisableWebSocket(port)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao desabilitar: %v", err))
	} else {
		db.SetConfig("websocket_port", "")
		ui.PrintSuccess("WebSocket desabilitado com sucesso!")
	}
	ui.DrawLine()
	pause()
}

func handleCreateTrialUser() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("CRIAR USUÁRIO TESTE", ui.PanelWidth-4))
	ui.DrawLine()

	// 1. Gerar username automático
	username := system.GenerateRandomUser()
	// Verificar se já existe (raro com rand, mas possível)
	for {
		existing, _ := db.GetUserByUsername(username)
		if existing == nil {
			break
		}
		username = system.GenerateRandomUser()
	}

	// 2. Gerar senha automática
	password := system.GenerateRandomPassword()

	// 3. Perguntar tempo de duração
	hoursStr := ui.GetInput("Digite a duração (em horas, máx 24)")
	hours, _ := strconv.Atoi(hoursStr)
	if hours < 1 {
		hours = 1
	}
	if hours > 24 {
		hours = 24
	}

	expirationDate := time.Now().Add(time.Duration(hours) * time.Hour)
	limit := 1 // Padrão para teste

	// ── Integração automática com XRAY ──────────────────────────────────────────────
	xrayUUID := ""
	if host, _ := db.GetConfig("xray_host"); host != "" {
		xrayUUID = system.GenerateUUID()
		_ = system.AddXrayUser(username, xrayUUID)
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

	// System operations
	err := system.CreateSSHUser(username, password, expirationDate)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao criar usuário no sistema: %v", err))
		ui.DrawLine()
		pause()
		return
	}

	_ = system.SetConnectionLimit(username, limit)

	// DB operation
	user := &models.User{
		Username:       username,
		Password:       password,
		Limit:          limit,
		ExpirationDate: expirationDate,
		XrayUUID:       xrayUUID,
		Type:           "teste",
		CreatedAt:      time.Now(),
	}
	err = db.SaveUser(user)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao salvar no banco de dados: %v", err))
	}

	// EXIBIÇÃO FINAL
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("🧪 USUÁRIO TESTE CRIADO", ui.PanelWidth-4))
	ui.DrawLine()
	ui.FormatLine(fmt.Sprintf("👤 %sUSUÁRIO:%s %s", ui.Bold, ui.Reset, username))
	ui.FormatLine(fmt.Sprintf("🔑 %sSENHA:%s   %s", ui.Bold, ui.Reset, password))
	ui.FormatLine(fmt.Sprintf("⏳ %sEXPIRA EM:%s %d HORAS", ui.Bold, ui.Reset, hours))
	ui.FormatLine(fmt.Sprintf("🧪 %sTIPO:%s      %sTESTE%s", ui.Bold, ui.Reset, ui.Yellow, ui.Reset))
	ui.DrawLine()
	if xrayUUID != "" {
		ui.FormatLine(fmt.Sprintf("🔗 %sUUID XRAY:%s %s", ui.Bold, ui.Reset, xrayUUID))
		ui.DrawLine()
	}
	ui.PrintSuccess("Usuário teste pronto para uso!")
	ui.DrawLine()
	pause()
}

func handleCreateUser() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("CRIAR USUÁRIO SSH", ui.PanelWidth-4))
	ui.DrawLine()

	username := ui.GetInput("Digite o nome do usuário")
	if username == "" {
		ui.PrintError("Nome de usuário não pode ser vazio!")
		ui.DrawLine()
		pause()
		return
	}

	// Check if user already exists in DB
	existing, _ := db.GetUserByUsername(username)
	if existing != nil {
		ui.PrintError("Usuário já existe!")
		ui.DrawLine()
		pause()
		return
	}

	password, _ := ui.GetPasswordInput("Digite a senha")
	if password == "" {
		ui.PrintError("Senha não pode ser vazia!")
		ui.DrawLine()
		pause()
		return
	}

	limitStr := ui.GetInput("Digite o limite de conexões simultâneas")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 1
	}

	daysStr := ui.GetInput("Digite o tempo de expiração (em dias)")
	days, _ := strconv.Atoi(daysStr)

	var expirationDate time.Time
	if days > 0 {
		expirationDate = time.Now().AddDate(0, 0, days)
	}

	// ── Integração automática com XRAY ──────────────────────────────────────────────
	xrayUUID := ""
	if host, _ := db.GetConfig("xray_host"); host != "" {
		// Gera UUID automaticamente e vincula ao usuário
		xrayUUID = system.GenerateUUID()
		if err := system.AddXrayUser(username, xrayUUID); err != nil {
			ui.PrintWarning(fmt.Sprintf("Aviso: UUID XRAY não adicionado ao config.json: %v", err))
		}
		// Persiste na tabela xray_users
		xusr := &models.XrayUser{
			Username:        username,
			UUID:            xrayUUID,
			ExpiresAt:       expirationDate,
			ConnectionLimit: limit,
			Status:          "active",
			CreatedAt:       time.Now(),
		}
		if err := db.SaveXrayUser(xusr); err != nil {
			ui.PrintWarning(fmt.Sprintf("Aviso: erro ao salvar UUID XRAY no banco: %v", err))
		} else {
			ui.PrintSuccess("UUID XRAY gerado e vinculado automaticamente!")
		}
	}

	// System operations
	err := system.CreateSSHUser(username, password, expirationDate)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao criar usuário no sistema: %v", err))
		ui.DrawLine()
		pause()
		return
	}

	err = system.SetConnectionLimit(username, limit)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("Erro ao definir limite: %v", err))
	}

	// DB operation
	user := &models.User{
		Username:       username,
		Password:       password,
		Limit:          limit,
		ExpirationDate: expirationDate,
		XrayUUID:       xrayUUID,
		Type:           "premium",
		CreatedAt:      time.Now(),
	}
	err = db.SaveUser(user)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao salvar no banco de dados: %v", err))
	}

	ui.PrintSuccess("Usuário criado com sucesso!")
	ui.DrawLine()
	ui.FormatLine(fmt.Sprintf("Usuário: %s", username))
	if !expirationDate.IsZero() {
		ui.FormatLine(fmt.Sprintf("Expira em: %s", expirationDate.Format("02/01/2006")))
	} else {
		ui.FormatLine("Expira em: Nunca")
	}
	ui.FormatLine(fmt.Sprintf("Limite: %d", limit))
	if xrayUUID != "" {
		ui.FormatLine(fmt.Sprintf("UUID XRAY: %s", xrayUUID))
	}
	ui.DrawLine()
	pause()
}

func handleListUsers() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("LISTA DE USUÁRIOS", ui.PanelWidth-4))
	ui.DrawLine()

	users, err := db.GetUsers()
	if err != nil {
		ui.FormatLine(fmt.Sprintf("%sErro ao buscar usuários: %v%s", ui.Red, err, ui.Reset))
		ui.DrawLine()
		pause()
		return
	}

	ui.FormatLine(fmt.Sprintf("%-12s | %-10s | %-4s | %-8s | %-8s", "Usuário", "Expira", "Lim", "Status", "Tipo"))
	ui.DrawLine()

	for _, user := range users {
		expStr := "Nunca"
		isExpired := false
		if !user.ExpirationDate.IsZero() {
			expStr = user.ExpirationDate.UTC().Format("02/01/06")
			if time.Now().UTC().After(user.ExpirationDate.UTC()) {
				isExpired = true
				expStr = ui.Red + expStr + ui.Reset
			}
		}

		status := ui.Red + "OFFLINE" + ui.Reset
		if system.IsUserOnline(user.Username) {
			status = ui.Green + "ONLINE" + ui.Reset
		}

		if isExpired {
			status = ui.Red + "EXPIRADO" + ui.Reset
		}

		userType := user.Type
		if userType == "" {
			userType = "premium"
		}

		typeStr := ui.Green + "💎 PREM" + ui.Reset
		if userType == "teste" {
			typeStr = ui.Yellow + "🧪 TESTE" + ui.Reset
		}

		ui.FormatLine(fmt.Sprintf("%-12s | %-10s | %-4d | %-8s | %-8s", user.Username, expStr, user.Limit, status, typeStr))
	}
	ui.DrawLine()

	pause()
}

func handleMoreOptions() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("MAIS OPÇÕES", ui.PanelWidth-4))
		ui.DrawLine()
		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s SISTEMA DE BANNER", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s OTIMIZAR SERVIDOR", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s ATIVAR API CRIAR", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s ATIVAR API MONITOR", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 05 ]%s ATIVAR API REGISTROS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 06 ]%s ATIVAR API SERVIDOR", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 07 ]%s PÁGINA DOCUMENTAÇÃO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 08 ]%s REQUISIÇÕES APIS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			handleBannerSystemMenu()
		case "02", "2":
			handleOptimizeServer()
		case "03", "3":
			handleApiActivationMenu()
		case "04", "4":
			handleMonitorApiMenu()
		case "05", "5":
			handleRegistrosApiMenu()
		case "06", "6":
			handleServidorApiMenu()
		case "07", "7":
			handleDocsApiMenu()
		case "08", "8":
			handleApiRequestsMonitor()
		case "00", "0":
			return
		default:
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
		}
	}
}

func handleApiRequestsMonitor() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("MONITOR DE REQUISIÇÕES APIS", ui.PanelWidth-4))
	ui.DrawLine()

	// Espaço reservado para os dados dinâmicos
	ui.FormatLine("🚀 API CRIAR USUÁRIO: Carregando...")
	ui.FormatLine("📊 API MONITOR ONLINES: Carregando...")
	ui.FormatLine("📝 API REGISTROS: Carregando...")
	ui.FormatLine("🖥️ API SERVIDOR: Carregando...")
	ui.DrawLine()
	ui.FormatLine(fmt.Sprintf("%s[ 0 ]%s VOLTAR AO MENU", ui.Red, ui.Reset))
	ui.DrawLine()

	stop := make(chan bool)
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Salva a posição atual do cursor
				fmt.Print("\033[s")

				// Atualiza a linha da API de Criação (Linha 4)
				fmt.Print("\033[4;1H")
				statusCriar := ui.Red + "OFFLINE" + ui.Reset
				if system.GetExternalApiStatus() {
					statusCriar = ui.Green + "ONLINE " + ui.Reset
				}
				ui.FormatLine(fmt.Sprintf("🚀 API CRIAR USUÁRIO: %s | Total: %d", statusCriar, system.ExternalApiRequests))

				// Atualiza a linha da API de Monitoramento (Linha 5)
				fmt.Print("\033[5;1H")
				statusMonitor := ui.Red + "OFFLINE" + ui.Reset
				if system.GetMonitorApiStatus() {
					statusMonitor = ui.Green + "ONLINE " + ui.Reset
				}
				ui.FormatLine(fmt.Sprintf("📊 API MONITOR ONLINES: %s | Total: %d", statusMonitor, system.MonitorApiRequests))

				// Atualiza a linha da API de Registros (Linha 6)
				fmt.Print("\033[6;1H")
				statusReg := ui.Red + "OFFLINE" + ui.Reset
				if system.GetRegistrosApiStatus() {
					statusReg = ui.Green + "ONLINE " + ui.Reset
				}
				ui.FormatLine(fmt.Sprintf("📝 API REGISTROS: %s | Total: %d", statusReg, system.RegistrosApiRequests))

				// Atualiza a linha da API de Servidor (Linha 7)
				fmt.Print("\033[7;1H")
				statusServ := ui.Red + "OFFLINE" + ui.Reset
				if system.GetServidorApiStatus() {
					statusServ = ui.Green + "ONLINE " + ui.Reset
				}
				ui.FormatLine(fmt.Sprintf("🖥️ API SERVIDOR: %s | Total: %d", statusServ, system.ServidorApiRequests))

				// Restaura o cursor
				fmt.Print("\033[u")
			case <-stop:
				return
			}
		}
	}()

	for {
		choice := ui.GetInput("Digite 0 para sair")
		if choice == "0" {
			stop <- true
			return
		}
	}
}

func handleRegistrosApiMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("MENU API REGISTROS", ui.PanelWidth-4))
		ui.DrawLine()

		statusStr := ui.Red + "INATIVA" + ui.Reset
		if system.GetRegistrosApiStatus() {
			statusStr = ui.Green + "ATIVA (PORTA 1010)" + ui.Reset
		}

		ui.FormatLine(fmt.Sprintf("STATUS: %s", statusStr))
		ui.FormatLine(fmt.Sprintf("PORTA:  1010"))
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s ATIVAR API REGISTROS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s DESATIVAR API REGISTROS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s VER URL DA API", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			err := system.StartRegistrosAPI()
			if err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao iniciar: %v", err))
			} else {
				ui.PrintSuccess("API REGISTROS ATIVADA NA PORTA 1010!")
			}
			pause()
		case "02", "2":
			system.StopRegistrosAPI()
			ui.PrintSuccess("API REGISTROS DESATIVADA!")
			pause()
		case "03", "3":
			token, _ := db.GetConfig("api_token")
			ip := system.GetPublicIP()
			ui.ClearScreen()
			ui.DrawLine()
			ui.FormatLine(ui.CenterText("DADOS DA API REGISTROS", ui.PanelWidth-4))
			ui.DrawLine()
			ui.FormatLine(fmt.Sprintf("URL: http://%s:1010/v1/usuarios-ssh", ip))
			ui.FormatLine(fmt.Sprintf("TOKEN: %s", token))
			ui.FormatLine("")
			ui.FormatLine("Exemplo de chamada:")
			ui.FormatLine(fmt.Sprintf("curl -H \"Authorization: Bearer %s\" \"http://%s:1010/v1/usuarios-ssh\"", token, ip))
			ui.DrawLine()
			pause()
		case "00", "0":
			return
		}
	}
}

func handleServidorApiMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("MENU API SERVIDOR", ui.PanelWidth-4))
		ui.DrawLine()

		statusStr := ui.Red + "INATIVA" + ui.Reset
		if system.GetServidorApiStatus() {
			statusStr = ui.Green + "ATIVA (PORTA 1030)" + ui.Reset
		}

		ui.FormatLine(fmt.Sprintf("STATUS: %s", statusStr))
		ui.FormatLine(fmt.Sprintf("PORTA:  1030"))
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s ATIVAR API SERVIDOR", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s DESATIVAR API SERVIDOR", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s VER URL DA API", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			err := system.StartServidorAPI()
			if err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao iniciar: %v", err))
			} else {
				ui.PrintSuccess("API SERVIDOR ATIVADA NA PORTA 1030!")
			}
			pause()
		case "02", "2":
			system.StopServidorAPI()
			ui.PrintSuccess("API SERVIDOR DESATIVADA!")
			pause()
		case "03", "3":
			token, _ := db.GetConfig("api_token")
			ip := system.GetPublicIP()
			ui.ClearScreen()
			ui.DrawLine()
			ui.FormatLine(ui.CenterText("DADOS DA API SERVIDOR", ui.PanelWidth-4))
			ui.DrawLine()
			ui.FormatLine(fmt.Sprintf("URL: http://%s:1030/v1/servidor-status", ip))
			ui.FormatLine(fmt.Sprintf("TOKEN: %s", token))
			ui.FormatLine("")
			ui.FormatLine("Exemplo de chamada:")
			ui.FormatLine(fmt.Sprintf("curl -H \"Authorization: Bearer %s\" \"http://%s:1030/v1/servidor-status\"", token, ip))
			ui.DrawLine()
			pause()
		case "00", "0":
			return
		}
	}
}

func handleDocsApiMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("PÁGINA DE DOCUMENTAÇÃO", ui.PanelWidth-4))
		ui.DrawLine()

		statusStr := ui.Red + "INATIVA" + ui.Reset
		if system.GetDocsStatus() {
			statusStr = ui.Green + "ATIVA (PORTA 333)" + ui.Reset
		}

		ui.FormatLine(fmt.Sprintf("STATUS: %s", statusStr))
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s ATIVAR PÁGINA DOCS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s DESATIVAR PÁGINA DOCS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s VER URL DA DOCS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			err := system.StartDocsAPI()
			if err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao iniciar Docs: %v", err))
			} else {
				ui.PrintSuccess("PÁGINA DE DOCUMENTAÇÃO ATIVADA NA PORTA 333!")
				ui.FormatLine(fmt.Sprintf("URL: http://%s:333/", system.GetPublicIP()))
			}
			pause()
		case "02", "2":
			system.StopDocsAPI()
			ui.PrintSuccess("PÁGINA DE DOCUMENTAÇÃO DESATIVADA!")
			pause()
		case "03", "3":
			ui.ClearScreen()
			ui.DrawLine()
			ui.FormatLine(ui.CenterText("URL DA DOCUMENTAÇÃO", ui.PanelWidth-4))
			ui.DrawLine()
			ui.FormatLine(fmt.Sprintf("ACESSE: http://%s:333/", system.GetPublicIP()))
			ui.FormatLine("")
			ui.FormatLine("A página contém todas as instruções para os")
			ui.FormatLine("endpoints de criação e monitoramento.")
			ui.DrawLine()
			pause()
		case "00", "0":
			return
		}
	}
}

func handleMonitorApiMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("ATIVAR API MONITOR", ui.PanelWidth-4))
		ui.DrawLine()

		statusStr := ui.Red + "INATIVA" + ui.Reset
		if system.GetMonitorApiStatus() {
			statusStr = ui.Green + "ATIVA (PORTA 3030)" + ui.Reset
		}

		token, _ := db.GetConfig("api_token")
		ui.FormatLine(fmt.Sprintf("STATUS: %s", statusStr))
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s ATIVAR API MONITOR", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s DESATIVAR API MONITOR", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s VER URL E TOKEN", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			if token == "" {
				token = system.GenerateApiToken()
				db.SetConfig("api_token", token)
			}
			err := system.StartMonitorAPI()
			if err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao iniciar API Monitor: %v", err))
			} else {
				ui.PrintSuccess("API MONITOR ATIVADA NA PORTA 3030!")
				ui.FormatLine(fmt.Sprintf("URL: http://%s:3030/v1/monitor-onlines", system.GetPublicIP()))
			}
			pause()
		case "02", "2":
			system.StopMonitorAPI()
			ui.PrintSuccess("API MONITOR DESATIVADA!")
			pause()
		case "03", "3":
			if token == "" {
				ui.PrintError("API nunca foi ativada. Ative-a primeiro para gerar um token.")
			} else {
				ui.ClearScreen()
				ui.DrawLine()
				ui.FormatLine(ui.CenterText("DADOS DA API MONITOR", ui.PanelWidth-4))
				ui.DrawLine()
				ui.FormatLine(fmt.Sprintf("URL: http://%s:3030/v1/monitor-onlines", system.GetPublicIP()))
				ui.FormatLine(fmt.Sprintf("TOKEN: %s", token))
				ui.FormatLine("")
				ui.FormatLine("Exemplo de chamada:")
				ui.FormatLine(fmt.Sprintf("curl -H \"Authorization: Bearer %s\" \"http://%s:3030/v1/monitor-onlines\"", token, system.GetPublicIP()))
				ui.DrawLine()
			}
			pause()
		case "00", "0":
			return
		}
	}
}

func handleApiActivationMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("ATIVAR API EXTERNA", ui.PanelWidth-4))
		ui.DrawLine()

		statusStr := ui.Red + "INATIVA" + ui.Reset
		if system.GetExternalApiStatus() {
			statusStr = ui.Green + "ATIVA (PORTA 2020)" + ui.Reset
		}

		token, _ := db.GetConfig("api_token")
		days, _ := db.GetConfig("api_days")
		limit, _ := db.GetConfig("api_limit")
		referer, _ := db.GetConfig("api_referer")
		domain, _ := db.GetConfig("api_domain")

		if days == "" {
			days = "30"
		}
		if limit == "" {
			limit = "1"
		}
		if referer == "" {
			referer = "Sem restrição"
		}
		if domain == "" {
			domain = "Não configurado"
		}

		ui.FormatLine(fmt.Sprintf("STATUS: %s", statusStr))
		ui.FormatLine(fmt.Sprintf("DIAS DE VALIDADE: %s dias", days))
		ui.FormatLine(fmt.Sprintf("LIMITE CONEXÕES: %s", limit))
		ui.FormatLine(fmt.Sprintf("REFERER PERMITIDO: %s", referer))
		ui.FormatLine(fmt.Sprintf("DOMÍNIO DA API:   %s", domain))
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s ATIVAR / REINICIAR API", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s DESATIVAR API", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s CONFIGURAR DIAS / LIMITE", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s VER TOKEN / URL", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 05 ]%s REVOGAR E CRIAR NOVO TOKEN", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 06 ]%s CONFIGURAR DOMÍNIOS (REFERER)", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 07 ]%s GERENCIAR DOMÍNIO DA API", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			if token == "" {
				token = system.GenerateApiToken()
				db.SetConfig("api_token", token)
			}
			err := system.StartExternalAPI()
			if err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao iniciar API: %v", err))
			} else {
				ui.PrintSuccess("API ATIVADA NA PORTA 2020!")
				ui.FormatLine(fmt.Sprintf("URL: http://%s:2020/v1/criar-usuario", system.GetPublicIP()))
				ui.FormatLine(fmt.Sprintf("TOKEN: %s", token))
			}
			pause()
		case "02", "2":
			system.StopExternalAPI()
			ui.PrintSuccess("API DESATIVADA!")
			pause()
		case "03", "3":
			d := ui.GetInput("Digite os dias de validade (ex: 30)")
			l := ui.GetInput("Digite o limite de conexões (ex: 1)")
			if d != "" {
				db.SetConfig("api_days", d)
			}
			if l != "" {
				db.SetConfig("api_limit", l)
			}
			ui.PrintSuccess("Configurações salvas!")
			pause()
		case "04", "4":
			if token == "" {
				ui.PrintError("API nunca foi ativada. Ative-a primeiro para gerar um token.")
			} else {
				ui.ClearScreen()
				ui.DrawLine()
				ui.FormatLine(ui.CenterText("DADOS DA API", ui.PanelWidth-4))
				ui.DrawLine()
				ui.FormatLine(fmt.Sprintf("URL: http://%s:2020/v1/criar-usuario", system.GetPublicIP()))
				ui.FormatLine(fmt.Sprintf("TOKEN: %s", token))
				ui.FormatLine("")
				ui.FormatLine("Exemplo de chamada:")
				ui.FormatLine(fmt.Sprintf("curl -H \"Authorization: Bearer %s\" \"http://%s:2020/v1/criar-usuario\"", token, system.GetPublicIP()))
				ui.DrawLine()
			}
			pause()
		case "05", "5":
			newToken := system.GenerateApiToken()
			db.SetConfig("api_token", newToken)
			ui.PrintSuccess("TOKEN REVOGADO E NOVO GERADO!")
			ui.FormatLine(fmt.Sprintf("Novo Token: %s", newToken))
			pause()
		case "06", "6":
			ref := ui.GetInput("Digite o domínio permitido (ex: meuapp.com) ou deixe vazio para todos")
			db.SetConfig("api_referer", ref)
			ui.PrintSuccess("Configuração de Referer atualizada!")
			pause()
		case "07", "7":
			handleApiDomainMenu()
		case "00", "0":
			return
		}
	}
}

func handleApiDomainMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("GERENCIAR DOMÍNIO DA API", ui.PanelWidth-4))
		ui.DrawLine()

		domain, _ := db.GetConfig("api_domain")
		if domain == "" {
			domain = "Nenhum domínio configurado."
		}

		ui.FormatLine(fmt.Sprintf("DOMÍNIO ATUAL: %s%s%s", ui.Yellow, domain, ui.Reset))
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s ADICIONAR / EDITAR DOMÍNIO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s REMOVER DOMÍNIO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			newDomain := ui.GetInput("Digite o novo domínio (ex: dns.meusite.com)")
			if newDomain != "" {
				db.SetConfig("api_domain", newDomain)
				ui.PrintSuccess("Domínio atualizado com sucesso!")
			} else {
				ui.PrintError("O domínio não pode ser vazio!")
			}
			pause()
		case "02", "2":
			confirm := ui.GetInput("Deseja realmente remover o domínio? (s/N)")
			if strings.ToLower(confirm) == "s" {
				db.SetConfig("api_domain", "")
				ui.PrintSuccess("Domínio removido!")
			}
			pause()
		case "00", "0":
			return
		}
	}
}

func handleOptimizeServer() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("OTIMIZAR SERVIDOR", ui.PanelWidth-4))
	ui.DrawLine()

	ui.PrintWarning("⏳ Iniciando otimização do servidor... Por favor, aguarde.")
	ui.DrawLine()

	logs, err := system.OptimizeServer()
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro na otimização: %v", err))
	} else {
		for _, log := range logs {
			ui.FormatLine(log)
		}
	}

	ui.DrawLine()
	ui.PrintSuccess("Otimização concluída!")
	pause()
}

func handleBannerSystemMenu() {
	for {
		status := system.GetNewBannerStatus()
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("CONFIGURAR BANNER", ui.PanelWidth-4))
		ui.DrawLine()
		ui.FormatLine(fmt.Sprintf("STATUS: %s", status))
		ui.DrawLine()
		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s ATIVAR BANNER", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s EDITAR MENSAGEM", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s ESCOLHER COR", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s ESCOLHER ESTILO (ASCII/BOLD)", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 05 ]%s DESATIVAR BANNER", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			if err := system.EnableBanner(); err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao ativar banner: %v", err))
			} else {
				ui.PrintSuccess("Banner ativado com sucesso!")
			}
			pause()
		case "02", "2":
			msg := ui.GetInput("Digite a mensagem do banner")
			if msg != "" {
				if err := system.SetBannerMessage(msg); err != nil {
					ui.PrintError(fmt.Sprintf("Erro ao definir mensagem: %v", err))
				} else {
					ui.PrintSuccess("Mensagem definida com sucesso!")
				}
			}
			pause()
		case "03", "3":
			ui.ClearScreen()
			ui.DrawLine()
			ui.FormatLine(ui.CenterText("ESCOLHER COR", ui.PanelWidth-4))
			ui.DrawLine()
			ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s VERDE", ui.Green, ui.Reset))
			ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s AMARELO", ui.Yellow, ui.Reset))
			ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s AZUL", ui.Blue, ui.Reset))
			ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s VERMELHO", ui.Red, ui.Reset))
			ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s CANCELAR", ui.White, ui.Reset))
			ui.DrawLine()
			color := ui.GetInput("Escolha uma cor")
			if color != "0" && color != "00" {
				if err := system.SetBannerColor(color); err != nil {
					ui.PrintError(fmt.Sprintf("Erro ao definir cor: %v", err))
				} else {
					ui.PrintSuccess("Cor definida com sucesso!")
				}
			}
			pause()
		case "04", "4":
			ui.ClearScreen()
			ui.DrawLine()
			ui.FormatLine(ui.CenterText("ESCOLHER ESTILO", ui.PanelWidth-4))
			ui.DrawLine()
			ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s NORMAL", ui.Yellow, ui.Reset))
			ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s NEGRITO (BOLD)", ui.Yellow, ui.Reset))
			ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s ASCII ART (FIGLET)", ui.Yellow, ui.Reset))
			ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s CANCELAR", ui.Red, ui.Reset))
			ui.DrawLine()
			styleChoice := ui.GetInput("Escolha um estilo")
			style := "normal"
			switch styleChoice {
			case "01", "1":
				style = "normal"
			case "02", "2":
				style = "bold"
			case "03", "3":
				style = "ascii"
			default:
				continue
			}
			if err := system.SetBannerStyle(style); err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao definir estilo: %v", err))
			} else {
				ui.PrintSuccess("Estilo definido com sucesso!")
			}
			pause()
		case "05", "5":
			if err := system.DisableBanner(); err != nil {
				ui.PrintError(fmt.Sprintf("Erro ao desativar banner: %v", err))
			} else {
				ui.PrintSuccess("Banner desativado com sucesso!")
			}
			pause()
		case "00", "0":
			return
		default:
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
		}
	}
}

func handleRemoveUser() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("REMOVER USUÁRIO", ui.PanelWidth-4))
	ui.DrawLine()

	username := ui.GetInput("Digite o nome do usuário para remover")
	if username == "" {
		return
	}

	// Check existence
	user, _ := db.GetUserByUsername(username)
	if user == nil {
		ui.PrintError("Usuário não encontrado!")
		ui.DrawLine()
		pause()
		return
	}

	// Confirm
	confirm := ui.GetInput(fmt.Sprintf("Tem certeza que deseja remover %s? (s/N)", username))
	if strings.ToLower(confirm) != "s" {
		return
	}

	// Remove from system
	err := system.RemoveSSHUser(username)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("Aviso: Erro ao remover do sistema (pode já não existir): %v", err))
	}

	// Remove from DB
	err = db.DeleteUser(username)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao remover do banco de dados: %v", err))
	} else {
		ui.PrintSuccess("Usuário removido com sucesso!")
	}
	ui.DrawLine()
	pause()
}

func handleChangePassword() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("ALTERAR SENHA", ui.PanelWidth-4))
	ui.DrawLine()

	username := ui.GetInput("Digite o nome do usuário")
	user, _ := db.GetUserByUsername(username)
	if user == nil {
		ui.PrintError("Usuário não encontrado!")
		ui.DrawLine()
		pause()
		return
	}

	newPassword, _ := ui.GetPasswordInput("Digite a nova senha")
	if newPassword == "" {
		return
	}

	err := system.SetUserPassword(username, newPassword)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao alterar senha no sistema: %v", err))
	} else {
		db.UpdatePassword(username, newPassword)
		ui.PrintSuccess("Senha alterada com sucesso!")
	}
	ui.DrawLine()
	pause()
}

func handleSetLimit() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("DEFINIR LIMITE", ui.PanelWidth-4))
	ui.DrawLine()

	username := ui.GetInput("Digite o nome do usuário")
	user, _ := db.GetUserByUsername(username)
	if user == nil {
		ui.PrintError("Usuário não encontrado!")
		ui.DrawLine()
		pause()
		return
	}

	limitStr := ui.GetInput("Digite o novo limite")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		return
	}

	err := system.SetConnectionLimit(username, limit)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao definir limite: %v", err))
	} else {
		db.UpdateLimit(username, limit)
		ui.PrintSuccess("Limite atualizado com sucesso!")
	}
	ui.DrawLine()
	pause()
}

func handleSetExpiration() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("DEFINIR DATA DE EXPIRAÇÃO", ui.PanelWidth-4))
	ui.DrawLine()

	username := ui.GetInput("Digite o nome do usuário")
	user, _ := db.GetUserByUsername(username)
	if user == nil {
		ui.PrintError("Usuário não encontrado!")
		ui.DrawLine()
		pause()
		return
	}

	daysStr := ui.GetInput("Digite os dias a partir de hoje (0 para remover)")
	days, _ := strconv.Atoi(daysStr)

	var expirationDate time.Time
	if days > 0 {
		expirationDate = time.Now().AddDate(0, 0, days)
	}

	err := system.SetUserExpiration(username, expirationDate)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao definir expiração: %v", err))
	} else {
		db.UpdateExpiration(username, expirationDate)

		// Se a nova data for no futuro, desbloquear o usuário caso esteja bloqueado
		if expirationDate.IsZero() || time.Now().UTC().Before(expirationDate.UTC()) {
			system.UnblockUser(username)

			// Se tiver XrayUUID, tentar reativar no Xray
			if user.XrayUUID != "" {
				_ = system.AddXrayUser(username, user.XrayUUID)
				_ = db.UpdateXrayUserStatus(username, "active")
			}
		}

		ui.PrintSuccess("Data de expiração atualizada!")
	}
	ui.DrawLine()
	pause()
}

func handleMonitorOnline() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("MONITORAR USUÁRIOS ONLINE", ui.PanelWidth-4))
	ui.DrawLine()

	// 1. SSH Users
	sshUsers, _ := db.GetUsers()
	onlineSSH := 0
	ui.FormatLine(fmt.Sprintf("%s[ USUÁRIOS SSH ]%s", ui.Bold+ui.Cyan, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%-15s | %-10s | %-10s", "Usuário", "Conexões", "Duração"))
	ui.DrawLine()

	for _, user := range sshUsers {
		count, details := system.GetSSHOnlineUsers(user.Username)

		if count > 0 {
			var maxDuration time.Duration
			for _, d := range details {
				if d.Duration > maxDuration {
					maxDuration = d.Duration
				}
			}
			durationStr := system.FormatDuration(maxDuration)
			if maxDuration == 0 {
				durationStr = "Conectando..."
			}
			ui.FormatLine(fmt.Sprintf("%-15s | %-10d | %-10s", user.Username, count, durationStr))
			onlineSSH++
		}
	}
	if onlineSSH == 0 {
		ui.FormatLine("Nenhum usuário SSH conectado.")
	}

	ui.FormatLine("")

	// 2. Xray Users
	xrayUsers, _ := db.GetXrayUsers()
	onlineXray := 0
	ui.FormatLine(fmt.Sprintf("%s[ USUÁRIOS XRAY ]%s", ui.Bold+ui.Cyan, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%-15s | %-10s | %-10s", "Usuário", "Conexões", "Duração"))
	ui.DrawLine()

	for _, u := range xrayUsers {
		details := system.GetXrayUserOnlineDetails(u.Username, u.UUID)
		if len(details) > 0 {
			var maxDuration time.Duration
			for _, d := range details {
				if d.Duration > maxDuration {
					maxDuration = d.Duration
				}
			}
			ui.FormatLine(fmt.Sprintf("%-15s | %-10d | %-10s", u.Username, len(details), system.FormatDuration(maxDuration)))
			onlineXray++
		}
	}
	if onlineXray == 0 {
		ui.FormatLine("Nenhum usuário XRAY conectado.")
	}

	ui.DrawLine()
	ui.FormatLine(fmt.Sprintf("TOTAL GERAL: %d conexões", getRealOnlineCount()))
	ui.DrawLine()
	pause()
}

func handleServerInfo(info *system.ServerInfo) {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("INFORMAÇÕES DO SERVIDOR", ui.PanelWidth-4))
	ui.DrawLine()

	ui.FormatLine(fmt.Sprintf("%sSISTEMA:%s %s", ui.Bold, ui.Reset, info.OS))
	ui.FormatLine(fmt.Sprintf("%sARQUITETURA:%s %s", ui.Bold, ui.Reset, info.Architecture))
	ui.FormatLine(fmt.Sprintf("%sPROCESSADOR:%s %s", ui.Bold, ui.Reset, info.CPUModel))
	ui.FormatLine(fmt.Sprintf("%sNÚCLEOS:%s %d", ui.Bold, ui.Reset, info.VCPUs))
	ui.FormatLine(fmt.Sprintf("%sRAM TOTAL:%s %s", ui.Bold, ui.Reset, info.RAMTotal))
	ui.FormatLine(fmt.Sprintf("%sRAM USADA:%s %s", ui.Bold, ui.Reset, info.RAMUsed))
	ui.FormatLine(fmt.Sprintf("%sUPTIME:%s %s", ui.Bold, ui.Reset, info.Uptime))
	ui.FormatLine(fmt.Sprintf("%sIP PÚBLICO:%s %s", ui.Bold, ui.Reset, info.PublicIP))
	ui.DrawLine()

	pause()
}

func getRealOnlineCount() int {
	total := 0
	// 1. SSH Users
	users, _ := db.GetUsers()
	for _, u := range users {
		count, _ := system.GetSSHOnlineUsers(u.Username)
		total += count
	}
	// 2. Xray Users
	xrayUsers, _ := db.GetXrayUsers()
	for _, u := range xrayUsers {
		total += len(system.GetXrayUserOnlineDetails(u.Username, u.UUID))
	}
	return total
}

func pause() {
	ui.FormatLine(ui.CenterText("Pressione ENTER para voltar ao menu...", ui.PanelWidth-4))
	ui.DrawLine()
	bufio.NewReader(os.Stdin).ReadBytes('\n')
}

func handleTorrentMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("PROTEÇÃO TORRENT", ui.PanelWidth-4))
		ui.DrawLine()

		isBlocked := system.GetTorrentStatus()
		level, _ := db.GetConfig("torrent_level")
		if level == "" {
			level = system.LevelPro
		}
		banned, _ := db.GetTotalBannedUsers()
		attempts, _ := db.GetTotalAbuseAttemptsToday()

		statusStr := ui.Red + "INATIVO" + ui.Reset
		if isBlocked {
			statusStr = ui.Green + "ATIVO" + ui.Reset
		}

		ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s", ui.Bold, ui.Reset, statusStr))
		ui.FormatLine(fmt.Sprintf("%sMODO:%s   %s", ui.Bold, ui.Reset, level))
		ui.FormatLine(fmt.Sprintf("%sUSUÁRIOS BANIDOS:%s %d", ui.Bold, ui.Reset, banned))
		ui.FormatLine(fmt.Sprintf("%sTENTATIVAS HOJE:%s %d", ui.Bold, ui.Reset, attempts))
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s ATIVAR PROTEÇÃO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s DESATIVAR PROTEÇÃO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s STATUS DETALHADO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s MONITORAR CONEXÕES", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 05 ]%s LOG DE ABUSO", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 06 ]%s RESETAR BANIMENTOS", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 07 ]%s CONFIGURAR NÍVEL", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			handleEnableTorrent()
		case "02", "2":
			handleDisableTorrent()
		case "03", "3":
			handleDetailedTorrentStatus()
		case "04", "4":
			handleMonitorConnections()
		case "05", "5":
			handleAbuseLogs()
		case "06", "6":
			handleResetBan()
		case "07", "7":
			handleSetTorrentLevel()
		case "00", "0":
			return
		default:
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
		}
	}
}

func handleEnableTorrent() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("ATIVAR PROTEÇÃO", ui.PanelWidth-4))
	ui.DrawLine()
	ui.FormatLine(fmt.Sprintf("%s⏳ Aplicando regras e iniciando monitoramento...%s", ui.Yellow, ui.Reset))

	if err := system.EnableTorrentProtection(); err != nil {
		ui.PrintError(fmt.Sprintf("Erro: %v", err))
	} else {
		go system.StartTorrentMonitor()
		ui.PrintSuccess("PROTEÇÃO ATIVADA!")
	}
	ui.DrawLine()
	pause()
}

func handleDisableTorrent() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("DESATIVAR PROTEÇÃO", ui.PanelWidth-4))
	ui.DrawLine()
	ui.FormatLine(fmt.Sprintf("%s⏳ Removendo regras e parando monitoramento...%s", ui.Yellow, ui.Reset))

	if err := system.DisableTorrentProtection(); err != nil {
		ui.PrintError(fmt.Sprintf("Erro: %v", err))
	} else {
		system.StopTorrentMonitor()
		ui.PrintSuccess("PROTEÇÃO DESATIVADA!")
	}
	ui.DrawLine()
	pause()
}

func handleDetailedTorrentStatus() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("STATUS DETALHADO", ui.PanelWidth-4))
	ui.DrawLine()
	status := system.GetDetailedTorrentStatus()

	for name, active := range status {
		check := ui.Red + "✖" + ui.Reset
		if active {
			check = ui.Green + "✔" + ui.Reset
		}
		ui.FormatLine(fmt.Sprintf("%-15s: %s", name, check))
	}
	ui.DrawLine()
	pause()
}

func handleMonitorConnections() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("MONITORAR CONEXÕES", ui.PanelWidth-4))
	ui.DrawLine()

	ui.FormatLine("Pressione Ctrl+C para parar (simulado)")
	ui.DrawLine()
	// Implementação simplificada para visualização
	usage := system.GetBadVPNUsage() // Reutilizando lógica de IP->User
	if len(usage) == 0 {
		ui.FormatLine("Nenhuma atividade suspeita detectada.")
	} else {
		for user, count := range usage {
			ui.FormatLine(fmt.Sprintf("%sUsuário:%s %-15s | %sConexões:%s %d", ui.Bold, ui.Reset, user, ui.Bold, ui.Reset, count))
		}
	}
	ui.DrawLine()
	pause()
}

func handleSetTorrentLevel() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("CONFIGURAR NÍVEL", ui.PanelWidth-4))
	ui.DrawLine()
	ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s BÁSICO (Apenas Portas)", ui.Yellow, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s PRO (Portas + Strings + IPSET)", ui.Yellow, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s AUTO-BAN (PRO + Kick/Ban automático)", ui.Yellow, ui.Reset))
	ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
	ui.DrawLine()

	choice := ui.GetInput("Escolha o nível")
	switch choice {
	case "01", "1":
		db.SetConfig("torrent_level", system.LevelBasic)
		ui.PrintSuccess("Nível BÁSICO configurado.")
	case "02", "2":
		db.SetConfig("torrent_level", system.LevelPro)
		ui.PrintSuccess("Nível PRO configurado.")
	case "03", "3":
		db.SetConfig("torrent_level", system.LevelAutoBan)
		ui.PrintSuccess("Nível AUTO-BAN configurado.")
	}
	time.Sleep(1 * time.Second)
}

func handleOpenVPNMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("MENU OPEN VPN", ui.PanelWidth-4))
		ui.DrawLine()

		status, isActive := system.GetOpenVPNStatus()
		if isActive {
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Green, status, ui.Reset))
		} else {
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Red, status, ui.Reset))
		}
		ui.FormatLine(fmt.Sprintf("%sPORTA:%s  1194 (UDP)", ui.Bold, ui.Reset))
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s INSTALAR OPEN VPN", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s ADICIONAR CLIENTE", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s REMOVER CLIENTE", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s LISTAR CLIENTES", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 05 ]%s DESINSTALAR OPEN VPN", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			handleInstallOpenVPN()
		case "02", "2":
			handleAddOpenVPNClient()
		case "03", "3":
			handleRemoveOpenVPNClient()
		case "04", "4":
			handleListOpenVPNClients()
		case "05", "5":
			handleUninstallOpenVPN()
		case "00", "0":
			return
		default:
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
		}
	}
}

func handleInstallOpenVPN() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("INSTALAR OPEN VPN", ui.PanelWidth-4))
	ui.DrawLine()
	ui.PrintWarning("⏳ Instalando OpenVPN... Isso pode levar alguns minutos.")

	if err := system.InstallOpenVPN(); err != nil {
		ui.PrintError(fmt.Sprintf("Erro na instalação: %v", err))
	} else {
		ui.PrintSuccess("OpenVPN instalado com sucesso!")
	}
	ui.DrawLine()
	pause()
}

func handleAddOpenVPNClient() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("ADICIONAR CLIENTE OPEN VPN", ui.PanelWidth-4))
	ui.DrawLine()

	clientName := ui.GetInput("Digite o nome do cliente")
	if clientName == "" {
		return
	}

	ui.PrintWarning(fmt.Sprintf("⏳ Gerando arquivo .ovpn para %s...", clientName))
	path, err := system.AddOpenVPNClient(clientName)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao criar cliente: %v", err))
	} else {
		ui.PrintSuccess("Cliente criado com sucesso!")
		ui.FormatLine(fmt.Sprintf("Arquivo: %s", path))
		ui.FormatLine("Você pode baixar este arquivo via SFTP/SCP.")
	}
	ui.DrawLine()
	pause()
}

func handleRemoveOpenVPNClient() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("REMOVER CLIENTE OPEN VPN", ui.PanelWidth-4))
	ui.DrawLine()

	clientName := ui.GetInput("Digite o nome do cliente para remover")
	if clientName == "" {
		return
	}

	if err := system.RemoveOpenVPNClient(clientName); err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao remover cliente: %v", err))
	} else {
		ui.PrintSuccess("Cliente removido com sucesso!")
	}
	ui.DrawLine()
	pause()
}

func handleListOpenVPNClients() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("CLIENTES OPEN VPN", ui.PanelWidth-4))
	ui.DrawLine()

	clients, err := system.ListOpenVPNClients()
	if err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao listar clientes: %v", err))
	} else if len(clients) == 0 {
		ui.FormatLine("Nenhum cliente encontrado.")
	} else {
		for i, client := range clients {
			ui.FormatLine(fmt.Sprintf("%s[ %02d ]%s %s", ui.Yellow, i+1, ui.Reset, client))
		}
	}
	ui.DrawLine()
	pause()
}

func handleUninstallOpenVPN() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("DESINSTALAR OPEN VPN", ui.PanelWidth-4))
	ui.DrawLine()

	confirm := ui.GetInput("Deseja realmente remover o OpenVPN do sistema? (s/N)")
	if strings.ToLower(confirm) != "s" {
		return
	}

	if err := system.UninstallOpenVPN(); err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao desinstalar: %v", err))
	} else {
		ui.PrintSuccess("OpenVPN removido completamente!")
	}
	ui.DrawLine()
	pause()
}

func handleHysteriaMenu() {
	for {
		ui.ClearScreen()
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("MENU HYSTERIA 2", ui.PanelWidth-4))
		ui.DrawLine()

		status, isActive := system.GetHysteriaStatus()
		port, _ := db.GetConfig("hysteria_port")
		password, _ := db.GetConfig("hysteria_password")

		if isActive {
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Green, status, ui.Reset))
			ui.FormatLine(fmt.Sprintf("%sPORTA:%s  %s (UDP)", ui.Bold, ui.Reset, port))
			ui.FormatLine(fmt.Sprintf("%sSENHA:%s  %s", ui.Bold, ui.Reset, password))
		} else {
			ui.FormatLine(fmt.Sprintf("%sSTATUS:%s %s%s%s", ui.Bold, ui.Reset, ui.Red, status, ui.Reset))
		}
		ui.DrawLine()

		ui.FormatLine(fmt.Sprintf("%s[ 01 ]%s INSTALAR HYSTERIA 2", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 02 ]%s EXIBIR CONFIGURAÇÃO / LINK", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 03 ]%s DESATIVAR HYSTERIA", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 04 ]%s DESINSTALAR HYSTERIA", ui.Yellow, ui.Reset))
		ui.FormatLine(fmt.Sprintf("%s[ 00 ]%s VOLTAR", ui.Red, ui.Reset))
		ui.DrawLine()

		choice := ui.GetInput("Escolha uma opção")
		switch choice {
		case "01", "1":
			handleInstallHysteria()
		case "02", "2":
			handleShowHysteriaConfig()
		case "03", "3":
			handleDisableHysteria()
		case "04", "4":
			handleUninstallHysteria()
		case "00", "0":
			return
		default:
			ui.PrintError("Opção inválida!")
			time.Sleep(1 * time.Second)
		}
	}
}

func handleInstallHysteria() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("INSTALAR HYSTERIA 2", ui.PanelWidth-4))
	ui.DrawLine()

	portStr := ui.GetInput("Digite a porta para o Hysteria 2 (ex: 443)")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		ui.PrintError("Porta inválida!")
		pause()
		return
	}

	password := ui.GetInput("Digite a senha para autenticação")
	if password == "" {
		ui.PrintError("Senha não pode ser vazia!")
		pause()
		return
	}

	ui.PrintWarning("⏳ Instalando Hysteria 2... Aguarde.")
	if err := system.InstallHysteria(port, password); err != nil {
		ui.PrintError(fmt.Sprintf("Erro na instalação: %v", err))
	} else {
		db.SetConfig("hysteria_port", portStr)
		db.SetConfig("hysteria_password", password)
		ui.PrintSuccess("Hysteria 2 instalado e iniciado com sucesso!")
	}
	ui.DrawLine()
	pause()
}

func handleShowHysteriaConfig() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("CONFIGURAÇÃO HYSTERIA 2", ui.PanelWidth-4))
	ui.DrawLine()

	portStr, _ := db.GetConfig("hysteria_port")
	password, _ := db.GetConfig("hysteria_password")
	port, _ := strconv.Atoi(portStr)

	if portStr == "" {
		ui.PrintError("Hysteria 2 não está configurado.")
	} else {
		ip := system.GetPublicIP()
		link := system.GenerateHysteriaLink(ip, password, port)

		ui.FormatLine(fmt.Sprintf("%sPORTA:%s  %d", ui.Bold, ui.Reset, port))
		ui.FormatLine(fmt.Sprintf("%sSENHA:%s  %s", ui.Bold, ui.Reset, password))
		ui.FormatLine(fmt.Sprintf("%sSNI:%s    www.google.com", ui.Bold, ui.Reset))
		ui.DrawLine()
		ui.FormatLine(ui.CenterText("LINK HY2", ui.PanelWidth-4))
		ui.DrawLine()
		ui.FormatLine(ui.Green + link + ui.Reset)
	}
	ui.DrawLine()
	pause()
}

func handleDisableHysteria() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("DESATIVAR HYSTERIA", ui.PanelWidth-4))
	ui.DrawLine()

	confirm := ui.GetInput("Deseja realmente desativar o Hysteria? (s/N)")
	if strings.ToLower(confirm) != "s" {
		return
	}

	if err := system.DisableHysteria(); err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao desativar: %v", err))
	} else {
		ui.PrintSuccess("Hysteria 2 desativado com sucesso!")
	}
	ui.DrawLine()
	pause()
}

func handleUninstallHysteria() {
	ui.ClearScreen()
	ui.DrawLine()
	ui.FormatLine(ui.CenterText("DESINSTALAR HYSTERIA", ui.PanelWidth-4))
	ui.DrawLine()

	confirm := ui.GetInput("Deseja realmente remover o Hysteria do sistema? (s/N)")
	if strings.ToLower(confirm) != "s" {
		return
	}

	if err := system.UninstallHysteria(); err != nil {
		ui.PrintError(fmt.Sprintf("Erro ao desinstalar: %v", err))
	} else {
		db.SetConfig("hysteria_port", "")
		db.SetConfig("hysteria_password", "")
		ui.PrintSuccess("Hysteria 2 removido completamente!")
	}
	ui.DrawLine()
	pause()
}
