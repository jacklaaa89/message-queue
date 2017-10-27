package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	defaultUDPPort = 18080
	defaultTCPPort = 18081
	protocolUDP    = "udp"
	protocolTCP    = "tcp"
)

const (
	// OperationGet the operation type to get a piece of data.
	OperationGet = "get"
	// OperationSet the operation type to set a piece of data.
	OperationSet = "set"
	// StatusOK status code when the operation was a success.
	StatusOK = 0
	// StatusErr status code when the operation resulted in an error.
	StatusErr = 1
)

// errInvalidMessage an error returned when an invalid message was received.
var errInvalidMessage = errors.New("invalid message")

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
		Server: serverConfig{UDP: defaultUDPPort, TCP: defaultTCPPort},
		Store:  storeConfig{TTL: defaultDuration},
	}
}

// connections structure to hold open connections.
type connections struct {
	sync.RWMutex
	// the internal collection of connections.
	C map[string]io.Closer
}

// state allows us to gracefully shutdown.
type state struct {
	// signals the channel in which os.Signals
	// are triggered on, this is so we respond correctly
	// to os.Interrupt / os.Kill signals.
	signals chan os.Signal
	// errs the channel in which errors are passed to.
	// this is so we can respond to errors.
	errs chan error
}

// newState generates a new state structure.
func newState() state {
	s := state{
		errs:    make(chan error),
		signals: make(chan os.Signal),
	}

	// ensure we are notified about os signals.
	signal.Notify(s.signals, os.Interrupt, os.Kill)

	return s
}

// listen listens for state changes in the server.
// this function is blocking until a close signal is triggered.
func (s *state) listen() error {
	for {
		select {
		case err := <-s.errs:
			return err
		case <-s.signals:
			return nil
		}
	}
}

// connectionWriter a ReadWriteCloser that can also
// get the remote address of a connection.
type connectionWriter interface {
	io.ReadWriteCloser
	RemoteAddr() string
}

// connection an encapsulation of a connection.
// connection implements a io.ReadWriteCloser
type connection struct {
	// conn the connection.
	conn net.Conn
	// addr the remote connection address.
	addr net.Addr
}

// newConnection initialises a new conection.
func newConnection(conn net.Conn) *connection {
	return &connection{
		conn: conn,
		addr: conn.RemoteAddr(),
	}
}

// RemoteAddr returns the remote address of the connection.
func (c *connection) RemoteAddr() string { return c.addr.String() }

// Write writes to the underlined connection
func (c *connection) Write(b []byte) (int, error) {
	switch conn := c.conn.(type) {
	case *net.UDPConn:
		n, err := conn.WriteToUDP(b, c.addr.(*net.UDPAddr))
		return n, err
	}

	return c.conn.Write(b)
}

// Read reads from the connection, if the connection is
// UDP it also reads the remote address.
func (c *connection) Read(b []byte) (int, error) {
	switch conn := c.conn.(type) {
	case *net.UDPConn:
		n, addr, err := conn.ReadFromUDP(b)
		c.addr = addr
		return n, err
	}

	return c.conn.Read(b)
}

// Close closes the connection.
func (c *connection) Close() error { return c.conn.Close() }

// Add adds a new connection to the pool.
func (c *connections) Add(protocol string, conn io.Closer) {
	c.Lock()
	defer c.Unlock()
	c.C[protocol] = conn
}

// Get gets an existing connection.
func (c *connections) Get(protocol string) io.Closer {
	c.RLock()
	defer c.RUnlock()

	return c.C[protocol]
}

// Server an instance of the store server.
type Server struct {
	// logger the logger to write to.
	Logger
	// state the internal state of the server.
	state state
	// config the supplied server config.
	config Config
	// store the internal datastore.
	store Store
	// connections the map of open connection handlers.
	connections connections
}

// Listen initialises and listen on UDP/TCP and HTTP connections for input.
func (s *Server) Listen() error {
	var st = s.state

	// start listening to UDP / TCP connections.
	go st.withError(s.listenUDP)
	go st.withError(s.listenTCP)

	// listen until an error occurs on any
	// protocol, this could be improved to restart listening
	// on a particular protocol rather than killing off the
	// entire server.
	err := s.state.listen()
	// gracefully close connection handlers.
	s.close()

	return err
}

