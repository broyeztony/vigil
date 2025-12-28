package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stoik/vigil/internal/models"
	"github.com/stoik/vigil/services/discovery-service/internal/db"
	discoverymodels "github.com/stoik/vigil/services/discovery-service/internal/models"
	"github.com/stoik/vigil/services/discovery-service/internal/provider"
)

// UserMessage represents a message from user discovery to email discovery
type UserMessage struct {
	Type   string // "ADD_USER" or "REMOVE_USER"
	UserID uuid.UUID
}

type Service struct {
	provider provider.Provider
	// Message channel for user discovery to communicate with email discovery
	userMessages chan UserMessage
	activeUsers  sync.Map // map[uuid.UUID]*userEmailDiscovery
	// Channel to notify fan-in when user channels change
	channelsChanged chan struct{}
	// Track if initial batch discovery is complete
	initialDiscoveryDone  bool
	initialDiscoveryMutex sync.Mutex
	// Performance metrics
	emailsPerUser    sync.Map // map[uuid.UUID]*int64 (atomic counter)
	emailsToQueue    int64    // atomic counter
	emailsDiscovered int64    // atomic counter
	// WaitGroup to track active email processing goroutines
	processingWg sync.WaitGroup
}

type userEmailDiscovery struct {
	user    discoverymodels.User
	ctx     context.Context
	cancel  context.CancelFunc
	channel <-chan EmailWithUser
}

const (
	MessageAddUser    = "ADD_USER"
	MessageRemoveUser = "REMOVE_USER"
	PollingInterval   = 30 * time.Second // Fixed 30 seconds for all users
	ChannelBufferSize = 50               // Buffered channel size per user
	PollingJitterMax  = 30 * time.Second // Maximum jitter to stagger initial polls
)

func NewService() *Service {
	return &Service{
		provider:        provider.NewProvider(),
		userMessages:    make(chan UserMessage), // Unbuffered channel
		channelsChanged: make(chan struct{}),    // Unbuffered channel
	}
}

func (s *Service) Run(ctx context.Context, tenantIDStr string) error {
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	log.Printf("Starting discovery service for tenant: %s", tenantID)

	// Start email discovery service (waits for messages and manages fan-in)
	go s.emailDiscoveryService(ctx)

	// Start user discovery service (sends messages)
	go s.userDiscoveryService(ctx, tenantID)

	// Start performance metrics logger
	go s.logPerformanceMetrics(ctx)

	// Start dynamic fan-in and process emails directly
	s.dynamicFanInAndProcess(ctx)

	return nil
}

// Shutdown gracefully shuts down the service, waiting for all processing goroutines to complete
// with a timeout. Returns true if shutdown completed gracefully, false if timeout was reached.
func (s *Service) Shutdown(timeout time.Duration) bool {
	log.Printf("Shutting down discovery service, waiting up to %v for processing to complete...", timeout)

	// Channel to signal when WaitGroup completes
	done := make(chan struct{})
	go func() {
		s.processingWg.Wait()
		close(done)
	}()

	// Wait for either completion or timeout
	select {
	case <-done:
		log.Println("All processing goroutines completed successfully")
		return true
	case <-time.After(timeout):
		log.Printf("Shutdown timeout (%v) reached, some processing may still be in progress", timeout)
		return false
	}
}

// userDiscoveryService periodically discovers users and sends ADD_USER/REMOVE_USER messages
func (s *Service) userDiscoveryService(ctx context.Context, tenantID uuid.UUID) {
	ticker := time.NewTicker(1 * time.Minute) // Discover users every minute
	defer ticker.Stop()

	// Initial discovery
	if err := s.discoverUsersOnce(ctx, tenantID); err != nil {
		log.Printf("Error in initial user discovery: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.discoverUsersOnce(ctx, tenantID); err != nil {
				log.Printf("Error discovering users: %v", err)
			}
		}
	}
}

