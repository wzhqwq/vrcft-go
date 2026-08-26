package osc

import (
	"context"
	"errors"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
)

func TestNewControllerValidatesTargetMode(t *testing.T) {
	tests := []struct {
		name      string
		mode      TargetMode
		target    OSCTarget
		preferred string
		wantErr   bool
		wantMode  TargetMode
	}{
		{"zero is auto", "", OSCTarget{}, "", false, TargetModeAuto},
		{"auto rejects manual fields", TargetModeAuto, OSCTarget{Host: "127.0.0.1", Port: 9000}, "", true, ""},
		{"manual IPv4", TargetModeManual, OSCTarget{Host: "127.0.0.1", Port: 9000}, "", false, TargetModeManual},
		{"manual IPv6", TargetModeManual, OSCTarget{Host: "::1", Port: 9000}, "", false, TargetModeManual},
		{"manual DNS rejected", TargetModeManual, OSCTarget{Host: "localhost", Port: 9000}, "", true, ""},
		{"manual unspecified", TargetModeManual, OSCTarget{Host: "0.0.0.0", Port: 9000}, "", true, ""},
		{"manual multicast", TargetModeManual, OSCTarget{Host: "239.1.1.1", Port: 9000}, "", true, ""},
		{"manual broadcast", TargetModeManual, OSCTarget{Host: "255.255.255.255", Port: 9000}, "", true, ""},
		{"manual zero port", TargetModeManual, OSCTarget{Host: "127.0.0.1"}, "", true, ""},
		{"manual rejects preferred service", TargetModeManual, OSCTarget{Host: "127.0.0.1", Port: 9000}, "VRChat", true, ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			controller, err := NewController(ControllerConfig{
				TargetMode:             test.mode,
				ManualTarget:           test.target,
				PreferredVRChatService: test.preferred,
			}, nil, nil)
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("NewController error = %v, want error %t", err, test.wantErr)
			}
			if !test.wantErr && controller.config.TargetMode != test.wantMode {
				t.Fatalf("TargetMode = %q, want %q", controller.config.TargetMode, test.wantMode)
			}
		})
	}
}

