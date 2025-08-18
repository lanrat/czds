package czds

// Logger specifies the methods required for the verbose logger for the API
type Logger interface {
	Printf(format string, v ...any)
}

// SetLogger enables verbose printing for most API calls with the provided logger.
// Defaults to nil/off.
func (c *Client) SetLogger(l Logger) {
	c.log = l
}

// v logs a formatted message using the configured logger if verbose logging is enabled.
// This is used internally for debug output throughout the CZDS client.
func (c *Client) v(format string, v ...any) {
	if c.log != nil {
		c.log.Printf(format, v...)
	}
}
