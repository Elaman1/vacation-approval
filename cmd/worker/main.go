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

	"vacation-approval/internal/core/vacation"
	"vacation-approval/internal/integration/camunda"
	"vacation-approval/internal/integration/postgres"
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
			if err := processTask(ctx, repo, client, task); err != nil {
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

func processTask(ctx context.Context, repo *postgres.VacationRepository, client *camunda.Client, task camunda.ExternalTask) error {
	requestID, err := task.RequestID()
	if err != nil {
		return err
	}

	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	variables := map[string]any{}
	switch task.TopicName {
	case "check-balance":
		balance, requested, err := repo.GetBalanceForRequest(queryCtx, requestID)
		if err != nil {
			return err
		}
		enough := vacation.HasEnoughBalance(balance, requested)
		variables = map[string]any{
			"hasEnoughBalance": camunda.BoolVar(enough),
			"requestedDays":    camunda.IntVar(requested),
		}
		log.Printf("request=%d balance=%d requested=%d enough=%t", requestID, balance, requested, enough)

	case "mark-pending-manager":
		if err := repo.MarkPendingManager(queryCtx, requestID); err != nil {
			return err
		}
		log.Printf("request=%d is pending manager review", requestID)

	case "finalize-vacation":
		decision, err := task.Decision()
		if err != nil {
			return err
		}
		actual, err := repo.Finalize(queryCtx, requestID, vacation.Status(decision))
		if err != nil {
			return err
		}
		variables = map[string]any{"finalDecision": camunda.StringVar(string(actual))}
		log.Printf("request=%d DMN=%s final=%s", requestID, decision, actual)

	default:
		return fmt.Errorf("unknown topic %q", task.TopicName)
	}

	// If DB commit succeeded but complete fails, Camunda retries the task.
	// Finalize() is idempotent, so a second attempt cannot deduct days twice.
	if err := client.Complete(ctx, task.ID, variables); err != nil {
		return err
	}
	return nil
}
