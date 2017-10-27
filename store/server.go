package store

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"

	"github.com/google/uuid"
)

const (
	DefaultUDPPort = 18080
	DefaultTCPPort = 18081
)

// port a representation of a port.
type port int

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

func NewDefaultConfig() Config {
	return Config{Server: serverConfig{UDP: DefaultUDPPort}}
}

type connections struct {
	C map[string]net.Conn
	sync.Mutex
}

// state allows us to gracefully shutdown.
type state struct {
	signals chan os.Signal
	errs    chan error
}

// newState generates a new state structure.
func newState() state {
	s := state{
		errs:    make(chan error),
		signals: make(chan os.Signal),
	}
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

func (c *connections) Add(protocol string, conn net.Conn) {
	c.Lock()
	defer c.Unlock()
	c.C[protocol] = conn
}

// Server an instance of the store server.
type Server struct {
	state  state
	config Config
	// store the internal datastore.
	store       Store
	connections connections
}

// Listen initialises and listen on UDP/TCP and HTTP connections for input.
func (s *Server) Listen() error {
	go s.listenUDP()
	go s.listenTCP()

	err := s.state.listen()
	s.close()

	return err
}

func (s *Server) close() {
	for _, conn := range s.connections.C {
		conn.Close()
	}
}

func (s *Server) listenUDP() {
	var err error
	defer func() {
		if err != nil {
			s.state.errs <- err
		}
	}()

	var (
		addr *net.UDPAddr
		conn *net.UDPConn
	)

	port := ":" + strconv.Itoa(int(s.config.Server.UDP))
	addr, err = net.ResolveUDPAddr("udp", port)
	if err != nil {
		return
	}

	conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return
	}

	// set the new connection to the server.
	s.connections.Add("udp", conn)
	fmt.Printf("listening for udp on port: %v\n", port)

	var buf = make([]byte, 1024)

	for {
		rLen, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		go s.handleRequest(&context{ID: uuid.New(), conn: conn, payload: buf[0:rLen]})
	}
}

func (s *Server) listenTCP() {

}

type context struct {
	ID      uuid.UUID
	payload []byte
	conn    net.Conn
}

func (c *context) Write(data []byte) {
	c.conn.Write(data)
}

func (s *Server) handleRequest(c *context) {
	// parse request, handle response.
	fmt.Println(c.ID.String() + " - " + string(c.payload))
	c.Write([]byte("1, 2, 3, 4"))
}

// New initialises a new server instance.
func New(config Config) (*Server, error) {
	return &Server{
		config:      config,
		store:       new(config.Store),
		connections: connections{C: make(map[string]net.Conn)},
		state:       newState(),
	}, nil
}
