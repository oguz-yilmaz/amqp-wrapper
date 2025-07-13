package amqpwraper

import (
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Notes on Channels
// - Channels are lightweight and designed to be used concurrently, but they
// are not "thread-safe" — each goroutine should use its own channel if doing
// concurrent work.
// - All channels share the same connection, so creating multiple channels over
// one connection is normal and recommended for parallelism.
// - Resources like exchanges or queues are scoped to the broker, not the
// channel — once declared, they can be accessed from any channel (as long as
// permissions allow).

// Connection (1) --> (N) Channel (1) --> (N) Exchange (N) --via bindings--> (N) Queue (1) --> (N) Consumers
//
// Connection ⇒ long-lived socket; heavy to open; few per app.
// Channel    ⇒ cheap, throwaway, gives you concurrency; open many.
// Exchange   ⇒ broker object; declared once; later referenced by name from any channel.
type Connection struct {
	AMQPUrl        string
	ConnectionName string
	Heartbeat      time.Duration
	Locale         string
	Channels       []*amqp.Channel
	// Also each AMQPConn.Channel() creates new unique channel
	AMQPConn *amqp.Connection
	m        sync.Mutex
}

type ConnectionConfig struct {
	AMQPUrl        string
	ConnectionName string
	Locale         string
	Hearbeat       time.Duration
}

func NewConnectionConfig() *ConnectionConfig {
	return &ConnectionConfig{
		AMQPUrl:        amqpURL,
		ConnectionName: "default_connection",
		Locale:         "en_US",
		Hearbeat:       10 * time.Second,
	}
}

func (c *Connection) HasChannel() bool {
	return len(c.Channels) > 0
}

// Reuses the same connection if we already have one
func (c *Connection) Dial() (*amqp.Connection, error) {
	if c.AMQPConn != nil && !c.AMQPConn.IsClosed() {
		return c.AMQPConn, nil
	}

	conn, err := amqp.DialConfig(c.AMQPUrl, amqp.Config{
		Properties: amqp.Table{"connection_name": c.ConnectionName},
		Heartbeat:  c.Heartbeat,
		Locale:     c.Locale,
	})

	c.m.Lock()
	defer c.m.Unlock()
	c.AMQPConn = conn

	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (c *Connection) IsClosed() bool {
	if c.AMQPConn != nil {
		return c.AMQPConn.IsClosed()
	}

	return true
}
