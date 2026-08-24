package osc

import (
	"context"
	"errors"
	"net"
	"reflect"
	"testing"
)

func TestOSCServiceConstructorReturnsErrors(t *testing.T) {
	service, err := NewOSCService(ControllerConfig{CatalogMode: CatalogMode(255)})
	if err == nil || service != nil {
		t.Fatalf("NewOSCService invalid mode = (%#v, %v), want nil service and error", service, err)
	}

	service, err = NewOSCService(ControllerConfig{CatalogMode: CatalogExternal})
	if err != nil || service == nil {
		t.Fatalf("NewOSCService external = (%#v, %v), want service", service, err)
	}
}

func TestOSCServiceForwardsRuntimeMailboxEventsAndStatus(t *testing.T) {
	controller := newRuntimeController(t, CatalogExternal, &recordingPacketSender{})
	controller.udp = &UDPTransport{}
	controller.udp.SetTarget(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9000})
	controller.active = &activeVRChat{service: DiscoveredService{Instance: "VRChat"}}
	controller.setRunning(true)
	service := &baseOSCService{controller: controller}

	changes := service.AvatarChanges(context.Background())
	controller.publishAvatarChange("avtr_a")
	if got := receiveAvatarChange(t, changes); got.AvatarID != "avtr_a" {
		t.Fatalf("avatar change = %#v", got)
	}
	if service.Events() != controller.Events() {
		t.Fatal("Events did not forward controller channel")
	}

	catalog := runtimeTestCatalog(t, 7)
	if err := service.InstallCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	if err := service.Publish(6, &testValueSource{}); !errors.Is(err, ErrRuntimeGeneration) {
		t.Fatalf("Publish stale error = %v, want %v", err, ErrRuntimeGeneration)
	}
	if got := controller.Catalog(); !reflect.DeepEqual(got, catalog) {
		t.Fatalf("installed catalog = %#v", got)
	}
	status := service.Status()
	if !status.Running || !status.Connected || !status.HasTarget || status.Target != (OSCTarget{Host: "127.0.0.1", Port: 9000}) {
		t.Fatalf("status = %#v", status)
	}

	service.ClearRuntime()
	if got := controller.Catalog(); got != nil {
		t.Fatalf("catalog after clear = %#v, want nil", got)
	}
}

func TestOSCServiceStatusSanitizesLastError(t *testing.T) {
	controller := newRuntimeController(t, CatalogExternal, &recordingPacketSender{})
	controller.recordError(errors.New("  connection\nfailed\t safely  "))
	service := &baseOSCService{controller: controller}

	if got, want := service.Status().LastError, "connection failed safely"; got != want {
		t.Fatalf("LastError = %q, want %q", got, want)
	}
}