func (s *Service) discoverUsersOnce(ctx context.Context, tenantID uuid.UUID) error {
	// Get current users from provider
	providerUsers, err := s.provider.GetUsers(tenantID)
	if err != nil {
		return fmt.Errorf("failed to get users from provider: %w", err)
	}

	log.Printf("Discovered %d users from provider for tenant %s", len(providerUsers), tenantID)

	// Get current users from database
	dbUsers, err := s.getUsers(ctx)
	if err != nil {
		return fmt.Errorf("failed to get users from database: %w", err)
	}

	// Create maps for comparison
	providerUserMap := make(map[uuid.UUID]bool)

	// Check if this is initial discovery (batch mode) or incremental (message mode)
	s.initialDiscoveryMutex.Lock()
	isInitial := !s.initialDiscoveryDone
	if isInitial {
		s.initialDiscoveryDone = true
	}
	s.initialDiscoveryMutex.Unlock()

	var usersToAdd []discoverymodels.User

	for _, pUser := range providerUsers {
		providerUserMap[pUser.ID] = true
		// Upsert user in database
		if err := s.upsertUser(ctx, pUser); err != nil {
			log.Printf("Error upserting user %s: %v", pUser.ID, err)
		}
		// Collect users to add
		if _, exists := s.activeUsers.Load(pUser.ID); !exists {
			if isInitial {
				// Batch mode: collect for batch addition
				dbUser, err := s.getUserByID(ctx, pUser.ID)
				if err == nil {
					usersToAdd = append(usersToAdd, dbUser)
				}
			} else {
				// Incremental mode: send message for individual handling
				s.userMessages <- UserMessage{Type: MessageAddUser, UserID: pUser.ID}
			}
		}
	}

	// Batch add all users synchronously only for initial discovery
	if isInitial && len(usersToAdd) > 0 {
		log.Printf("Initial discovery: batch adding %d users to email discovery", len(usersToAdd))
		for _, user := range usersToAdd {
			// Create context for this user's email discovery
			userCtx, cancel := context.WithCancel(ctx)

			// Start email discovery for this user
			emailCh := s.discoverEmailsForUser(userCtx, user)

			// Store the user discovery state
			ued := &userEmailDiscovery{
				user:    user,
				ctx:     userCtx,
				cancel:  cancel,
				channel: emailCh,
			}
			s.activeUsers.Store(user.ID, ued)
		}
		log.Printf("Initial discovery: added %d users, notifying fan-in once", len(usersToAdd))
		// Notify channels changed once after all additions
		select {
		case s.channelsChanged <- struct{}{}:
		default:
		}
	}

	// Check for removed users
	for _, dbUser := range dbUsers {
		if !providerUserMap[dbUser.ID] {
			// User was removed from provider, send REMOVE_USER message
			if _, exists := s.activeUsers.Load(dbUser.ID); exists {
				s.userMessages <- UserMessage{Type: MessageRemoveUser, UserID: dbUser.ID}
			}
		}
	}

	return nil
}

func (s *Service) upsertUser(ctx context.Context, pUser models.ProviderUser) error {
	query := `
		INSERT INTO users (id, email)
		VALUES ($1, $2)
		ON CONFLICT (email) 
		DO NOTHING
	`

	_, err := db.Pool.Exec(ctx, query,
		pUser.ID,
		pUser.Email,
	)

	return err
}

// emailDiscoveryService waits for messages and manages user email discovery goroutines
func (s *Service) emailDiscoveryService(ctx context.Context) {
	log.Println("Email discovery service started, waiting for messages...")

	for {
		select {
		case <-ctx.Done():
			// Cleanup all active users
			s.activeUsers.Range(func(key, value interface{}) bool {
				ued := value.(*userEmailDiscovery)
				ued.cancel()
				return true
			})
			return
		case msg := <-s.userMessages:
			switch msg.Type {
			case MessageAddUser:
				s.handleAddUser(ctx, msg.UserID)
			case MessageRemoveUser:
				s.handleRemoveUser(msg.UserID)
			default:
				log.Printf("Unknown message type: %s", msg.Type)
			}
		}
	}
}

