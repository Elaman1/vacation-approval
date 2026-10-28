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

	"vacation-approval/internal/integration/camunda"
	"vacation-approval/internal/integration/postgres"
	"vacation-approval/internal/worker"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	camundaURL := os.Getenv("CAMUNDA_URL")
	if databaseURL == "" || camundaURL == "" {
		log.Fatal("DATABASE_URL and CAMUNDA_URL are required")
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
		log.Fatalf("connect PostgreSQL: %v", err)
	}

	repo := postgres.NewVacationRepository(db)
	workerID := fmt.Sprintf("vacation-worker-%d", time.Now().UnixNano())
	client := camunda.NewClient(camundaURL, workerID)
	log.Printf("worker started: %s", workerID)

	for ctx.Err() == nil {
		tasks, err := client.FetchTasks(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			log.Printf("fetch tasks: %v", err)
			select {
			case <-time.After(3 * time.Second):
			case <-ctx.Done():
			}
			continue
		}
		for _, task := range tasks {
			if err := worker.ProcessTask(ctx, repo, client, task); err != nil {
				log.Printf("task %s (%s): %v", task.ID, task.TopicName, err)
				if ctx.Err() != nil {
					break
				}
				if reportErr := client.Fail(ctx, task, err); reportErr != nil {
					log.Printf("report failure: %v", reportErr)
				}
			}
		}
	}
	log.Println("worker stopped")
}
