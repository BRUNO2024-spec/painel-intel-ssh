package db

import (
	"database/sql"
	"os/exec"
	"painel-ssh/internal/models"
	"path/filepath"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

var database *sql.DB

func InitDB(dbPath string) error {
	var err error
	database, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	// Garantir que o diretório pai tenha permissões de execução para todos (necessário para o banner)
	dir := filepath.Dir(dbPath)
	exec.Command("chmod", "755", dir).Run()

	// Garantir que o banco de dados tenha permissão de leitura para todos os usuários (necessário para o banner)
	exec.Command("chmod", "666", dbPath).Run()

	createTableQuery := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL,
		conn_limit INTEGER DEFAULT 1,
		expiration_date DATETIME,
		xray_uuid TEXT,
		type TEXT DEFAULT 'premium',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err = database.Exec(createTableQuery)
	if err != nil {
		return err
	}

	// Adicionar coluna 'type' se não existir (Migração básica)
	_, _ = database.Exec("ALTER TABLE users ADD COLUMN type TEXT DEFAULT 'premium'")

	createConfigTable := `
	CREATE TABLE IF NOT EXISTS config (
		key TEXT PRIMARY KEY,
		value TEXT
	);`
	_, err = database.Exec(createConfigTable)
	if err != nil {
		return err
	}

	createXrayUsersTable := `
	CREATE TABLE IF NOT EXISTS xray_users (
		id               INTEGER PRIMARY KEY AUTOINCREMENT,
		username         TEXT UNIQUE NOT NULL,
		uuid             TEXT NOT NULL,
		expires_at       DATETIME,
		connection_limit INTEGER DEFAULT 1,
		status           TEXT DEFAULT 'active',
		created_at       DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	_, err = database.Exec(createXrayUsersTable)
	if err != nil {
		return err
	}

	createAbuseLogsTable := `
	CREATE TABLE IF NOT EXISTS abuse_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL,
		ip TEXT NOT NULL,
		attempts INTEGER DEFAULT 0,
		last_attempt DATETIME,
		status TEXT DEFAULT 'active'
	);`
	_, err = database.Exec(createAbuseLogsTable)
	return err
}

func AddAbuseAttempt(username, ip string) (int, error) {
	var attempts int
	err := database.QueryRow("SELECT attempts FROM abuse_logs WHERE username = ?", username).Scan(&attempts)
	if err == sql.ErrNoRows {
		_, err = database.Exec("INSERT INTO abuse_logs (username, ip, attempts, last_attempt) VALUES (?, ?, 1, ?)", username, ip, time.Now())
		return 1, err
	} else if err != nil {
		return 0, err
	}

	attempts++
	_, err = database.Exec("UPDATE abuse_logs SET attempts = ?, ip = ?, last_attempt = ? WHERE username = ?", attempts, ip, time.Now(), username)
	return attempts, err
}

func GetUserAbuseStatus(username string) (int, string, error) {
	var attempts int
	var status string
	err := database.QueryRow("SELECT attempts, status FROM abuse_logs WHERE username = ?", username).Scan(&attempts, &status)
	if err == sql.ErrNoRows {
		return 0, "active", nil
	}
	return attempts, status, err
}

func BanUser(username string) error {
	_, err := database.Exec("UPDATE abuse_logs SET status = 'banned' WHERE username = ?", username)
	return err
}

func ResetUserAbuse(username string) error {
	_, err := database.Exec("DELETE FROM abuse_logs WHERE username = ?", username)
	return err
}

func GetTotalBannedUsers() (int, error) {
	var count int
	err := database.QueryRow("SELECT COUNT(*) FROM abuse_logs WHERE status = 'banned'").Scan(&count)
	return count, err
}

func GetTotalAbuseAttemptsToday() (int, error) {
	var count int
	today := time.Now().Format("2006-01-02")
	err := database.QueryRow("SELECT SUM(attempts) FROM abuse_logs WHERE last_attempt >= ?", today).Scan(&count)
	if err != nil {
		return 0, nil // Pode ser NULL se não houver tentativas
	}
	return count, err
}

func GetAbuseLogs() ([]map[string]interface{}, error) {
	rows, err := database.Query("SELECT username, ip, attempts, last_attempt, status FROM abuse_logs ORDER BY last_attempt DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var username, ip, status string
		var attempts int
		var lastAttempt time.Time
		err := rows.Scan(&username, &ip, &attempts, &lastAttempt, &status)
		if err != nil {
			return nil, err
		}
		logs = append(logs, map[string]interface{}{
			"username":     username,
			"ip":           ip,
			"attempts":     attempts,
			"last_attempt": lastAttempt,
			"status":       status,
		})
	}
	return logs, nil
}

func SetConfig(key, value string) error {
	_, err := database.Exec("INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)", key, value)
	return err
}

func GetConfig(key string) (string, error) {
	var value string
	err := database.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func SaveUser(user *models.User) error {
	query := `INSERT INTO users (username, password, conn_limit, expiration_date, xray_uuid, type, created_at) 
			  VALUES (?, ?, ?, ?, ?, ?, ?)`
	// Garantir que salvamos em UTC para evitar problemas de timezone
	expDate := user.ExpirationDate.UTC()
	createdAt := user.CreatedAt.UTC()
	_, err := database.Exec(query, user.Username, user.Password, user.Limit, expDate, user.XrayUUID, user.Type, createdAt)
	return err
}

func GetUsers() ([]models.User, error) {
	rows, err := database.Query("SELECT id, username, password, conn_limit, expiration_date, xray_uuid, type, created_at FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		var expDate sql.NullTime
		var xrayUUID sql.NullString
		var userType sql.NullString
		err := rows.Scan(&user.ID, &user.Username, &user.Password, &user.Limit, &expDate, &xrayUUID, &userType, &user.CreatedAt)
		if err != nil {
			return nil, err
		}
		if expDate.Valid {
			user.ExpirationDate = expDate.Time
		}
		if xrayUUID.Valid {
			user.XrayUUID = xrayUUID.String
		}
		if userType.Valid {
			user.Type = userType.String
		} else {
			user.Type = "premium"
		}
		users = append(users, user)
	}
	return users, nil
}

func GetUserByUsername(username string) (*models.User, error) {
	var user models.User
	var expDate sql.NullTime
	var xrayUUID sql.NullString
	var userType sql.NullString
	// Usar COLLATE NOCASE para busca insensível a maiúsculas/minúsculas
	query := "SELECT id, username, password, conn_limit, expiration_date, xray_uuid, type, created_at FROM users WHERE username = ? COLLATE NOCASE"
	err := database.QueryRow(query, username).Scan(&user.ID, &user.Username, &user.Password, &user.Limit, &expDate, &xrayUUID, &userType, &user.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if expDate.Valid {
		user.ExpirationDate = expDate.Time
	}
	if xrayUUID.Valid {
		user.XrayUUID = xrayUUID.String
	}
	if userType.Valid {
		user.Type = userType.String
	} else {
		user.Type = "premium"
	}
	return &user, nil
}

func DeleteUser(username string) error {
	// Usar COLLATE NOCASE para busca insensível a maiúsculas/minúsculas
	_, err := database.Exec("DELETE FROM users WHERE username = ? COLLATE NOCASE", username)
	if err != nil {
		return err
	}
	// Também remover dos logs de abuso e usuários xray
	_, _ = database.Exec("DELETE FROM xray_users WHERE username = ? COLLATE NOCASE", username)
	_, _ = database.Exec("DELETE FROM abuse_logs WHERE username = ? COLLATE NOCASE", username)
	return nil
}

func UpdatePassword(username, password string) error {
	_, err := database.Exec("UPDATE users SET password = ? WHERE username = ?", password, username)
	return err
}

func UpdateLimit(username string, limit int) error {
	_, err := database.Exec("UPDATE users SET conn_limit = ? WHERE username = ?", limit, username)
	return err
}

func UpdateExpiration(username string, expiration time.Time) error {
	_, err := database.Exec("UPDATE users SET expiration_date = ? WHERE username = ?", expiration.UTC(), username)
	return err
}

func GetTotalUsers() (int, error) {
	var count int
	err := database.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func GetExpiredUsersCount() (int, error) {
	var count int
	err := database.QueryRow("SELECT COUNT(*) FROM users WHERE expiration_date IS NOT NULL AND expiration_date < ?", time.Now()).Scan(&count)
	return count, err
}