func (s *Service) handleAddUser(ctx context.Context, userID uuid.UUID) {
	// Check if already active
	if _, exists := s.activeUsers.Load(userID); exists {
		log.Printf("User %s already has active email discovery", userID)
		return
	}

	// Get user from database
	user, err := s.getUserByID(ctx, userID)
	if err != nil {
		log.Printf("Error getting user %s: %v", userID, err)
		return
	}

	// Create context for this user's email discovery
	userCtx, cancel := context.WithCancel(ctx)

	// Start email discovery for this user
	emailCh := s.discoverEmailsForUser(userCtx, user)

	// Store the user discovery state
	ued := &userEmailDiscovery{
		user:    user,
		ctx:     userCtx,
		cancel:  cancel,
		channel: emailCh,
	}
	s.activeUsers.Store(userID, ued)

	log.Printf("Started email discovery for user %s (%s)", user.Email, userID)

	// Notify fan-in that channels have changed (for incremental additions)
	s.channelsChanged <- struct{}{}
}

func (s *Service) handleRemoveUser(userID uuid.UUID) {
	value, exists := s.activeUsers.Load(userID)
	if !exists {
		log.Printf("User %s not found in active users", userID)
		return
	}

	ued := value.(*userEmailDiscovery)
	ued.cancel() // This will close the channel and trigger cleanup
	s.activeUsers.Delete(userID)
	log.Printf("Stopped email discovery for user %s", userID)

	// Notify fan-in that channels have changed
	s.channelsChanged <- struct{}{}
}

func (s *Service) getUserByID(ctx context.Context, userID uuid.UUID) (discoverymodels.User, error) {
	query := `SELECT id, email, last_email_check, last_email_received 
		FROM users WHERE id = $1`

	var user discoverymodels.User
	err := db.Pool.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.Email,
		&user.LastEmailCheck,
		&user.LastEmailReceived,
	)

	return user, err
}

func (s *Service) getUsers(ctx context.Context) ([]discoverymodels.User, error) {
	query := `SELECT id, email, last_email_check, last_email_received 
		FROM users`

	rows, err := db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []discoverymodels.User
	for rows.Next() {
		var user discoverymodels.User
		if err := rows.Scan(
			&user.ID,
			&user.Email,
			&user.LastEmailCheck,
			&user.LastEmailReceived,
		); err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, rows.Err()
}

type EmailWithUser struct {
	Email  models.ProviderEmail // Full email from provider (for analysis queue)
	UserID uuid.UUID
}

// discoverEmailsForUser polls for emails for a single user with fixed 30-second interval
// Returns a buffered channel (channel generator pattern)
// Buffered to avoid blocking polling goroutine if processing is slow
// Uses staggered initial polling to avoid thundering herd problem
func (s *Service) discoverEmailsForUser(ctx context.Context, user discoverymodels.User) <-chan EmailWithUser {
	emailCh := make(chan EmailWithUser, ChannelBufferSize) // Buffered channel

	go func() {
		defer close(emailCh)

		// Calculate initial delay based on user ID to stagger polling
		// This ensures users don't all poll at the same time
		initialDelay := s.calculateInitialDelay(user.ID)

		// Wait for initial delay before first poll
		select {
		case <-ctx.Done():
			return
		case <-time.After(initialDelay):
			// Initial poll after staggered delay
			s.pollEmailsForUser(user, emailCh)
		}

		// Create ticker for subsequent polls (every 30 seconds)
		ticker := time.NewTicker(PollingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.pollEmailsForUser(user, emailCh)
			}
		}
	}()

	return emailCh
}

