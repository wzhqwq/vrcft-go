package osc

import (
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
)

func TestControllerEventCatalogIsolatedFromInstalledCatalog(t *testing.T) {
	catalog := buildSenderTestCatalog(t, false)
	transport := &recordingPacketSender{}
	sender := newParameterSender(transport, SenderConfig{})
	sender.SetCatalog(catalog)
	controller := &Controller{
		events: make(chan ControllerEvent, 1),
		sender: sender,
	}
	controller.catalog.Store(catalog)

	controller.emit(ControllerEvent{Kind: EventCatalogUpdated, Catalog: catalog})
	event := <-controller.events
	if event.Catalog == nil || len(event.Catalog.Outputs) == 0 || len(event.Catalog.RawMethods) == 0 {
		t.Fatalf("event catalog = %#v, want compiled outputs", event.Catalog)
	}
	event.Catalog.Outputs = nil
	event.Catalog.RawMethods[0].Address = "/mutated/event"

	installed := controller.Catalog()
	if got, want := len(installed.Outputs), 3; got != want {
		t.Errorf("controller catalog outputs = %d, want %d", got, want)
	}
	if got, want := installed.RawMethods[0].Address, "/a/Float"; got != want {
		t.Errorf("controller raw endpoint = %q, want %q", got, want)
	}
	source := &testValueSource{
		floats: map[parameters.ParameterID]float32{0: 0.25},
		bools:  map[parameters.ParameterID]bool{1: true},
	}
	if err := sender.Send(source); err != nil {
		t.Fatal(err)
	}
	values := decodedValuesByAddress(t, transport.packets)
	if _, ok := values["/a/Float"]; !ok {
		t.Error("sender did not retain installed /a/Float output")
	}
	if got, want := len(values), 3; got != want {
		t.Errorf("sender outputs after event mutation = %d, want %d", got, want)
	}
}
