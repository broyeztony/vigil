package provider

import (
	"time"

	"github.com/google/uuid"
	"github.com/stoik/vigil/internal/models"
)

// Provider defines the interface for email provider clients (Google, Microsoft, etc.)
type Provider interface {
	// GetUsers retrieves all users for a given tenant
	GetUsers(tenantID uuid.UUID) ([]models.ProviderUser, error)

	// GetEmails retrieves emails for a given user, filtered by receivedAfter timestamp
	// orderBy specifies the sort order (e.g., "received_at")
	GetEmails(userID uuid.UUID, receivedAfter time.Time, orderBy string) ([]models.ProviderEmail, error)
}
