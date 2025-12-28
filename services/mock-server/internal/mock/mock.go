package mock

import (
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/stoik/vigil/services/mock-server/internal/models"
)

var (
	firstNames = []string{"John", "Jane", "Bob", "Alice", "Charlie", "Diana", "Eve", "Frank"}
	lastNames  = []string{"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis"}
	domains    = []string{"example.com", "company.com", "business.org", "enterprise.net"}
	subjects   = []string{
		"Meeting tomorrow",
		"Project update",
		"Budget review",
		"Team lunch",
		"Quarterly report",
		"Client feedback",
		"Urgent: Action required",
		"Follow up",
	}

	// Static user list - maintained across calls
	userList        []models.ProviderUser
	userListMutex   sync.RWMutex
	defaultTenantID uuid.UUID
	userCounter     int // Counter for generating unique user names

	// Email storage - maintained in memory per user
	emailStore           map[uuid.UUID][]models.ProviderEmail
	emailStoreMutex      sync.RWMutex
	emailGenerationStart time.Time
)

func init() {
	// Initialize with a default tenant ID
	defaultTenantID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

	// Initialize with 5000 users
	userList = make([]models.ProviderUser, 0, 5000)
	emailStore = make(map[uuid.UUID][]models.ProviderEmail)
	emailGenerationStart = time.Now()

	for i := 0; i < 5000; i++ {
		user := generateUser(defaultTenantID, i)
		userList = append(userList, user)
		// Initialize empty email list for each user
		emailStore[user.ID] = make([]models.ProviderEmail, 0)
	}
	userCounter = 5000

	// Start background goroutine to generate emails every 30 seconds
	go generateEmailsPeriodically()
}

func generateUser(tenantID uuid.UUID, index int) models.ProviderUser {
	firstName := firstNames[index%len(firstNames)]
	lastName := lastNames[index%len(lastNames)]
	domain := domains[index%len(domains)]

	return models.ProviderUser{
		ID:        uuid.New(),
		Email:     fmt.Sprintf("%s.%s.%d@%s", firstName, lastName, index, domain),
		Name:      fmt.Sprintf("%s %s", firstName, lastName),
		TenantID:  tenantID,
		Active:    true,
		CreatedAt: time.Now().Add(-time.Duration(rand.Intn(365)) * 24 * time.Hour),
	}
}

// GetGoogleUsers returns the static list of mocked Google users
// Always returns the same list in the same order, regardless of tenantID
func GetGoogleUsers(tenantID uuid.UUID) ([]models.ProviderUser, error) {
	userListMutex.RLock()
	defer userListMutex.RUnlock()

	// Return a copy of the list to prevent external modification
	users := make([]models.ProviderUser, len(userList))
	copy(users, userList)

	return users, nil
}

// AddUsers adds new users to the static list
func AddUsers(numUsers int) (int, error) {
	if numUsers < 1 {
		return 0, fmt.Errorf("numUsers must be at least 1")
	}

	userListMutex.Lock()
	emailStoreMutex.Lock()
	defer userListMutex.Unlock()
	defer emailStoreMutex.Unlock()

	for i := 0; i < numUsers; i++ {
		user := generateUser(defaultTenantID, userCounter)
		userList = append(userList, user)
		// Initialize empty email list for new user
		emailStore[user.ID] = make([]models.ProviderEmail, 0)
		userCounter++
	}

	return len(userList), nil
}

// generateEmailsPeriodically generates 0-3 emails for each user every 30 seconds
func generateEmailsPeriodically() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		userListMutex.RLock()
		users := make([]models.ProviderUser, len(userList))
		copy(users, userList)
		userListMutex.RUnlock()

		emailStoreMutex.Lock()
		now := time.Now()

		for _, user := range users {
			// Generate 0-3 emails for this user
			numEmails := rand.Intn(4) // 0, 1, 2, or 3

			for i := 0; i < numEmails; i++ {
				// Generate timestamp slightly before now (within last 30 seconds)
				// Spread them out a bit
				secondsAgo := time.Duration(rand.Intn(30)) * time.Second
				receivedAt := now.Add(-secondsAgo)

				// Get current email count for this user to use as unique identifier
				emailCount := len(emailStore[user.ID])
				email := generateEmail(user.ID, user.Email, user.Name, receivedAt, emailCount, i)
				emailStore[user.ID] = append(emailStore[user.ID], email)
			}
		}

		emailStoreMutex.Unlock()
	}
}

func generateEmail(userID uuid.UUID, userEmail string, userName string, receivedAt time.Time, emailIndex int, batchIndex int) models.ProviderEmail {
	subject := subjects[rand.Intn(len(subjects))]
	fromDomain := domains[rand.Intn(len(domains))]
	fromEmail := fmt.Sprintf("sender%d@%s", rand.Intn(50000), fromDomain)
	messageID := uuid.New()

	// Include recipient info in body to make emails unique per user
	// Add multiple unique identifiers to ensure each email has a unique fingerprint
	bodyContent := fmt.Sprintf(
		"Dear %s (%s),\n\n"+
			"Full email body for: %s\n\n"+
			"This is mock content specifically for you.\n"+
			"Received at: %s\n"+
			"Message ID: %s\n"+
			"Email index: %d\n"+
			"Batch index: %d\n"+
			"Random token: %d\n"+
			"User ID: %s\n\n"+
			"Best regards,\nThe Mock Server",
		userName,
		userEmail,
		subject,
		receivedAt.Format(time.RFC3339Nano), // Use nanosecond precision
		messageID.String(),
		emailIndex,
		batchIndex,
		rand.Intn(5000000), // Random token for extra uniqueness
		userID.String(),
	)

	return models.ProviderEmail{
		MessageID:  messageID.String(),
		UserID:     userID,
		From:       fromEmail,
		To:         userEmail,                                   // Send to the actual user
		Subject:    fmt.Sprintf("%s [%d]", subject, emailIndex), // Add index to subject too
		Snippet:    fmt.Sprintf("This is a snippet for: %s", subject),
		ReceivedAt: receivedAt,
		Body:       bodyContent,
	}
}

// GetGoogleEmails returns emails for a user, filtered by receivedAfter
func GetGoogleEmails(userID uuid.UUID, receivedAfter time.Time, orderBy string) ([]models.ProviderEmail, error) {
	emailStoreMutex.RLock()
	defer emailStoreMutex.RUnlock()

	userEmails, exists := emailStore[userID]
	if !exists {
		// User doesn't exist, return empty list
		return []models.ProviderEmail{}, nil
	}

	// Filter emails by receivedAfter
	filtered := make([]models.ProviderEmail, 0)
	for _, email := range userEmails {
		if email.ReceivedAt.After(receivedAfter) || email.ReceivedAt.Equal(receivedAfter) {
			filtered = append(filtered, email)
		}
	}

	// Sort by received_at
	if orderBy == "received_at" || orderBy == "" {
		// Sort ascending
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].ReceivedAt.Before(filtered[j].ReceivedAt)
		})
	} else if orderBy == "received_at desc" {
		// Sort descending
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].ReceivedAt.After(filtered[j].ReceivedAt)
		})
	}

	return filtered, nil
}
