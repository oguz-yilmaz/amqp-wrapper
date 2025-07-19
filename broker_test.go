package amqpwraper

import (
	"testing"
	"time"

	// amqp "github.com/rabbitmq/amqp091-go"
	// "github.com/stretchr/testify/assert"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: Run `docker run -it --rm --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3.13-management`
// before running these tests to start a RabbitMQ instance.

func TestBroker_MultipleBindings(t *testing.T) {
	b := NewBroker(BrokerConfig{
		AMQPUrl:    "amqp://guest:guest@localhost:5672",
		MaxChannel: 10,
	})

	ex := Exchange{
		Name:    "ex.multi.direct",
		Type:    DIRECT,
		Durable: true,
	}
	q := QueueConfig{
		Name:    "q.multi",
		Durable: true,
	}
	bindings := []QueueBinding{
		{BindingKey: "error"},
		{BindingKey: "info"},
		{BindingKey: "warning"},
	}

	client, err := b.InitMessaging("my_connection", QueueSetup{
		QueueConfig: q,
		Exchange:    ex,
		Bindings:    bindings,
	})
	require.NoError(t, err)
	// Ensure the client is ready
	require.NotNil(t, client)
	// Ensure the exchange and queue are created

	// Publish messages with all routing keys
	publish := func(key, msg string) {
		err := client.Publish(amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(msg),
		}, PublishOptions{
			Immediate: false,
			Key:       key,
		})
		require.NoError(t, err)
	}

	publish("info", "Info Message")
	publish("error", "Error Message")
	publish("warning", "Warning Message")

	// Read 3 messages
	received := make([]string, 0, 3)
	msgs, err := client.Consume(ConsumeOptions{
		AutoAck: true,
	})
	require.NoError(t, err)

	timeout := time.After(2 * time.Second)
	for len(received) < 3 {
		select {
		case msg := <-msgs:
			received = append(received, string(msg.Body))
		case <-timeout:
			t.Fatal("timeout waiting for all messages")
		}
	}

	assert.Contains(t, received, "Info Message")
	assert.Contains(t, received, "Error Message")
	assert.Contains(t, received, "Warning Message")
}

func TestBroker_InitMessaging_e2e(t *testing.T) {
	b := NewBroker(BrokerConfig{
		AMQPUrl:    "amqp://guest:guest@localhost:5672",
		MaxChannel: 10,
	})

	// DIRECT Exchange setup
	exDirect := Exchange{Name: "ex.direct", Type: DIRECT, Durable: true}
	qDirect := QueueConfig{Name: "q.direct", Durable: true}
	bindDirect := QueueBinding{BindingKey: "info.direct"}

	// TOPIC Exchange setup
	exTopic := Exchange{Name: "ex.topic", Type: TOPIC, Durable: true}
	qTopic := QueueConfig{Name: "q.topic", Durable: true}
	bindTopic := QueueBinding{BindingKey: "user.*.created"}

	// FANOUT Exchange setup (binding key is ignored)
	exFanout := Exchange{Name: "ex.fanout", Type: FANOUT, Durable: true}
	qFanout1 := QueueConfig{Name: "q.fanout.1", Durable: true}
	qFanout2 := QueueConfig{Name: "q.fanout.2", Durable: true}
	bindFanout := QueueBinding{BindingKey: ""}

	// HEADERS Exchange setup
	exHeaders := Exchange{Name: "ex.headers", Type: HEADERS, Durable: true}
	qHeaders := QueueConfig{Name: "q.headers", Durable: true}
	bindHeaders := QueueBinding{
		Args: amqp.Table{
			"x-match": "all",
			"type":    "report",
			"format":  "pdf",
		},
	}

	// Initialize all exchanges and queues
	clientDirect, err := b.InitMessaging("conn.direct", QueueSetup{
		QueueConfig: qDirect,
		Exchange:    exDirect,
		Bindings:    []QueueBinding{bindDirect},
	})
	require.NoError(t, err)

	clientTopic, err := b.InitMessaging("conn.topic", QueueSetup{
		QueueConfig: qTopic,
		Exchange:    exTopic,
		Bindings:    []QueueBinding{bindTopic},
	})
	require.NoError(t, err)

	clientFanout1, err := b.InitMessaging("conn.fanout1", QueueSetup{
		QueueConfig: qFanout1,
		Exchange:    exFanout,
		Bindings:    []QueueBinding{bindFanout},
	})
	require.NoError(t, err)

	clientFanout2, err := b.InitMessaging("conn.fanout2", QueueSetup{
		QueueConfig: qFanout2,
		Exchange:    exFanout,
		Bindings:    []QueueBinding{bindFanout},
	})
	require.NoError(t, err)

	clientHeaders, err := b.InitMessaging("conn.headers", QueueSetup{
		QueueConfig: qHeaders,
		Exchange:    exHeaders,
		Bindings:    []QueueBinding{bindHeaders},
	})
	require.NoError(t, err)

	// Publish messages
	err = clientDirect.Publish(amqp.Publishing{
		ContentType: "text/plain",
		Body:        []byte("Hello Direct"),
	}, PublishOptions{
		Key: "info.direct",
	})
	require.NoError(t, err)

	err = clientTopic.Publish(amqp.Publishing{
		ContentType: "text/plain",
		Body:        []byte("Hello Topic"),
	}, PublishOptions{
		Key: "user.admin.created",
	})
	require.NoError(t, err)

	err = clientFanout1.Publish(amqp.Publishing{
		ContentType: "text/plain",
		Body:        []byte("Hello Fanout"),
	}, PublishOptions{})
	require.NoError(t, err)

	err = clientHeaders.Publish(amqp.Publishing{
		ContentType: "text/plain",
		Body:        []byte("Hello Headers"),
		Headers: amqp.Table{
			"type":   "report",
			"format": "pdf",
		},
	}, PublishOptions{})
	require.NoError(t, err)

	// Consume messages (with timeout)
	readMsg := func(c Client) string {
		msgs, err := c.Consume(ConsumeOptions{
			AutoAck: true,
		})
		require.NoError(t, err)

		select {
		case msg := <-msgs:
			return string(msg.Body)
		case <-time.After(2 * time.Second):
			return ""
		}
	}

	assert.Equal(t, "Hello Direct", readMsg(*clientDirect))
	assert.Equal(t, "Hello Topic", readMsg(*clientTopic))
	assert.Equal(t, "Hello Fanout", readMsg(*clientFanout1))
	assert.Equal(t, "Hello Fanout", readMsg(*clientFanout2))
	assert.Equal(t, "Hello Headers", readMsg(*clientHeaders))
}