// calculateInitialDelay calculates a deterministic but distributed delay for a user
// based on their UUID. This ensures each user starts polling at a slightly different time
// to avoid thundering herd, while being deterministic (same user = same delay).
func (s *Service) calculateInitialDelay(userID uuid.UUID) time.Duration {
	// Use first 8 bytes of UUID as a seed for delay calculation
	bytes := userID[:8]
	seed := binary.BigEndian.Uint64(bytes)

	// Map to 0-PollingJitterMax range
	delayNanos := seed % uint64(PollingJitterMax.Nanoseconds())
	return time.Duration(delayNanos)
}

// pollEmailsForUser polls for emails and sends them to the channel
func (s *Service) pollEmailsForUser(user discoverymodels.User, emailCh chan<- EmailWithUser) {
	// Fetch fresh user data from DB to get latest last_email_check
	ctx := context.Background()
	freshUser, err := s.getUserByID(ctx, user.ID)
	if err != nil {
		log.Printf("Error getting fresh user data for %s: %v", user.ID, err)
		// Fall back to passed user data
		freshUser = user
	}

	// Determine receivedAfter timestamp from fresh data
	// Use last_email_received if available (more accurate than last_email_check)
	// Otherwise fall back to last_email_check, or 24 hours ago if neither exists
	// Subtract 1 second as a buffer to avoid missing emails due to timing/clock skew
	var receivedAfter time.Time
	if freshUser.LastEmailReceived != nil {
		receivedAfter = freshUser.LastEmailReceived.Add(-1 * time.Second)
	} else if freshUser.LastEmailCheck != nil {
		receivedAfter = freshUser.LastEmailCheck.Add(-1 * time.Second)
	} else {
		// First time checking - go back 24 hours
		receivedAfter = time.Now().Add(-24 * time.Hour)
	}

	emails, err := s.provider.GetEmails(user.ID, receivedAfter, "received_at")
	if err != nil {
		log.Printf("Error getting emails for user %s: %v", user.ID, err)
		return
	}

	// Send emails to channel with user context (full email for analysis queue)
	// Metrics are updated in storeEmail() when emails are actually stored in DB
	for _, pEmail := range emails {
		emailCh <- EmailWithUser{Email: pEmail, UserID: user.ID}
	}
}

// processEmail processes a single email (called from fan-in loop)
func (s *Service) processEmail(ctx context.Context, ewu EmailWithUser) {
	// DB operations in goroutine to avoid blocking channel processing
	s.processingWg.Add(1)
	go func(ewu EmailWithUser) {
		defer s.processingWg.Done()

		// Check if context is already cancelled before starting work
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Store minimal metadata in DB first to check if it's a new unique email
		isNew, err := s.storeEmail(ctx, ewu.Email, ewu.UserID)
		if err != nil {
			log.Printf("Error storing email %s: %v", ewu.Email.MessageID, err)
			return
		}

		// Only send to analysis queue if it's a new unique email
		if isNew {
			s.sendToAnalysisQueue(ewu.Email)
		}

		// Update last_email_check (when email is processed from channel)
		now := time.Now()
		_, err = db.Pool.Exec(ctx,
			"UPDATE users SET last_email_check = $1 WHERE id = $2",
			now, ewu.UserID,
		)
		if err != nil {
			log.Printf("Error updating last_email_check: %v", err)
		}

		// Update last_email_received only if this is a new email and it's newer
		if isNew {
			_, err = db.Pool.Exec(ctx,
				`UPDATE users 
				SET last_email_received = $1 
				WHERE id = $2 
					AND (last_email_received IS NULL OR $1 > last_email_received)`,
				ewu.Email.ReceivedAt, ewu.UserID,
			)
			if err != nil {
				log.Printf("Error updating last_email_received: %v", err)
			}
		}
	}(ewu)
}

