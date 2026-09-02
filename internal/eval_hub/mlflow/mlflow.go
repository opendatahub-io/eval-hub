package mlflow

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/eval-hub/eval-hub/internal/eval_hub/config"
	"github.com/eval-hub/eval-hub/internal/eval_hub/messages"
	"github.com/eval-hub/eval-hub/internal/eval_hub/serviceerrors"
	"github.com/eval-hub/eval-hub/pkg/api"
	"github.com/eval-hub/eval-hub/pkg/mlflowclient"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	workspaceProbeTimeout    = 5 * time.Second
	workspaceProbeMaxRetries = 3
	workspaceProbeRetryDelay = 2 * time.Second
)

func SetupMLFlowClient(config *config.Config, logger *slog.Logger) (*mlflowclient.Client, string, string, error) {
	mlflowClient, err := NewMLFlowClient(config, logger)
	if err != nil {
		return nil, "", "", err
	}
	if mlflowClient == nil {
		// this is the case when no tracking URI is set
		return nil, "", "", nil
	}
	serverVersion, err := mlflowClient.GetVersion()
	if err != nil {
		// for now a failure to get the mlflow server version will not stop the server startup
		// because maybe the port is not yet ready or the server is not yet running
		logger.Warn("Failed to get MLFlow server version", "error", err.Error())
	}
	// if we get here then we have a valid tracking URI
	return mlflowClient, config.MLFlow.TrackingURI, serverVersion, nil
}

func NewMLFlowClient(config *config.Config, logger *slog.Logger) (*mlflowclient.Client, error) {
	url := ""
	if config.MLFlow != nil && config.MLFlow.TrackingURI != "" {
		url = config.MLFlow.TrackingURI
	}

	if url == "" {
		logger.Warn("MLFlow tracking URI is not set, skipping MLFlow client creation")
		return nil, nil
	}

	if config.MLFlow.HTTPTimeout == 0 {
		config.MLFlow.HTTPTimeout = 30 * time.Second
	}

	// Build TLS config if not already provided
	if config.MLFlow.TLSConfig == nil {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS13,
		}

		// Load custom CA certificate if specified
		if config.MLFlow.CACertPath != "" {
			caCert, err := os.ReadFile(config.MLFlow.CACertPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read MLflow CA certificate at %s: %w", config.MLFlow.CACertPath, err)
			}
			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				return nil, fmt.Errorf("failed to parse MLflow CA certificate at %s: file contains no valid PEM certificates", config.MLFlow.CACertPath)
			}
			tlsConfig.RootCAs = caCertPool
			logger.Info("Loaded MLflow CA certificate", "path", config.MLFlow.CACertPath)
		}

		config.MLFlow.TLSConfig = tlsConfig
	}

	httpClient := &http.Client{
		Timeout: config.MLFlow.HTTPTimeout,
		Transport: &http.Transport{
			TLSClientConfig: config.MLFlow.TLSConfig,
		},
	}

	client := mlflowclient.NewClient(url).
		WithContext(context.Background()).
		WithLogger(logger).
		WithHTTPClient(httpClient)

	// Configure auth token. Two modes are supported:
	//   1. Token file path (WithTokenPath) — re-read on each request, supports
	//      Kubernetes projected SA tokens that are rotated on disk by the kubelet.
	//   2. Static token (WithToken) — for local development without a token file.
	// At runtime, the token file takes precedence over the static token.
	tokenPath := config.MLFlow.TokenPath
	// Always configure the token path; resolveAuthToken handles transient
	// absence at request time (e.g. projected volume not yet mounted).
	if tokenPath != "" {
		client = client.WithTokenPath(tokenPath)
		logger.Info("MLflow auth token path configured (per-request reading)", "path", tokenPath)
	}
	if config.MLFlow.Token != "" {
		client = client.WithToken(config.MLFlow.Token)
		logger.Info("MLflow static auth token configured")
	}

	if config.IsOTELEnabled() {
		currentHTTPClient := client.GetHTTPClient()
		client = client.WithHTTPClient(&http.Client{
			Transport: otelhttp.NewTransport(currentHTTPClient.Transport),
			Timeout:   currentHTTPClient.Timeout,
		})
		logger.Info("Enabled OTEL transport for MLFlow client")
	}

	workspacesEnabled := probeWorkspacesWithRetry(client, logger, config.MLFlow.Workspace)
	client = client.WithWorkspacesSupport(workspacesEnabled)
	logger.Info("MLflow workspace support probed", "workspaces_enabled", workspacesEnabled)

	if config.MLFlow.Workspace != "" {
		if workspacesEnabled {
			client = client.WithWorkspace(config.MLFlow.Workspace)
			logger.Info("MLflow workspace configured", "workspace", config.MLFlow.Workspace)
		} else {
			logger.Warn(
				"MLFLOW_WORKSPACE is set but the MLflow server does not support workspaces; ignoring",
				"workspace", config.MLFlow.Workspace,
			)
		}
	}

	logger.Info("MLFlow tracking enabled", "mlflow_experiment_url", client.GetExperimentsURL())

	return client, nil
}

