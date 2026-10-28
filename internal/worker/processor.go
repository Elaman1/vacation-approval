package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"vacation-approval/internal/core/vacation"
	"vacation-approval/internal/integration/camunda"
	"vacation-approval/internal/integration/postgres"
)

func ProcessTask(ctx context.Context, repo *postgres.VacationRepository, client *camunda.Client, task camunda.ExternalTask) error {
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
