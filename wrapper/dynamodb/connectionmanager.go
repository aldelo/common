package dynamodb

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
)

type ServiceConnectionManager struct {
	semaphore  chan struct{}
	mu         sync.RWMutex
	wg         sync.WaitGroup
	shutdownCh chan struct{} // non-nil while manager is active; closed on shutdown
	once       sync.Once
	closing    bool // prevents new wg.Add after shutdown begins to avoid WaitGroup misuse
}

var (
	globalConnMgr      *ServiceConnectionManager
	globalConnMgrMutex sync.RWMutex
)

// convertAwsContextSafely tries to extract a stdlib context.Context from aws.Context.
// If not possible, it returns context.Background().
func convertAwsContextSafely(awsCtx aws.Context) context.Context {
	if awsCtx == nil {
		return context.Background()
	}

	// Most aws.Context implementations embed context.Context; try a type assertion.
	if c, ok := awsCtx.(context.Context); ok && c != nil {
		return c
	}

	// Fallback: best-effort background context.
	return context.Background()
}

func ensureAwsContext(ctx aws.Context) aws.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

// InitGlobalConnectionManager initializes the global manager if it is nil or shut down.
func InitGlobalConnectionManager(maxConcurrentConnections int) error {
	if maxConcurrentConnections <= 0 {
		err := fmt.Errorf("maxConcurrentConnections must be greater than 0")
		log.Printf("InitGlobalConnectionManager: %v", err)
		return err
	}

	// Fast path with read lock.
	globalConnMgrMutex.RLock()
	mgr := globalConnMgr
	globalConnMgrMutex.RUnlock()

	if mgr != nil && !mgr.isShutdown() {
		log.Printf("InitGlobalConnectionManager: already initialized and open")
		return nil
	}

	// Slow path: acquire write lock and re-check.
	globalConnMgrMutex.Lock()
	defer globalConnMgrMutex.Unlock()

	if globalConnMgr == nil || globalConnMgr.isShutdown() {
		globalConnMgr = &ServiceConnectionManager{
			semaphore:  make(chan struct{}, maxConcurrentConnections),
			shutdownCh: make(chan struct{}),
			closing:    false,
		}
		log.Printf("InitGlobalConnectionManager: initialized with capacity=%d", maxConcurrentConnections)
	} else {
		log.Printf("InitGlobalConnectionManager: already initialized and open (write-lock check)")
	}
	return nil
}

func IsGlobalConnectionManagerInitialized() bool {
	globalConnMgrMutex.RLock()
	mgr := globalConnMgr
	globalConnMgrMutex.RUnlock()
	return mgr != nil && !mgr.isShutdown()
}

func GetGlobalConnectionManager() *ServiceConnectionManager {
	globalConnMgrMutex.RLock()
	mgr := globalConnMgr
	globalConnMgrMutex.RUnlock()
	if mgr == nil || mgr.isShutdown() {
		return nil
	}
	return mgr
}

func LogGlobalConnectionManagerStats() {
	connMgr := GetGlobalConnectionManager()
	if connMgr != nil {
		current := connMgr.GetCurrentLoad()
		maxLoad := connMgr.GetMaxCapacity()
		log.Printf("Connection Manager Stats: %d/%d semaphore usage", current, maxLoad)
	} else {
		log.Printf("Connection Manager Stats: Not initialized or shutdown")
	}
}

func ShutdownGlobalConnectionManager() {
	globalConnMgrMutex.Lock()
	mgr := globalConnMgr
	globalConnMgr = nil
	globalConnMgrMutex.Unlock()

	if mgr != nil {
		log.Printf("ShutdownGlobalConnectionManager: initiating shutdown")
		mgr.shutdown()
	} else {
		log.Printf("ShutdownGlobalConnectionManager: no manager to shutdown")
	}
}

// isShutdown is safe to call concurrently.
func (scm *ServiceConnectionManager) isShutdown() bool {
	if scm == nil {
		return true
	}
	scm.mu.RLock()
	defer scm.mu.RUnlock()
	if scm.shutdownCh == nil {
		return true
	}
	select {
	case <-scm.shutdownCh:
		return true
	default:
		return false
	}
}

