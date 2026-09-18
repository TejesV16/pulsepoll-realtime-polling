package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
)

type Service struct{ secret []byte }

func New(secret string) *Service { return &Service{[]byte(secret)} }

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *Service) Token(userID primitive.ObjectID) (string, error) {
	claims := jwt.MapClaims{
		"sub": userID.Hex(),
		"exp": time.Now().Add(24 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *Service) UserID(tokenString string) (primitive.ObjectID, error) {
	t, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil || !t.Valid {
		return primitive.NilObjectID, errors.New("invalid token")
	}
	sub, err := t.Claims.GetSubject()
	if err != nil {
		return primitive.NilObjectID, err
	}
	return primitive.ObjectIDFromHex(sub)
}
