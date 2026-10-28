package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vacation-approval/internal/core/vacation"
	"vacation-approval/internal/integration/clickhouse"
	"vacation-approval/internal/integration/kafka"
)

func main() {
	broker := os.Getenv("KAFKA_BROKER")
	clickhouseURL := os.Getenv("CLICKHOUSE_URL")
	clickhouseUser := os.Getenv("CLICKHOUSE_USER")
	clickhousePassword := os.Getenv("CLICKHOUSE_PASSWORD")
	if broker == "" || clickhouseURL == "" || clickhouseUser == "" {
		log.Fatal("KAFKA_BROKER, CLICKHOUSE_URL and CLICKHOUSE_USER are required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	groupID := os.Getenv("KAFKA_CONSUMER_GROUP")
	if groupID == "" {
		log.Fatal("KAFKA_CONSUMER_GROUP is required")
	}
	reader := kafka.NewReader(broker, groupID)
	defer reader.Close()
	dlq := kafka.NewDeadLetterPublisher(broker)
	defer dlq.Close()
	store := clickhouse.NewClient(clickhouseURL, clickhouseUser, clickhousePassword)
	log.Println("ClickHouse consumer started")

	for ctx.Err() == nil {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			log.Printf("read Kafka message: %v", err)
			pause(ctx)
			continue
		}

		// Never FetchMessage again until THIS message was stored and committed.
		// Otherwise the next offset could be committed over a failed message.
		for ctx.Err() == nil {
			var event vacation.DecisionEvent
			parseErr := json.Unmarshal(msg.Value, &event)
			if parseErr == nil && (event.EventID <= 0 || event.RequestID <= 0 || !vacation.IsFinalDecision(event.Decision)) {
				parseErr = fmt.Errorf("invalid decision event fields")
			}
			if parseErr != nil {
				// Do not skip a poison message unless it is durably accepted by the DLQ.
				dlqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				err := dlq.Publish(dlqCtx, msg, parseErr.Error())
				cancel()
				if err != nil {
					log.Printf("DLQ publish failed offset=%d: %v", msg.Offset, err)
					pause(ctx)
					continue
				}
				commitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				err = reader.CommitMessages(commitCtx, msg)
				cancel()
				if err != nil {
					log.Printf("DLQ offset commit failed offset=%d: %v", msg.Offset, err)
					pause(ctx)
					continue
				}
				log.Printf("sent invalid event offset=%d to DLQ: %v", msg.Offset, parseErr)
				break
			}
			insertCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := store.Insert(insertCtx, event)
			cancel()
			if err != nil {
				log.Printf("ClickHouse event=%d: %v", event.EventID, err)
				pause(ctx)
				continue
			}
			// Inserting twice is acceptable; the table uses event_id for dedup.
			commitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err = reader.CommitMessages(commitCtx, msg)
			cancel()
			if err != nil {
				log.Printf("commit Kafka event=%d: %v", event.EventID, err)
				pause(ctx)
				continue
			}
			log.Printf("stored event=%d request=%d decision=%s", event.EventID, event.RequestID, event.Decision)
			break
		}
	}
}

func pause(ctx context.Context) {
	select {
	case <-time.After(2 * time.Second):
	case <-ctx.Done():
	}
}
