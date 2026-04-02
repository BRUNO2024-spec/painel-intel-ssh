package db

import (
	"database/sql"
	"painel-ssh/internal/models"
	"time"
)

// ── CRUD xray_users ───────────────────────────────────────────────────────────

// SaveXrayUser insere ou substitui um XrayUser no banco.
func SaveXrayUser(user *models.XrayUser) error {
	query := `INSERT OR REPLACE INTO xray_users
		(username, uuid, expires_at, connection_limit, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`
	_, err := database.Exec(query,
		user.Username,
		user.UUID,
		nullableTime(user.ExpiresAt.UTC()),
		user.ConnectionLimit,
		user.Status,
		user.CreatedAt.UTC(),
	)
	return err
}

// GetXrayUsers retorna todos os usuários XRAY ordenados por data de criação.
func GetXrayUsers() ([]models.XrayUser, error) {
	rows, err := database.Query(
		`SELECT id, username, uuid, expires_at, connection_limit, status, created_at
		 FROM xray_users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.XrayUser
	for rows.Next() {
		var u models.XrayUser
		var expiresAt sql.NullTime
		err := rows.Scan(
			&u.ID, &u.Username, &u.UUID,
			&expiresAt, &u.ConnectionLimit, &u.Status, &u.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		if expiresAt.Valid {
			u.ExpiresAt = expiresAt.Time
		}
		users = append(users, u)
	}
	return users, nil
}

// GetXrayUserByUsername busca um XrayUser pelo username SSH.
func GetXrayUserByUsername(username string) (*models.XrayUser, error) {
	var u models.XrayUser
	var expiresAt sql.NullTime
	err := database.QueryRow(
		`SELECT id, username, uuid, expires_at, connection_limit, status, created_at
		 FROM xray_users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.UUID,
			&expiresAt, &u.ConnectionLimit, &u.Status, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		u.ExpiresAt = expiresAt.Time
	}
	return &u, nil
}

// UpdateXrayUserStatus atualiza o status de um usuário XRAY ("active" | "suspended").
func UpdateXrayUserStatus(username, status string) error {
	_, err := database.Exec(
		"UPDATE xray_users SET status = ? WHERE username = ?", status, username)
	return err
}

// UpdateXrayUserExpiration atualiza a data de expiração de um usuário XRAY.
func UpdateXrayUserExpiration(username string, expiresAt time.Time) error {
	_, err := database.Exec(
		"UPDATE xray_users SET expires_at = ? WHERE username = ?",
		nullableTime(expiresAt.UTC()), username)
	return err
}

// DeleteXrayUser remove um usuário XRAY do banco de dados.
func DeleteXrayUser(username string) error {
	_, err := database.Exec("DELETE FROM xray_users WHERE username = ?", username)
	return err
}

// nullableTime retorna nil para o zero value de time.Time (NULL no SQLite).
func nullableTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}
