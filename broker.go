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

const (
	amqpURL     = "amqp://guest:guest@localhost:5672/"
	exchange    = "my.direct"
	exchangeTyp = "direct"
	queue       = "my.queue"
	routingKey  = "my.key"
)

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

type Broker struct {
	Connections []*Connection
	Conn        *Connection // represents the current connection
	Exchanges   []*Exchange
	Queues      map[string]*Queue
	m           sync.Mutex
}

func NewBroker() *Broker {
	return &Broker{
		Connections: []*Connection{},
		Conn:        nil,
	}
}

func (b *Broker) GetConnection(name string) *Connection {
	for _, c := range b.Connections {
		if name == c.ConnectionName {
			return c
		}
	}

	return nil
}

func (b *Broker) HasConnection(url string) bool {
	for _, v := range b.Connections {
		if url == v.AMQPUrl {
			return true
		}
	}

	return false
}

func (b *Broker) GetActiveChannel() *amqp.Channel {
	for _, c := range b.Conn.Channels {
		if !c.IsClosed() {
			return c
		}
	}

	return nil
}

func (b *Broker) NewConnection(conf *ConnectionConfig) (*Connection, error) {
	if conf.AMQPUrl == "" {
		return nil, errors.New("AMQP url must be provided")
	}

	b.m.Lock()
	defer b.m.Unlock()

	if b.HasConnection(conf.AMQPUrl) {
		return nil, errors.New("connection with that url already exists")
	}

	c := &Connection{
		AMQPUrl:        conf.AMQPUrl,
		ConnectionName: conf.ConnectionName,
		Heartbeat:      conf.Hearbeat,
		Locale:         conf.Locale,
		Channels:       []*amqp.Channel{},
	}

	b.Connections = append(b.Connections, c)

	return c, nil
}

func (b *Broker) GetExchange(name string) *Exchange {
	for _, ex := range b.Exchanges {
		if ex.Name == name {
			return ex
		}
	}

	return nil
}

//   - A single queue can be bound to multiple exchanges
//   - An exchange can be bound to multiple queues
//
// # Creates new connection
//
// Reuses if the same connection name passed
func (b *Broker) Dial(
	amqpUrl string,
	connectionName string,
) error {
	if amqpUrl == "" {
		return errors.New("AMQP URL must be provided")
	}

	conf := NewConnectionConfig()
	conf.AMQPUrl = amqpUrl

	if connectionName != "" {
		conf.ConnectionName = connectionName
	}

	var conn *Connection
	cExist := b.GetConnection(conf.ConnectionName)
	if cExist != nil {
		conn = cExist
	} else {
		cNew, err := b.NewConnection(conf)
		if err != nil {
			return fmt.Errorf("new connection error: %w", err)
		}

		conn = cNew
	}

	amqpConn, err := conn.Dial() // reuses existing connection if available
	if err != nil {
		return fmt.Errorf("dial error: %w", err)
	}

	b.m.Lock()
	defer b.m.Unlock()

	conn.AMQPConn = amqpConn
	b.Conn = conn // Set current connection for the Broker

	return nil
}

