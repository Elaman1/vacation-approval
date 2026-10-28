package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"vacation-approval/internal/core/vacation"
)

const TopicDecisions = "vacation.decisions"

// Publisher uses synchronous Kafka writes with acknowledgement from the broker.
type Publisher struct {
	writer *kafkago.Writer
}

func NewPublisher(broker string) *Publisher {
	return &Publisher{writer: &kafkago.Writer{
		Addr:         kafkago.TCP(broker),
		Topic:        TopicDecisions,
		RequiredAcks: kafkago.RequireAll,
		Async:        false,
		BatchTimeout: 100 * time.Millisecond,
	}}
}

func (p *Publisher) Publish(ctx context.Context, event vacation.DecisionEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	if err := p.writer.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(strconv.FormatInt(event.RequestID, 10)),
		Value: payload,
	}); err != nil {
		return fmt.Errorf("publish decision event %d: %w", event.EventID, err)
	}
	return nil
}

func (p *Publisher) Close() error { return p.writer.Close() }

func NewReader(broker string) *kafkago.Reader {
	return kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        []string{broker},
		Topic:          TopicDecisions,
		GroupID:        "vacation-clickhouse-v1",
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: 0, // synchronous explicit offset commit after ClickHouse insert
		StartOffset:    kafkago.FirstOffset,
	})
}
