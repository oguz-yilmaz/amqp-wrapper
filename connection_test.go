package amqpwraper

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewConnectionConfig_Defaults(t *testing.T) {
	name := "test-conn"
	conf := NewConnectionConfig(name)

	assert.Equal(t, name, conf.ConnectionName)
	assert.Equal(t, "en_US", conf.Locale)
	assert.Equal(t, 10*time.Second, conf.Heartbeat)
}

func TestNewConnection_Creation(t *testing.T) {
	conf := &ConnectionConfig{
		ConnectionName: "conn1",
		Locale:         "en_US",
		Heartbeat:      5 * time.Second,
	}

	conn := NewConnection(conf, nil)

	assert.Equal(t, conf.ConnectionName, conn.ConnectionName)
	assert.Equal(t, conf.Locale, conn.Locale)
	assert.Equal(t, conf.Heartbeat, conn.Heartbeat)
	assert.Nil(t, conn.AMQPConn)
}
