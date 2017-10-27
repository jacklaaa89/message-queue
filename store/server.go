package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

/* internal constants. */
const (
	defaultUDPPort  = 18080
	defaultTCPPort  = 18081
	defaultHTTPPort = 18082
	protocolUDP     = "udp"
	protocolTCP     = "tcp"
	protocolHTTP    = "http"
	keyURLParameter = "key"
)

/* HTTP routes. */
const (
	routeStore = "/store/{%v}"

	// api routes.
	groupApi   = "/api"
	routeCount = groupApi + "/count"
)

/* Exported constants. */
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

// connections structure to hold open connections.
type connections struct {
	sync.RWMutex
	// the internal collection of connections.
	C map[string]io.Closer
}

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

	// start listening to UDP / TCP / HTTP connections.
	go st.withError(s.listenUDP)
	go st.withError(s.listenTCP)
	go st.withError(s.listenHTTP)

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
	case protocolHTTP:
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
		s.handle(protocolUDP, c)
	}
}

// listenHTTP initialises a HTTP server.
func (s *Server) listenHTTP() error {
	port := s.getAddress(protocolHTTP)

	router := mux.NewRouter()
	router.HandleFunc(fmt.Sprintf(routeStore, keyURLParameter), s.handleHTTP)

	// initialise api routes.
	api := newApi(s.store)
	router.HandleFunc(routeCount, api.count)

	svr := &http.Server{Addr: port, Handler: router}

	// http.Server implements io.Closer so we can add it to our connections.
	s.connections.Add(protocolHTTP, svr)

	// log that we are listening.
	s.Logf(LevelInfo, "listening for http connections on: %v", port)

	return svr.ListenAndServe()
}

// handleHTTP wraps a http.HandlerFunc so that we can call s.handle internally.
func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	// this method is already handled concurrently by http.Server
	// that means the call to handle has to be blocking.
	errs, err := s.handle(protocolHTTP, requestWriter{ResponseWriter: w, Request: r})
	if err != nil {
		w.WriteHeader(http.StatusMethodNotAllowed)
		io.WriteString(w, err.Error())
		return
	}
	<-errs
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

	// tcp needs to be handles slightly differently, we need to accept connections.
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		s.handle(protocolTCP, newConnection(conn))
	}
}

// handle handles a single connection.
// this function returns a channel where
func (s *Server) handle(protocol string, conn connectionWriter) (<-chan error, error) {
	errs := make(chan error)

	// make a buffer to write to.
	var buf = make([]byte, 64<<10)

	// read data from the connection.
	// the read cannot be done concurrently, because this is the block
	// on UDP requests.
	rLen, err := conn.Read(buf)
	//an io.EOF is fine, that means we reached the end of input.
	if err != nil && err != io.EOF {
		return nil, err
	}

	// generate a context. This is an encapsulation of a single request.
	ctx := &context{
		ID:         uuid.New().String(),
		conn:       conn,
		Payload:    buf[0:rLen],
		Protocol:   protocol,
		Time:       time.Now().UTC(),
		RemoteAddr: conn.RemoteAddr(),
		Logger:     s.Logger,
	}

	// handle the request in a new routine.
	go func(ctx *context) { errs <- s.handleRequest(ctx) }(ctx)

	return errs, nil
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
		store:       newStore(config.Store),
		connections: connections{C: make(map[string]io.Closer)},
		state:       newState(),
		Logger:      newLogger(),
	}, nil
}
