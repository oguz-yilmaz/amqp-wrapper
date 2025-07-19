package amqpwraper

import (
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

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
