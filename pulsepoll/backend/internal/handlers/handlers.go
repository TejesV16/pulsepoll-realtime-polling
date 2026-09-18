package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"net/http"
	"pulsepoll/backend/internal/auth"
	"pulsepoll/backend/internal/models"
	"pulsepoll/backend/internal/realtime"
	"pulsepoll/backend/internal/repository"
	"strconv"
	"strings"
	"time"
)

type Handler struct {
	Repo     *repository.Repository
	Redis    *redis.Client
	Hub      *realtime.Hub
	Auth     *auth.Service
	WSOrigin string
}

func New(repo *repository.Repository, rdb *redis.Client, hub *realtime.Hub, a *auth.Service, wsOrigin string) *Handler {
	return &Handler{Repo: repo, Redis: rdb, Hub: hub, Auth: a, WSOrigin: wsOrigin}
}

type authRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}
type createPollRequest struct {
	Question        string   `json:"question"`
	Options         []string `json:"options"`
	DurationMinutes int      `json:"duration_minutes"`
}
type voteRequest struct {
	OptionID string `json:"option_id"`
}

func decode(c *gin.Context, v any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	return c.ShouldBindJSON(v)
}
func normalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func (h *Handler) Signup(c *gin.Context) {
	var req authRequest
	if err := decode(c, &req); err != nil || len(strings.TrimSpace(req.Name)) < 2 || len(req.Name) > 80 || !strings.Contains(normalizeEmail(req.Email), "@") || len(req.Password) < 8 || len(req.Password) > 72 {
		c.JSON(400, gin.H{"error": "name, valid email and password of 8-72 characters are required"})
		return
	}
	email := normalizeEmail(req.Email)
	if _, err := h.Repo.FindUserByEmail(c, email); err == nil {
		c.JSON(409, gin.H{"error": "email already registered"})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(500, gin.H{"error": "could not create account"})
		return
	}
	u := &models.User{ID: primitive.NewObjectID(), Name: strings.TrimSpace(req.Name), Email: email, PasswordHash: hash, CreatedAt: time.Now().UTC()}
	if err = h.Repo.CreateUser(c, u); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			c.JSON(409, gin.H{"error": "email already registered"})
			return
		}
		c.JSON(500, gin.H{"error": "could not create account"})
		return
	}
	token, err := h.Auth.Token(u.ID)
	if err != nil {
		c.JSON(500, gin.H{"error": "could not create session"})
		return
	}
	c.JSON(201, gin.H{"token": token, "user": u})
}
func (h *Handler) Login(c *gin.Context) {
	var req authRequest
	if err := decode(c, &req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	u, err := h.Repo.FindUserByEmail(c, normalizeEmail(req.Email))
	if err != nil || !auth.CheckPassword(u.PasswordHash, req.Password) {
		c.JSON(401, gin.H{"error": "invalid credentials"})
		return
	}
	token, err := h.Auth.Token(u.ID)
	if err != nil {
		c.JSON(500, gin.H{"error": "could not create session"})
		return
	}
	c.JSON(200, gin.H{"token": token, "user": u})
}

func (h *Handler) CreatePoll(c *gin.Context) {
	var req createPollRequest
	if err := decode(c, &req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	q := strings.TrimSpace(req.Question)
	if len(q) < 5 || len(q) > 240 || len(req.Options) < 2 || len(req.Options) > 10 || req.DurationMinutes < 0 || req.DurationMinutes > 10080 {
		c.JSON(400, gin.H{"error": "question must be 5-240 chars, 2-10 options, duration 0-10080 minutes"})
		return
	}
	seen := map[string]bool{}
	opts := make([]models.PollOption, 0, len(req.Options))
	for _, raw := range req.Options {
		label := strings.TrimSpace(raw)
		key := strings.ToLower(label)
		if len(label) < 1 || len(label) > 100 || seen[key] {
			c.JSON(400, gin.H{"error": "options must be unique and 1-100 characters"})
			return
		}
		seen[key] = true
		opts = append(opts, models.PollOption{ID: primitive.NewObjectID().Hex(), Label: label})
	}
	ownerID, _ := c.Get("userID")
	p := &models.Poll{ID: primitive.NewObjectID(), OwnerID: ownerID.(primitive.ObjectID), Question: q, Options: opts, Status: "active", CreatedAt: time.Now().UTC()}
	if req.DurationMinutes > 0 {
		t := time.Now().UTC().Add(time.Duration(req.DurationMinutes) * time.Minute)
		p.ExpiresAt = &t
	}
	if err := h.Repo.CreatePoll(c, p); err != nil {
		c.JSON(500, gin.H{"error": "could not create poll"})
		return
	}
	if err := h.seedCounts(c, p); err != nil {
		c.JSON(500, gin.H{"error": "poll created but realtime initialization failed"})
		return
	}
	c.JSON(201, p)
}

func (h *Handler) ListPolls(c *gin.Context) {
	ownerID := c.MustGet("userID").(primitive.ObjectID)
	polls, err := h.Repo.ListPollsByOwner(c, ownerID)
	if err != nil {
		c.JSON(500, gin.H{"error": "could not load polls"})
		return
	}
	for i := range polls {
		if polls[i].Status == "active" && polls[i].ExpiresAt != nil && time.Now().UTC().After(*polls[i].ExpiresAt) {
			polls[i].Status = "closed"
			_ = h.Repo.SetPollStatus(c, polls[i].ID, ownerID, "closed")
		}
	}
	c.JSON(200, gin.H{"polls": polls})
}
func (h *Handler) ClosePoll(c *gin.Context) {
	id, err := pollID(c)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid poll id"})
		return
	}
	owner := c.MustGet("userID").(primitive.ObjectID)
	if err = h.Repo.SetPollStatus(c, id, owner, "closed"); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			c.JSON(404, gin.H{"error": "poll not found"})
			return
		}
		c.JSON(500, gin.H{"error": "could not close poll"})
		return
	}
	h.publishEvent(c, id, realtime.Event{Type: "poll_closed", PollID: id.Hex(), Status: "closed"})
	c.JSON(200, gin.H{"status": "closed"})
}
func (h *Handler) DeletePoll(c *gin.Context) {
	id, err := pollID(c)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid poll id"})
		return
	}
	owner := c.MustGet("userID").(primitive.ObjectID)
	if err = h.Repo.DeletePoll(c, id, owner); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			c.JSON(404, gin.H{"error": "poll not found"})
			return
		}
		c.JSON(500, gin.H{"error": "could not delete poll"})
		return
	}
	_ = h.Redis.Del(c, "poll:"+id.Hex()+":counts").Err()
	c.JSON(204, nil)
}

