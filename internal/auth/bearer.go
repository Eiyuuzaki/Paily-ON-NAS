package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AdminJWTMiddleware validates the Bearer JWT for admin endpoints.
// Requires the secret to be injected at server startup.
func AdminJWTMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		if err := ParseAccessToken(jwtSecret, token); err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}
		c.Next()
	}
}

// ServiceBearerMiddleware validates the shared service secret for fetcher/checker.
func ServiceBearerMiddleware(serviceSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(serviceSecret)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid service secret"})
			return
		}
		c.Next()
	}
}

// AdminOrServiceMiddleware accepts an administrator JWT or a service credential.
// Use it only for routes that are intentionally shared with service clients.
func AdminOrServiceMiddleware(jwtSecret, serviceSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(serviceSecret)) == 1 {
			c.Next()
			return
		}
		if err := ParseAccessToken(jwtSecret, token); err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}
		c.Next()
	}
}

// SubscribeTokenMiddleware guards the subscription endpoints. A subscription
// body contains usable node credentials, so an exposed port leaks them. The
// token may be supplied as ?token= or as a Bearer header, which keeps normal
// Clash clients working (they can use the full URL).
// expected is a provider so the panel can rotate the token at runtime
// (configs-table backed) without a core restart.
func SubscribeTokenMiddleware(expected func() string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Query("token")
		if token == "" {
			if h, ok := bearerToken(c); ok {
				token = h
			}
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(expected())) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or missing subscribe token"})
			return
		}
		c.Next()
	}
}

// bearerToken extracts the raw token from "Authorization: Bearer <token>".
func bearerToken(c *gin.Context) (string, bool) {
	header := c.GetHeader("Authorization")
	if header == "" {
		return "", false
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	// An empty token must never be treated as a credential: comparing it to an
	// empty service secret would otherwise authenticate the caller.
	if token == "" {
		return "", false
	}
	return token, true
}
