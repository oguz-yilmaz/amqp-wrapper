package amqpwraper

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// type Client interface {
//     Publish(key string, msg amqp091.Publishing) error
//     Consume(opts ConsumeOptions) (<-chan amqp091.Delivery, error)
//     OnChannelClose(fn func(*Channel, error))
//     OnConnectionClose(fn func(*Connection, error))
//     Close() // closes both the connection and channels
//     CloseChannel()
//     CloseConnection()
// }

// Client.Publish(key string, mandatory bool, immediate bool, msg amqp091.Publishing)
// Client.Publish(key string, msg amqp091.Publishing) -> WE WONT USE mandatory and immediate(deprecated)
// Client.Consume(consumer string, autoAck bool, exclusive bool, noLocal bool, noWait bool, args amqp091.Table)

type Client struct {
	m *sync.Mutex

	infolog *log.Logger
	errlog  *log.Logger

	exchange *Exchange
	conn     *Connection
	channel  *amqp.Channel
	queue    *Queue

	done            chan bool
	notifyConnClose chan *amqp.Error
	notifyChanClose chan *amqp.Error
	notifyConfirm   chan amqp.Confirmation

	isReady bool
}

func NewClient(qName string) *Client {
	client := Client{
		m:       &sync.Mutex{},
		infolog: log.New(os.Stdout, "[INFO] ", log.LstdFlags|log.Lmsgprefix),
		errlog:  log.New(os.Stderr, "[ERROR] ", log.LstdFlags|log.Lmsgprefix),
		done:    make(chan bool),
	}
	go client.handleReconnect()
	return &client
}

func (c *Client) addConnection(conn *Connection) {
	c.conn = conn
}

func (c *Client) addQueue(q *Queue) {
	c.queue = q
}

func (c *Client) addExchange(e *Exchange) {
	c.exchange = e
}

func (c *Client) registerConnectionClose() {
	// Create a new Go channel for Connection Close notifications
	c.notifyConnClose = make(chan *amqp.Error, 1)
	// If the connection to RabbitMQ is closed for any reason, send the error
	// to this channel (notifyConnClose) so the client can handle it.
	if c.conn != nil && c.conn.AMQPConn != nil {
		c.conn.AMQPConn.NotifyClose(c.notifyConnClose)
	}
}

func (c *Connection) IsClosed() bool {
	if c.AMQPConn != nil {
		return c.AMQPConn.IsClosed()
	}

	return true
}

func (c *Client) Close() {
	ch := c.channel
	conn := c.conn

	// close the channel before the conneciton
	if ch != nil {
		if err := ch.Close(); err != nil {
			log.Printf("channel close error: %v", err)
		}
	}

	if conn != nil {
		amqpConn := conn.AMQPConn
		if amqpConn != nil {
			amqpConn.Close()
		}
	}
}

func (c *Client) handleReconnect() {

}

func (client *Client) changeConnection(connection *Connection) {
	client.conn = connection

	conn := client.conn.AMQPConn

	client.notifyConnClose = make(chan *amqp.Error, 1)

	// If the connection to RabbitMQ is closed for any reason, send the error
	// to this channel (notifyConnClose) so the client can handle it.
	conn.NotifyClose(client.notifyConnClose)
}

func (client *Client) changeChannel(channel *amqp.Channel) {
	client.channel = channel

	client.notifyChanClose = make(chan *amqp.Error, 1)
	client.notifyConfirm = make(chan amqp.Confirmation, 1)

	// If the channel is closed unexpectedly (e.g., RabbitMQ crashes, or your
	// code misuses it), notify us
	client.channel.NotifyClose(client.notifyChanClose)
	// Notify us when the broker confirms that it has received and persisted a
	// published message
	client.channel.NotifyPublish(client.notifyConfirm)
}

type PublishOptions struct {
	NoWait    bool
	Key       string
	Mandatory bool
	Immediate bool
}

func (c *Client) Publish(msg amqp.Publishing, opts PublishOptions) error {
	if c.conn == nil {
		return errors.New("no connection available")
	}

	ch, err := c.conn.AMQPConn.Channel()
	if err != nil {
		return fmt.Errorf("failed to create channel: %w", err)
	}

	return ch.Publish(
		c.exchange.Name,
		opts.Key,
		opts.Mandatory,
		opts.Immediate,
		msg,
	)
}

type ConsumeOptions struct {
	Consumer  string
	AutoAck   bool
	Exclusive bool
	NoLocal   bool
	NoWait    bool
	Args      amqp.Table
}

func (c *Client) Consume(opts ConsumeOptions) (<-chan amqp.Delivery, error) {
	if c.conn == nil {
		return nil, errors.New("no connection available")
	}

	ch, err := c.conn.AMQPConn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to create channel: %w", err)
	}

	return ch.Consume(
		c.queue.Name,
		opts.Consumer,
		opts.AutoAck,
		opts.Exclusive,
		opts.NoLocal,
		opts.NoWait,
		opts.Args,
	)
}
