package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/last9/go-agent"
	"github.com/last9/go-agent/integrations/database"
	httpagent "github.com/last9/go-agent/integrations/http"
	redisagent "github.com/last9/go-agent/integrations/redis"
	ginagent "github.com/last9/go-agent/instrumentation/gin"
	"github.com/redis/go-redis/v9"

	_ "github.com/lib/pq" // PostgreSQL driver
)

var (
	db  *sql.DB
	rdb *redis.Client
)

func main() {
	// Initialize Last9 agent
	if err := agent.Start(); err != nil {
		log.Fatalf("Failed to start Last9 agent: %v", err)
	}
	defer agent.Shutdown()

	// Setup database (optional - only if DATABASE_URL is set)
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		var err error
		db, err = database.Open(database.Config{
			DriverName:   "postgres",
			DSN:          dsn,
			DatabaseName: "example",
		})
		if err != nil {
			log.Printf("Warning: Failed to connect to database: %v", err)
		} else {
			defer db.Close()
			log.Println("✓ Database connected and instrumented")
		}
	}

	// Setup Redis (optional - only if REDIS_URL is set)
	redisAddr := os.Getenv("REDIS_URL")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb = redisagent.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	defer rdb.Close()

	// Test Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("Warning: Redis not available: %v", err)
	} else {
		log.Println("✓ Redis connected and instrumented")
	}

	// Create instrumented Gin router
	r := ginagent.Default()

	// Routes
	setupRoutes(r)

	// Start server
	log.Println("\n🚀 Server starting on :8080")
	log.Println("Try these endpoints:")
	log.Println("  GET  http://localhost:8080/")
	log.Println("  GET  http://localhost:8080/health")
	log.Println("  GET  http://localhost:8080/users")
	log.Println("  GET  http://localhost:8080/users/:id")
	log.Println("  POST http://localhost:8080/users")
	log.Println("  GET  http://localhost:8080/cache/:key")
	log.Println("  POST http://localhost:8080/cache/:key")
	log.Println("  GET  http://localhost:8080/external")
	log.Println()

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func setupRoutes(r *gin.Engine) {
	// Home
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Last9 Go Agent - Full Example",
			"version": "1.0.0",
			"features": []string{
				"HTTP tracing",
				"Database instrumentation",
				"Redis instrumentation",
				"External API tracing",
			},
		})
	})

	// Health check
	r.GET("/health", healthCheck)

	// User routes (demonstrates database operations)
	users := r.Group("/users")
	{
		users.GET("", listUsers)
		users.GET("/:id", getUser)
		users.POST("", createUser)
	}

	// Cache routes (demonstrates Redis operations)
	cache := r.Group("/cache")
	{
		cache.GET("/:key", getCacheValue)
		cache.POST("/:key", setCacheValue)
	}

	// External API call (demonstrates HTTP client tracing)
	r.GET("/external", callExternalAPI)
}

func healthCheck(c *gin.Context) {
	health := gin.H{
		"status": "healthy",
		"timestamp": time.Now().Unix(),
	}

	// Check database
	if db != nil {
		if err := db.Ping(); err != nil {
			health["database"] = "unhealthy"
			health["database_error"] = err.Error()
		} else {
			health["database"] = "healthy"
		}
	}

	// Check Redis
	ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		health["redis"] = "unhealthy"
		health["redis_error"] = err.Error()
	} else {
		health["redis"] = "healthy"
	}

	c.JSON(200, health)
}

func listUsers(c *gin.Context) {
	if db == nil {
		c.JSON(503, gin.H{"error": "Database not configured"})
		return
	}

	// This query is automatically traced!
	rows, err := db.QueryContext(c.Request.Context(),
		"SELECT id, name, email FROM users LIMIT 10")
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	users := []gin.H{}
	for rows.Next() {
		var id int
		var name, email string
		if err := rows.Scan(&id, &name, &email); err != nil {
			continue
		}
		users = append(users, gin.H{
			"id":    id,
			"name":  name,
			"email": email,
		})
	}

	c.JSON(200, gin.H{
		"users": users,
		"count": len(users),
	})
}

func getUser(c *gin.Context) {
	id := c.Param("id")

	// Try cache first
	cacheKey := "user:" + id
	cached, err := rdb.Get(c.Request.Context(), cacheKey).Result()
	if err == nil {
		c.JSON(200, gin.H{
			"id":     id,
			"name":   cached,
			"source": "cache",
		})
		return
	}

	// Cache miss - query database
	if db == nil {
		c.JSON(503, gin.H{"error": "Database not configured"})
		return
	}

	var name, email string
	err = db.QueryRowContext(c.Request.Context(),
		"SELECT name, email FROM users WHERE id = $1", id).Scan(&name, &email)

	if err == sql.ErrNoRows {
		c.JSON(404, gin.H{"error": "User not found"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// Cache the result
	rdb.Set(c.Request.Context(), cacheKey, name, 5*time.Minute)

	c.JSON(200, gin.H{
		"id":     id,
		"name":   name,
		"email":  email,
		"source": "database",
	})
}

func createUser(c *gin.Context) {
	if db == nil {
		c.JSON(503, gin.H{"error": "Database not configured"})
		return
	}

	var user struct {
		Name  string `json:"name" binding:"required"`
		Email string `json:"email" binding:"required"`
	}

	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Insert user (automatically traced!)
	var id int
	err := db.QueryRowContext(c.Request.Context(),
		"INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id",
		user.Name, user.Email).Scan(&id)

	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(201, gin.H{
		"id":    id,
		"name":  user.Name,
		"email": user.Email,
	})
}

func getCacheValue(c *gin.Context) {
	key := c.Param("key")

	// Redis operation is automatically traced!
	val, err := rdb.Get(c.Request.Context(), key).Result()
	if err == redis.Nil {
		c.JSON(404, gin.H{"error": "Key not found"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"key":   key,
		"value": val,
	})
}

func setCacheValue(c *gin.Context) {
	key := c.Param("key")

	var body struct {
		Value string `json:"value" binding:"required"`
		TTL   int    `json:"ttl"` // seconds, 0 = no expiry
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ttl := time.Duration(body.TTL) * time.Second
	if body.TTL == 0 {
		ttl = 0 // No expiry
	}

	// Redis operation is automatically traced!
	err := rdb.Set(c.Request.Context(), key, body.Value, ttl).Err()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"key":   key,
		"value": body.Value,
		"ttl":   body.TTL,
	})
}

func callExternalAPI(c *gin.Context) {
	// Create instrumented HTTP client
	client := httpagent.NewClient(&http.Client{
		Timeout: 10 * time.Second,
	})

	// Make request (automatically traced with context propagation!)
	req, err := http.NewRequestWithContext(
		c.Request.Context(),
		"GET",
		"https://api.github.com/users/github",
		nil,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	c.JSON(200, gin.H{
		"status":      resp.StatusCode,
		"api":         "GitHub API",
		"endpoint":    req.URL.String(),
		"traced":      true,
	})
}
