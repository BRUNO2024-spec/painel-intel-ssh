package ui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

const (
	Reset  = "\033[0m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Cyan   = "\033[36m"
	White  = "\033[37m"
	Bold   = "\033[1m"
)

func ClearScreen() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func PrintError(msg string) {
	fmt.Printf("%s[!] %s%s\n", Red, msg, Reset)
}

func PrintSuccess(msg string) {
	fmt.Printf("%s[✓] %s%s\n", Green, msg, Reset)
}

func PrintWarning(msg string) {
	fmt.Printf("%s[!] %s%s\n", Yellow, msg, Reset)
}

const PanelWidth = 60

func DrawLine() {
	fmt.Printf("%s%s%s%s\n", Bold, Cyan, strings.Repeat("=", PanelWidth), Reset)
}

func visualWidth(text string) int {
	return utf8.RuneCountInString(stripANSI(text))
}

func CenterText(text string, width int) string {
	vw := visualWidth(text)
	if vw >= width {
		return text
	}
	padding := (width - vw) / 2
	return strings.Repeat(" ", padding) + text + strings.Repeat(" ", width-vw-padding)
}

func FormatLine(content string) {
	vw := visualWidth(content)
	padding := PanelWidth - 4 - vw
	if padding < 0 {
		padding = 0
	}
	fmt.Printf("%s%s| %s%s%s%s %s%s|%s\n", Bold, Cyan, Reset, content, strings.Repeat(" ", padding), Bold, Cyan, Reset, Reset)
}

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

func GetInput(prompt string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s[+] %s%s\n> ", Cyan, prompt, Reset)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func GetPasswordInput(prompt string) (string, error) {
	fmt.Printf("%s[+] %s%s\n> ", Cyan, prompt, Reset)

	bytePassword, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}

	fmt.Println() // Newline after input
	return strings.TrimSpace(string(bytePassword)), nil
}
