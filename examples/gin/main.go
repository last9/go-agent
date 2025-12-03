package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/last9/go-agent"
	ginagent "github.com/last9/go-agent/instrumentation/gin"
)

func main() {
	// Initialize Last9 agent - reads config from environment variables
	if err := agent.Start(); err != nil {
		log.Fatalf("Failed to start Last9 agent: %v", err)
	}
	defer agent.Shutdown()

	// Create instrumented Gin router (drop-in replacement for gin.Default())
	r := ginagent.Default()

	// Define routes - no additional instrumentation needed!
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	r.GET("/hello/:name", func(c *gin.Context) {
		name := c.Param("name")

		// Simulate some work
		time.Sleep(50 * time.Millisecond)

		c.JSON(200, gin.H{
			"greeting": "Hello, " + name + "!",
		})
	})

	r.POST("/users", func(c *gin.Context) {
		var user struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}

		if err := c.ShouldBindJSON(&user); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		// Simulate database save
		time.Sleep(100 * time.Millisecond)

		c.JSON(201, gin.H{
			"id":    "123",
			"name":  user.Name,
			"email": user.Email,
		})
	})

	r.GET("/external", func(c *gin.Context) {
		// Example of calling external API with instrumentation
		client := &http.Client{Timeout: 5 * time.Second}

		req, _ := http.NewRequestWithContext(
			c.Request.Context(),
			"GET",
			"https://api.github.com/users/github",
			nil,
		)

		resp, err := client.Do(req)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		defer resp.Body.Close()

		c.JSON(200, gin.H{
			"status": resp.StatusCode,
		})
	})

	r.GET("/error", func(c *gin.Context) {
		// Simulate an error
		c.JSON(500, gin.H{
			"error": "Something went wrong!",
		})
	})

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "healthy",
		})
	})

	log.Println("Starting server on :8080")
	log.Println("Try these endpoints:")
	log.Println("  GET  http://localhost:8080/ping")
	log.Println("  GET  http://localhost:8080/hello/World")
	log.Println("  POST http://localhost:8080/users")
	log.Println("  GET  http://localhost:8080/external")
	log.Println("  GET  http://localhost:8080/health")

	// Start server with graceful shutdown
	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	// Graceful shutdown on interrupt
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	log.Println("Server started. Press Ctrl+C to stop.")
	<-make(chan struct{}) // Block forever (in real app, use signal handling)

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}
