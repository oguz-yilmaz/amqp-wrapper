package messaging

type EventHandler = func(event *BaseEvent) error

type Consumer interface {
	Subscribe(topic string, handler EventHandler)

	// Close stops the consumer and cleans up resources.
	Close() error
}