func (b *Broker) ExchangeDeclare(ex Exchange) (*amqp.Channel, error) {
	if b.Conn.AMQPConn == nil {
		return nil, errors.New("connection is not established")
	}

	ch, err := b.Conn.AMQPConn.Channel()
	if err != nil {
		return nil, fmt.Errorf("channel error: %w", err)
	}

	err = ch.ExchangeDeclare(
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
	defer b.m.Unlock()

	b.Conn.Channels = append(b.Conn.Channels, ch)
	b.Exchanges = append(b.Exchanges, &ex)

	return ch, nil
}

func (b *Broker) GetQueue(name string) *Queue {
	if b.Queues == nil {
		return nil
	}

	queue, exists := b.Queues[name]
	if !exists {
		return nil
	}

	return queue
}

func (b *Broker) QueueDeclare(qconf QueueConfig, qb QueueBinding, exName string) (*Queue, error) {
	b.m.Lock()
	defer b.m.Unlock()

	q := NewQueue(qconf)

	// Get Connection
	conn := b.Conn
	if conn == nil || conn.IsClosed() {
		return nil, fmt.Errorf("no active connection found")
	}

	// Get or create a Channel
	var amqpChannel *amqp.Channel
	existingChannel := b.GetActiveChannel()
	if existingChannel == nil {
		newChannel, err := b.Conn.AMQPConn.Channel()
		if err != nil {
			return nil, fmt.Errorf("channel error: %w", err)
		}

		amqpChannel = newChannel
		conn.Channels = append(conn.Channels, newChannel)
	} else {
		amqpChannel = existingChannel
	}

	// Create a new Queue instance
	var amqpQueue amqp.Queue
	var err error
	if !q.Passive {
		amqpQueue, err = amqpChannel.QueueDeclare(
			q.Name,
			q.Durable,
			q.AutoDelete,
			q.Exclusive,
			q.NoWait,
			q.Args,
		)
	} else {
		amqpQueue, err = amqpChannel.QueueDeclarePassive(
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

	// Bind the queue to the exchange
	err = amqpChannel.QueueBind(q.Name, qb.BindingKey, exName, qb.NoWait, qb.Args)
	if err != nil {
		return nil, fmt.Errorf("queue bind error: %w", err)
	}

	// We will check if the queue already exists in the broker
	existingQueue := b.GetQueue(q.Name)
	if existingQueue == nil {
		if b.Queues == nil {
			b.Queues = make(map[string]*Queue)
		}

		b.Queues[q.Name] = q
		existingQueue = q
	} else {
		UpdatePropertiesQueue(existingQueue, q)
	}

	existingQueue.AMQPQueue = &amqpQueue

	exchange := b.GetExchange(exName)
	if exchange == nil {
		return nil, fmt.Errorf("exchange %s not found", exName)
	}

	qb.Queue = existingQueue
	qb.Exchange = exchange
	existingQueue.Bindings = append(existingQueue.Bindings, &qb)
	exchange.Bindings = append(exchange.Bindings, &qb)

	return existingQueue, nil
}

func (b *Broker) InitMessaging(
	brokerURL string,
	connectionName string,
	q QueueConfig,
	qb QueueBinding,
	ex Exchange,
) (*amqp.Channel, *Queue, error) {
	if ex.Name == "" {
		return nil, nil, errors.New("exchange name must be provided")
	}

	if q.Name == "" {
		return nil, nil, errors.New("queue name must be provided")
	}

	err := b.Dial(brokerURL, connectionName)
	if err != nil {
		return nil, nil, fmt.Errorf("dial error: %w", err)
	}

	ch, err := b.ExchangeDeclare(ex)
	if err != nil {
		return nil, nil, fmt.Errorf("exchange declare error: %w", err)
	}

	rq, err := b.QueueDeclare(q, qb, ex.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("queue declare error: %w", err)
	}

	qb.Exchange = &ex

	return ch, rq, nil
}

func UpdatePropertiesQueue(
	q *Queue,
	q2 *Queue,
) {
	if q2.Name != "" {
		q.Name = q2.Name
	}
	if q2.Passive {
		q.Passive = q2.Passive
	}
	if q2.Durable {
		q.Durable = q2.Durable
	}
	if q2.AutoDelete {
		q.AutoDelete = q2.AutoDelete
	}
	if q2.Exclusive {
		q.Exclusive = q2.Exclusive
	}
	if q2.NoWait {
		q.NoWait = q2.NoWait
	}
	if q2.Args != nil {
		if q.Args == nil {
			q.Args = make(amqp.Table)
		}
		for k, v := range q2.Args {
			q.Args[k] = v
		}
	}
	if q2.AMQPQueue != nil {
		q.AMQPQueue = q2.AMQPQueue
	}
}
