package handlers_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/eval-hub/eval-hub/internal/eval_hub/abstractions"
	"github.com/eval-hub/eval-hub/internal/eval_hub/executioncontext"
	"github.com/eval-hub/eval-hub/internal/eval_hub/handlers"
	"github.com/eval-hub/eval-hub/internal/testhelpers"
	"github.com/eval-hub/eval-hub/pkg/api"
	"github.com/eval-hub/eval-hub/pkg/cards"
)

type recordingResultsExporter struct {
	called  bool
	cardURL string
}

func (r *recordingResultsExporter) Export(_ context.Context, _ *api.EvaluationJobResource, _ *cards.EvaluationCard) (string, error) {
	r.called = true
	return r.cardURL, nil
}

type terminalExportStorage struct {
	*fakeStorage
}

func (s *terminalExportStorage) WithLogger(_ *slog.Logger) abstractions.Storage { return s }
func (s *terminalExportStorage) WithContext(_ context.Context) abstractions.Storage {
	return s
}
func (s *terminalExportStorage) WithTenant(_ api.Tenant) abstractions.Storage { return s }
func (s *terminalExportStorage) WithOwner(_ api.User) abstractions.Storage    { return s }

func (s *terminalExportStorage) UpdateEvaluationJob(_ string, status *api.StatusEvent) (*abstractions.EvaluationJobUpdate, error) {
	var prev api.OverallState
	if s.job != nil && s.job.Status != nil {
		prev = s.job.Status.State
	}
	if s.job != nil && s.job.Status != nil && status != nil && status.BenchmarkStatusEvent != nil {
		s.job.Status.State = api.OverallState(status.BenchmarkStatusEvent.Status)
	}
	result := &abstractions.EvaluationJobUpdate{
		PreviousState: prev,
		ComputedState: s.job.Status.State,
		Job:           s.job,
	}
	if result.IsTerminalTransition() {
		result.TerminalMessage = s.job.Status.Message
	}
	return result, nil
}

