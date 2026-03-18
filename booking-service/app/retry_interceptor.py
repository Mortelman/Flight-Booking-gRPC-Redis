import time
import logging
import grpc
from functools import wraps

logger = logging.getLogger(__name__)

# Retry only for these error codes
RETRYABLE_CODES = [
    grpc.StatusCode.UNAVAILABLE,
    grpc.StatusCode.DEADLINE_EXCEEDED,
]

# Do NOT retry for these error codes
NON_RETRYABLE_CODES = [
    grpc.StatusCode.INVALID_ARGUMENT,
    grpc.StatusCode.NOT_FOUND,
    grpc.StatusCode.RESOURCE_EXHAUSTED,
]


def retry_with_backoff(max_retries=3, initial_delay=0.1):
    """
    Retry decorator with exponential backoff.
    Only retries for UNAVAILABLE and DEADLINE_EXCEEDED errors.
    """
    def decorator(func):
        @wraps(func)
        def wrapper(*args, **kwargs):
            last_exception = None
            delay = initial_delay
            
            for attempt in range(max_retries):
                try:
                    return func(*args, **kwargs)
                except grpc.RpcError as e:
                    last_exception = e
                    code = e.code()
                    
                    # Don't retry for non-retryable errors
                    if code in NON_RETRYABLE_CODES:
                        logger.warning(f"Non-retryable error: {code}, not retrying")
                        raise
                    
                    # Don't retry for other errors
                    if code not in RETRYABLE_CODES:
                        logger.warning(f"Error code {code} is not retryable, not retrying")
                        raise
                    
                    # Retry for retryable errors
                    if attempt < max_retries - 1:
                        logger.warning(
                            f"Attempt {attempt + 1}/{max_retries} failed with {code}, "
                            f"retrying in {delay:.2f}s..."
                        )
                        time.sleep(delay)
                        delay *= 2  # Exponential backoff
                    else:
                        logger.error(f"All {max_retries} attempts failed")
                        raise
            
            # This should never be reached, but just in case
            if last_exception:
                raise last_exception
        
        return wrapper
    return decorator
