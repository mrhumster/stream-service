package middleware

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mrhumster/stream-service/internal/delivery/http/dto/response"
	"github.com/mrhumster/stream-service/internal/service"

	"github.com/mrhumster/identity-service/pkg/auth"
	"github.com/mrhumster/identity-service/pkg/middleware"
)

func AuthMiddleware(tokenService *service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c.Request)
		claims, err := tokenService.ValidateAccessToken(token)
		if err != nil {
			log.Printf("⚠️ AuthMiddleware error: %v", err)
			c.JSON(http.StatusUnauthorized, response.ErrorResponse("invalid token claims"))
			c.Abort()
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("claims", claims)
		c.Next()
	}
}

func extractToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 || parts[0] == "Bearer" {
			return parts[1]
		}
	}
	return r.URL.Query().Get("token")
}

func Authorize(obj string, act string, client auth.PermissionClient) gin.HandlerFunc {
	return middleware.Authorize(client, obj, act)
}
