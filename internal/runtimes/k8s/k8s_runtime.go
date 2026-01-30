package k8s

import (
	"log/slog"

	"github.com/eval-hub/eval-hub/internal/abstractions"
	"github.com/eval-hub/eval-hub/pkg/api"
)

type K8sRuntime struct {
	logger *slog.Logger
	helper *KubernetesHelper
}

// NewK8sRuntime creates a Kubernetes runtime with the injected KubernetesHelper.
// helper must be non-nil; create it with NewKubernetesHelper() when LocalMode is false.
func NewK8sRuntime(logger *slog.Logger, helper *KubernetesHelper) (abstractions.Runtime, error) {
	return &K8sRuntime{logger: logger, helper: helper}, nil
}

func (r *K8sRuntime) RunEvaluationJob(evaluation *api.EvaluationJobResource, storage *abstractions.Storage) error {
	return nil
}

func (r *K8sRuntime) Name() string {
	return "kubernetes"
}