func (h *Handler) GetPoll(c *gin.Context) {
	id, err := pollID(c)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid poll id"})
		return
	}
	p, err := h.Repo.FindPoll(c, id)
	if err != nil {
		c.JSON(404, gin.H{"error": "poll not found"})
		return
	}
	if p.Status == "active" && p.ExpiresAt != nil && time.Now().UTC().After(*p.ExpiresAt) {
		p.Status = "closed"
		_ = h.Repo.SetPollStatus(c, id, p.OwnerID, "closed")
		h.publishEvent(c, id, realtime.Event{Type: "poll_closed", PollID: id.Hex(), Status: "closed"})
	}
	counts, err := h.liveCounts(c, p)
	if err != nil {
		c.JSON(500, gin.H{"error": "could not load live counts"})
		return
	}
	c.JSON(200, gin.H{"poll": p, "counts": counts})
}

func (h *Handler) Vote(c *gin.Context) {
	if !h.allowVote(c) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many vote requests; try again shortly"})
		return
	}
	id, err := pollID(c)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid poll id"})
		return
	}
	p, err := h.Repo.FindPoll(c, id)
	if err != nil {
		c.JSON(404, gin.H{"error": "poll not found"})
		return
	}
	if p.Status != "active" || (p.ExpiresAt != nil && time.Now().UTC().After(*p.ExpiresAt)) {
		c.JSON(409, gin.H{"error": "poll is closed"})
		return
	}
	var req voteRequest
	if err = decode(c, &req); err != nil || req.OptionID == "" {
		c.JSON(400, gin.H{"error": "option_id is required"})
		return
	}
	valid := false
	for _, o := range p.Options {
		if o.ID == req.OptionID {
			valid = true
			break
		}
	}
	if !valid {
		c.JSON(400, gin.H{"error": "invalid option"})
		return
	}
	raw := strings.TrimSpace(c.GetHeader("X-Voter-Key"))
	if len(raw) < 16 || len(raw) > 128 {
		c.JSON(400, gin.H{"error": "missing voter key"})
		return
	}
	voterHash := hashVoter(raw)
	v := &models.Vote{ID: primitive.NewObjectID(), PollID: id, OptionID: req.OptionID, VoterHash: voterHash, CreatedAt: time.Now().UTC()}
	if err = h.Repo.CreateVote(c, v); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			c.JSON(409, gin.H{"error": "you have already voted"})
			return
		}
		c.JSON(500, gin.H{"error": "vote could not be saved"})
		return
	}
	key := "poll:" + id.Hex() + ":counts"
	if err = h.Redis.HIncrBy(c, key, req.OptionID, 1).Err(); err != nil {
		_ = h.reconcile(c, p)
		c.JSON(503, gin.H{"error": "vote saved; live update temporarily unavailable"})
		return
	}
	counts, err := h.liveCounts(c, p)
	if err != nil {
		c.JSON(503, gin.H{"error": "vote saved; live update temporarily unavailable"})
		return
	}
	e := realtime.Event{Type: "vote", PollID: id.Hex(), Counts: counts, Status: p.Status}
	if err = h.publishEvent(c, id, e); err != nil {
		c.JSON(503, gin.H{"error": "vote saved; live update temporarily unavailable"})
		return
	}
	c.JSON(201, gin.H{"counts": counts})
}

