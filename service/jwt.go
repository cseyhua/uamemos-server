package service

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"uamemos/api"
	"uamemos/common"
	"uamemos/service/auth"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
)

const (
	userIDContextKey = "user-id"
)

type Claims struct {
	Name string `json:"name"`
	jwt.RegisteredClaims
}

func audienceContains(audience jwt.ClaimStrings, token string) bool {
	for _, v := range audience {
		if v == token {
			return true
		}
	}
	return false
}

func getUserIDContextKey() string {
	return userIDContextKey
}

func extractTokenFromHeader(ctx *gin.Context) (string, error) {
	authHeader := ctx.Request.Header.Get("Authorization")
	if authHeader == "" {
		return "", nil
	}
	authHeaderParts := strings.Fields(authHeader)
	if len(authHeaderParts) != 2 || strings.ToLower(authHeaderParts[0]) != "bearer" {
		return "", errors.New("Authorization header format must be Bearer {token}")
	}
	return authHeaderParts[1], nil
}

func findAccessToken(ctx *gin.Context) string {
	accessToken := ""
	cookie, err := ctx.Cookie(auth.AccessTokenCookieName)
	if err == nil {
		accessToken = cookie
	}
	if accessToken == "" {
		accessToken, _ = extractTokenFromHeader(ctx)
	}
	return accessToken
}

func GenerateTokensAndSetCookies(ctx *gin.Context, user *api.User, secret string) error {
	accessToken, err := auth.GenerateAccessToken(user.Name, user.ID, secret)
	if err != nil {
		return errors.Wrap(err, "failed to generate access token")
	}

	cookieExp := time.Now().Add(auth.CookieExpDuration)
	setTokenCookie(ctx, auth.AccessTokenCookieName, accessToken, cookieExp)

	// We generate here a new refresh token and saving it to the cookie.
	refreshToken, err := auth.GenerateRefreshToken(user.Name, user.ID, secret)
	if err != nil {
		return errors.Wrap(err, "failed to generate refresh token")
	}
	setTokenCookie(ctx, auth.RefreshTokenCookieName, refreshToken, cookieExp)

	return nil
}

func setTokenCookie(c *gin.Context, name, token string, expiration time.Time) {
	cookie := new(http.Cookie)
	cookie.Name = name
	cookie.Value = token
	cookie.MaxAge = expiration.Second() - time.Now().Second()
	cookie.Path = "/"
	// Http-only helps mitigate the risk of client side script accessing the protected cookie.
	cookie.HttpOnly = true
	cookie.SameSite = http.SameSiteStrictMode
	c.SetCookie(cookie.Name, cookie.Value, expiration.Second()-time.Now().Second(), cookie.Path, "", true, cookie.HttpOnly)
}