func (s *Service) storeEmail(ctx context.Context, pEmail models.ProviderEmail, userID uuid.UUID) (bool, error) {
	// Parse message_id as UUID (it's already a UUID string from the provider)
	emailID, err := uuid.Parse(pEmail.MessageID)
	if err != nil {
		return false, fmt.Errorf("invalid message_id format: %w", err)
	}

	// Generate fingerprint from email body/content (SHA256 hash)
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(pEmail.Body)))

	// Insert or update email (minimal metadata only - zero copy principle)
	// First, check if email with this fingerprint already exists
	var existingEmailID uuid.UUID
	checkQuery := `SELECT id FROM emails WHERE fingerprint = $1 LIMIT 1`
	err = db.Pool.QueryRow(ctx, checkQuery, fingerprint).Scan(&existingEmailID)

	isNewEmail := false
	if err == nil {
		// Email with this fingerprint already exists, use that ID
		emailID = existingEmailID
	} else if errors.Is(err, pgx.ErrNoRows) {
		// No existing email, try to insert with the message_id
		insertQuery := `
			INSERT INTO emails (id, fingerprint, received_at)
			VALUES ($1, $2, $3)
			ON CONFLICT (id) DO UPDATE SET received_at = EXCLUDED.received_at
		`
		_, err = db.Pool.Exec(ctx, insertQuery, emailID, fingerprint, pEmail.ReceivedAt)
		if err != nil {
			// If fingerprint conflict, find existing email
			if strings.Contains(err.Error(), "fingerprint") || strings.Contains(err.Error(), "23505") {
				err = db.Pool.QueryRow(ctx, checkQuery, fingerprint).Scan(&existingEmailID)
				if err == nil {
					emailID = existingEmailID
				} else if errors.Is(err, pgx.ErrNoRows) {
					return false, fmt.Errorf("failed to find existing email by fingerprint: no rows found")
				} else {
					return false, fmt.Errorf("failed to find existing email by fingerprint: %w", err)
				}
			} else {
				return false, fmt.Errorf("failed to insert email: %w", err)
			}
		} else {
			// Successfully inserted a new email
			isNewEmail = true
		}
	} else {
		return false, fmt.Errorf("failed to check for existing email: %w", err)
	}

	// Update metrics only for new emails actually stored in DB
	if isNewEmail {
		atomic.AddInt64(&s.emailsDiscovered, 1)

		// Get or create counter for this user
		var counter *int64
		if val, ok := s.emailsPerUser.Load(userID); ok {
			counter = val.(*int64)
		} else {
			counter = new(int64)
			s.emailsPerUser.Store(userID, counter)
		}
		atomic.AddInt64(counter, 1)
	}

	// Link email to user via user_emails junction table
	linkQuery := `
		INSERT INTO user_emails (user_id, email_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, email_id) DO NOTHING
	`

	_, err = db.Pool.Exec(ctx, linkQuery, userID, emailID)
	if err != nil {
		return false, fmt.Errorf("failed to link email to user: %w", err)
	}

	return isNewEmail, nil
}

// dynamicFanInAndProcess implements the fan-in pattern and processes emails directly
// It recreates the fan-in whenever channels are added or removed
func (s *Service) dynamicFanInAndProcess(ctx context.Context) {
	var currentFanIn <-chan EmailWithUser

	// Helper function to collect all active channels
	collectChannels := func() []<-chan EmailWithUser {
		var channels []<-chan EmailWithUser
		s.activeUsers.Range(func(key, value interface{}) bool {
			ued := value.(*userEmailDiscovery)
			channels = append(channels, ued.channel)
			return true
		})
		return channels
	}

	// Helper function to recreate fan-in
	recreateFanIn := func() {
		channels := collectChannels()
		if len(channels) == 0 {
			log.Println("No active user channels for fan-in")
			currentFanIn = nil
			return
		}

		log.Printf("Recreating fan-in with %d user channels", len(channels))
		currentFanIn = fanIn(channels)
	}

	// Initial fan-in creation (wait for first channels)
	select {
	case <-s.channelsChanged:
		recreateFanIn()
	case <-ctx.Done():
		return
	}

	// Main loop: process emails directly from fan-in
	for {
		if currentFanIn == nil {
			// No channels, wait for change
			select {
			case <-s.channelsChanged:
				recreateFanIn()
			case <-ctx.Done():
				return
			}
			continue
		}

		select {
		case <-ctx.Done():
			return
		case <-s.channelsChanged:
			// Channels changed, recreate fan-in
			recreateFanIn()
		case email, ok := <-currentFanIn:
			if !ok {
				// Fan-in channel closed (all user channels closed), recreate
				recreateFanIn()
				continue
			}
			// Process email directly (unbuffered = natural backpressure)
			s.processEmail(ctx, email)
		}
	}
}

