import os
import time
import logging
import threading
from enum import Enum
from typing import Callable, Any
import grpc

logger = logging.getLogger(__name__)


class CircuitState(Enum):
    CLOSED = "CLOSED"
    OPEN = "OPEN"
    HALF_OPEN = "HALF_OPEN"


class CircuitBreaker:
    def __init__(
        self,
        failure_threshold: int = 5,
        recovery_timeout: int = 30,
        name: str = "default"
    ):
        self.failure_threshold = failure_threshold
        self.recovery_timeout = recovery_timeout
        self.name = name
        
        self.state = CircuitState.CLOSED
        self.failure_count = 0
        self.last_failure_time = None
        self.lock = threading.Lock()
    
    def call(self, func: Callable, *args, **kwargs) -> Any:
        """Execute function with circuit breaker protection"""
        with self.lock:
            if self.state == CircuitState.OPEN:
                if self._should_attempt_reset():
                    logger.info(f"Circuit breaker {self.name}: OPEN -> HALF_OPEN")
                    self.state = CircuitState.HALF_OPEN
                else:
                    logger.warning(f"Circuit breaker {self.name}: OPEN, rejecting request")
                    raise grpc.RpcError(
                        grpc.StatusCode.UNAVAILABLE,
                        f"Circuit breaker {self.name} is OPEN"
                    )
        
        try:
            result = func(*args, **kwargs)
            
            with self.lock:
                if self.state == CircuitState.HALF_OPEN:
                    logger.info(f"Circuit breaker {self.name}: HALF_OPEN -> CLOSED")
                    self.state = CircuitState.CLOSED
                    self.failure_count = 0
                elif self.state == CircuitState.CLOSED:
                    self.failure_count = 0
            
            return result
        
        except grpc.RpcError as e:
            with self.lock:
                self.failure_count += 1
                self.last_failure_time = time.time()
                
                if self.failure_count >= self.failure_threshold:
                    if self.state == CircuitState.CLOSED:
                        logger.warning(
                            f"Circuit breaker {self.name}: CLOSED -> OPEN "
                            f"(failures: {self.failure_count})"
                        )
                    elif self.state == CircuitState.HALF_OPEN:
                        logger.warning(
                            f"Circuit breaker {self.name}: HALF_OPEN -> OPEN "
                            f"(test request failed)"
                        )
                    self.state = CircuitState.OPEN
            
            raise
    
    def _should_attempt_reset(self) -> bool:
        """Check if enough time has passed to attempt reset"""
        if self.last_failure_time is None:
            return True
        return time.time() - self.last_failure_time >= self.recovery_timeout
    
    def get_state(self) -> CircuitState:
        """Get current circuit breaker state"""
        with self.lock:
            return self.state
    
    def reset(self):
        """Manually reset the circuit breaker"""
        with self.lock:
            logger.info(f"Circuit breaker {self.name}: manually reset to CLOSED")
            self.state = CircuitState.CLOSED
            self.failure_count = 0
            self.last_failure_time = None


# Global circuit breaker instance
circuit_breaker = CircuitBreaker(
    failure_threshold=int(os.getenv("CB_FAILURE_THRESHOLD", "5")),
    recovery_timeout=int(os.getenv("CB_RECOVERY_TIMEOUT", "30")),
    name="flight-service"
)
