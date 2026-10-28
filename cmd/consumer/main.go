package main

import (
	"context"
	"encoding/json"
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
	reader := kafka.NewReader(broker)
	defer reader.Close()
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
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				log.Printf("invalid event at offset %d (not committing): %v", msg.Offset, err)
				pause(ctx)
				continue
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
