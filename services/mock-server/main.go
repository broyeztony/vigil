package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stoik/vigil/services/mock-server/internal/mock"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	r := gin.Default()

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Google provider endpoints
	google := r.Group("/google")
	{
		google.GET("/users/:tenantId", handleGetGoogleUsers)
		google.GET("/emails/:userId", handleGetGoogleEmails)
	}
	
	// Admin endpoints for testing
	admin := r.Group("/admin")
	{
		admin.POST("/users/add", handleAddUsers)
	}

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Starting Vigil Mock API server on %s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}

func handleGetGoogleUsers(c *gin.Context) {
	tenantIDStr := c.Param("tenantId")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id"})
		return
	}

	users, err := mock.GetGoogleUsers(tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, users)
}

func handleGetGoogleEmails(c *gin.Context) {
	userIDStr := c.Param("userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	// Parse query parameters
	receivedAfterStr := c.DefaultQuery("receivedAfter", "")
	orderBy := c.DefaultQuery("orderBy", "received_at")

	var receivedAfter time.Time
	if receivedAfterStr == "" {
		// Default to 24 hours ago
		receivedAfter = time.Now().Add(-24 * time.Hour)
	} else {
		var err error
		receivedAfter, err = time.Parse(time.RFC3339, receivedAfterStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid receivedAfter format (use RFC3339)"})
			return
		}
	}

	emails, err := mock.GetGoogleEmails(userID, receivedAfter, orderBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, emails)
}

func handleAddUsers(c *gin.Context) {
	var req struct {
		NumUsers int `json:"numUsers"`
	}
	
	// Try JSON body first
	if err := c.ShouldBindJSON(&req); err != nil {
		// Fall back to query parameter
		numUsersStr := c.DefaultQuery("numUsers", "1")
		if num, err := strconv.Atoi(numUsersStr); err == nil {
			req.NumUsers = num
		} else {
			req.NumUsers = 1
		}
	}
	
	// Default to 1 if not specified or invalid
	if req.NumUsers < 1 {
		req.NumUsers = 1
	}
	
	totalUsers, err := mock.AddUsers(req.NumUsers)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"added": req.NumUsers,
		"total":  totalUsers,
		"message": fmt.Sprintf("Added %d user(s). Total users: %d", req.NumUsers, totalUsers),
	})
}

