package middleware

import (
	"log"
	"net/http"
	"strings"

	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/mrhumster/stream-service/internal/delivery/http/dto/response"
	"github.com/mrhumster/stream-service/internal/service"

	"github.com/mrhumster/web-server-gin/pkg/auth"
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
	if authHeader == "" {
		return ""
	}
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ""
	}
	return parts[1]
}

func Authorize(obj string, act string) gin.HandlerFunc {
	client, err := auth.NewPermissionClient("auth-service:50051")
	if err != nil {
		log.Fatal("Failed to create auth client:", err)
	}
	return func(c *gin.Context) {
		userIDinterface, exists := c.Get("userID")
		userID := fmt.Sprintf("%v", userIDinterface)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.ErrorResponse("User hasn't logged in yet"))
			return
		}

		resourceID := c.Param("id")

		fullResource := obj
		if resourceID != "" {
			fullResource = fmt.Sprintf("%s/%s", obj, resourceID)
		}

		if ok, _ := client.CheckPermission(c.Request.Context(), userID, fullResource, act); !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, response.ErrorResponse("Access denied"))
			return
		}

		c.Next()
	}
}
