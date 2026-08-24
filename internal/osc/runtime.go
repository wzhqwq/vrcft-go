package osc

import (
	"errors"
	"sync"
)

var (
	ErrRuntimeMode       = errors.New("OSC runtime catalog mode")
	ErrRuntimeCatalog    = errors.New("OSC runtime catalog")
	ErrRuntimeGeneration = errors.New("OSC runtime generation")
)

type CatalogMode uint8

const (
	CatalogOSCQuery CatalogMode = iota
	CatalogExternal
)

// sendRuntime keeps a catalog and its value source in one synchronization
// boundary. A send holds the read lock until the transport has accepted the
// packet, so clear and replacement cannot overlap an old-runtime send.
type sendRuntime struct {
	mu         sync.RWMutex
	mode       CatalogMode
	sender     *ParameterSender
	fixed      ValueSource
	generation uint64
	source     ValueSource
}

func newSendRuntime(sender *ParameterSender, mode CatalogMode, fixed ValueSource) *sendRuntime {
	return &sendRuntime{sender: sender, mode: mode, fixed: fixed}
}

func (runtime *sendRuntime) clear() {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.generation = 0
	runtime.source = nil
	runtime.sender.SetCatalog(nil)
	runtime.sender.ResetChangeDetection()
}

func (runtime *sendRuntime) installQuery(catalog *Catalog) error {
	if runtime.mode != CatalogOSCQuery {
		return ErrRuntimeMode
	}
	if err := validRuntimeCatalog(catalog); err != nil {
		return err
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.sender.SetCatalog(catalog.Clone())
	runtime.generation = catalog.Generation
	runtime.source = runtime.fixed
	return nil
}

func (runtime *sendRuntime) installExternal(catalog *Catalog) error {
	if runtime.mode != CatalogExternal {
		return ErrRuntimeMode
	}
	if err := validRuntimeCatalog(catalog); err != nil {
		return err
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.sender.SetCatalog(catalog.Clone())
	runtime.generation = catalog.Generation
	runtime.source = nil
	return nil
}

func (runtime *sendRuntime) publish(generation uint64, source ValueSource) error {
	if runtime.mode != CatalogExternal {
		return ErrRuntimeMode
	}
	if source == nil {
		return ErrRuntimeCatalog
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if generation == 0 || runtime.generation == 0 || generation != runtime.generation {
		return ErrRuntimeGeneration
	}
	runtime.source = source
	return nil
}

func (runtime *sendRuntime) resetChangeDetection() {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.sender.ResetChangeDetection()
}

func (runtime *sendRuntime) catalog() *Catalog {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()

	return runtime.sender.Catalog()
}

func (runtime *sendRuntime) send() error {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()

	if runtime.source == nil {
		return nil
	}
	return runtime.sender.Send(runtime.source)
}

func validRuntimeCatalog(catalog *Catalog) error {
	if catalog == nil {
		return ErrRuntimeCatalog
	}
	if catalog.Generation == 0 {
		return ErrRuntimeGeneration
	}
	return nil
}
