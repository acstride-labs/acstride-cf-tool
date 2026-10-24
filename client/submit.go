package client

import "errors"

// Submit is retained as a source-compatible stub. Unsafe legacy matching is retired.
func (c *Client) Submit(info Info, langID, source string) error {
	return errors.New("Legacy submission is disabled. Use the versioned CLI with explicit source confirmation and durable receipts")
}
