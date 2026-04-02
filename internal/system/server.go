package system

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
)

type ServerInfo struct {
	OS           string
	Architecture string
	VCPUs        int
	CPUModel     string
	Uptime       string
	RAMTotal     string
	RAMUsed      string
	RAMPercent   float64
	CPUUsage     float64
	DiskPercent  float64
	DiskUsed     string
	DiskTotal    string
	DiskFree     string
	NetDownload  string
	NetUpload    string
	PublicIP     string
}

// GetServerInfo returns information about the Linux system
func GetServerInfo() (*ServerInfo, error) {
	info := &ServerInfo{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		VCPUs:        runtime.NumCPU(),
		CPUModel:     "Unknown",
	}

	// CPU info
	if content, err := ioutil.ReadFile("/proc/cpuinfo"); err == nil {
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "model name") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					info.CPUModel = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}

	// Use grep -c cpu[0-9] /proc/stat for cores count as reference
	if output, err := exec.Command("bash", "-c", "grep -c cpu[0-9] /proc/stat").Output(); err == nil {
		if count, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &info.VCPUs); err == nil && count > 0 {
			// Success
		}
	}

	// OS details
	if content, err := ioutil.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				info.OS = strings.Trim(strings.Split(line, "=")[1], "\"")
				break
			}
		}
	}

	// RAM info
	percent, used, total := GetRAMUsage()
	info.RAMPercent = percent
	info.RAMUsed = used
	info.RAMTotal = total

	// CPU Usage (Initial)
	info.CPUUsage = GetCPUUsage()

	// Disk info
	diskPercent, diskUsed, diskTotal, diskFree := GetDiskUsage()
	info.DiskPercent = diskPercent
	info.DiskUsed = diskUsed
	info.DiskTotal = diskTotal
	info.DiskFree = diskFree

	// Network speed (Initial)
	info.NetDownload, info.NetUpload = GetNetworkSpeed()

	// Uptime
	if output, err := exec.Command("uptime", "-p").Output(); err == nil {
		info.Uptime = strings.TrimSpace(string(output))
	} else {
		info.Uptime = "Unknown"
	}

	// Public IP
	info.PublicIP = GetPublicIP()

	return info, nil
}

func OptimizeServer() ([]string, error) {
	var logs []string

	// 1. Atualizar pacotes
	logs = append(logs, "📦 Atualizando lista de pacotes...")
	_ = exec.Command("apt-get", "update", "-y").Run()

	// 2. Corrigir dependências
	logs = append(logs, "🛠️ Corrigindo dependências...")
	_ = exec.Command("apt-get", "-f", "install", "-y").Run()

	// 3. Limpeza de pacotes
	logs = append(logs, "🧹 Removendo pacotes inúteis...")
	_ = exec.Command("apt-get", "autoremove", "-y").Run()
	_ = exec.Command("apt-get", "autoclean", "-y").Run()

	// 4. Limpeza de cache de memória RAM
	logs = append(logs, "🧠 Limpando cache da memória RAM e SWAP...")
	_ = exec.Command("sync").Run()
	_ = exec.Command("bash", "-c", "echo 3 > /proc/sys/vm/drop_caches").Run()
	_ = exec.Command("swapoff", "-a").Run()
	_ = exec.Command("swapon", "-a").Run()

	logs = append(logs, "✅ Otimização concluída com sucesso!")
	return logs, nil
}

func GetPublicIP() string {
	resp, err := http.Get("https://api.ipify.org")
	if err != nil {
		return "Unknown"
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "Unknown"
	}
	return string(body)
}
