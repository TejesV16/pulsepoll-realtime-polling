package main

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"os"
	"pulsepoll/backend/internal/auth"
	"pulsepoll/backend/internal/handlers"
	"pulsepoll/backend/internal/realtime"
	"pulsepoll/backend/internal/repository"
	"pulsepoll/backend/internal/routes"
	"time"
)

func main() {
	_ = godotenv.Load()
	mongoURI := env("MONGO_URI", "mongodb://localhost:27017")
	dbName := env("MONGO_DB", "pulsepoll")
	redisAddr := env("REDIS_ADDR", "localhost:6379")
	frontendOrigin := env("FRONTEND_ORIGIN", "http://localhost:5173")
	jwtSecret := env("JWT_SECRET", "")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET must be set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}
	defer mongoClient.Disconnect(context.Background())
	if err = mongoClient.Ping(ctx, nil); err != nil {
		log.Fatal(err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()
	if err = rdb.Ping(ctx).Err(); err != nil {
		log.Fatal(err)
	}
	repo := repository.New(mongoClient.Database(dbName))
	if err = repo.EnsureIndexes(ctx); err != nil {
		log.Fatal(err)
	}
	hub := realtime.NewHub()
	authService := auth.New(jwtSecret)
	h := handlers.New(repo, rdb, hub, authService, frontendOrigin)
	realtime.StartRedisSubscriber(context.Background(), rdb, hub)
	gin.SetMode(env("GIN_MODE", gin.ReleaseMode))
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	routes.Register(r, h, authService, frontendOrigin)
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
