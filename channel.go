package rpcpool

import (
	"errors"
	"fmt"
	"net/rpc"
	"sync"
)

// channelPool implements the Pool interface based on buffered channels.
type channelPool struct {
	// storage for our net.Conn connections
	mu    sync.Mutex
	conns chan *rpc.Client
	// used connections
	workConns chan *rpc.Client

	// net.Conn generator
	factory Factory
}

// Factory is a function to create new connections.
type Factory func() (*rpc.Client, error)

// NewChannelPool returns a new pool based on buffered channels with an initial
// capacity and maximum capacity. Factory is used when initial capacity is
// greater than zero to fill the pool. A zero initialCap doesn't fill the Pool
// until a new Get() is called. During a Get(), If there is no new connection
// available in the pool, a new connection will be created via the Factory()
// method.
func NewChannelPool(initialCap, maxCap int, factory Factory) (Pool, error) {
	if initialCap < 0 || maxCap <= 0 || initialCap > maxCap {
		return nil, errors.New("invalid capacity settings")
	}

	c := &channelPool{
		conns:     make(chan *rpc.Client, maxCap),
		workConns: make(chan *rpc.Client, maxCap),
		factory:   factory,
	}

	// create initial connections, if something goes wrong,
	// just close the pool error out.
	for i := 0; i < initialCap; i++ {
		conn, err := factory()
		if err != nil {
			c.Close()
			return nil, fmt.Errorf("factory is not able to fill the pool: %s", err)
		}
		c.conns <- conn
	}

	return c, nil
}

func (c *channelPool) getConnsAndFactory() (chan *rpc.Client, Factory) {
	c.mu.Lock()
	conns := c.conns
	factory := c.factory
	c.mu.Unlock()
	return conns, factory
}

// Get implements the Pool interfaces Get() method. If there is no new
// connection available in the pool, a new connection will be created via the
// Factory() method.
func (c *channelPool) Get() (*rpc.Client, error) {
	conns, factory := c.getConnsAndFactory()
	if conns == nil {
		return nil, ErrClosed
	}

	// wrap our connections with out custom net.Conn implementation (wrapConn
	// method) that puts the connection back to the pool if it's closed.
	for {
		select {
		case conn := <-conns:
			c.workConns <- conn
			if conn == nil {
				return nil, ErrClosed
			}
			return conn, nil
		default:
			if len(c.workConns) >= cap(c.workConns) {
				continue
			}
			conn, err := factory()
			if err != nil {
				return nil, err
			}
			c.workConns <- conn
			return conn, nil
		}
	}

}

// put puts the connection back to the pool. If the pool is full or closed,
// conn is simply closed. A nil conn will be rejected.
func (c *channelPool) PutBack(conn *rpc.Client) error {
	if conn == nil {
		return errors.New("connection is nil. rejecting")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conns == nil {
		// pool is closed, close passed connection
		return conn.Close()
	}

	// put the resource back into the pool. If the pool is full, this will
	// block and the default case will be executed.
	select {
	case c.conns <- conn:
		<-c.workConns
		return nil
	default:
		// pool is full, close passed connection
		<-c.workConns
		return conn.Close()
	}
}

func (c *channelPool) Close() {
	c.mu.Lock()
	conns := c.conns
	workConns := c.workConns
	c.conns = nil
	c.factory = nil
	c.mu.Unlock()

	if conns == nil {
		return
	}

	close(conns)
	for conn := range conns {
		conn.Close()
	}
	if workConns == nil {
		return
	}
	close(workConns)
	for conn := range workConns {
		conn.Close()
	}
}

func (c *channelPool) Len() int {
	conns, _ := c.getConnsAndFactory()
	return len(conns)
}