func TestControllerManualTargetSurvivesDiscoveryTransitions(t *testing.T) {
	controller, err := NewController(ControllerConfig{
		TargetMode:   TargetModeManual,
		ManualTarget: OSCTarget{Host: "127.0.0.1", Port: 9000},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	controller.ctx = context.Background()
	controller.udp = &UDPTransport{}
	controller.udp.SetTarget(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9000})
	controller.queryClient = &fakeControllerQueryClient{
		hostInfo:    HostInfo{Name: "VRChat", OSCIP: "127.0.0.2", OSCPort: 9001, OSCTransport: "UDP"},
		hasHostInfo: true,
		nodes:       map[string]*QueryNode{"/": NewQueryRoot()},
	}
	service := DiscoveredService{Instance: "VRChat", Service: ServiceOSCQuery, HostName: "localhost", Port: 8000}

	for _, step := range []struct {
		name      string
		connected bool
	}{
		{name: "connect", connected: true},
		{name: "disconnect", connected: false},
		{name: "reconnect", connected: true},
	} {
		t.Run(step.name, func(t *testing.T) {
			if step.connected {
				active, err := controller.probeService(service)
				if err != nil {
					t.Fatal(err)
				}
				controller.active = active
			} else {
				controller.clearActive(errors.New("lost"))
			}

			status := controller.Status()
			if status.Connected != step.connected {
				t.Fatalf("Connected = %t, want %t", status.Connected, step.connected)
			}
			if !status.HasTarget || status.Target != (OSCTarget{Host: "127.0.0.1", Port: 9000}) {
				t.Fatalf("target status = %#v, want manual target", status)
			}
		})
	}
}

func TestControllerManualTargetStillPublishesAvatarChanges(t *testing.T) {
	controller, err := NewController(ControllerConfig{
		TargetMode:   TargetModeManual,
		ManualTarget: OSCTarget{Host: "127.0.0.1", Port: 9000},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	controller.ctx = ctx
	controller.cancel = cancel
	udp, err := ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	controller.udp = udp
	controller.udp.SetTarget(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9000})
	controller.wg.Add(1)
	go controller.runUDPReceiver()
	defer func() {
		cancel()
		_ = udp.Close()
		controller.wg.Wait()
	}()

	changes := controller.AvatarChanges(context.Background())
	packet, err := MarshalMessage(Message{Address: "/avatar/change", Args: []Value{String("avtr_manual")}})
	if err != nil {
		t.Fatal(err)
	}
	sender, err := net.DialUDP("udp", nil, udp.LocalAddr())
	if err != nil {
		t.Fatal(err)
	}
	defer sender.Close()
	if _, err := sender.Write(packet); err != nil {
		t.Fatal(err)
	}
	if got := receiveAvatarChange(t, changes); got.AvatarID != "avtr_manual" {
		t.Fatalf("avatar change = %#v", got)
	}
	if status := controller.Status(); !status.HasTarget || status.Target != (OSCTarget{Host: "127.0.0.1", Port: 9000}) {
		t.Fatalf("target status = %#v, want manual target", status)
	}
}

func TestControllerPreferredMissingServiceDoesNotSelectAnotherVRChat(t *testing.T) {
	controller, err := NewController(ControllerConfig{PreferredVRChatService: "wanted"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	controller.ctx = context.Background()
	controller.udp = &UDPTransport{}
	controller.queryClient = &fakeControllerQueryClient{nodes: map[string]*QueryNode{"/": NewQueryRoot()}}
	controller.services[serviceKey(DiscoveredService{Instance: "other", Service: ServiceOSCQuery})] = DiscoveredService{
		Instance: "other", Service: ServiceOSCQuery, HostName: "localhost", Port: 8000, LastSeen: time.Now(),
	}

	controller.connectBest()

	if active := controller.currentActive(); active != nil {
		t.Fatalf("active service = %#v, want nil", active)
	}
	if target := controller.udp.Target(); target != nil {
		t.Fatalf("target = %v, want nil", target)
	}
}

func TestControllerEventCatalogIsolatedFromInstalledCatalog(t *testing.T) {
	catalog := buildSenderTestCatalog(t, false)
	transport := &recordingPacketSender{}
	controller := newRuntimeController(t, CatalogOSCQuery, transport)
	if err := controller.runtime.installQuery(catalog); err != nil {
		t.Fatal(err)
	}

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
	if err := controller.sender.Send(source); err != nil {
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

func TestControllerExternalCatalogDefaultQueryModeRefreshesCatalog(t *testing.T) {
	root := NewQueryRoot()
	if err := root.Add(NewMethod("/avatar/parameters/v2/JawOpen", "f", AccessWriteOnly)); err != nil {
		t.Fatal(err)
	}
	controller := newRuntimeController(t, CatalogOSCQuery, &recordingPacketSender{})
	query := &fakeControllerQueryClient{nodes: map[string]*QueryNode{"/avatar/parameters": root}}
	controller.queryClient = query

	if err := controller.refreshCatalog(context.Background(), &activeVRChat{baseURL: "http://vrchat.test"}, true); err != nil {
		t.Fatal(err)
	}
	if got := controller.Catalog(); got == nil || got.Generation != 1 || len(got.Outputs) != 1 {
		t.Fatalf("catalog = %#v, want compiled generation 1 catalog", got)
	}
}

func TestControllerExternalCatalogModeDoesNotRefreshCatalog(t *testing.T) {
	controller := newRuntimeController(t, CatalogExternal, &recordingPacketSender{})
	installed := runtimeTestCatalog(t, 7)
	if err := controller.InstallCatalog(installed); err != nil {
		t.Fatal(err)
	}
	query := &fakeControllerQueryClient{nodes: map[string]*QueryNode{"/": NewQueryRoot()}}
	controller.queryClient = query

	if err := controller.refreshCatalog(context.Background(), &activeVRChat{baseURL: "http://vrchat.test"}, true); err != nil {
		t.Fatal(err)
	}
	if got := controller.Catalog(); !reflect.DeepEqual(got, installed) {
		t.Fatalf("catalog = %#v, want retained external catalog", got)
	}
	if !reflect.DeepEqual(query.paths, []string{"/"}) {
		t.Fatalf("queried paths = %v, want health probe only", query.paths)
	}
}

func TestControllerExternalCatalogOwnsInstallAndFencesPublish(t *testing.T) {
	controller := newRuntimeController(t, CatalogExternal, &recordingPacketSender{})
	catalog := runtimeTestCatalog(t, 7)
	want := catalog.Clone()
	if err := controller.InstallCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	mutateEveryCatalogLayer(catalog)
	if got := controller.Catalog(); !reflect.DeepEqual(got, want) {
		t.Fatalf("catalog = %#v, want owned clone %#v", got, want)
	}
	if err := controller.Publish(6, &testValueSource{}); !errors.Is(err, ErrRuntimeGeneration) {
		t.Fatalf("Publish stale error = %v, want %v", err, ErrRuntimeGeneration)
	}
}

func TestControllerExternalCatalogQueryModeRejectsRuntimeMutation(t *testing.T) {
	controller := newRuntimeController(t, CatalogOSCQuery, &recordingPacketSender{})
	if err := controller.InstallCatalog(runtimeTestCatalog(t, 7)); !errors.Is(err, ErrRuntimeMode) {
		t.Fatalf("InstallCatalog error = %v, want %v", err, ErrRuntimeMode)
	}
	if err := controller.Publish(7, &testValueSource{}); !errors.Is(err, ErrRuntimeMode) {
		t.Fatalf("Publish error = %v, want %v", err, ErrRuntimeMode)
	}
}

func TestControllerExternalCatalogDisconnectRetainsRuntimeAndClearsTarget(t *testing.T) {
	controller := newRuntimeController(t, CatalogExternal, &recordingPacketSender{})
	catalog := runtimeTestCatalog(t, 7)
	if err := controller.InstallCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	controller.udp = &UDPTransport{}
	controller.udp.SetTarget(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9000})
	controller.active = &activeVRChat{service: DiscoveredService{Instance: "VRChat"}, cancel: func() {}}

	controller.clearActive(errors.New("lost"))

	if target := controller.udp.Target(); target != nil {
		t.Fatalf("target = %v, want nil", target)
	}
	if got := controller.Catalog(); !reflect.DeepEqual(got, catalog) {
		t.Fatalf("catalog = %#v, want retained external runtime", got)
	}
}

func TestControllerExternalCatalogReconnectRetainsRuntimeAndResetsChanges(t *testing.T) {
	transport := &recordingPacketSender{}
	controller := newRuntimeController(t, CatalogExternal, transport)
	catalog := runtimeTestCatalog(t, 7)
	if err := controller.InstallCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	if err := controller.Publish(7, &testValueSource{floats: map[parameters.ParameterID]float32{0: 0.25}}); err != nil {
		t.Fatal(err)
	}
	if err := controller.runtime.send(); err != nil {
		t.Fatal(err)
	}
	firstPackets := len(transport.packets)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	controller.ctx = ctx
	controller.udp = &UDPTransport{}
	controller.queryClient = &fakeControllerQueryClient{}
	controller.config.QueryPollInterval = time.Hour
	controller.setActive(&activeVRChat{service: DiscoveredService{Instance: "VRChat"}})
	controller.wg.Wait()
	if err := controller.runtime.send(); err != nil {
		t.Fatal(err)
	}

	if got := len(transport.packets); got <= firstPackets {
		t.Fatalf("packet count after reconnect = %d, want greater than %d", got, firstPackets)
	}
	if got := controller.Catalog(); !reflect.DeepEqual(got, catalog) {
		t.Fatalf("catalog = %#v, want retained external runtime", got)
	}
}

func newUnstartedController(t testing.TB, mode CatalogMode) *Controller {
	t.Helper()
	controller, err := NewController(ControllerConfig{CatalogMode: mode}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return controller
}

func newRuntimeController(t testing.TB, mode CatalogMode, transport packetSender) *Controller {
	t.Helper()
	controller := newUnstartedController(t, mode)
	controller.sender = newParameterSender(transport, SenderConfig{})
	controller.runtime = newSendRuntime(controller.sender, mode, controller.source)
	return controller
}

type fakeControllerQueryClient struct {
	hostInfo    HostInfo
	hasHostInfo bool
	nodes       map[string]*QueryNode
	paths       []string
	err         error
}

func (client *fakeControllerQueryClient) HostInfo(context.Context, string) (HostInfo, error) {
	if !client.hasHostInfo {
		return HostInfo{Name: "VRChat", OSCIP: "127.0.0.1", OSCPort: 9000, OSCTransport: "UDP"}, client.err
	}
	return client.hostInfo, client.err
}

func (client *fakeControllerQueryClient) Node(_ context.Context, _ string, path string) (*QueryNode, error) {
	client.paths = append(client.paths, path)
	if client.err != nil {
		return nil, client.err
	}
	if node := client.nodes[path]; node != nil {
		return node.Clone(), nil
	}
	return nil, ErrNodeNotFound
}

func (client *fakeControllerQueryClient) WatchChanges(ctx context.Context, _ string, _ HostInfo, _ func(QueryCommand)) error {
	<-ctx.Done()
	return ctx.Err()
}
