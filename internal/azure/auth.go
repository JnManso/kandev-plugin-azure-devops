package azure

import (
	"context"
	"encoding/base64"
	"strings"
)

// Authorizer supplies the Authorization header of one workspace connection.
type Authorizer interface {
	// Header returns the Authorization header value for the next request.
	Header(ctx context.Context) (string, error)
	// Renew replaces the credential that produced rejectedHeader. It returns
	// false when the credential cannot be renewed without the user.
	Renew(ctx context.Context, rejectedHeader string) (bool, error)
	// Redact removes credential material from provider error text.
	Redact(text string) string
}

type patAuthorizer struct{ pat string }

// NewPATAuthorizer authenticates with a personal access token over Basic auth.
func NewPATAuthorizer(pat string) Authorizer { return patAuthorizer{pat: pat} }

func (a patAuthorizer) Header(context.Context) (string, error) {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+a.pat)), nil
}

func (patAuthorizer) Renew(context.Context, string) (bool, error) { return false, nil }

func (a patAuthorizer) Redact(text string) string {
	if a.pat == "" {
		return text
	}
	return strings.ReplaceAll(text, a.pat, "[REDACTED]")
}

// BearerPrefix starts an Authorization header carrying an OAuth access token.
const BearerPrefix = "Bearer "

const bearerPrefix = BearerPrefix

type bearerAuthorizer struct{ token string }

// NewBearerAuthorizer authenticates with one known access token, as during
// sign-in before the token is stored. It never renews.
func NewBearerAuthorizer(token string) Authorizer { return bearerAuthorizer{token: token} }

func (a bearerAuthorizer) Header(context.Context) (string, error) { return bearerPrefix + a.token, nil }

func (bearerAuthorizer) Renew(context.Context, string) (bool, error) { return false, nil }

func (a bearerAuthorizer) Redact(text string) string { return RedactToken(text, a.token) }

// RedactToken replaces every occurrence of token in text.
func RedactToken(text, token string) string {
	if token == "" {
		return text
	}
	return strings.ReplaceAll(text, token, "[REDACTED]")
}
