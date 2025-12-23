package models

import (
	"time"

	"github.com/google/uuid"
)

// ProviderUser represents a user from any email provider (Google, Microsoft, etc.)
type ProviderUser struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	TenantID  uuid.UUID `json:"tenant_id"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// GoogleUser is an alias for ProviderUser (backward compatibility)
type GoogleUser = ProviderUser

