package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"vacation-approval/internal/integration/kafka"
	"vacation-approval/internal/integration/postgres"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	broker := os.Getenv("KAFKA_BROKER")
	if databaseURL == "" || broker == "" {
		log.Fatal("DATABASE_URL and KAFKA_BROKER are required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = db.Ping(pingCtx)
	cancel()
	if err != nil {
		log.Fatal(err)
	}

	// Session-level advisory lock: only one active outbox publisher per database.
	// Holding the acquired connection keeps the lock until shutdown.
	lockConn, err := db.Acquire(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer lockConn.Release()
	var locked bool
	if err := lockConn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", int64(826451093)).Scan(&locked); err != nil {
		log.Fatal(fmt.Errorf("acquire outbox lock: %w", err))
	}
	if !locked {
		log.Fatal("another outbox publisher is already running")
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var unlocked bool
		if err := lockConn.QueryRow(unlockCtx, "SELECT pg_advisory_unlock($1)", int64(826451093)).Scan(&unlocked); err != nil {
			log.Printf("release outbox lock: %v", err)
		}
	}()

	repo := postgres.NewOutboxRepository(db)
	publisher := kafka.NewPublisher(broker)
	defer publisher.Close()
	log.Println("outbox publisher started")

	for ctx.Err() == nil {
		if err := publishPending(ctx, repo, publisher); err != nil {
			log.Printf("outbox: %v", err)
		}
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
		}
	}
}

func publishPending(ctx context.Context, repo *postgres.OutboxRepository, publisher *kafka.Publisher) error {
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	events, err := repo.Pending(queryCtx, 20)
	cancel()
	if err != nil {
		return err
	}
	for _, event := range events {
		publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := publisher.Publish(publishCtx, event)
		cancel()
		if err != nil {
			return err // retry on next poll
		}
		if err := repo.MarkPublished(ctx, event.EventID); err != nil {
			return err // Kafka may receive a duplicate; event_id stays stable
		}
		log.Printf("published event=%d request=%d decision=%s", event.EventID, event.RequestID, event.Decision)
	}
	return nil
}
