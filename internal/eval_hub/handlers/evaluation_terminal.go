package handlers

import (
	"context"
	"log/slog"

	"github.com/eval-hub/eval-hub/internal/eval_hub/abstractions"
	"github.com/eval-hub/eval-hub/internal/otel"
	"github.com/eval-hub/eval-hub/pkg/api"
)

// finalizeEvaluationJobUpdate handles the post-update lifecycle for an evaluation
// job. When the update computed a new terminal state (deferred by the storage
// layer), this function:
//  1. Exports evaluation results (MLflow card, OCI) BEFORE committing the terminal state,
//     ensuring clients never observe "completed" without persisted artifacts.
//  2. Commits the terminal state via UpdateEvaluationJobStatus.
//  3. Records metrics, threshold violations, and OTEL container log exports.
//
// For non-terminal updates, only metrics recording is performed.
func (h *Handlers) finalizeEvaluationJobUpdate(
	ctx context.Context,
	storage abstractions.Storage,
	update *abstractions.EvaluationJobUpdate,
	logger *slog.Logger,
) {
	if update == nil || update.Job == nil || update.Job.Status == nil {
		return
	}

	if update.IsTerminalTransition() {
		// Step 1: Export evaluation results BEFORE the terminal state is visible
		// to clients. This eliminates the race where a client polls "completed"
		// but the MLflow evaluation-card artifact has not been created yet.
		h.exportEvaluationResults(ctx, update.Job, logger)

		// Step 2: Commit the terminal state to the database.
		if err := storage.UpdateEvaluationJobStatus(
			update.Job.Resource.ID,
			update.ComputedState,
			update.TerminalMessage,
		); err != nil {
			if logger != nil {
				logger.Error("Failed to finalize terminal state after export",
					"job_id", update.Job.Resource.ID,
					"computed_state", update.ComputedState,
					"error", err,
				)
			}
		}
	}

	// Record metrics using the computed state (mirrors the old
	// recordEvaluationJobTerminalStateAfterUpdate behavior but uses the
	// in-memory job instead of re-reading from the database).
	recordEvaluationJobTerminalStateAfterUpdate(ctx, func() (*api.EvaluationJobResource, error) {
		return update.Job, nil
	}, update.PreviousState)

	if !update.IsTerminalTransition() {
		return
	}

	// Post-terminal side effects: threshold violations, OTEL log export.
	if h.runtime != nil && update.Job.Results != nil {
		h.notifyThresholdViolations(ctx, update.Job, logger)
	}

	if h.serviceConfig == nil || !h.serviceConfig.IsOTELJobContainerLogsEnabled() || h.runtime == nil {
		return
	}

	benchmarks, err := h.resolveJobBenchmarksForStorage(storage, update.Job)
	if err != nil {
		if logger != nil {
			logger.WarnContext(ctx, "failed to resolve benchmarks for OTEL container log export",
				"job_id", update.Job.Resource.ID,
				"error", err,
			)
		}
		return
	}

	otel.ExportJobContainerLogsAsync(ctx, h.runtime, update.Job, benchmarks, logger)
}

// notifyThresholdViolations emits EvaluationThresholdViolated signals for every benchmark result
// that has a failing threshold test. Signals are best-effort: errors are absorbed by the runtime.
func (h *Handlers) notifyThresholdViolations(ctx context.Context, job *api.EvaluationJobResource, logger *slog.Logger) {
	for _, bench := range job.Results.Benchmarks {
		if bench.Test == nil || bench.Test.Pass {
			continue
		}
		if logger != nil {
			logger.InfoContext(ctx, "threshold violation detected",
				"job_id", job.Resource.ID,
				"benchmark_index", bench.BenchmarkIndex,
				"metric", bench.Test.PrimaryScoreMetric,
				"actual", bench.Test.PrimaryScore,
				"threshold", bench.Test.Threshold,
			)
		}
		h.runtime.NotifyThresholdViolation(ctx, job, bench.BenchmarkIndex, bench.Test.PrimaryScoreMetric, bench.Test.PrimaryScore, bench.Test.Threshold)
	}
}

func (h *Handlers) resolveJobBenchmarksForStorage(storage abstractions.Storage, job *api.EvaluationJobResource) ([]api.EvaluationBenchmarkConfig, error) {
	var collection *api.CollectionResource
	if job.Collection != nil && job.Collection.ID != "" {
		var err error
		collection, err = storage.GetCollection(job.Collection.ID)
		if err != nil {
			return nil, err
		}
	}
	return GetJobBenchmarks(job, collection)
}
