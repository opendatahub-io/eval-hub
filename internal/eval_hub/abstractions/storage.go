package abstractions

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"time"

	"github.com/eval-hub/eval-hub/pkg/api"
)

const (
	ScopeSystem = "system"
	ScopeTenant = "tenant"

	OwnerSystem = "system"
)

type QueryResults[T any] struct {
	Items      []T
	TotalCount int
	Errors     []string
}

type QueryFilter struct {
	Limit  int
	Offset int
	Params map[string]any
}

// ExtractQueryParams returns the limit, offset, and filtered params
func (filter *QueryFilter) ExtractQueryParams() *QueryFilter {
	params := maps.Clone(filter.Params)
	// delete empty values
	maps.DeleteFunc(params, func(k string, v any) bool {
		return v == ""
	})
	return &QueryFilter{
		Limit:  filter.Limit,
		Offset: filter.Offset,
		Params: params,
	}
}

// HasParams returns true if all the given params are present in the filter and have non-empty values
func (filter *QueryFilter) HasParams(params ...string) bool {
	queryParams := filter.ExtractQueryParams().Params
	for _, param := range params {
		if _, exists := queryParams[param]; !exists {
			return false
		}
	}
	return true
}

func (filter *QueryFilter) String() string {
	return fmt.Sprintf(`{"limit":%d,"offset":%d,"params":%v}`, filter.Limit, filter.Offset, filter.Params)
}

// EvaluationJobUpdate holds the result of an UpdateEvaluationJob call.
// When the computed overall state is terminal and differs from the previous state,
// the storage layer persists benchmark results but defers the terminal state commit.
// Callers must inspect the result and call UpdateEvaluationJobStatus to finalize.
type EvaluationJobUpdate struct {
	// PreviousState is the overall state before this update.
	PreviousState api.OverallState
	// ComputedState is the overall state computed by this update (may be terminal).
	ComputedState api.OverallState
	// Job is the fully populated job with benchmark results and ComputedState
	// set on Status.State. When the terminal state is deferred, the DB entity
	// still has the non-terminal state; this in-memory copy carries the computed
	// terminal state so callers can use it for export without a re-read.
	Job *api.EvaluationJobResource
	// TerminalMessage is the status message for the terminal state. Non-nil only
	// when ComputedState is terminal and differs from PreviousState.
	TerminalMessage *api.MessageInfo
}

// IsTerminalTransition returns true when the update computed a new terminal state
// that differs from the previous state and was deferred in the database.
func (u *EvaluationJobUpdate) IsTerminalTransition() bool {
	return u != nil &&
		u.ComputedState.IsTerminalState() &&
		u.PreviousState != u.ComputedState
}

type Storage interface {
	WithLogger(logger *slog.Logger) Storage
	WithContext(ctx context.Context) Storage
	WithTenant(tenant api.Tenant) Storage
	WithOwner(owner api.User) Storage

	Ping(timeout time.Duration) error

	// Evaluation job operations
	CreateEvaluationJob(evaluation *api.EvaluationJobResource) error
	GetEvaluationJob(id string) (*api.EvaluationJobResource, error)
	GetEvaluationJobs(filter *QueryFilter) (*QueryResults[api.EvaluationJobResource], error)
	DeleteEvaluationJob(id string) error
	// UpdateEvaluationJob merges a benchmark status event into the job, computes
	// the new overall state, and persists benchmark results. When the computed
	// state is terminal (and differs from the previous state), the overall state
	// is NOT committed to the database; callers must first perform any pre-terminal
	// work (e.g. MLflow export) and then call UpdateEvaluationJobStatus to finalize.
	UpdateEvaluationJob(id string, runStatus *api.StatusEvent) (*EvaluationJobUpdate, error)
	// UpdateEvaluationJobStatus is used to update the status of an evaluation job and is internal - do we need it here?
	UpdateEvaluationJobStatus(id string, state api.OverallState, message *api.MessageInfo) error
	// UpdateEvaluationJobResolvedSHA records the resolved test-data identity (e.g. git commit SHA)
	// on the benchmark at benchmarkIndex as TestDataRef.ResolvedSHA. Idempotent: if already set, no-op.
	UpdateEvaluationJobResolvedSHA(id string, benchmarkIndex int, sha string) error

	// Collection operations
	CreateCollection(collection *api.CollectionResource) error
	GetCollection(id string) (*api.CollectionResource, error)
	GetCollections(filter *QueryFilter) (*QueryResults[api.CollectionResource], error)
	UpdateCollection(id string, collection *api.CollectionConfig) (*api.CollectionResource, error)
	PatchCollection(id string, patches *api.Patch) (*api.CollectionResource, error)
	DeleteCollection(id string) error

	// Provider operations
	CreateProvider(provider *api.ProviderResource) error
	GetProvider(id string) (*api.ProviderResource, error)
	GetProviders(filter *QueryFilter) (*QueryResults[api.ProviderResource], error)
	UpdateProvider(id string, providerConfig *api.ProviderConfig) (*api.ProviderResource, error)
	PatchProvider(id string, patches *api.Patch) (*api.ProviderResource, error)
	DeleteProvider(id string) error

	// LoadSystemResources reloads system-owned providers and collections into
	// the database. Existing system resources are deleted and replaced.
	// CreatedAt is preserved for existing IDs; UpdatedAt is preserved only when
	// the serialized config is unchanged.
	LoadSystemResources(systemCollections map[string]api.CollectionResource, systemProviders map[string]api.ProviderResource) error

	// Close the storage connection
	Close() error
}

// This interface must be decoupled from the service HTTP layer.
// Do not pass ExecutionContext, Request or Response wrappers either.
