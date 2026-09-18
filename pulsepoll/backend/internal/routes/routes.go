package routes

import (
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"pulsepoll/backend/internal/auth"
	"pulsepoll/backend/internal/handlers"
	"strings"
)

func Register(r *gin.Engine, h *handlers.Handler, a *auth.Service, frontendOrigin string) {
	r.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if frontendOrigin == "*" || origin == frontendOrigin {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			if frontendOrigin == "*" {
				c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
			}
		}
		c.Writer.Header().Set("Vary", "Origin")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Voter-Key")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})
	r.GET("/health", h.Health)
	api := r.Group("/api")
	api.POST("/auth/signup", h.Signup)
	api.POST("/auth/login", h.Login)
	api.GET("/polls/:id", h.GetPoll)
	api.POST("/polls/:id/vote", h.Vote)
	api.GET("/polls/:id/ws", h.WebSocket)
	protected := api.Group("")
	protected.Use(requireAuth(a))
	protected.POST("/polls", h.CreatePoll)
	protected.GET("/polls", h.ListPolls)
	protected.POST("/polls/:id/close", h.ClosePoll)
	protected.DELETE("/polls/:id", h.DeletePoll)
}
func requireAuth(a *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if !strings.HasPrefix(raw, "Bearer ") {
			c.AbortWithStatusJSON(401, gin.H{"error": "authentication required"})
			return
		}
		id, err := a.UserID(strings.TrimPrefix(raw, "Bearer "))
		if err != nil || id == primitive.NilObjectID {
			c.AbortWithStatusJSON(401, gin.H{"error": "invalid token"})
			return
		}
		c.Set("userID", id)
		c.Next()
	}
}
