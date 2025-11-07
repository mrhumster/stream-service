package models

import "github.com/golang-jwt/jwt/v5"

type AccessClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}