// func TestBroker_InitMessaging_e2e(t *testing.T) {
// 	b := NewBroker("amqp://guest:guest@localhost:5672")

// 	// DIRECT Exchange setup
// 	exDirect := Exchange{Name: "ex.direct", Type: DIRECT, Durable: true}
// 	qDirect := QueueConfig{Name: "q.direct", Durable: true}
// 	bindDirect := QueueBinding{BindingKey: "info.direct"}

// 	// TOPIC Exchange setup
// 	exTopic := Exchange{Name: "ex.topic", Type: TOPIC, Durable: true}
// 	qTopic := QueueConfig{Name: "q.topic", Durable: true}
// 	bindTopic := QueueBinding{BindingKey: "user.*.created"}

// 	// FANOUT Exchange setup (binding key is ignored)
// 	exFanout := Exchange{Name: "ex.fanout", Type: FANOUT, Durable: true}
// 	qFanout1 := QueueConfig{Name: "q.fanout.1", Durable: true}
// 	qFanout2 := QueueConfig{Name: "q.fanout.2", Durable: true}
// 	bindFanout := QueueBinding{BindingKey: ""} // ignored

// 	// HEADERS Exchange setup
// 	exHeaders := Exchange{Name: "ex.headers", Type: HEADERS, Durable: true}
// 	qHeaders := QueueConfig{Name: "q.headers", Durable: true}
// 	bindHeaders := QueueBinding{
// 		Args: amqp.Table{
// 			"x-match": "all",
// 			"type":    "report",
// 			"format":  "pdf",
// 		},
// 	}

// 	// Initialize all exchanges and queues
// 	clientDirect, err := b.InitMessaging("conn", QueueSetup{
// 		Config:   qDirect,
// 		Exchange: exDirect,
// 		Bindings: []QueueBinding{bindDirect},
// 	})
// 	require.NoError(t, err)

// 	clientTopic, err := b.InitMessaging("conn", QueueSetup{
// 		Config:   qTopic,
// 		Exchange: exTopic,
// 		Bindings: []QueueBinding{bindTopic},
// 	})
// 	require.NoError(t, err)

// 	clientFanout1, err := b.InitMessaging("conn", QueueSetup{
// 		Config:   qFanout1,
// 		Exchange: exFanout,
// 		Bindings: []QueueBinding{bindFanout},
// 	})
// 	require.NoError(t, err)

// 	clientFanout2, err := b.InitMessaging("conn", QueueSetup{
// 		Config:   qFanout2,
// 		Exchange: exFanout,
// 		Bindings: []QueueBinding{bindFanout},
// 	})
// 	require.NoError(t, err)

// 	clientHeaders, err := b.InitMessaging("conn", QueueSetup{
// 		Config:   qHeaders,
// 		Exchange: exHeaders,
// 		Bindings: []QueueBinding{bindHeaders},
// 	})
// 	require.NoError(t, err)

// 	// Publish messages
// 	_ = clientDirect.Publish("info.direct", amqp.Publishing{
// 		ContentType: "text/plain",
// 		Body:        []byte("Hello Direct"),
// 	})

// 	_ = clientTopic.Publish("user.admin.created", amqp.Publishing{
// 		ContentType: "text/plain",
// 		Body:        []byte("Hello Topic"),
// 	})

// 	_ = clientFanout1.Publish("", amqp.Publishing{
// 		ContentType: "text/plain",
// 		Body:        []byte("Hello Fanout"),
// 	})

// 	_ = clientHeaders.Publish("", amqp.Publishing{
// 		ContentType: "text/plain",
// 		Body:        []byte("Hello Headers"),
// 		Headers: amqp.Table{
// 			"type":   "report",
// 			"format": "pdf",
// 		},
// 	})

// 	// Consume messages (use a short timeout)
// 	readMsg := func(c Client, queueName string) string {
// 		msgs, err := c.Consume(ConsumeOptions{
// 			Queue:   queueName,
// 			AutoAck: true,
// 			NoWait:  true,
// 		})
// 		require.NoError(t, err)

// 		select {
// 		case msg := <-msgs:
// 			return string(msg.Body)
// 		case <-time.After(1 * time.Second):
// 			return ""
// 		}
// 	}

// 	assert.Equal(t, "Hello Direct", readMsg(clientDirect, "q.direct"))
// 	assert.Equal(t, "Hello Topic", readMsg(clientTopic, "q.topic"))
// 	assert.Equal(t, "Hello Fanout", readMsg(clientFanout1, "q.fanout.1"))
// 	assert.Equal(t, "Hello Fanout", readMsg(clientFanout2, "q.fanout.2"))
// 	assert.Equal(t, "Hello Headers", readMsg(clientHeaders, "q.headers"))
// }
