package czds

// Logger specifies the methods required for the verbose logger for the API
type Logger interface {
	Printf(format string, v ...interface{})
}

// SetLogger enables verbose printing for most API calls with the provided logger
// defaults to nil/off.
func (c *Client) SetLogger(l Logger) {
	c.log = l
}

func (c *Client) v(format string, v ...interface{}) {
	if c.log != nil {
		c.log.Printf(format, v...)
	}
}