// fanIn combines multiple channels into a single channel (fan-in pattern)
// Output is unbuffered for natural backpressure - if processing is slow, polling slows down
func fanIn(channels []<-chan EmailWithUser) <-chan EmailWithUser {
	multiplexer := make(chan EmailWithUser) // Unbuffered output channel

	// If no channels, close immediately
	if len(channels) == 0 {
		close(multiplexer)
		return multiplexer
	}

	// Track when all channels are closed
	var wg sync.WaitGroup
	wg.Add(len(channels))

	// Forward from each channel to multiplexer
	for _, ch := range channels {
		go func(c <-chan EmailWithUser) {
			defer wg.Done()
			for emailWithUser := range c {
				multiplexer <- emailWithUser
			}
		}(ch)
	}

	// Close multiplexer when all channels are closed
	go func() {
		wg.Wait()
		close(multiplexer)
	}()

	return multiplexer
}

// logPerformanceMetrics logs aggregated performance metrics periodically
// Uses jittered intervals to avoid synchronized log bursts
func (s *Service) logPerformanceMetrics(ctx context.Context) {
	baseInterval := 5 * time.Second
	jitterRange := 2 * time.Second // ±1 second jitter

	for {
		// Calculate jittered interval (4-6 seconds)
		jitter := time.Duration(rand.Int63n(int64(jitterRange))) - jitterRange/2
		interval := baseInterval + jitter

		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
			s.logMetrics()
		}
	}
}

func (s *Service) logMetrics() {
	// Collect all user email counts
	type userStat struct {
		userID uuid.UUID
		email  string
		count  int64
	}

	var stats []userStat
	s.emailsPerUser.Range(func(key, value interface{}) bool {
		userID := key.(uuid.UUID)
		counter := value.(*int64)
		count := atomic.LoadInt64(counter)
		if count > 0 {
			// Get user email for display
			if val, ok := s.activeUsers.Load(userID); ok {
				ued := val.(*userEmailDiscovery)
				stats = append(stats, userStat{
					userID: userID,
					email:  ued.user.Email,
					count:  count,
				})
			}
		}
		return true
	})

	// Sort by count (descending)
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].count > stats[j].count
	})

	// Get totals
	totalDiscovered := atomic.LoadInt64(&s.emailsDiscovered)
	totalToQueue := atomic.LoadInt64(&s.emailsToQueue)

	// Log performance summary (column-based format for readability)
	log.Printf("📊 Metrics | Discovered: %d | Queued: %d", totalDiscovered, totalToQueue)

	if len(stats) > 0 {
		topN := 3 // Show top 3 users
		if len(stats) < topN {
			topN = len(stats)
		}

		// Display top users in column format
		for i := 0; i < topN; i++ {
			log.Printf("   %d. %-50s %d emails", i+1, stats[i].email, stats[i].count)
		}
	}
}

// sendToAnalysisQueue sends an email to the analysis queue for fraud detection.
// This is a placeholder implementation that tracks metrics. In production, this would
// integrate with a message queue (Kafka/RabbitMQ/NATS) to send emails to analysis workers.
func (s *Service) sendToAnalysisQueue(email models.ProviderEmail) {
	// TODO: Integrate with message queue (Kafka/RabbitMQ/NATS)
	atomic.AddInt64(&s.emailsToQueue, 1)
}
