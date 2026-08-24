package osc

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
)

func TestSendRuntimeExternalOwnsCatalogAndFencesGeneration(t *testing.T) {
	catalog := runtimeTestCatalog(t, 7)
	originalCatalog := catalog.Clone()
	transport := &recordingPacketSender{}
	runtime := newSendRuntime(newParameterSender(transport, SenderConfig{UseBundles: true}), CatalogExternal, nil)
	if err := runtime.installExternal(catalog); err != nil {
		t.Fatal(err)
	}

	mutateEveryCatalogLayer(catalog)
	if got := runtime.catalog(); !reflect.DeepEqual(got, originalCatalog) {
		t.Fatalf("installed catalog changed: %#v", got)
	}

	source := &testValueSource{floats: map[parameters.ParameterID]float32{0: 0.25}}
	for _, test := range []struct {
		name       string
		generation uint64
		want       error
	}{
		{name: "stale", generation: 6, want: ErrRuntimeGeneration},
		{name: "future", generation: 8, want: ErrRuntimeGeneration},
		{name: "current", generation: 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := runtime.publish(test.generation, source)
			if !errors.Is(err, test.want) {
				t.Fatalf("publish(%d) error = %v, want %v", test.generation, err, test.want)
			}
		})
	}
	if err := runtime.send(); err != nil {
		t.Fatal(err)
	}
	if got := len(transport.packets); got == 0 {
		t.Fatal("current generation did not send a packet")
	}
}

func TestSendRuntimeRejectsInvalidTransitions(t *testing.T) {
	catalog := runtimeTestCatalog(t, 7)
	zeroGeneration := catalog.Clone()
	zeroGeneration.Generation = 0
	source := &testValueSource{floats: map[parameters.ParameterID]float32{0: 0.25}}

	for _, test := range []struct {
		name string
		run  func() error
		want error
	}{
		{
			name: "external nil catalog",
			run: func() error {
				return newSendRuntime(newParameterSender(&recordingPacketSender{}, SenderConfig{}), CatalogExternal, nil).installExternal(nil)
			},
			want: ErrRuntimeCatalog,
		},
		{
			name: "external zero generation catalog",
			run: func() error {
				return newSendRuntime(newParameterSender(&recordingPacketSender{}, SenderConfig{}), CatalogExternal, nil).installExternal(zeroGeneration)
			},
			want: ErrRuntimeGeneration,
		},
		{
			name: "external nil source",
			run: func() error {
				runtime := newSendRuntime(newParameterSender(&recordingPacketSender{}, SenderConfig{}), CatalogExternal, nil)
				if err := runtime.installExternal(catalog); err != nil {
					return err
				}
				return runtime.publish(7, nil)
			},
			want: ErrRuntimeCatalog,
		},
		{
			name: "query rejects external catalog",
			run: func() error {
				return newSendRuntime(newParameterSender(&recordingPacketSender{}, SenderConfig{}), CatalogOSCQuery, source).installExternal(catalog)
			},
			want: ErrRuntimeMode,
		},
		{
			name: "external rejects query catalog",
			run: func() error {
				return newSendRuntime(newParameterSender(&recordingPacketSender{}, SenderConfig{}), CatalogExternal, nil).installQuery(catalog)
			},
			want: ErrRuntimeMode,
		},
		{
			name: "query rejects external source",
			run: func() error {
				runtime := newSendRuntime(newParameterSender(&recordingPacketSender{}, SenderConfig{}), CatalogOSCQuery, source)
				if err := runtime.installQuery(catalog); err != nil {
					return err
				}
				return runtime.publish(7, source)
			},
			want: ErrRuntimeMode,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.run(); !errors.Is(got, test.want) {
				t.Fatalf("error = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSendRuntimeExternalCatalogWithoutSourceSendsNothing(t *testing.T) {
	transport := &recordingPacketSender{}
	runtime := newSendRuntime(newParameterSender(transport, SenderConfig{}), CatalogExternal, nil)
	if err := runtime.installExternal(runtimeTestCatalog(t, 7)); err != nil {
		t.Fatal(err)
	}
	if err := runtime.send(); err != nil {
		t.Fatal(err)
	}
	if got := len(transport.packets); got != 0 {
		t.Fatalf("packets = %d, want none without a source", got)
	}
}

func TestSendRuntimeQueryUsesOnlyFixedSource(t *testing.T) {
	transport := &recordingPacketSender{}
	fixed := &testValueSource{floats: map[parameters.ParameterID]float32{0: 0.25}}
	runtime := newSendRuntime(newParameterSender(transport, SenderConfig{}), CatalogOSCQuery, fixed)
	if err := runtime.installQuery(runtimeTestCatalog(t, 7)); err != nil {
		t.Fatal(err)
	}
	if err := runtime.send(); err != nil {
		t.Fatal(err)
	}
	values := decodedValuesByAddress(t, transport.packets)
	assertFloatValue(t, values["/a/Float"], 0.25)
}

func TestSendRuntimeClearWaitsForInFlightSend(t *testing.T) {
	transport := newBlockingPacketSender()
	runtime := newSendRuntime(newParameterSender(transport, SenderConfig{UseBundles: true}), CatalogExternal, nil)
	if err := runtime.installExternal(runtimeTestCatalog(t, 7)); err != nil {
		t.Fatal(err)
	}
	if err := runtime.publish(7, &testValueSource{floats: map[parameters.ParameterID]float32{0: 0.25}}); err != nil {
		t.Fatal(err)
	}

	sendDone := make(chan error, 1)
	go func() { sendDone <- runtime.send() }()
	<-transport.entered

	clearDone := make(chan struct{})
	go func() { runtime.clear(); close(clearDone) }()
	assertNotClosed(t, clearDone)
	close(transport.release)
	if err := <-sendDone; err != nil {
		t.Fatal(err)
	}
	assertClosed(t, clearDone)

	if err := runtime.send(); err != nil {
		t.Fatal(err)
	}
	if got := transport.calls; got != 1 {
		t.Fatalf("transport calls after clear = %d, want 1", got)
	}
}

func runtimeTestCatalog(t testing.TB, generation uint64) *Catalog {
	t.Helper()
	catalog := buildSenderTestCatalog(t, true)
	catalog.Generation = generation
	return catalog
}

func mutateEveryCatalogLayer(catalog *Catalog) {
	catalog.Generation = 99
	catalog.UpdatedAt = time.Time{}
	catalog.Hash = 0
	catalog.RawMethods[0].Address = "/mutated/raw"
	catalog.Outputs[0].Address = "/mutated/output"
	for id, binding := range catalog.Bindings {
		if len(binding.Binary) == 0 {
			continue
		}
		binding.Direct[0].Address = "/mutated/direct"
		binding.Binary[0].Negative.Address = "/mutated/negative"
		binding.Binary[0].Bits[0].Endpoint.Address = "/mutated/bit"
		catalog.Bindings[id] = binding
		break
	}
}

type blockingPacketSender struct {
	entered chan struct{}
	release chan struct{}
	calls   int
}

func newBlockingPacketSender() *blockingPacketSender {
	return &blockingPacketSender{entered: make(chan struct{}), release: make(chan struct{})}
}

func (sender *blockingPacketSender) Send([]byte) error {
	sender.calls++
	close(sender.entered)
	<-sender.release
	return nil
}

func assertNotClosed(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
		t.Fatal("operation returned before blocked transport released")
	case <-time.After(25 * time.Millisecond):
	}
}

func assertClosed(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("operation did not return")
	}
}