func TestHandleUpdateEvaluationSkipsCardExportWhenNotTerminal(t *testing.T) {
	t.Parallel()
	exporter := &recordingResultsExporter{cardURL: "https://example.com/card.json"}
	storage := &terminalExportStorage{
		fakeStorage: &fakeStorage{
			job: &api.EvaluationJobResource{
				Resource: api.EvaluationResource{Resource: api.Resource{ID: "job-1"}},
				Status: &api.EvaluationJobStatus{
					EvaluationJobState: api.EvaluationJobState{State: api.OverallStateRunning},
				},
			},
		},
	}
	h := handlers.New(storage, testhelpers.NewValidator(t), nil, nil, nil, exporter)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := executioncontext.NewExecutionContext(context.Background(), "req-running", logger, "test-user", "test-tenant")

	body := `{"benchmark_status_event":{"provider_id":"p1","id":"b1","status":"running"}}`
	req := &updateEvaluationRequest{
		bodyRequest: &bodyRequest{
			MockRequest: createMockRequest("POST", "/api/v1/evaluations/jobs/job-1/events"),
			body:        []byte(body),
		},
		pathValues: map[string]string{"job_id": "job-1"},
	}
	recorder := httptest.NewRecorder()
	resp := MockResponseWrapper{recorder: recorder}

	h.HandleUpdateEvaluation(ctx, req, resp)

	if recorder.Code != 204 {
		t.Fatalf("expected status 204, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if exporter.called {
		t.Fatal("expected card export to be skipped for non-terminal job")
	}
}

func TestHandleUpdateEvaluationExportsCardOnTerminalTransition(t *testing.T) {
	t.Parallel()
	exporter := &recordingResultsExporter{cardURL: "https://example.com/card.json"}
	storage := &terminalExportStorage{
		fakeStorage: &fakeStorage{
			job: &api.EvaluationJobResource{
				Resource: api.EvaluationResource{Resource: api.Resource{ID: "job-1"}},
				Status: &api.EvaluationJobStatus{
					EvaluationJobState: api.EvaluationJobState{State: api.OverallStateRunning},
				},
			},
		},
	}
	h := handlers.New(storage, testhelpers.NewValidator(t), nil, nil, nil, exporter)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := executioncontext.NewExecutionContext(context.Background(), "req-completed", logger, "test-user", "test-tenant")

	body := `{"benchmark_status_event":{"provider_id":"p1","id":"b1","status":"completed"}}`
	req := &updateEvaluationRequest{
		bodyRequest: &bodyRequest{
			MockRequest: createMockRequest("POST", "/api/v1/evaluations/jobs/job-1/events"),
			body:        []byte(body),
		},
		pathValues: map[string]string{"job_id": "job-1"},
	}
	recorder := httptest.NewRecorder()
	resp := MockResponseWrapper{recorder: recorder}

	h.HandleUpdateEvaluation(ctx, req, resp)

	if recorder.Code != 204 {
		t.Fatalf("expected status 204, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if !exporter.called {
		t.Fatal("expected card export when job transitions to terminal state")
	}
}

func TestHandleUpdateEvaluationExportsCardOnFailedTransition(t *testing.T) {
	t.Parallel()
	exporter := &recordingResultsExporter{cardURL: "https://example.com/card.json"}
	storage := &terminalExportStorage{
		fakeStorage: &fakeStorage{
			job: &api.EvaluationJobResource{
				Resource: api.EvaluationResource{Resource: api.Resource{ID: "job-1"}},
				Status: &api.EvaluationJobStatus{
					EvaluationJobState: api.EvaluationJobState{State: api.OverallStateRunning},
				},
			},
		},
	}
	h := handlers.New(storage, testhelpers.NewValidator(t), nil, nil, nil, exporter)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := executioncontext.NewExecutionContext(context.Background(), "req-failed", logger, "test-user", "test-tenant")

	body := `{"benchmark_status_event":{"provider_id":"p1","id":"b1","status":"failed"}}`
	req := &updateEvaluationRequest{
		bodyRequest: &bodyRequest{
			MockRequest: createMockRequest("POST", "/api/v1/evaluations/jobs/job-1/events"),
			body:        []byte(body),
		},
		pathValues: map[string]string{"job_id": "job-1"},
	}
	recorder := httptest.NewRecorder()
	resp := MockResponseWrapper{recorder: recorder}

	h.HandleUpdateEvaluation(ctx, req, resp)

	if recorder.Code != 204 {
		t.Fatalf("expected status 204, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if !exporter.called {
		t.Fatal("expected card export when job transitions to failed")
	}
}

func TestHandleUpdateEvaluationSkipsCardExportWhenTerminalStateUnchanged(t *testing.T) {
	t.Parallel()
	exporter := &recordingResultsExporter{cardURL: "https://example.com/card.json"}
	storage := &terminalExportStorage{
		fakeStorage: &fakeStorage{
			job: &api.EvaluationJobResource{
				Resource: api.EvaluationResource{Resource: api.Resource{ID: "job-1"}},
				Status: &api.EvaluationJobStatus{
					EvaluationJobState: api.EvaluationJobState{State: api.OverallStateCompleted},
				},
			},
		},
	}
	h := handlers.New(storage, testhelpers.NewValidator(t), nil, nil, nil, exporter)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := executioncontext.NewExecutionContext(context.Background(), "req-completed-again", logger, "test-user", "test-tenant")

	body := `{"benchmark_status_event":{"provider_id":"p1","id":"b1","status":"completed"}}`
	req := &updateEvaluationRequest{
		bodyRequest: &bodyRequest{
			MockRequest: createMockRequest("POST", "/api/v1/evaluations/jobs/job-1/events"),
			body:        []byte(body),
		},
		pathValues: map[string]string{"job_id": "job-1"},
	}
	recorder := httptest.NewRecorder()
	resp := MockResponseWrapper{recorder: recorder}

	h.HandleUpdateEvaluation(ctx, req, resp)

	if recorder.Code != 204 {
		t.Fatalf("expected status 204, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if exporter.called {
		t.Fatal("expected card export to be skipped when terminal state is unchanged")
	}
}

// contextCancellingExportStorage wraps terminalExportStorage but cancels a
// context after UpdateEvaluationJob returns, simulating an HTTP client
// disconnect between the status update commit and the export side effects.
type contextCancellingExportStorage struct {
	*terminalExportStorage
	cancelFn context.CancelFunc
}

func (s *contextCancellingExportStorage) WithLogger(_ *slog.Logger) abstractions.Storage { return s }
func (s *contextCancellingExportStorage) WithContext(_ context.Context) abstractions.Storage {
	return s
}
func (s *contextCancellingExportStorage) WithTenant(_ api.Tenant) abstractions.Storage { return s }
func (s *contextCancellingExportStorage) WithOwner(_ api.User) abstractions.Storage    { return s }

func (s *contextCancellingExportStorage) UpdateEvaluationJob(id string, status *api.StatusEvent) (*abstractions.EvaluationJobUpdate, error) {
	result, err := s.terminalExportStorage.UpdateEvaluationJob(id, status)
	if err == nil && s.cancelFn != nil {
		s.cancelFn()
	}
	return result, err
}

func TestHandleUpdateEvaluationExportsCardEvenWhenRequestContextCancelled(t *testing.T) {
	t.Parallel()
	exporter := &recordingResultsExporter{cardURL: "https://example.com/card.json"}
	cancelCtx, cancel := context.WithCancel(context.Background())
	innerStorage := &terminalExportStorage{
		fakeStorage: &fakeStorage{
			job: &api.EvaluationJobResource{
				Resource: api.EvaluationResource{Resource: api.Resource{ID: "job-ctx"}},
				Status: &api.EvaluationJobStatus{
					EvaluationJobState: api.EvaluationJobState{State: api.OverallStateRunning},
				},
			},
		},
	}
	storage := &contextCancellingExportStorage{
		terminalExportStorage: innerStorage,
		cancelFn:              cancel,
	}
	h := handlers.New(storage, testhelpers.NewValidator(t), nil, nil, nil, exporter)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := executioncontext.NewExecutionContext(cancelCtx, "req-cancel", logger, "test-user", "test-tenant")

	body := `{"benchmark_status_event":{"provider_id":"p1","id":"b1","status":"completed"}}`
	req := &updateEvaluationRequest{
		bodyRequest: &bodyRequest{
			MockRequest: createMockRequest("POST", "/api/v1/evaluations/jobs/job-ctx/events"),
			body:        []byte(body),
		},
		pathValues: map[string]string{"job_id": "job-ctx"},
	}
	recorder := httptest.NewRecorder()
	resp := MockResponseWrapper{recorder: recorder}

	h.HandleUpdateEvaluation(ctx, req, resp)

	if recorder.Code != 204 {
		t.Fatalf("expected status 204, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if !exporter.called {
		t.Fatal("expected card export to succeed even after request context cancellation")
	}
}

// orderTrackingExporter records the wall-clock time it was called so we
// can verify export happens BEFORE the terminal state is committed.
type orderTrackingExporter struct {
	exportedBeforeFinalize bool
	exported               bool
	storage                *orderTrackingStorage
}

func (e *orderTrackingExporter) Export(_ context.Context, _ *api.EvaluationJobResource, _ *cards.EvaluationCard) (string, error) {
	e.exported = true
	// At this point the storage should NOT have been finalized yet.
	e.exportedBeforeFinalize = !e.storage.finalized
	return "https://example.com/card.json", nil
}

// orderTrackingStorage tracks whether UpdateEvaluationJobStatus (finalize) has been called.
type orderTrackingStorage struct {
	*fakeStorage
	finalized bool
}

func (s *orderTrackingStorage) WithLogger(_ *slog.Logger) abstractions.Storage { return s }
func (s *orderTrackingStorage) WithContext(_ context.Context) abstractions.Storage {
	return s
}
func (s *orderTrackingStorage) WithTenant(_ api.Tenant) abstractions.Storage { return s }
func (s *orderTrackingStorage) WithOwner(_ api.User) abstractions.Storage    { return s }

func (s *orderTrackingStorage) UpdateEvaluationJob(_ string, status *api.StatusEvent) (*abstractions.EvaluationJobUpdate, error) {
	var prev api.OverallState
	if s.job != nil && s.job.Status != nil {
		prev = s.job.Status.State
	}
	if s.job != nil && s.job.Status != nil && status != nil && status.BenchmarkStatusEvent != nil {
		s.job.Status.State = api.OverallState(status.BenchmarkStatusEvent.Status)
	}
	result := &abstractions.EvaluationJobUpdate{
		PreviousState: prev,
		ComputedState: s.job.Status.State,
		Job:           s.job,
	}
	if result.IsTerminalTransition() {
		result.TerminalMessage = &api.MessageInfo{Message: "completed", MessageCode: "DONE"}
	}
	return result, nil
}

func (s *orderTrackingStorage) UpdateEvaluationJobStatus(_ string, _ api.OverallState, _ *api.MessageInfo) error {
	s.finalized = true
	return nil
}

// TestHandleUpdateEvaluationExportsCardBeforeTerminalStateCommit verifies
// that the MLflow evaluation-card export happens BEFORE the terminal state
// is committed to the database, preventing the race where a client polls
// "completed" but the MLflow artifact has not been created yet.
func TestHandleUpdateEvaluationExportsCardBeforeTerminalStateCommit(t *testing.T) {
	t.Parallel()
	innerStorage := &orderTrackingStorage{
		fakeStorage: &fakeStorage{
			job: &api.EvaluationJobResource{
				Resource: api.EvaluationResource{Resource: api.Resource{ID: "job-order"}},
				Status: &api.EvaluationJobStatus{
					EvaluationJobState: api.EvaluationJobState{State: api.OverallStateRunning},
				},
			},
		},
	}
	exporter := &orderTrackingExporter{storage: innerStorage}
	h := handlers.New(innerStorage, testhelpers.NewValidator(t), nil, nil, nil, exporter)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := executioncontext.NewExecutionContext(context.Background(), "req-order", logger, "test-user", "test-tenant")

	body := `{"benchmark_status_event":{"provider_id":"p1","id":"b1","status":"completed"}}`
	req := &updateEvaluationRequest{
		bodyRequest: &bodyRequest{
			MockRequest: createMockRequest("POST", "/api/v1/evaluations/jobs/job-order/events"),
			body:        []byte(body),
		},
		pathValues: map[string]string{"job_id": "job-order"},
	}
	recorder := httptest.NewRecorder()
	resp := MockResponseWrapper{recorder: recorder}

	h.HandleUpdateEvaluation(ctx, req, resp)

	if recorder.Code != 204 {
		t.Fatalf("expected status 204, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if !exporter.exported {
		t.Fatal("expected card export to be called on terminal transition")
	}
	if !exporter.exportedBeforeFinalize {
		t.Fatal("expected card export to happen BEFORE UpdateEvaluationJobStatus (terminal state commit)")
	}
	if !innerStorage.finalized {
		t.Fatal("expected UpdateEvaluationJobStatus to be called after export to commit terminal state")
	}
}
