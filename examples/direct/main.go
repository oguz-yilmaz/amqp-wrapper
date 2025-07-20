package main

import (
	"context"
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

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second*10))
	defer cancel()

	counter := 1
loop:
	for {
		select {
		case <-time.After(2 * time.Second):
			msg := fmt.Sprintf("New Order Created %d", counter)
			fmt.Printf("Sending message: %s \n", msg)
			err = client.Publish(amqp.Publishing{
				ContentType: "text/plain",
				Body:        []byte(msg),
			}, amqpwrapper.PublishOptions{
				Key: "order.created",
			})
			if err != nil {
				log.Fatalf("Publish failed: %v", err)
			}

			counter++
		case <-ctx.Done():
			fmt.Println("Sending done!")
			break loop
		}
	}

	msgs, err := client.Consume(amqpwrapper.ConsumeOptions{
		AutoAck: true,
	})
	if err != nil {
		log.Fatalf("Consume failed: %v", err)
	}

	for {
		select {
		case msg := <-msgs:
			fmt.Printf("Received: %s\n", string(msg.Body))
		case <-time.After(3 * time.Second):
			fmt.Println("Receiving done!")
			return
		}
	}
}
