package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stoik/vigil/services/discovery-service/internal/db"
	"github.com/stoik/vigil/services/discovery-service/internal/discovery"
)

var rootCmd = &cobra.Command{
	Use:   "discovery",
	Short: "Vigil Discovery Service",
	Long:  "Discovers users and emails for tenants using the mock provider API",
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the discovery service",
	Long:  "Continuously discovers users and emails for configured tenants",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Initialize database
		if err := db.Init(ctx); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer db.Close()

		// Get tenant ID from config
		tenantIDStr := viper.GetString("tenant_id")
		if tenantIDStr == "" {
			return fmt.Errorf("tenant_id not configured")
		}

		// Start discovery service
		service := discovery.NewService()

		// Handle graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		// Run discovery in background
		errChan := make(chan error, 1)
		go func() {
			errChan <- service.Run(ctx, tenantIDStr)
		}()

		// Wait for signal or error
		select {
		case <-sigChan:
			fmt.Println("\nShutting down gracefully...")
			cancel()
			
			// Wait for service to stop (with timeout)
			graceful := service.Shutdown(10 * time.Second)
			if !graceful {
				fmt.Println("Warning: Some operations may not have completed")
			}
			
			// Wait for Run() to return
			select {
			case err := <-errChan:
				if err != nil {
					return err
				}
			case <-time.After(2 * time.Second):
				fmt.Println("Service did not stop within timeout")
			}
			
			return nil
		case err := <-errChan:
			return err
		}
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	// Flags
	rootCmd.PersistentFlags().String("database.url", "postgres://user:password@localhost:5432/vigil?sslmode=disable", "Database connection URL")
	rootCmd.PersistentFlags().String("tenant_id", "", "Tenant ID to discover users and emails for")
	rootCmd.PersistentFlags().String("provider.type", "google", "Provider type: 'google' or 'microsoft'")
	rootCmd.PersistentFlags().String("provider.api_url", "http://localhost:8080", "Provider API base URL")

	// Bind flags to viper
	viper.BindPFlag("database.url", rootCmd.PersistentFlags().Lookup("database.url"))
	viper.BindPFlag("tenant_id", rootCmd.PersistentFlags().Lookup("tenant_id"))
	viper.BindPFlag("provider.type", rootCmd.PersistentFlags().Lookup("provider.type"))
	viper.BindPFlag("provider.api_url", rootCmd.PersistentFlags().Lookup("provider.api_url"))

	rootCmd.AddCommand(runCmd)
}

func initConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./services/discovery-service")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintf(os.Stderr, "Using config file: %s\n", viper.ConfigFileUsed())
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
