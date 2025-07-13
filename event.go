package messaging

import (
	"encoding/json"
)

type Event interface {
	EventName() string
	EventVersion() int
	AggregateId() string
	Marshall([]byte, error)
	Unmarshall([]byte)
}

type BaseEvent struct {
	Name      string `json:"name"`
	Version   int    `json:"version"`
	Aggregate string `json:"aggregate_id"`
	Payload   any    `json:"payload,omitempty"`
}

func (e *BaseEvent) EventName() string {
	return e.Name
}

func (e *BaseEvent) EventVersion() int {
	return e.Version
}

func (e *BaseEvent) AggregateId() string {
	return e.Aggregate
}

func (e *BaseEvent) Marshall() ([]byte, error) {
	return json.Marshal(e)
}

func (e *BaseEvent) Unmarshall(data []byte) error {
	return json.Unmarshal(data, e)
}
