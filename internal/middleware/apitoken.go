package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// APITokenHeader carries the external token. A header rather than a query
// parameter, which would end up in access logs and browser history.
const APITokenHeader = "X-API-Token"

// APITokenRequired allows a request through when it carries the configured
// token. Setting one happens through the settings API, which session auth
// already guards. The lookup runs per request so regenerating takes effect
// without a restart.
func APITokenRequired(tokenLookup func() (string, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := tokenLookup()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			c.Abort()
			return
		}
		if token == "" {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "external API access is not configured; generate a token in Settings",
			})
			c.Abort()
			return
		}

		// Constant-time, same as the trigger token check, so the comparison
		// itself does not leak the token a byte at a time.
		provided := c.GetHeader(APITokenHeader)
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API token"})
			c.Abort()
			return
		}
		c.Next()
	}
}
