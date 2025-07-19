package amqpwraper

import (
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Notes on Channels
// - Channels are lightweight and designed to be used concurrently, but they
// are not "thread-safe" — each goroutine should use its own channel if doing
// concurrent work
// - All channels share the same connection, so creating multiple channels over
// one connection is normal and recommended for parallelism
// - Resources like exchanges or queues are scoped to the broker, not the
// channel — once declared, they can be accessed from any channel (as long as
// permissions allow)
//
// You can see it as:
//   TCP connection (1 socket)
//   ├── Channel ID 1 (publishing messages)
//   ├── Channel ID 2 (consuming messages)
//   ├── Channel ID 3 (RPC calls, queue declare, etc.)

// Connection(1) --> (N)Channel(1) --> (N)Exchange(N) --via bindings--> (N)Queue(1) --> (N)Consumers
//
// Connection ⇒ long-lived socket; heavy to open; few per app.
// Channel    ⇒ cheap, throwaway, gives you concurrency; open many.
// Exchange   ⇒ broker object; declared once; later referenced by name from any channel.
type Connection struct {
	ConnectionName string
	Heartbeat      time.Duration
	Locale         string
	// Also each AMQPConn.Channel() creates new unique channel
	AMQPConn *amqp.Connection
}

type ConnectionConfig struct {
	ConnectionName string
	Locale         string
	Heartbeat      time.Duration
}

func NewConnectionConfig(name string) *ConnectionConfig {
	return &ConnectionConfig{
		ConnectionName: name,
		Locale:         "en_US",
		Heartbeat:      10 * time.Second,
	}
}

func NewConnection(conf *ConnectionConfig, conn *amqp.Connection) *Connection {
	return &Connection{
		ConnectionName: conf.ConnectionName,
		Heartbeat:      conf.Heartbeat,
		Locale:         conf.Locale,
		AMQPConn:       conn,
	}
}