// close closes all of the open connections.
func (s *Server) close() {
	s.Logf(LevelInfo, "closing connections...")
	for _, conn := range s.connections.C {
		conn.Close()
	}
}

// getAddress gets the address to listen on for a particular protocol.
func (s *Server) getAddress(protocol string) string {
	config := s.config.Server
	var port int
	switch protocol {
	case protocolUDP:
		port = int(config.UDP)
	case protocolTCP:
		port = int(config.TCP)
	case "http":
		port = int(config.HTTP)
	}

	return ":" + strconv.Itoa(port)
}

// withError wraps a function that returns an error into a func which can be
// started by a go routine. The returned error is pushed to the err channel in the
// state.
func (s *state) withError(f func() error) {
	err := f()
	if err != nil {
		s.errs <- err
	}
}

// listenUDP initialises a UDP connection.
func (s *Server) listenUDP() error {
	// attempt to initialise a connection.
	port := s.getAddress(protocolUDP)
	addr, err := net.ResolveUDPAddr(protocolUDP, port)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP(protocolUDP, addr)
	if err != nil {
		return err
	}

	c := newConnection(conn)

	// set the new connection to the server.
	s.connections.Add(protocolUDP, c)

	// log that we are listening.
	s.Logf(LevelInfo, "listening for udp connections on: %v", port)

	// handle incoming connections on this protocol.
	for {
		err := s.handle(protocolUDP, c)
		if err != nil {
			return err
		}
	}
}

// handle handles a single connection..
func (s *Server) handle(protocol string, conn connectionWriter) error {
	// make a buffer to write to.
	var buf = make([]byte, 1024)

	// read data from the connection.
	rLen, err := conn.Read(buf)
	if err != nil {
		return err
	}

	context := &context{
		ID:         uuid.New().String(),
		conn:       conn,
		Payload:    buf[0:rLen],
		Protocol:   protocol,
		Time:       time.Now().UTC(),
		RemoteAddr: conn.RemoteAddr(),
		Logger:     s.Logger,
	}

	// handle the request in a new routine.
	go s.handleRequest(context)

	return nil
}

// listenTCP initialises a TCP connection.
func (s *Server) listenTCP() error {
	// attempt to initialise a connection.
	port := s.getAddress(protocolTCP)
	addr, err := net.ResolveTCPAddr(protocolTCP, port)
	if err != nil {
		return err
	}

	l, err := net.ListenTCP(protocolTCP, addr)
	if err != nil {
		return err
	}

	// defer a close to the connection.
	defer l.Close()

	// add the connection to our connection pool.
	s.connections.Add(protocolTCP, l)

	s.Logf(LevelInfo, "listening for tcp connections on: %v", port)

	// tcp needs to be handles slightly differently, we need to accept conns.
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		if err := s.handle(protocolTCP, newConnection(conn)); err != nil {
			return err
		}
	}
}

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

// handleRequest func which handles reading and writing to
// the store based on the request received.
func (s *Server) handleRequest(c *context) error {
	st := s.store
	s.Logf(
		LevelInfo,
		"received request: %v - protocol: %v, remote address: %v",
		c.ID,
		c.Protocol,
		c.RemoteAddr,
	)

	mes, res := Message{}, Response{StatusCode: StatusOK}
	err := json.Unmarshal(c.Payload, &mes)
	if err != nil {
		return c.WithError(fmt.Errorf("invalid json provided: %v", err))
	}

	// ensure the message is valid.
	if !mes.Validate() {
		return c.WithError(errInvalidMessage)
	}

	// set the key that was sent.
	res.Key = mes.Key

	switch mes.Operation {
	case OperationGet:
		data, err := st.Get(mes.Key)
		if err != nil {
			return c.WithError(err)
		}
		res.Data = string(data)
	case OperationSet:
		if err := st.Set(mes.Key, []byte(mes.Data)); err != nil {
			return c.WithError(err)
		}
	default:
		return c.WithError(errInvalidMessage)
	}

	return c.Write(res)
}

// New initialises a new server instance.
func New(config Config) (*Server, error) {
	return &Server{
		config:      config,
		store:       new(config.Store),
		connections: connections{C: make(map[string]io.Closer)},
		state:       newState(),
		Logger:      newLogger(),
	}, nil
}
