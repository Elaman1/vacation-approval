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

const (
	TopicDecisions   = "vacation.decisions"
	TopicDeadLetters = "vacation.decisions.dlq"
)

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

func NewReader(broker, groupID string) *kafkago.Reader {
	return kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        []string{broker},
		Topic:          TopicDecisions,
		GroupID:        groupID,
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: 0, // synchronous explicit offset commit after ClickHouse insert
		StartOffset:    kafkago.FirstOffset,
	})
}

// DeadLetterPublisher stores malformed/invalid messages before their source offsets are committed.
// The original bytes are preserved for investigation and replay.
type DeadLetterPublisher struct{ writer *kafkago.Writer }

func NewDeadLetterPublisher(broker string) *DeadLetterPublisher {
	return &DeadLetterPublisher{writer: &kafkago.Writer{Addr: kafkago.TCP(broker), Topic: TopicDeadLetters, RequiredAcks: kafkago.RequireAll, Async: false}}
}
func (p *DeadLetterPublisher) Publish(ctx context.Context, msg kafkago.Message, reason string) error {
	envelope := struct {
		SourceTopic     string `json:"source_topic"`
		SourcePartition int    `json:"source_partition"`
		SourceOffset    int64  `json:"source_offset"`
		Reason          string `json:"reason"`
		Payload         []byte `json:"payload"`
	}{msg.Topic, msg.Partition, msg.Offset, reason, msg.Value}
	b, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafkago.Message{Key: []byte(fmt.Sprintf("%s:%d:%d", msg.Topic, msg.Partition, msg.Offset)), Value: b})
}
func (p *DeadLetterPublisher) Close() error { return p.writer.Close() }
