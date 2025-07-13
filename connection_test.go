package amqpwraper

import (
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

func TestNewConnectionConfigDefaults(t *testing.T) {
	conf := NewConnectionConfig()

	assert.Equal(t, "amqp://guest:guest@localhost:5672/", conf.AMQPUrl)
	assert.Equal(t, "default_connection", conf.ConnectionName)
	assert.Equal(t, "en_US", conf.Locale)
	assert.Equal(t, 10*time.Second, conf.Hearbeat)
}

func TestConnectionDial_NewConnection(t *testing.T) {
	conn := &Connection{
		AMQPUrl:        "amqp://guest:guest@localhost:5672/",
		ConnectionName: "test_connection",
		Heartbeat:      5 * time.Second,
		Locale:         "en_US",
	}

	amqpConn, err := conn.Dial()
	assert.NoError(t, err)
	assert.NotNil(t, amqpConn)
	assert.False(t, conn.IsClosed())
}

func TestConnectionDial_ReusesConnection(t *testing.T) {
	conn := &Connection{
		AMQPUrl:        "amqp://guest:guest@localhost:5672/",
		ConnectionName: "test_connection",
		Heartbeat:      5 * time.Second,
		Locale:         "en_US",
	}

	// First dial
	firstConn, err1 := conn.Dial()
	assert.NoError(t, err1)

	// Second dial (should reuse)
	secondConn, err2 := conn.Dial()
	assert.NoError(t, err2)

	assert.Equal(t, firstConn, secondConn)
	assert.False(t, conn.IsClosed())
}

func TestConnection_IsClosed_NilConn(t *testing.T) {
	conn := &Connection{}
	assert.True(t, conn.IsClosed())
}

func TestConnection_HasChannel(t *testing.T) {
	conn := &Connection{}
	assert.False(t, conn.HasChannel())

	conn.Channels = append(conn.Channels, &amqp.Channel{})
	assert.True(t, conn.HasChannel())
}
