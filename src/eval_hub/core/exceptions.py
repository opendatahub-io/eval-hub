"""Custom exceptions for the evaluation service."""

from typing import Any


class EvaluationServiceError(Exception):
    """Base exception for evaluation service errors."""

    def __init__(self, message: str, details: dict[str, Any] | None = None):
        super().__init__(message)
        self.message = message
        self.details = details or {}


class ValidationError(EvaluationServiceError):
    """Exception raised for validation errors."""

    pass


class ExecutionError(EvaluationServiceError):
    """Exception raised for execution errors."""

    pass


class BackendError(EvaluationServiceError):
    """Exception raised for backend-related errors."""

    pass


class MLFlowError(EvaluationServiceError):
    """Exception raised for MLFlow-related errors."""

    pass


class TimeoutError(EvaluationServiceError):
    """Exception raised for timeout errors."""

    pass


class ConfigurationError(EvaluationServiceError):
    """Exception raised for configuration errors."""

    pass
