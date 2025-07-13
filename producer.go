package amqpwraper

type Producer interface {
	Publish(event Event) error

	// Close releases any resources held by the producer (e.g., connections).
	Close() error
}
