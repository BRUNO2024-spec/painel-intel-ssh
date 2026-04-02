package models

import "time"

// XrayUser representa um cliente XRAY vinculado a um usuário SSH.
type XrayUser struct {
	ID              int       `json:"id"`
	Username        string    `json:"username"`        // Login SSH = campo email no config.json do XRAY
	UUID            string    `json:"uuid"`            // UUID do protocolo VLESS/VMESS
	ExpiresAt       time.Time `json:"expires_at"`      // Data de expiração (zero = sem expiração)
	ConnectionLimit int       `json:"connection_limit"` // Limite de conexões simultâneas
	Status          string    `json:"status"`          // "active" | "suspended"
	CreatedAt       time.Time `json:"created_at"`
}

// IsExpired retorna true se a data de expiração já passou.
func (u *XrayUser) IsExpired() bool {
	if u.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(u.ExpiresAt)
}

// IsActive retorna true se o usuário está com status "active".
func (u *XrayUser) IsActive() bool {
	return u.Status == "active"
}

// ExpiresAtStr retorna a data de expiração formatada ou "Nunca".
func (u *XrayUser) ExpiresAtStr() string {
	if u.ExpiresAt.IsZero() {
		return "Nunca"
	}
	return u.ExpiresAt.Format("02/01/2006")
}
