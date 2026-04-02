package models

import "time"

type User struct {
	ID             int       `json:"id"`
	Username       string    `json:"username"`
	Password       string    `json:"password"`
	Limit          int       `json:"limit"`
	ExpirationDate time.Time `json:"expiration_date"`
	XrayUUID       string    `json:"xray_uuid"`
	Type           string    `json:"type"` // "premium" or "teste"
	CreatedAt      time.Time `json:"created_at"`
}

func (u *User) IsExpired() bool {
	if u.ExpirationDate.IsZero() {
		return false
	}
	return time.Now().After(u.ExpirationDate)
}
