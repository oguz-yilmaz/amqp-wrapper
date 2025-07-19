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
- Declare the exchange
- Declare the queue
- Bind them
- Return a ready-to-use `Client` for publish/consume

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
        Body:        []byte("New Order Created"),
    }, amqpwrapper.PublishOptions{
        Key: "order.created",
    })
    if err != nil {
        log.Fatalf("Publish failed: %v", err)
    }

    // Consume the message
    msgs, err := client.Consume(amqpwrapper.ConsumeOptions{
        AutoAck: true,
    })
    if err != nil {
        log.Fatalf("Consume failed: %v", err)
    }

    select {
    case msg := <-msgs:
        fmt.Printf("Received: %s\n", string(msg.Body))
    case <-time.After(2 * time.Second):
        log.Println("No message received")
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
go test ./...
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
