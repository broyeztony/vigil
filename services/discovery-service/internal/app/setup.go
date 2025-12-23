package app

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stoik/vigil/services/discovery-service/internal/db"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup database and create initial tenant",
	Long:  "Creates database tables and inserts an initial tenant record for development/testing",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Initialize database
		if err := db.Init(ctx); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer db.Close()

		// Run migrations
		fmt.Println("Running migrations...")
		migrationSQL := `
			-- Tenant table (single record per database)
			CREATE TABLE IF NOT EXISTS tenant (
			    id UUID PRIMARY KEY,
			    name VARCHAR(255),
			    provider VARCHAR(2)
			);

			-- Users table
			CREATE TABLE IF NOT EXISTS users (
			    id UUID PRIMARY KEY,
			    email VARCHAR(255) NOT NULL UNIQUE,
			    last_email_check TIMESTAMP WITH TIME ZONE,
			    last_email_received TIMESTAMP WITH TIME ZONE
			);

			CREATE INDEX IF NOT EXISTS idx_users_last_email_received ON users(last_email_received);

			-- Emails table (stores minimal metadata only - zero copy principle)
			CREATE TABLE IF NOT EXISTS emails (
			    id UUID PRIMARY KEY,
			    fingerprint VARCHAR(64) NOT NULL UNIQUE,
			    received_at TIMESTAMP WITH TIME ZONE NOT NULL
			);

			CREATE INDEX IF NOT EXISTS idx_emails_received_at ON emails(received_at);
			CREATE INDEX IF NOT EXISTS idx_emails_fingerprint ON emails(fingerprint);

			-- User to Emails junction table (many-to-many relationship)
			CREATE TABLE IF NOT EXISTS user_emails (
			    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			    email_id UUID NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
			    PRIMARY KEY (user_id, email_id)
			);

			CREATE INDEX IF NOT EXISTS idx_user_emails_user_id ON user_emails(user_id);
			CREATE INDEX IF NOT EXISTS idx_user_emails_email_id ON user_emails(email_id);
		`

		if _, err := db.Pool.Exec(ctx, migrationSQL); err != nil {
			return fmt.Errorf("failed to run migrations: %w", err)
		}

		// Insert test tenant
		fmt.Println("Inserting test tenant...")
		testTenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
		insertTenantSQL := `
			INSERT INTO tenant (id, name, provider)
			VALUES ($1, $2, $3)
			ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, provider = EXCLUDED.provider
		`

		if _, err := db.Pool.Exec(ctx, insertTenantSQL, testTenantID, "ACME Corp.", "GA"); err != nil {
			return fmt.Errorf("failed to insert test tenant: %w", err)
		}

		fmt.Printf("✓ Database setup complete. Test tenant: %s (ACME Corp., GA)\n", testTenantID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
