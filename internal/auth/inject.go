package auth

import (
	"encoding/base64"
	"net/http"

	"github.com/angelmsger/jenkins-cli/pkg/transport"
)

// Header returns the Authorization header value for the credential.
//
// Jenkins authenticates API requests with HTTP Basic auth in both schemes:
// username:apiToken (token) or username:password (basic). Either way the wire
// form is "Basic base64(username:secret)".
func (c Credential) Header() string {
	switch c.Scheme {
	case SchemeToken, SchemeBasic:
		raw := c.Username + ":" + c.Secret
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
	default:
		return ""
	}
}

// Decorator returns a transport.Decorator that authenticates every request.
func (c Credential) Decorator() transport.Decorator {
	header := c.Header()
	return func(req *http.Request) {
		if header != "" {
			req.Header.Set("Authorization", header)
		}
	}
}
