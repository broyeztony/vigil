package models

import (
	"time"

	"github.com/google/uuid"
)

// User model for database (not shared with provider API)
type User struct {
	ID               uuid.UUID  `db:"id"`
	Email            string     `db:"email"`
	LastEmailCheck   *time.Time `db:"last_email_check"`
	LastEmailReceived *time.Time `db:"last_email_received"`
}