// probeWorkspacesWithRetry retries the MLflow workspace probe up to
// workspaceProbeMaxRetries times with workspaceProbeRetryDelay between
// attempts.  Each attempt uses a fresh context bounded by
// workspaceProbeTimeout.
//
// If all attempts fail with an error (as opposed to the probe explicitly
// returning workspacesEnabled=false) AND configuredWorkspace is non-empty,
// the function assumes workspace support is enabled.  The rationale is that
// the operator explicitly configured a workspace, so a probe failure is
// likely transient (e.g. MLflow not yet ready during Kubernetes pod startup).
func probeWorkspacesWithRetry(client *mlflowclient.Client, logger *slog.Logger, configuredWorkspace string) bool {
	return probeWorkspacesWithRetryParams(client, logger, configuredWorkspace,
		workspaceProbeMaxRetries, workspaceProbeRetryDelay, workspaceProbeTimeout)
}

// probeWorkspacesWithRetryParams is the parameterised implementation used by
// tests to override retry counts and delays.
func probeWorkspacesWithRetryParams(
	client *mlflowclient.Client,
	logger *slog.Logger,
	configuredWorkspace string,
	maxRetries int,
	retryDelay time.Duration,
	probeTimeout time.Duration,
) bool {
	if maxRetries <= 0 {
		return false
	}
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		probeCtx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		enabled, err := client.WithContext(probeCtx).ProbeWorkspacesEnabled()
		cancel()

		if err == nil {
			// Probe succeeded — trust whatever the server returned.
			return enabled
		}

		lastErr = err
		logger.Warn(
			"MLflow workspace probe failed",
			"attempt", attempt,
			"max_retries", maxRetries,
			"error", err.Error(),
		)

		if attempt < maxRetries {
			time.Sleep(retryDelay)
		}
	}

	// All retries exhausted.  If the operator configured a workspace, assume
	// workspace support is available (the probe failure is likely transient).
	if configuredWorkspace != "" {
		logger.Warn(
			"All MLflow workspace probe retries exhausted; assuming workspace support is enabled because MLFLOW_WORKSPACE is configured",
			"workspace", configuredWorkspace,
			"last_error", lastErr.Error(),
		)
		return true
	}

	logger.Warn(
		"All MLflow workspace probe retries exhausted; workspace headers will not be sent",
		"last_error", lastErr.Error(),
	)
	return false
}

func injectEvaluationJobTags(jobID string, evaluation *api.EvaluationJobConfig) []api.ExperimentTag {
	if evaluation.Experiment != nil {
		tags := evaluation.Experiment.Tags
		if tags == nil {
			tags = make([]api.ExperimentTag, 0)
		}

		tags = append(tags, api.ExperimentTag{
			Key:   "context",
			Value: "eval-hub",
		})

		if evaluation.Name != "" {
			tags = append(tags, api.ExperimentTag{
				Key:   "evaluation_job_name",
				Value: evaluation.Name,
			})
		}
		if evaluation.Description != nil {
			tags = append(tags, api.ExperimentTag{
				Key:   "evaluation_job_description",
				Value: *evaluation.Description,
			})
		}
		tags = append(tags, api.ExperimentTag{
			Key:   "evaluation_job_id",
			Value: jobID,
		})
		return tags
	}
	return []api.ExperimentTag{}
}

// HasExperimentName is true when the job config has a non-empty MLflow experiment name.
func HasExperimentName(jobConfig *api.EvaluationJobConfig) bool {
	return jobConfig.Experiment != nil && strings.TrimSpace(jobConfig.Experiment.Name) != ""
}

func GetOrCreateExperimentID(mlflowClient *mlflowclient.Client, jobConfig *api.EvaluationJobConfig, jobID string) (experimentID string, experimentURL string, err error) {
	if !HasExperimentName(jobConfig) {
		return "", "", nil
	}

	// if we get here then we have an experiment name so we need an MLFlow client

	if mlflowClient == nil {
		return "", "", serviceerrors.NewServiceError(messages.MLFlowRequiredForExperiment)
	}

	if err := mlflowClient.EnsureWorkspace(); err != nil {
		return "", "", serviceerrors.NewServiceError(messages.MLFlowRequestFailed, "Error", err.Error())
	}

	tags := injectEvaluationJobTags(jobID, jobConfig)
	req := mlflowclient.CreateExperimentRequest{
		Name:             jobConfig.Experiment.Name,
		ArtifactLocation: jobConfig.Experiment.ArtifactLocation,
		Tags:             tags,
	}
	mlflowExperiment, err := mlflowClient.GetOrCreateExperiment(&req)
	if err != nil {
		return "", "", serviceerrors.NewServiceError(messages.MLFlowRequestFailed, "Error", err.Error())
	}

	mlflowClient.GetLogger().Info("Resolved experiment", "experiment_name", jobConfig.Experiment.Name, "experiment_id", mlflowExperiment.Experiment.ExperimentID)
	return mlflowExperiment.Experiment.ExperimentID, mlflowClient.GetExperimentsURL(), nil
}
