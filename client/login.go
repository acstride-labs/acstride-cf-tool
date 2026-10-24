package client

import (
	"errors"
	"github.com/acstride-labs/codeforces-client/internal/core"
)

const ErrorNotLogged = "Not logged in"

func findHandle(body []byte) (string, error) {
	h, e := core.Handle(string(body))
	if e != nil {
		return "", errors.New(ErrorNotLogged)
	}
	return h, nil
}
func findCsrf(body []byte) (string, error) { return core.CSRF(string(body)) }

// Legacy password login is intentionally retired. Old password files are not read.
func (c *Client) Login() error {
	return errors.New("Legacy login is disabled. Use codeforces-client session login --experimental --credentials-stdin --store; passwords are never saved")
}
func (c *Client) ConfigLogin() error { return c.Login() }
