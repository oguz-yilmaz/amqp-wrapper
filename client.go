package amqpwraper

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Client struct {
	m *sync.Mutex

	infolog *log.Logger
	errlog  *log.Logger

	broker   *Broker
	exchange *Exchange
	conn     *Connection
	channel  *amqp.Channel
	queue    *Queue

	done              chan bool
	notifyConnClose   chan *amqp.Error
	notifyChanClose   chan *amqp.Error
	notifyConfirm     chan amqp.Confirmation
	notifyChannCancel chan string

	isReady bool
}

func NewClient(qName string) *Client {
	client := Client{
		m:       &sync.Mutex{},
		infolog: log.New(os.Stdout, "[INFO] ", log.LstdFlags|log.Lmsgprefix),
		errlog:  log.New(os.Stderr, "[ERROR] ", log.LstdFlags|log.Lmsgprefix),
		done:    make(chan bool),
	}
	client.registerChannels()

	go client.handleReconnect()

	return &client
}

func (c *Client) handleReconnect() {
	for {
		select {
		case err := <-c.notifyConnClose:
			c.errlog.Printf("Connection closed: %v. Reconnecting...", err)
			c.reconnect()
		case err := <-c.notifyChanClose:
			c.errlog.Printf("Channel closed: %v. Reinitializing channel...", err)
			c.reconnect() // Optional: Separate channel reconnect logic
		case <-c.done:
			return
		}
	}
}

func (c *Client) addChannel(ch *amqp.Channel) {
	c.channel = ch
	c.registerChannels()
}

func (c *Client) addConnection(conn *Connection) {
	c.conn = conn
}

func (c *Client) addQueue(q *Queue) {
	c.queue = q
}

func (c *Client) registerConnectionClose() {
	c.notifyConnClose = make(chan *amqp.Error, 1)
	if c.conn != nil && c.conn.AMQPConn != nil {
		c.conn.AMQPConn.NotifyClose(c.notifyConnClose)
	}
}

func (c *Client) registerChannelClose() {
	c.notifyChanClose = make(chan *amqp.Error, 1)
	if c.conn != nil && c.conn.AMQPConn != nil {
		c.channel.NotifyClose(c.notifyChanClose)
	}
}

func (c *Client) registerPublishConfirm() {
	c.notifyConfirm = make(chan amqp.Confirmation, 1)
	if c.conn != nil && c.conn.AMQPConn != nil {
		c.channel.NotifyPublish(c.notifyConfirm)
	}
}

func (c *Client) registerChannelCancel() {
	c.notifyChannCancel = make(chan string, 1)
	if c.conn != nil && c.conn.AMQPConn != nil {
		c.channel.NotifyCancel(c.notifyChannCancel)
	}
}

func (c *Client) registerChannels() {
	if c.conn == nil || c.conn.AMQPConn == nil {
		return
	}

	c.registerConnectionClose()
	c.registerChannelClose()
	c.registerPublishConfirm()
	c.registerChannelCancel()
}

func (c *Connection) IsClosed() bool {
	if c.AMQPConn != nil {
		return c.AMQPConn.IsClosed()
	}

	return true
}

func (c *Client) Close() {
	close(c.done)

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

func (c *Client) reconnect() {
	c.m.Lock()
	defer c.m.Unlock()

	for {
		c.errlog.Println("Attempting to reconnect...")

		time.Sleep(5 * time.Second) // TODO: Add exponential backoff

		if c.broker == nil || c.broker.setup == nil {
			c.errlog.Println("Cannot reconnect: broker or setup not available")
			continue
		}

		newClient, err := c.broker.InitMessaging(c.broker.AMQPUrl, *c.broker.setup)
		if err != nil {
			c.errlog.Printf("Reconnect failed: %v", err)
			continue
		}

		// Reuse internal state
		c.changeConnection(newClient.conn)
		c.changeChannel(newClient.channel)
		c.addQueue(newClient.queue)
		c.exchange = newClient.exchange

		c.registerChannels()
		c.infolog.Println("Reconnected successfully")
		return
	}
}

func (client *Client) changeConnection(connection *Connection) {
	client.conn = connection

	conn := client.conn.AMQPConn

	client.notifyConnClose = make(chan *amqp.Error, 1)
	conn.NotifyClose(client.notifyConnClose)
}

func (client *Client) changeChannel(channel *amqp.Channel) {
	client.channel = channel

	client.notifyChanClose = make(chan *amqp.Error, 1)
	client.notifyConfirm = make(chan amqp.Confirmation, 1)

	client.channel.NotifyClose(client.notifyChanClose)
	client.channel.NotifyPublish(client.notifyConfirm)
}

type PublishOptions struct {
	NoWait    bool
	Key       string
	Mandatory bool
	Immediate bool
}

func (c *Client) Publish(msg amqp.Publishing, opts PublishOptions) error {
	if c.conn == nil || c.channel == nil || c.conn.IsClosed() || c.channel.IsClosed() {
		go c.reconnect()
		return errors.New("connection or channel not available")
	}

	return c.channel.Publish(
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
