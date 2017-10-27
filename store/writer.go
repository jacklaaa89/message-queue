package store

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"net"

	"io/ioutil"

	"github.com/gorilla/mux"
)

// requestWriter allows us to treat a request and a response as a connectionWriter
type requestWriter struct {
	// the response writer. This allows us to write to the response directly.
	http.ResponseWriter
	// request. This allows us to access all the request details, including
	// the remote address, the initial request body, the http.Method etc.
	*http.Request
}

// Write calls the write method of the response, i.e. writes to the http.Response.
func (r requestWriter) Write(b []byte) (int, error) { return r.ResponseWriter.Write(b) }

// RemoveAddr returns the requests RemoteAddr field.
func (r requestWriter) RemoteAddr() string { return r.Request.RemoteAddr }

// Close there is nothing to close at this point.
func (requestWriter) Close() error { return nil }

// errInvalidMethod error returned when an invalid HTTP method is used.
var errInvalidaMethod = errors.New("invalid http method")

// errNoKeyProvided error returned when no key is provided in the request
var errNoKeyProvided = errors.New("no key provided in request")

// Read reads the request body
// this method formats the response based the request received.
func (r requestWriter) Read(b []byte) (int, error) {
	re := r.Request

	// read parameters in form url to get key.
	vars := mux.Vars(re)
	key, ok := vars[keyURLParameter]
	if !ok {
		return 0, errNoKeyProvided
	}

	// the new message.
	var mes = Message{result: result{Key: Key(key)}}

	switch re.Method {
	case http.MethodGet:
		// set the correct operation type.
		mes.Operation = OperationGet
	case http.MethodPost:
		// read the request body and set it inside
		// the data variable & set the operation type.
		mes.Operation = OperationSet
		var (
			buf []byte
			err error
		)
		if buf, err = ioutil.ReadAll(re.Body); err != nil && err != io.EOF {
			return 0, err
		}

		re.Body.Close()
		mes.Data = string(buf)
	default:
		return 0, errInvalidaMethod
	}

	// copy the contents to mes into the supplied buffer.
	data, err := json.Marshal(mes)
	if err != nil {
		return 0, err
	}

	// ensure the capacity of the buffer
	if len(b) < len(data) {
		b = make([]byte, len(data))
	}

	// copy the contents of data into the buffer.
	n := copy(b[0:], data)

	// return the amount of values we copied in.
	return n, nil
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