var upgrader = websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024, CheckOrigin: func(r *http.Request) bool { return true }}

func (h *Handler) allowVote(c *gin.Context) bool {
	ip := c.ClientIP()
	key := "rate:vote:" + hashVoter(ip)
	n, err := h.Redis.Incr(c, key).Result()
	if err != nil {
		return true
	}
	if n == 1 {
		_ = h.Redis.Expire(c, key, time.Minute).Err()
	}
	return n <= 60
}
func (h *Handler) WebSocket(c *gin.Context) {
	origin := c.GetHeader("Origin")
	if h.WSOrigin != "*" && origin != "" && origin != h.WSOrigin {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	id := c.Param("id")
	if _, err := primitive.ObjectIDFromHex(id); err != nil {
		c.AbortWithStatusJSON(400, gin.H{"error": "invalid poll id"})
		return
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.Hub.Add(id, conn)
	defer h.Hub.Remove(id, conn)
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error { _ = conn.SetReadDeadline(time.Now().Add(60 * time.Second)); return nil })
	for {
		if _, _, err = conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *Handler) Health(c *gin.Context) {
	if err := h.Redis.Ping(c).Err(); err != nil {
		c.JSON(503, gin.H{"status": "degraded", "redis": "down"})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}
func pollID(c *gin.Context) (primitive.ObjectID, error) {
	return primitive.ObjectIDFromHex(c.Param("id"))
}
func hashVoter(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
func (h *Handler) seedCounts(c *gin.Context, p *models.Poll) error {
	counts, err := h.Repo.CountVotes(c, p.ID)
	if err != nil {
		return err
	}
	vals := map[string]any{}
	for _, o := range p.Options {
		n := counts[o.ID]
		vals[o.ID] = n
	}
	return h.Redis.HSet(c, "poll:"+p.ID.Hex()+":counts", vals).Err()
}
func (h *Handler) reconcile(c *gin.Context, p *models.Poll) error { return h.seedCounts(c, p) }
func (h *Handler) liveCounts(c *gin.Context, p *models.Poll) (map[string]int64, error) {
	key := "poll:" + p.ID.Hex() + ":counts"
	vals, err := h.Redis.HGetAll(c, key).Result()
	if err != nil {
		return nil, err
	}
	if len(vals) == 0 {
		if err := h.reconcile(c, p); err != nil {
			return nil, err
		}
		vals, err = h.Redis.HGetAll(c, key).Result()
		if err != nil {
			return nil, err
		}
	}
	out := map[string]int64{}
	for _, o := range p.Options {
		out[o.ID] = 0
	}
	for k, v := range vals {
		var n int64
		_, err := fmtSscan(v, &n)
		if err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, nil
}
func (h *Handler) publishEvent(c *gin.Context, id primitive.ObjectID, e realtime.Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return h.Redis.Publish(c, "poll:"+id.Hex()+":events", b).Err()
}
func fmtSscan(s string, n *int64) (int, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	*n = v
	return 0, nil
}
