package amqpwraper

import (
	"errors"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// TODO
// - Add auth mechanism
// - Add TLS
// - Add Quorum Queues

type ExchangeType string

const (
	DIRECT  ExchangeType = "direct"
	TOPIC   ExchangeType = "topic"
	FANOUT  ExchangeType = "fanout"
	HEADERS ExchangeType = "headers"
)

type Exchange struct {
	Name       string
	Type       ExchangeType
	Durable    bool       // Indicates if the exchange should survive a broker restart
	AutoDelete bool       // Indicates if the exchange should be deleted when no queues are bound to it
	Internal   bool       // Indicates if the exchange is internal (not accessible by clients)
	NoWait     bool       // Indicates if the exchange declaration should not wait for a confirmation from the broker
	Args       amqp.Table // Additional arguments for the exchange declaration
	Bindings   []*QueueBinding
}

type QueueBinding struct {
	Queue      *Queue    // Not set manually, set by QueueDeclare
	Exchange   *Exchange // Not set manually, set by QueueDeclare
	BindingKey string
	NoWait     bool
	Args       amqp.Table
}

type Queue struct {
	Name       string
	Passive    bool
	Durable    bool
	AutoDelete bool
	Exclusive  bool
	NoWait     bool
	Args       amqp.Table
	AMQPQueue  *amqp.Queue
	Bindings   []*QueueBinding
}

type QueueConfig struct {
	Name       string
	Passive    bool
	Durable    bool
	AutoDelete bool
	Exclusive  bool
	NoWait     bool
	Args       amqp.Table
}

func NewQueue(conf QueueConfig) *Queue {
	return &Queue{
		Name:       conf.Name,
		Passive:    conf.Passive,
		Durable:    conf.Durable,
		AutoDelete: conf.AutoDelete,
		Exclusive:  conf.Exclusive,
		NoWait:     conf.NoWait,
		Args:       conf.Args,
	}
}

type BrokerConfig struct {
	AMQPUrl    string
	MaxChannel int
}

type Broker struct {
	AMQPUrl  string
	Clients  map[string]*Client // key: ex:<exchange_name>|queue:<queue_name>
	channels []*amqp.Channel
	conn     *Connection
	m        sync.RWMutex
	setup    *QueueSetup
	BrokerConfig
}

func NewBroker(config BrokerConfig) *Broker {
	maxChannel := config.MaxChannel
	if maxChannel <= 0 {
		maxChannel = 10
	}

	return &Broker{
		AMQPUrl:  config.AMQPUrl,
		Clients:  make(map[string]*Client),
		channels: make([]*amqp.Channel, 0),
		m:        sync.RWMutex{},
		BrokerConfig: BrokerConfig{
			MaxChannel: maxChannel,
		},
	}
}

func (b *Broker) addChannel() *amqp.Channel {
	if !b.hasExistingAMQPConnection() {
		return nil
	}

	ch, err := b.conn.AMQPConn.Channel()
	if err != nil {
		// You might want to log this
		return nil
	}

	b.channels = append(b.channels, ch)
	return ch
}

func (b *Broker) getUsableChannel() *amqp.Channel {
	b.m.RLock()
	for _, ch := range b.channels {
		if ch != nil && !ch.IsClosed() {
			b.m.RUnlock()
			return ch
		}
	}
	b.m.RUnlock()

	b.cleanClosedChannels()

	return b.addChannel()
}

func (b *Broker) cleanClosedChannels() {
	activeChannels := make([]*amqp.Channel, 0, len(b.channels))
	for _, ch := range b.channels {
		if ch != nil && !ch.IsClosed() {
			activeChannels = append(activeChannels, ch)
		}
	}

	b.channels = activeChannels
}

// # Creates new connection
//
// Reuses if the same connection name passed
func (b *Broker) Dial(
	amqpUrl string,
	connectionName string,
	client *Client,
) (*Client, error) {
	if amqpUrl == "" {
		return nil, errors.New("AMQP URL must be provided")
	}
	conf := NewConnectionConfig(connectionName)

	conn, err := b.CreateConnection(conf, amqpUrl)
	if err != nil {
		return nil, fmt.Errorf("dial error: %w", err)
	}

	b.addConnection(conn)
	client.addConnection(conn)

	return client, nil
}

func (b *Broker) addConnection(c *Connection) {
	if b == nil {
		panic("Broker is nil in addConnection")
	}
	if b.hasExistingAMQPConnection() {
		return
	}

	b.conn = c
}

func (b *Broker) hasExistingAMQPConnection() bool {
	if b != nil && b.conn != nil && b.conn.AMQPConn != nil && !b.conn.AMQPConn.IsClosed() {
		return true
	}

	return false
}

func (b *Broker) GetAMQPConnection() *amqp.Connection {
	if b.hasExistingAMQPConnection() {
		return b.conn.AMQPConn
	}

	return nil
}

func (b *Broker) CreateConnection(conf *ConnectionConfig, amqpUrl string) (*Connection, error) {
	if b.hasExistingAMQPConnection() {
		return b.conn, nil
	}

	cn, err := amqp.DialConfig(amqpUrl, amqp.Config{
		Properties: amqp.Table{"connection_name": conf.ConnectionName},
		Heartbeat:  conf.Heartbeat,
		Locale:     conf.Locale,
	})
	if err != nil {
		return nil, err
	}

	return NewConnection(conf, cn), nil
}

func (b *Broker) ExchangeDeclare(ex Exchange, c *Client) (*amqp.Channel, error) {
	ch := b.getUsableChannel()

	err := ch.ExchangeDeclare(
		ex.Name,
		string(ex.Type),
		ex.Durable,
		ex.AutoDelete,
		ex.Internal,
		ex.NoWait,
		ex.Args,
	)
	if err != nil {
		return nil, fmt.Errorf("exchange declare error: %w", err)
	}

	b.m.Lock()
	c.exchange = &ex
	c.addChannel(ch)
	b.m.Unlock()

	return ch, nil
}

func (b *Broker) addClient(qName, exName string, c *Client) {
	clientMapKey := fmt.Sprintf("ex:%s|queue:%s", exName, qName)
	_, ok := b.Clients[clientMapKey]
	if !ok {
		b.Clients[clientMapKey] = c
	}
}

func (b *Broker) QueueDeclare(qconf QueueConfig, qb QueueBinding, exName string, c *Client) (*Queue, error) {
	q := NewQueue(qconf)

	conn := b.GetAMQPConnection()
	if conn == nil || conn.IsClosed() {
		return nil, fmt.Errorf("no active connection found")
	}

	b.addClient(qconf.Name, exName, c)

	ch := b.getUsableChannel()

	// Create a new Queue instance
	var amqpQueue amqp.Queue
	var err error
	if !q.Passive {
		amqpQueue, err = ch.QueueDeclare(
			q.Name,
			q.Durable,
			q.AutoDelete,
			q.Exclusive,
			q.NoWait,
			q.Args,
		)
	} else {
		amqpQueue, err = ch.QueueDeclarePassive(
			q.Name,
			q.Durable,
			q.AutoDelete,
			q.Exclusive,
			q.NoWait,
			q.Args,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("queue declare error: %w", err)
	}

	q.AMQPQueue = &amqpQueue

	// Bind the queue to the exchange
	err = ch.QueueBind(q.Name, qb.BindingKey, exName, qb.NoWait, qb.Args)
	if err != nil {
		return nil, fmt.Errorf("queue bind error: %w", err)
	}

	Bind(q, c.exchange, &qb)

	return q, nil
}

// Caller should ensure that this is called in a thread-safe manner
func Bind(
	q *Queue,
	ex *Exchange,
	b *QueueBinding,
) {
	if q == nil || ex == nil || b == nil || b.Queue != nil || b.Exchange != nil {
		return
	}

	queueHasBinding := false
	exchangeHasBinding := false

	for _, existing := range q.Bindings {
		if existing.BindingKey == b.BindingKey && existing.Exchange == ex {
			queueHasBinding = true
			break
		}
	}

	for _, existing := range ex.Bindings {
		if existing.BindingKey == b.BindingKey && existing.Queue == q {
			exchangeHasBinding = true
			break
		}
	}

	if queueHasBinding && exchangeHasBinding {
		return
	}

	b.Queue = q
	b.Exchange = ex

	if !queueHasBinding {
		q.Bindings = append(q.Bindings, b)
	}
	if !exchangeHasBinding {
		ex.Bindings = append(ex.Bindings, b)
	}
}

type QueueSetup struct {
	Exchange    Exchange
	QueueConfig QueueConfig
	Bindings    []QueueBinding
}

func (b *Broker) InitMessaging(
	connectionName string,
	setup QueueSetup,
) (*Client, error) {
	if setup.QueueConfig.Name == "" {
		return nil, errors.New("queue name must be provided")
	}
	if setup.Exchange.Name == "" {
		return nil, errors.New("exchange name must be provided")
	}

	if b.AMQPUrl == "" {
		return nil, errors.New("queue name must be provided")
	}

	client := NewClient(b.AMQPUrl)
	client, err := b.Dial(b.AMQPUrl, connectionName, client)
	if err != nil {
		return nil, fmt.Errorf("dial error: %w", err)
	}

	// Set client reference in the broker
	b.m.Lock()
	b.setup = &setup
	b.Clients[fmt.Sprintf("ex:%s|queue:%s", setup.Exchange.Name, setup.QueueConfig.Name)] = client
	client.broker = b
	b.m.Unlock()

	ch, err := b.ExchangeDeclare(setup.Exchange, client)
	if err != nil {
		return nil, fmt.Errorf("exchange declare error: %w", err)
	}
	client.changeChannel(ch)

	// Pick the first binding just to declare the queue
	var baseBinding QueueBinding
	if len(setup.Bindings) > 0 {
		baseBinding = setup.Bindings[0]
	} else {
		return nil, fmt.Errorf("at least one binding must be provided")
	}

	rq, err := b.QueueDeclare(setup.QueueConfig, baseBinding, setup.Exchange.Name, client)
	if err != nil {
		return nil, fmt.Errorf("queue declare error: %w", err)
	}

	client.addQueue(rq)

	// Bind all other keys (including the first one again if needed)
	for _, binding := range setup.Bindings {
		err = b.bindQueueToExchange(rq, &setup.Exchange, binding)
		if err != nil {
			return nil, fmt.Errorf("queue bind error for key '%s': %w", binding.BindingKey, err)
		}
	}

	return client, nil
}

func (b *Broker) bindQueueToExchange(
	q *Queue,
	ex *Exchange,
	binding QueueBinding,
) error {
	ch := b.getUsableChannel()
	if ch == nil {
		return fmt.Errorf("no usable channel found")
	}

	err := ch.QueueBind(q.Name, binding.BindingKey, ex.Name, binding.NoWait, binding.Args)
	if err != nil {
		return err
	}

	// Maintain binding state in memory
	bnd := &QueueBinding{
		BindingKey: binding.BindingKey,
		NoWait:     binding.NoWait,
		Args:       binding.Args,
	}
	Bind(q, ex, bnd)
	return nil
}
