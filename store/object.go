package store

import (
	"encoding/json"
	"io"
	"time"
)

// result encapsulation of a single result.
type result struct {
	Key  Key    `json:"key"`
	Data string `json:"data,omitempty"`
}

// Message a single message. This
// is how get our data from the request in
// a structured format.
type Message struct {
	result
	Operation string `json:"operation"`
}

// Validate validates that the message is valid.
func (m *Message) Validate() bool {
	return len(m.Key) > 0
}

// Response a response contains the result and
// the status code of the operation.
type Response struct {
	result
	StatusCode int   `json:"status_code"`
	Error      error `json:"error,omitempty"`
}

// context a structure which encapsulates a single request.
type context struct {
	// Logger allows us to log from the context.
	Logger
	// ID the unique id of the request
	ID string
	// Payload the body of the request.
	Payload []byte
	// conn the open connection.
	conn io.WriteCloser
	// Protocol the protocol the connection is for.
	// either UDP / TCP / HTTP
	Protocol string
	// Time the time the request was received.
	Time time.Time
	// RemoteAddr the remote address.
	RemoteAddr string
}

// write writes data to the connection & then closes.
func (c *context) write(data []byte) error {
	defer c.close()
	_, err := c.conn.Write(data)

	return err
}

// close closes the connection.
func (c *context) close() error {
	if c.Protocol != protocolUDP {
		return c.conn.Close()
	}

	return nil
}

// Write writes a response to the connection.
func (c *context) Write(res Response) error {
	data, err := json.Marshal(res)
	if err != nil {
		return err
	}

	return c.write(data)
}

// WithError writes a response with an error.
func (c *context) WithError(err error) error {
	c.Logf(LevelWarn, "error performing request: %v: %v", c.ID, err)

	return c.Write(Response{StatusCode: StatusErr, Error: err})
}

// port a representation of a port.
type port int

// serverConfig configuration relevant to the server.
type serverConfig struct {
	// UDP the port to listen for UDP connections.
	UDP port
	// TCP the port to listen for TCP connections.
	TCP port
	// HTTP the port to listen for HTTP connections.
	HTTP port
}

// Config the config for the data store.
type Config struct {
	// Server the config for the server.
	Server serverConfig
	// Store the config for the datastore.
	Store storeConfig
}

// NewDefaultConfig initialises default config.
func NewDefaultConfig() Config {
	return Config{
		Server: serverConfig{
			UDP:  defaultUDPPort,
			TCP:  defaultTCPPort,
			HTTP: defaultHTTPPort,
		},
		Store: storeConfig{TTL: defaultDuration},
	}
}
