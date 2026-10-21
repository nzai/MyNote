package ui

type documentCloseDisposition uint8

const (
	documentCloseDestroyed documentCloseDisposition = iota + 1
	documentCloseDeferred
	documentCloseCanceled
)

type exitCoordinator struct {
	pending bool
}

// closeBatch reports whether the controller can be destroyed immediately.
// A deferred document keeps the exit intent alive until its save finishes.
func (e *exitCoordinator) closeBatch(
	results []documentCloseDisposition,
) bool {
	deferred := false
	for _, result := range results {
		switch result {
		case documentCloseCanceled:
			e.pending = false
			return false
		case documentCloseDeferred:
			deferred = true
		}
	}
	e.pending = deferred
	return !deferred
}

func (e *exitCoordinator) documentDestroyed(remaining int) bool {
	return e.pending && remaining == 0
}

func (e *exitCoordinator) cancel() {
	e.pending = false
}
