package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"pulsepoll/backend/internal/auth"
	"pulsepoll/backend/internal/handlers"
	"pulsepoll/backend/internal/realtime"
	"pulsepoll/backend/internal/repository"
	"pulsepoll/backend/internal/routes"
)

func main() {
	_ = godotenv.Load()

	mongoURI := env("MONGO_URI", "mongodb://localhost:27017")
	dbName := env("MONGO_DB", "pulsepoll")

	// Support both local Redis and Render Redis.
	redisURL := os.Getenv("REDIS_URL")
	redisAddr := env("REDIS_ADDR", "localhost:6379")

	frontendOrigin := env("FRONTEND_ORIGIN", "http://localhost:5173")
	jwtSecret := env("JWT_SECRET", "")

	if jwtSecret == "" {
		log.Fatal("JWT_SECRET must be set")
	}

	// MongoDB connection
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	mongoClient, err := mongo.Connect(
		ctx,
		options.Client().ApplyURI(mongoURI),
	)
	if err != nil {
		log.Fatal(err)
	}

	defer mongoClient.Disconnect(context.Background())

	if err = mongoClient.Ping(ctx, nil); err != nil {
		log.Fatal(err)
	}

	// Redis connection
	var rdb *redis.Client

	if redisURL != "" {
		// Production: Render Redis / Redis-compatible URL
		redisOpts, err := redis.ParseURL(redisURL)
		if err != nil {
			log.Fatalf("invalid REDIS_URL: %v", err)
		}

		rdb = redis.NewClient(redisOpts)
		log.Println("Using REDIS_URL for Redis connection")
	} else {
		// Local development: localhost:6379
		rdb = redis.NewClient(&redis.Options{
			Addr: redisAddr,
		})
		log.Printf("Using REDIS_ADDR for Redis connection: %s", redisAddr)
	}

	defer rdb.Close()

	if err = rdb.Ping(ctx).Err(); err != nil {
		log.Fatal(err)
	}

	// Repository
	repo := repository.New(mongoClient.Database(dbName))

	if err = repo.EnsureIndexes(ctx); err != nil {
		log.Fatal(err)
	}

	// Realtime hub
	hub := realtime.NewHub()

	// Authentication
	authService := auth.New(jwtSecret)

	// Handlers
	h := handlers.New(
		repo,
		rdb,
		hub,
		authService,
		frontendOrigin,
	)

	// Redis Pub/Sub subscriber
	realtime.StartRedisSubscriber(
		context.Background(),
		rdb,
		hub,
	)

	// Gin
	gin.SetMode(env("GIN_MODE", gin.ReleaseMode))

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// Routes
	routes.Register(
		r,
		h,
		authService,
		frontendOrigin,
	)

	// Start server
	port := env("PORT", "8080")

	log.Printf("PulsePoll API listening on :%s", port)

	log.Fatal(r.Run(":" + port))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}