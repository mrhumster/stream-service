package service

import (
	"context"
	"crypto/rsa"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mrhumster/stream-service/config"
	"github.com/mrhumster/stream-service/internal/domain/models"
)

type TokenService struct {
	accessPublicKey *rsa.PublicKey
}

func NewTokenService(cfg *config.JWT) (*TokenService, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", cfg.AccessPublicKeyUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("⚠️ Error getting access public key: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("⚠️ Error getting access public key: %w", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("⚠️ Error getting access public key: %w", err)
	}

	accessPublicKey, err := jwt.ParseRSAPublicKeyFromPEM([]byte(body))

	return &TokenService{
		accessPublicKey: accessPublicKey,
	}, nil
}

func (s *TokenService) ValidateAccessToken(tokenString string) (*models.AccessClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &models.AccessClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.accessPublicKey, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*models.AccessClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, fmt.Errorf("⚠️ Error invalid token")
}
