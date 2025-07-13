# amqp-wrapper

A thin—but opinionated—wrapper around **RabbitMQ**’s
[`amqp091-go`](https://github.com/rabbitmq/amqp091-go) client that balances
ergonomic convenience with fine-grained control.  
It lets you

* open **named connections** (reused automatically),
* declare exchanges & queues idempotently,
* bind them with clear, strongly-typed structs,
* publish / consume events via a fluent API.

> **Status:** early alpha – API may change.

---

## Table of Contents

1. [Features](#features)  
2. [Installation](#installation)  
3. [Quick-Start (all-in-one)](#quick-start-all-in-one) – `Broker.InitMessaging`  
4. [Granular Usage](#granular-usage) – dial, declare, bind, publish  
5. [Concepts and Architecture](#concepts-and-architecture)  
6. [Testing](#testing)  
7. [Contributing](#contributing)  
8. [License](#license)

---

## Features

| Category      | Details                                                                                   |
| ------------- | ---------------------------------------------------------------------------------------   |
| Connection    | Reuses sockets by **connection name**, configurable heartbeat & locale                    |
| Channels      | Keeps a slice of live channels per connection and auto-picks one that isn’t closed        |
| Topology      | Declarative structs for exchanges, queues, and bindings; idempotent declarations          |
| Event Model   | Pluggable `Event`, `Producer`, and `Consumer` interfaces with JSON helpers in `BaseEvent` |
| Safety        | Thread-safe                                                                               |
| Extensible    | TODO: TLS, auth, quorum queues, publisher confirms, retry back-off, tracing               |

---

## Installation

```bash
go get github.com/oguz-yilmaz/amqp-wrapper
```

The wrapper targets Go 1.24+ and RabbitMQ 3.12+ (any server that supports the AMQP 0-9-1 spec).

## Quick-Start (all-in-one)

`InitMessaging` is the one-liner that:

- Dials (or reuses) a connection
- Declares the exchange
- Declares the queue
- Binds them with a routing key
- Returns a ready channel and queue handle

```go
package main

import (
    "fmt"
    "log"

    amqpwrapper "github.com/oguz-yilmaz/amqp-wrapper"
    amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
    b := amqpwrapper.NewBroker()

    // --- 1) Define exchange, queue, and binding ----------
    ex := amqpwrapper.Exchange{
        Name:       "orders.direct",
        Type:       amqpwrapper.DIRECT,
        Durable:    true,
        AutoDelete: false,
    }

    qc := amqpwrapper.QueueConfig{
        Name:       "orders.created",
        Durable:    true,
        AutoDelete: false,
    }

    qb := amqpwrapper.QueueBinding{
        BindingKey: "order.created",
    }

    // --- 2) Bring it all together ------------------------
    ch, q, err := b.InitMessaging(
        "amqp://guest:guest@localhost:5672/",
        "orders-service", // connection name (Optional, you can leave it "")
        qc,
        qb,
        ex,
    )
    if err != nil {
        log.Fatalf("setup failed: %v", err)
    }
    defer ch.Close()

    fmt.Printf("Queue %s is ready on channel %d\n", q.Name, ch.ChannelID)
}
```

Run RabbitMQ locally 
```bash
$ docker run -it --rm --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3.13-management
```
and execute the program — both the exchange and queue appear automatically.

## Granular Usage

Need more control? Use the individual helpers directly.

### 1. Dial (or reuse) a connection

```go
connName := "billing-svc"
if err := b.Dial("amqp://user:pass@rabbit.local:5672/", connName); err != nil {
    log.Fatal(err)
}
```
`Broker.Dial` is idempotent per `ConnectionName`—a second call returns the
existing ``*amqp.Connection`.

### 2. Declare an exchange

```go
chn, err := b.ExchangeDeclare(amqpwrapper.Exchange{
    Name:    "billing.topic",
    Type:    amqpwrapper.TOPIC,
    Durable: true,
})
```
The returned `*amqp.Channel` is stored inside Broker for later reuse.

### 3. Declare a queue & bind

```go
qconf := amqpwrapper.QueueConfig{
    Name: "invoice.paid",
}

qbind := amqpwrapper.QueueBinding{
    BindingKey: "invoice.*.paid",
}

q, err := b.QueueDeclare(qconf, qbind, "billing.topic")
```

### 4. Publish an event

```go
// publish
```

### 5. Consume events

```go
// Consume
```

## Concepts and Architecture

```txt
Connection (1) --> (N) Channel (1) --> (N) Exchange (N) 
                                --via bindings--> (N) Queue (1) --> (N) Consumers
```

- Connection ⇒ long-lived socket; heavy to open; few per app.
- Channel    ⇒ cheap, throwaway, gives you concurrency; open many.
- Exchange   ⇒ broker object; declared once; later referenced by name from any channel.

## Testing

You can run the tests against a local RabbitMQ instance.

```bash
./dpl.sh test
```

## Contributing

Pull requests are very much welcomed. Create your pull request on a non-master
branch, make sure a test or example is included that covers your change, and
your commits represent coherent changes that include a reason for the change.

### License

MIT
