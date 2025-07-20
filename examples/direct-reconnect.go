package main

import (
	"context"
	"fmt"
	"log"
	"time"

	amqpwrapper "github.com/oguz-yilmaz/amqp-wrapper"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Notes on this example:
// When testing consumer reconnection, we set AutoAck to false and Qos to 1.
// And also we need to start and stop the rabbbitmq with `start` and `stop` commands
// to simulate the connection loss and reconnection. Because if we just
// CTRL+C the running rabbitmq continaer, and then try to start it again,
// with the same command, it will not work because the published messages
// won't be in the same database for the new rabbitmq container.

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

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second*25))
	defer cancel()

	counter := 1
loop:
	for {
		select {
		case <-time.After(2 * time.Second):
			msg := fmt.Sprintf("New Order Created %d", counter)
			err = client.Publish(amqp.Publishing{
				DeliveryMode: amqp.Persistent,
				ContentType:  "text/plain",
				Body:         []byte(msg),
			}, amqpwrapper.PublishOptions{
				Key: "order.created",
			})
			if err != nil {
				log.Println("Publish failed: %v", err)
			} else {
				fmt.Printf("Published: %s\n", msg)
			}

			counter++
		case <-ctx.Done():
			fmt.Println("Sending done!")
			break loop
		}
	}

	msgs, err := client.Consume(amqpwrapper.ConsumeOptions{
		AutoAck: false, // IMPORTANT for testing consumer reconnection
		Qos:     1,     // Set prefetch count to 1 to limit the number of unacknowledged messages
	})
	if err != nil {
		log.Fatalf("Consume failed: %v", err)
	}

	timer := time.NewTimer(30 * time.Second) // one timer outside the loop
	defer timer.Stop()

	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				fmt.Println("consumer channel closed, waiting for reconnect...")
				continue
			}

			fmt.Printf("Received: %s\n", msg.Body)
			time.Sleep(3 * time.Second) // simulate work
			if err := msg.Ack(false); err != nil {
				log.Printf("ack failed: %v", err)
			}

			// reset the inactivity timer AFTER every successful message
			if !timer.Stop() {
				<-timer.C // drain if already fired
			}
			timer.Reset(30 * time.Second)

		case <-timer.C:
			fmt.Println("Receiving done (idle 30 s).")
			return
		}
	}

}
