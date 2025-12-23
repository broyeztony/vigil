package models

import (
	"time"

	"github.com/google/uuid"
)

// ProviderEmail represents an email from any email provider (Google, Microsoft, etc.)
type ProviderEmail struct {
	MessageID  string    `json:"message_id"`
	UserID     uuid.UUID `json:"user_id"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	Subject    string    `json:"subject"`
	Snippet    string    `json:"snippet"`
	ReceivedAt time.Time `json:"received_at"`
	Body       string    `json:"body,omitempty"` // Full content, optional
}

// GoogleEmail is an alias for ProviderEmail (backward compatibility)
type GoogleEmail = ProviderEmail

// Email database model (minimal metadata only - zero copy principle)
// id is the message_id from the provider API (parsed as UUID)
// fingerprint is a hash of email body content for identification
// Full content is not stored - fetch from provider API when needed
type Email struct {
	ID          uuid.UUID `db:"id"`
	Fingerprint string    `db:"fingerprint"`
	ReceivedAt  time.Time `db:"received_at"`
}

type UserEmail struct {
	UserID  uuid.UUID `db:"user_id"`
	EmailID uuid.UUID `db:"email_id"`
}