// shutdown closes shutdownCh once and waits for all in-flight operations.
func (scm *ServiceConnectionManager) shutdown() {
	if scm == nil {
		log.Printf("ServiceConnectionManager.shutdown: scm is nil")
		return
	}

	scm.once.Do(func() {
		log.Printf("ServiceConnectionManager.shutdown: closing shutdownCh")

		// Protect shutdownCh mutation with write lock to avoid races with isShutdown.
		scm.mu.Lock()
		scm.closing = true
		if scm.shutdownCh != nil {
			close(scm.shutdownCh)
			// leave shutdownCh non-nil but closed; isShutdown handles this
		}
		scm.mu.Unlock()

		log.Printf("ServiceConnectionManager.shutdown: waiting for active operations to complete")
		scm.wg.Wait()
		log.Printf("ServiceConnectionManager.shutdown: all operations completed")
	})
}

func (scm *ServiceConnectionManager) GetCurrentLoad() int {
	if scm == nil || scm.isShutdown() {
		return 0
	}
	if scm.semaphore == nil {
		return 0
	}
	return len(scm.semaphore)
}

func (scm *ServiceConnectionManager) GetMaxCapacity() int {
	if scm == nil || scm.isShutdown() {
		return 0
	}
	if scm.semaphore == nil {
		return 0
	}
	return cap(scm.semaphore)
}

// ExecuteWithLimit runs operation respecting the semaphore and shutdown.
// Operations that have already acquired the semaphore may continue running
// after shutdown is initiated; shutdown waits for them via wg.
func (scm *ServiceConnectionManager) ExecuteWithLimit(ctx context.Context, operation func() error) error {
	if operation == nil {
		err := fmt.Errorf("operation is nil")
		log.Printf("ExecuteWithLimit: %v", err)
		return err
	}
	if scm == nil {
		log.Printf("ExecuteWithLimit: scm is nil, running operation without limit")
		return operation()
	}

	var cancel context.CancelFunc
	if ctx == nil {
		ctx = context.Background()
	}

	if _, ok := ctx.Deadline(); !ok {
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	}
	if cancel != nil {
		defer cancel()
	}

	// Decide based on shutdown and context under read lock.
	scm.mu.RLock()
	shutdownCh := scm.shutdownCh
	semaphore := scm.semaphore
	closing := scm.closing
	scm.mu.RUnlock()

	if semaphore == nil {
		err := fmt.Errorf("semaphore is nil")
		log.Printf("ExecuteWithLimit: %v", err)
		return err
	}

	if shutdownCh == nil || closing || scm.isShutdown() {
		err := fmt.Errorf("connection manager shutdown")
		log.Printf("ExecuteWithLimit: %v", err)
		return err
	}

	for {
		select {
		case <-shutdownCh:
			err := fmt.Errorf("connection manager shutdown")
			log.Printf("ExecuteWithLimit: %v", err)
			return err

		case <-ctx.Done():
			err := fmt.Errorf("context timed out while waiting for semaphore: %w", ctx.Err())
			log.Printf("ExecuteWithLimit: %v", err)
			return err

		case semaphore <- struct{}{}:
			// success path
			log.Printf("ExecuteWithLimit: semaphore acquired")

			scm.mu.RLock()
			if scm.closing {
				scm.mu.RUnlock()
				<-semaphore
				err := fmt.Errorf("connection manager shutdown")
				log.Printf("ExecuteWithLimit: %v", err)
				return err
			}
			scm.wg.Add(1)
			scm.mu.RUnlock()

			defer func() {
				<-semaphore
				log.Printf("ExecuteWithLimit: semaphore released")
				scm.wg.Done()
			}()

			goto runOperation
		}
	}

	// At this point, the goroutine is registered in wg and holds a semaphore slot.
	// Shutdown will wait for completion; we do not re-check shutdownCh here to
	// avoid unsynchronized reads and to keep semantics simple.

runOperation:
	var opErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("ExecuteWithLimit.run: recovered panic: %v", r)
				if e, ok := r.(error); ok {
					opErr = fmt.Errorf("panic: %w", e)
				} else {
					opErr = fmt.Errorf("panic: %v", r)
				}
			}
		}()
		log.Printf("ExecuteWithLimit: executing operation")
		opErr = operation()
		if opErr != nil {
			log.Printf("ExecuteWithLimit: operation returned error: %v", opErr)
		} else {
			log.Printf("ExecuteWithLimit: operation completed successfully")
		}
	}()
	return opErr
}

// =====================================================================================================================
// SCM Setup in Consuming Application main()
// =====================================================================================================================

// func main() {
// 	// Initialize global connection manager early
// 	_ = InitGlobalConnectionManager(150) // Limit to 150 concurrent DynamoDB operations
//
// 	// ... rest of service initialization
// }