func JWTMiddleware(server *Service, ctx *gin.Context, secret string) {
	path := ctx.Request.URL.Path
	method := ctx.Request.Method

	if server.defaultAuthSkipper(ctx) {
		ctx.Next()
		return
	}

	if common.HasPrefixes(path, "/api/ping", "/api/idp", "/api/user/:id") && method == http.MethodGet {
		ctx.Next()
		return
	}

	token := findAccessToken(ctx)

	if token == "" {
		// Allow the user to access the public endpoints.
		if common.HasPrefixes(path, "/o") {
			ctx.Next()
			return
		}
		// When the request is not authenticated, we allow the user to access the memo endpoints for those public memos.
		if common.HasPrefixes(path, "/api/status", "/api/memo") && method == http.MethodGet {
			ctx.Next()
			return
		}
		ctx.String(http.StatusUnauthorized, "Missing access token")
		return
	}

	claims := &Claims{}
	accessToken, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Name {
			return nil, errors.Errorf("unexpected access token signing method=%v, expect %v", t.Header["alg"], jwt.SigningMethodHS256)
		}
		if kid, ok := t.Header["kid"].(string); ok {
			if kid == "v1" {
				return []byte(secret), nil
			}
		}
		return nil, errors.Errorf("unexpected access token kid=%v", t.Header["kid"])
	})

	if !audienceContains(claims.Audience, auth.AccessTokenAudienceName) {
		ctx.String(http.StatusUnauthorized,
			fmt.Sprintf("Invalid access token, audience mismatch, got %q, expected %q. you may send request to the wrong environment",
				claims.Audience,
				auth.AccessTokenAudienceName,
			))
		return
	}

	generateToken := time.Until(claims.ExpiresAt.Time) < auth.RefreshThresholdDuration
	if err != nil {
		var ve *jwt.ValidationError
		if errors.As(err, &ve) {
			// If expiration error is the only error, we will clear the err
			// and generate new access token and refresh token
			if ve.Errors == jwt.ValidationErrorExpired {
				generateToken = true
			}
		} else {
			ctx.String(http.StatusUnauthorized, "Invalid or expired access token")
			return
		}
	}

	// We either have a valid access token or we will attempt to generate new access token and refresh token
	userID, err := strconv.Atoi(claims.Subject)
	if err != nil {
		ctx.String(http.StatusUnauthorized, "Malformed ID in the token.")
		return
	}

	// Even if there is no error, we still need to make sure the user still exists.
	user, err := server.Store.FindUser(ctx, &api.UserFind{
		ID: &userID,
	})
	if err != nil {
		ctx.String(http.StatusInternalServerError, fmt.Sprintf("Server error to find user ID: %d", userID))
		return
	}
	if user == nil {
		ctx.String(http.StatusUnauthorized, fmt.Sprintf("Failed to find user ID: %d", userID))
		return
	}

	if generateToken {
		generateTokenFunc := func() (int, string) {
			rc, err := ctx.Cookie(auth.RefreshTokenCookieName)

			if err != nil {
				return http.StatusUnauthorized, "Failed to generate access token. Missing refresh token."
			}

			// Parses token and checks if it's valid.
			refreshTokenClaims := &Claims{}
			refreshToken, err := jwt.ParseWithClaims(rc, refreshTokenClaims, func(t *jwt.Token) (any, error) {
				if t.Method.Alg() != jwt.SigningMethodHS256.Name {
					return nil, errors.Errorf("unexpected refresh token signing method=%v, expected %v", t.Header["alg"], jwt.SigningMethodHS256)
				}

				if kid, ok := t.Header["kid"].(string); ok {
					if kid == "v1" {
						return []byte(secret), nil
					}
				}
				return nil, errors.Errorf("unexpected refresh token kid=%v", t.Header["kid"])
			})
			if err != nil {
				if err == jwt.ErrSignatureInvalid {
					return http.StatusUnauthorized, "Failed to generate access token. Invalid refresh token signature."
				}
				return http.StatusInternalServerError, fmt.Sprintf("Server error to refresh expired token. User Id %d", userID)
			}

			if !audienceContains(refreshTokenClaims.Audience, auth.RefreshTokenAudienceName) {
				return http.StatusUnauthorized,
					fmt.Sprintf("Invalid refresh token, audience mismatch, got %q, expected %q. you may send request to the wrong environment",
						refreshTokenClaims.Audience,
						auth.RefreshTokenAudienceName,
					)
			}

			// If we have a valid refresh token, we will generate new access token and refresh token
			if refreshToken != nil && refreshToken.Valid {
				if err := GenerateTokensAndSetCookies(ctx, user, secret); err != nil {
					return http.StatusInternalServerError, fmt.Sprintf("Server error to refresh expired token. User Id %d", userID)
				}
			}

			return 0, ""
		}

		// It may happen that we still have a valid access token, but we encounter issue when trying to generate new token
		// In such case, we won't return the error.
		if code, str := generateTokenFunc(); code != 0 && !accessToken.Valid {
			ctx.String(code, str)
			return
		}
	}

	// Stores userID into context.
	ctx.Set(getUserIDContextKey(), userID)
	ctx.Next()
}
