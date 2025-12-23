package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/viper"
	"github.com/stoik/vigil/internal/models"
)

// GoogleProvider implements the Provider interface for Google Workspace
type GoogleProvider struct {
	baseURL string
	client  *http.Client
}

// NewGoogleProvider creates a new Google provider client
func NewGoogleProvider() *GoogleProvider {
	baseURL := viper.GetString("provider.api_url")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	return &GoogleProvider{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetUsers implements Provider.GetUsers for Google Workspace
func (g *GoogleProvider) GetUsers(tenantID uuid.UUID) ([]models.ProviderUser, error) {
	url := fmt.Sprintf("%s/google/users/%s", g.baseURL, tenantID.String())
	
	resp, err := g.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var users []models.ProviderUser
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return users, nil
}

// GetEmails implements Provider.GetEmails for Google Workspace
func (g *GoogleProvider) GetEmails(userID uuid.UUID, receivedAfter time.Time, orderBy string) ([]models.ProviderEmail, error) {
	url := fmt.Sprintf("%s/google/emails/%s", g.baseURL, userID.String())
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Set("receivedAfter", receivedAfter.Format(time.RFC3339))
	q.Set("orderBy", orderBy)
	req.URL.RawQuery = q.Encode()

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get emails: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var emails []models.ProviderEmail
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return emails, nil
}

// MicrosoftProvider implements the Provider interface for Microsoft O365
type MicrosoftProvider struct {
	baseURL string
	client  *http.Client
}

// NewMicrosoftProvider creates a new Microsoft provider client
func NewMicrosoftProvider() *MicrosoftProvider {
	baseURL := viper.GetString("provider.api_url")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	return &MicrosoftProvider{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetUsers implements Provider.GetUsers for Microsoft O365
func (m *MicrosoftProvider) GetUsers(tenantID uuid.UUID) ([]models.ProviderUser, error) {
	url := fmt.Sprintf("%s/microsoft/users/%s", m.baseURL, tenantID.String())
	
	resp, err := m.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var users []models.ProviderUser
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return users, nil
}

// GetEmails implements Provider.GetEmails for Microsoft O365
func (m *MicrosoftProvider) GetEmails(userID uuid.UUID, receivedAfter time.Time, orderBy string) ([]models.ProviderEmail, error) {
	url := fmt.Sprintf("%s/microsoft/emails/%s", m.baseURL, userID.String())
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Set("receivedAfter", receivedAfter.Format(time.RFC3339))
	q.Set("orderBy", orderBy)
	req.URL.RawQuery = q.Encode()

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get emails: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var emails []models.ProviderEmail
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return emails, nil
}

// NewProvider creates a provider instance based on configuration
// provider.type can be "google" or "microsoft" (defaults to "google")
func NewProvider() Provider {
	providerType := viper.GetString("provider.type")
	if providerType == "" {
		providerType = "google" // Default to Google
	}

	switch providerType {
	case "microsoft":
		return NewMicrosoftProvider()
	case "google":
		fallthrough
	default:
		return NewGoogleProvider()
	}
}

