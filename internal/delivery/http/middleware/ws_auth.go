package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mrhumster/identity-service/pkg/dto"
)

type TokenServiceIFace interface {
	ValidateAccessToken(tokenString string) (*dto.AccessClaims, error)
}

func WSProtocolAuth(tokenService TokenServiceIFace) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Sec-WebSocket-Protocol")
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse("auth token required"))
			return
		}

		claims, err := tokenService.ValidateAccessToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse("invalid token"))
			return
		}

		userUUID, err := uuid.Parse(claims.UserID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse("invalid user id"))
			return
		}

		c.Set("user", userUUID)
		c.Set("claims", claims)
		c.Next()
	}
}