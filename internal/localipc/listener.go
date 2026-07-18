package localipc

import (
	"errors"
	"net"
	"sync"
)

// Listener owns a platform listener and its singleton/endpoint cleanup.
type Listener struct {
	net.Listener
	cleanup func() error
	once    sync.Once
	err     error
}

func newListener(listener net.Listener, cleanup func() error) *Listener {
	return &Listener{Listener: listener, cleanup: cleanup}
}

// Close stops accepting and releases the platform endpoint exactly once.
func (listener *Listener) Close() error {
	listener.once.Do(func() {
		closeErr := listener.Listener.Close()
		if errors.Is(closeErr, net.ErrClosed) {
			closeErr = nil
		}
		listener.err = errors.Join(closeErr, listener.cleanup())
	})
	return listener.err
}
