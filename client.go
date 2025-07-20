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

	lastConsumeOpts *ConsumeOptions
	consumerChan    chan amqp.Delivery
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
		time.Sleep(5 * time.Second)

		if c.broker == nil || c.broker.setup == nil {
			c.errlog.Println("Cannot reconnect: broker or setup not available")
			continue
		}

		newClient, err := c.broker.InitMessaging(c.broker.AMQPUrl, *c.broker.setup)
		if err != nil {
			c.errlog.Printf("Reconnect failed: %v", err)
			continue
		}

		c.changeConnection(newClient.conn)
		c.changeChannel(newClient.channel)
		c.addQueue(newClient.queue)
		c.exchange = newClient.exchange
		c.registerChannels()

		// Restart consumer if one existed
		if c.lastConsumeOpts != nil {
			if c.lastConsumeOpts.Qos > 0 {
				_ = c.channel.Qos(c.lastConsumeOpts.Qos, 0, false)
			}

			deliveryStream, err := c.channel.Consume(
				c.queue.Name,
				c.lastConsumeOpts.Consumer,
				c.lastConsumeOpts.AutoAck,
				c.lastConsumeOpts.Exclusive,
				c.lastConsumeOpts.NoLocal,
				c.lastConsumeOpts.NoWait,
				c.lastConsumeOpts.Args,
			)
			if err != nil {
				c.errlog.Printf("Failed to resume consuming: %v", err)
				continue
			}

			go func() {
				for msg := range deliveryStream {
					select {
					case c.consumerChan <- msg:
					case <-c.done:
						return
					}
				}
				go c.reconnect()
			}()
		}

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

func (c *Client) SetQos(prefetchCount int) error {
	if c.channel == nil {
		return errors.New("channel not initialized")
	}

	return c.channel.Qos(prefetchCount, 0, false)
}

type ConsumeOptions struct {
	Consumer  string
	AutoAck   bool
	Exclusive bool
	NoLocal   bool
	NoWait    bool
	Qos       int
	Args      amqp.Table
}

func (c *Client) Consume(opts ConsumeOptions) (<-chan amqp.Delivery, error) {
	if c.conn == nil || c.channel == nil || c.conn.IsClosed() || c.channel.IsClosed() {
		go c.reconnect()
		return nil, errors.New("connection or channel not available")
	}

	if opts.Qos > 0 {
		if err := c.SetQos(opts.Qos); err != nil {
			return nil, fmt.Errorf("failed to set QoS: %w", err)
		}
	}

	deliveries, err := c.channel.Consume(
		c.queue.Name,
		opts.Consumer,
		opts.AutoAck,
		opts.Exclusive,
		opts.NoLocal,
		opts.NoWait,
		opts.Args,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start consuming: %w", err)
	}

	c.lastConsumeOpts = &opts
	if c.consumerChan == nil {
		c.consumerChan = make(chan amqp.Delivery)
	}

	go func() {
		for msg := range deliveries {
			select {
			case c.consumerChan <- msg:
			case <-c.done:
				return
			}
		}
		go c.reconnect()
	}()

	return c.consumerChan, nil
}
