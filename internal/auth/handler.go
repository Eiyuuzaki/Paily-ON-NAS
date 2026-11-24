package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/Eiyuuzaki/paily-on-nas/internal/api/response"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// Handler handles auth-related HTTP endpoints.
type Handler struct {
	adminPassword string // plaintext; compared via bcrypt.CompareHashAndPassword
	jwtSecret     string
}

// NewHandler creates an auth handler.
// adminPassword is the plaintext password from config; it is never stored in DB.
func NewHandler(adminPassword, jwtSecret string) *Handler {
	return &Handler{adminPassword: adminPassword, jwtSecret: jwtSecret}
}

// isBcryptHash reports whether s is a bcrypt hash rather than a plaintext
// password, so the two modes can be kept strictly separate.
func isBcryptHash(s string) bool {
	return strings.HasPrefix(s, "$2a$") ||
		strings.HasPrefix(s, "$2b$") ||
		strings.HasPrefix(s, "$2y$")
}

type loginRequest struct {
	Password string `json:"password" binding:"required"`
}

type loginResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

// Login handles POST /api/v1/auth/login.
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	// A bcrypt hash is compared as a hash only. The previous plaintext fallback
	// let anyone who obtained the hash log in by submitting the hash itself,
	// which defeats the point of storing a hash.
	if isBcryptHash(h.adminPassword) {
		if err := bcrypt.CompareHashAndPassword([]byte(h.adminPassword), []byte(req.Password)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid password"})
			return
		}
	} else if subtle.ConstantTimeCompare([]byte(req.Password), []byte(h.adminPassword)) != 1 {
		// Plaintext config remains a development convenience only.
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid password"})
		return
	}

	accessToken, err := IssueAccessToken(h.jwtSecret)
	if err != nil {
		response.InternalError(c, "failed to issue token")
		return
	}
	refreshToken, err := IssueRefreshToken(h.jwtSecret)
	if err != nil {
		response.InternalError(c, "failed to issue refresh token")
		return
	}

	c.JSON(http.StatusOK, loginResponse{Token: accessToken, RefreshToken: refreshToken})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Refresh handles POST /api/v1/auth/refresh.
func (h *Handler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	if err := ParseRefreshToken(h.jwtSecret, req.RefreshToken); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
		return
	}

	accessToken, err := IssueAccessToken(h.jwtSecret)
	if err != nil {
		response.InternalError(c, "failed to issue token")
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": accessToken})
}
