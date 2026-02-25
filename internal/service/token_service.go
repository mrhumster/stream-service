package service

import (
	"context"
	"crypto/rsa"
	"errors"
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
	if cfg.AccessPublicKeyUrl == "" {
		return nil, errors.New("⚠️ Error Token Service. JWT_ACCESS_PUBLIC_KEY_URL not set")
	}

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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("⚠️ Error getting access public key. Invalid status codo: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("⚠️ Error getting access public key: %w", err)
	}

	fmt.Printf("Received public key (%d bytes):\n%s\n", len(body), string(body))

	accessPublicKey, _ := jwt.ParseRSAPublicKeyFromPEM(body)

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
	if claims, ok := token.Claims.(*models.AccessClaims); ok {
		fmt.Printf("Claims parsed: UserID=%s, Username=%s, Role=%s\n",
			claims.UserID, claims.Username, claims.Role)
		if token.Valid {
			return claims, nil
		}
	} else {
		if mapClaims, ok := token.Claims.(jwt.MapClaims); ok {
			fmt.Printf("MapClaims: %+v\n", mapClaims)
		}
	}
	return nil, fmt.Errorf("⚠️ Error invalid token")
}
