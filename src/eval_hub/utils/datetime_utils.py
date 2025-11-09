"""Datetime utilities for consistent timezone handling."""

from datetime import UTC, datetime


def utcnow() -> datetime:
    """Get current UTC time as timezone-aware datetime.

    Replacement for datetime.utcnow() to ensure all datetimes are timezone-aware.
    """
    return datetime.now(UTC)


def parse_iso_datetime(datetime_str: str) -> datetime:
    """Parse ISO 8601 datetime string to timezone-aware datetime.

    Handles various ISO formats including those with 'Z' suffix from Kubernetes.

    Args:
        datetime_str: ISO 8601 formatted datetime string

    Returns:
        Timezone-aware datetime object

    Raises:
        ValueError: If the datetime string cannot be parsed
    """
    if not datetime_str:
        raise ValueError("Empty datetime string")

    # Handle 'Z' suffix (UTC timezone)
    if datetime_str.endswith("Z"):
        datetime_str = datetime_str[:-1] + "+00:00"

    try:
        # Try parsing with timezone info
        return datetime.fromisoformat(datetime_str)
    except ValueError:
        try:
            # Fallback: parse without timezone and assume UTC
            dt = datetime.fromisoformat(datetime_str)
            if dt.tzinfo is None:
                dt = dt.replace(tzinfo=UTC)
            return dt
        except ValueError as e:
            raise ValueError(
                f"Unable to parse datetime string '{datetime_str}': {e}"
            ) from e


def ensure_timezone_aware(dt: datetime) -> datetime:
    """Ensure a datetime is timezone-aware, assuming UTC if naive.

    Args:
        dt: Datetime object that may or may not be timezone-aware

    Returns:
        Timezone-aware datetime object
    """
    if dt.tzinfo is None:
        return dt.replace(tzinfo=UTC)
    return dt


def safe_duration_seconds(end_time: datetime, start_time: datetime) -> float:
    """Calculate duration in seconds between two datetimes safely.

    Ensures both datetimes are timezone-aware before calculation.

    Args:
        end_time: End datetime
        start_time: Start datetime

    Returns:
        Duration in seconds as float
    """
    end_time = ensure_timezone_aware(end_time)
    start_time = ensure_timezone_aware(start_time)

    return (end_time - start_time).total_seconds()
