package messaging

import (
	"testing"

	_ "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

// NOTE: Run `docker run -it --rm --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3.13-management`
// before running these tests to start a RabbitMQ instance.

func TestNewBroker(t *testing.T) {
	b := NewBroker()
	assert.NotNil(t, b)
	assert.Nil(t, b.Conn)
	assert.Empty(t, b.Connections)
}

func TestBroker_NewConnection(t *testing.T) {
	b := NewBroker()
	conf := NewConnectionConfig()
	conf.AMQPUrl = "amqp://guest:guest@localhost:5672"
	conf.ConnectionName = "test_conn"

	conn, err := b.NewConnection(conf)
	assert.NoError(t, err)
	assert.Equal(t, conf.AMQPUrl, conn.AMQPUrl)
	assert.Equal(t, "test_conn", conn.ConnectionName)
	assert.Len(t, b.Connections, 1)
}

func TestBroker_Dial(t *testing.T) {
	b := NewBroker()
	err := b.Dial("amqp://guest:guest@localhost:5672", "dial_test")
	assert.NoError(t, err)
	assert.NotNil(t, b.Conn)
	assert.False(t, b.Conn.IsClosed())
}

func TestBroker_ExchangeDeclare(t *testing.T) {
	b := NewBroker()
	err := b.Dial("amqp://guest:guest@localhost:5672", "exchange_test")
	assert.NoError(t, err)

	ex := Exchange{
		Name:    "test_exchange",
		Type:    DIRECT,
		Durable: true,
	}

	ch, err := b.ExchangeDeclare(ex)
	assert.NoError(t, err)
	assert.NotNil(t, ch)

	stored := b.GetExchange("test_exchange")
	assert.NotNil(t, stored)
	assert.Equal(t, "test_exchange", stored.Name)
}

func TestBroker_QueueDeclare_WithBinding(t *testing.T) {
	b := NewBroker()
	err := b.Dial("amqp://guest:guest@localhost:5672", "queue_test")
	assert.NoError(t, err)

	ex := Exchange{
		Name:    "queue_test_exchange",
		Type:    DIRECT,
		Durable: true,
	}

	_, err = b.ExchangeDeclare(ex)
	assert.NoError(t, err)

	q := QueueConfig{
		Name:    "queue_test_queue",
		Durable: true,
	}
	qb := QueueBinding{
		BindingKey: "test.key",
	}

	queue, err := b.QueueDeclare(q, qb, ex.Name)
	assert.NoError(t, err)
	assert.NotNil(t, queue)
	assert.Equal(t, "queue_test_queue", queue.Name)
	assert.Len(t, queue.Bindings, 1)

	exFromBroker := b.GetExchange(ex.Name)
	assert.Len(t, exFromBroker.Bindings, 1)
}

func TestBroker_InitMessaging(t *testing.T) {
	b := NewBroker()

	ex := Exchange{
		Name:    "init_exchange",
		Type:    DIRECT,
		Durable: true,
	}
	q := QueueConfig{
		Name:    "init_queue",
		Durable: true,
	}
	qb := QueueBinding{
		BindingKey: "init.key",
	}

	ch, cq, err := b.InitMessaging("amqp://guest:guest@localhost:5672", "init_test", q, qb, ex)
	assert.NoError(t, err)
	assert.NotNil(t, ch)
	assert.NotNil(t, cq)
	assert.Equal(t, "init_queue", cq.Name)
}

func TestBroker_QueueDeclare_PassiveQueueExists(t *testing.T) {
	b := NewBroker()
	err := b.Dial("amqp://guest:guest@localhost:5672", "passive_exists_test")
	assert.NoError(t, err)

	ex := Exchange{
		Name:    "passive_exists_exchange",
		Type:    DIRECT,
		Durable: true,
	}
	_, err = b.ExchangeDeclare(ex)
	assert.NoError(t, err)

	// First declare queue actively
	q := QueueConfig{
		Name:    "passive_queue",
		Durable: true,
	}
	qb := QueueBinding{
		BindingKey: "passive.key",
	}
	_, err = b.QueueDeclare(q, qb, ex.Name)
	assert.NoError(t, err)

	// Now declare same queue passively
	q.Passive = true
	_, err = b.QueueDeclare(q, qb, ex.Name)
	assert.NoError(t, err)
}

func TestBroker_QueueDeclare_PassiveQueueMissing(t *testing.T) {
	b := NewBroker()
	err := b.Dial("amqp://guest:guest@localhost:5672", "passive_missing_test")
	assert.NoError(t, err)

	ex := Exchange{
		Name:    "passive_missing_exchange",
		Type:    DIRECT,
		Durable: true,
	}
	_, err = b.ExchangeDeclare(ex)
	assert.NoError(t, err)

	// Declare passive queue that doesn't exist
	q := QueueConfig{
		Name:    "missing_passive_queue",
		Durable: true,
		Passive: true,
	}
	qb := QueueBinding{
		BindingKey: "missing.key",
	}
	_, err = b.QueueDeclare(q, qb, ex.Name)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NOT_FOUND")
}
