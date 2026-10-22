package runner

import (
	"encoding/base64"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
)

// secretsOf lists credentials that must never appear in build logs, including HTTP Basic forms
func secretsOf(c agentproto.BuildCredentials) []string {
	return []string{
		c.Git.Password,
		basicAuth(c.Git.Username, c.Git.Password),
		c.Registry.Password,
		basicAuth(c.Registry.Username, c.Registry.Password),
	}
}

func basicAuth(username, password string) string {
	if password == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}
