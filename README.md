# amqp-wrapper

A thin—but opinionated—wrapper around **RabbitMQ**’s
[`amqp091-go`](https://github.com/rabbitmq/amqp091-go) client.

Designed for simplicity, it lets you:

- open **named connections** (automatically reused),
- declare exchanges and queues idempotently,
- bind them via typed structs,
- publish and consume messages with a clean API.

> **Status:** early alpha – API may change.

---

## Table of Contents

1. [Installation](#installation)  
2. [Quick-Start](#quick-start)  
3. [Testing](#testing)  
4. [Contributing](#contributing)  
5. [License](#license)

---

## Installation

```bash
go get github.com/oguz-yilmaz/amqp-wrapper
```

Supports Go 1.24+ and RabbitMQ 3.12+.

---

## Quick-Start

This library is designed to be minimal, declarative, and high-level.  
Use `Broker.InitMessaging` to:

- Dial (or reuse) a connection
- Handle channel management
- Handle automatic reconnections
- Declare the exchange
- Declare the queue
- Bind them
- Return a ready-to-use `Client` for publish/consume

---

Make sure to run RabbitMQ before testing examples:

```bash
docker run -it --rm --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3.13-management
```

Note: If you want to test reconnection logic, you should use something like:
```bash
# first time
docker run -d --name rabbitmq \
           -p 5672:5672 -p 15672:15672 \
           -v rabbitmq_data:/var/lib/rabbitmq rabbitmq:3.13-management

# later, instead of Ctrl‑C:
docker stop rabbitmq          # graceful shutdown
docker start rabbitmq         # same container
```

### Example 1: Minimal setup with `direct` exchange

This example shows the simple setup: a `direct` exchange, a durable queue,
and a single routing key (`order.created`). It publishes and consumes a message
using the high-level `Client` API.

```go
package main

import (
    "fmt"
    "log"
    "time"

    amqpwrapper "github.com/oguz-yilmaz/amqp-wrapper"
    amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
    b := amqpwrapper.NewBroker(amqpwrapper.BrokerConfig{
        AMQPUrl:    "amqp://guest:guest@localhost:5672",
        MaxChannel: 10,
    })

    ex := amqpwrapper.Exchange{
        Name:       "orders.direct",
        Type:       amqpwrapper.DIRECT,
        Durable:    true,
        AutoDelete: false,
    }

    q := amqpwrapper.QueueConfig{
        Name:       "orders.created",
        Durable:    true,
        AutoDelete: false,
    }

    bindings := []amqpwrapper.QueueBinding{
        {BindingKey: "order.created"},
    }

    client, err := b.InitMessaging("orders-connection", amqpwrapper.QueueSetup{
        QueueConfig: q,
        Exchange:    ex,
        Bindings:    bindings,
    })
    if err != nil {
        log.Fatalf("InitMessaging failed: %v", err)
    }

    // Publish a message
    err = client.Publish(amqp.Publishing{
        ContentType: "text/plain",
        Body:        []byte("New Order Created 1"),
    }, amqpwrapper.PublishOptions{
        Key: "order.created",
    })

    // Publish another message
    err = client.Publish(amqp.Publishing{
        ContentType: "text/plain",
        Body:        []byte("New Order Created 2"),
    }, amqpwrapper.PublishOptions{
        Key: "order.created",
    })
    if err != nil {
        log.Fatalf("Publish failed: %v", err)
    }

    msgs, err := client.Consume(amqpwrapper.ConsumeOptions{
        AutoAck: true,
    })
    if err != nil {
        log.Fatalf("Consume failed: %v", err)
    }

    // Consume messages
    for {
        select {
        case msg := <-msgs:
            fmt.Printf("Received: %s\n", string(msg.Body))
        case <-time.After(2 * time.Second):
            return
        }
    }
}
```

When you run this, the output will look like:
```bash
$ go run cmd/main.go
Received: New Order Created 1
Received: New Order Created 2
```

---

### Example 2: Topic exchange with multiple routing keys

This example demonstrates a more realistic use case with a `topic` exchange. It
binds the queue to multiple routing keys (`user.created`, `user.updated`,
`user.*.email`) and listens for matching messages.

```go
package main

import (
    "fmt"
    "log"
    "time"

    amqpwrapper "github.com/oguz-yilmaz/amqp-wrapper"
    amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
    b := amqpwrapper.NewBroker(amqpwrapper.BrokerConfig{
        AMQPUrl:    "amqp://guest:guest@localhost:5672",
        MaxChannel: 10,
    })

    ex := amqpwrapper.Exchange{
        Name:    "users.topic",
        Type:    amqpwrapper.TOPIC,
        Durable: true,
    }

    q := amqpwrapper.QueueConfig{
        Name:    "user.events",
        Durable: true,
    }

    bindings := []amqpwrapper.QueueBinding{
        {BindingKey: "user.created"},
        {BindingKey: "user.updated"},
        {BindingKey: "user.*.email"},
    }

    client, err := b.InitMessaging("user-connection", amqpwrapper.QueueSetup{
        QueueConfig: q,
        Exchange:    ex,
        Bindings:    bindings,
    })
    if err != nil {
        log.Fatalf("InitMessaging failed: %v", err)
    }

    // Simulate publishing three different types of events
    events := []struct {
        Key string
        Msg string
    }{
        {"user.created", "User was created"},
        {"user.updated", "User profile updated"},
        {"user.admin.email", "Admin email updated"},
    }

    for _, e := range events {
        err := client.Publish(amqp.Publishing{
            ContentType: "text/plain",
            Body:        []byte(e.Msg),
        }, amqpwrapper.PublishOptions{Key: e.Key})
        if err != nil {
            log.Fatalf("Publish failed for key %s: %v", e.Key, err)
        }
    }

    msgs, err := client.Consume(amqpwrapper.ConsumeOptions{
        AutoAck: true,
    })
    if err != nil {
        log.Fatalf("Consume failed: %v", err)
    }

    timeout := time.After(2 * time.Second)
    received := 0
    for received < 3 {
        select {
        case msg := <-msgs:
            fmt.Printf("Received: %s\n", string(msg.Body))
            received++
        case <-timeout:
            log.Println("Timeout waiting for all messages")
            return
        }
    }
}
```

---

## Testing

Start a local RabbitMQ instance:

```bash
docker run -it --rm --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3.13-management
```

Then run the tests:

```bash
go test .
```

---

## Contributing

Pull requests are welcome.  
Please:

- Use a non-main branch
- Include a test or example
- Keep commits focused and descriptive

---

## License

MIT
