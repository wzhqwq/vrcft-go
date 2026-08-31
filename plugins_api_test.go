package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/userconfig"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type fakePluginsBackend struct {
	mu sync.Mutex

	snapshots []plugins.RuntimeSnapshot
	configs   map[string]pluginapi.Config

	listCalls   int
	configCalls int
	setCalls    []fakeSetEnabledCall
	updateCalls []fakeUpdateConfigCall

	setErr    error
	updateErr error
	setFn     func(context.Context, string, bool) error
	updateFn  func(context.Context, string, pluginapi.Config) error
}

type fakeSetEnabledCall struct {
	id      string
	enabled bool
}

type fakeUpdateConfigCall struct {
	id     string
	config pluginapi.Config
}

func (fake *fakePluginsBackend) Plugins() []plugins.RuntimeSnapshot {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.listCalls++
	return append([]plugins.RuntimeSnapshot(nil), fake.snapshots...)
}

func (fake *fakePluginsBackend) PluginConfig(id string) (pluginapi.Config, bool) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.configCalls++
	config, ok := fake.configs[id]
	return config.Clone(), ok
}

func (fake *fakePluginsBackend) SetPluginEnabled(ctx context.Context, id string, enabled bool) error {
	fake.mu.Lock()
	fake.setCalls = append(fake.setCalls, fakeSetEnabledCall{id: id, enabled: enabled})
	callback, configuredErr := fake.setFn, fake.setErr
	fake.mu.Unlock()
	if callback != nil {
		if err := callback(ctx, id, enabled); err != nil {
			return err
		}
	} else if configuredErr != nil {
		return configuredErr
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for index := range fake.snapshots {
		if fake.snapshots[index].ID == id {
			fake.snapshots[index].Enabled = enabled
			return nil
		}
	}
	return plugins.ErrUnknownPlugin
}

func (fake *fakePluginsBackend) UpdatePluginConfig(ctx context.Context, id string, config pluginapi.Config) error {
	owned := config.Clone()
	fake.mu.Lock()
	fake.updateCalls = append(fake.updateCalls, fakeUpdateConfigCall{id: id, config: owned})
	callback, configuredErr := fake.updateFn, fake.updateErr
	fake.mu.Unlock()
	if callback != nil {
		if err := callback(ctx, id, owned.Clone()); err != nil {
			return err
		}
	} else if configuredErr != nil {
		return configuredErr
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.configs == nil {
		fake.configs = make(map[string]pluginapi.Config)
	}
	if _, ok := fake.configs[id]; !ok {
		return plugins.ErrUnknownPlugin
	}
	fake.configs[id] = owned.Clone()
	for index := range fake.snapshots {
		if fake.snapshots[index].ID == id {
			fake.snapshots[index].ConfigRevision = owned.Revision
		}
	}
	return nil
}

func (fake *fakePluginsBackend) SubscribePlugins(context.Context) <-chan []plugins.RuntimeSnapshot {
	updates := make(chan []plugins.RuntimeSnapshot)
	close(updates)
	return updates
}

func TestPluginsAPIExposesOnlyApprovedWailsMethods(t *testing.T) {
	typeOfAPI := reflect.TypeOf((*PluginsAPI)(nil))
	got := make([]string, 0, typeOfAPI.NumMethod())
	for index := 0; index < typeOfAPI.NumMethod(); index++ {
		got = append(got, typeOfAPI.Method(index).Name)
	}
	want := []string{"GetConfig", "List", "SetEnabled", "UpdateConfig"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("exported PluginsAPI methods = %v, want %v", got, want)
	}

	type fieldContract struct {
		name, typeName, jsonTag string
	}
	wantFields := []fieldContract{
		{"ID", "string", "id"}, {"Name", "string", "name"}, {"Description", "string", "description"},
		{"Version", "string", "version"}, {"Capabilities", "[]string", "capabilities"},
		{"Enabled", "bool", "enabled"}, {"Active", "bool", "active"}, {"State", "string", "state"},
		{"ConfigRevision", "uint64", "configRevision"}, {"FrameRate", "float64", "frameRate"},
		{"ConsecutiveFailures", "int", "consecutiveFailures"}, {"RestartCount", "int", "restartCount"},
		{"StartedAt", "*time.Time", "startedAt,omitempty"}, {"LastHeartbeatAt", "*time.Time", "lastHeartbeatAt,omitempty"},
		{"LastFrameAt", "*time.Time", "lastFrameAt,omitempty"}, {"NextRestartAt", "*time.Time", "nextRestartAt,omitempty"},
		{"LastError", "string", "lastError,omitempty"},
	}
	typeOfDTO := reflect.TypeOf(PluginDTO{})
	gotFields := make([]fieldContract, typeOfDTO.NumField())
	for index := 0; index < typeOfDTO.NumField(); index++ {
		field := typeOfDTO.Field(index)
		gotFields[index] = fieldContract{field.Name, field.Type.String(), field.Tag.Get("json")}
	}
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("PluginDTO allowlist = %#v, want %#v", gotFields, wantFields)
	}
}

func TestPluginsAPIListIsSortedOwnedBoundedAndAllowlisted(t *testing.T) {
	started := time.Unix(100, 0).UTC()
	longError := strings.Repeat("x", maxProblemMessageBytes+20)
	fake := &fakePluginsBackend{
		snapshots: []plugins.RuntimeSnapshot{
			{
				ID: "vendor.beta", Name: "Beta", State: plugins.StateRunning,
				Capabilities: trackingmodel.Capability(1 << 20), FrameRate: math.Inf(1),
				PID: 9001, SessionID: 44, SubscriptionGeneration: 8,
			},
			{
				ID: "vendor.alpha", Name: "Alpha", Description: "desc", Version: "1.2.3",
				Capabilities: trackingmodel.CapabilityLip | trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
				Enabled:      true, Active: true, State: plugins.StateBackoff, ConfigRevision: 4,
				FrameRate: math.NaN(), ConsecutiveFailures: 2, RestartCount: 3,
				StartedAt: started, LastHeartbeatAt: started.Add(time.Second), LastFrameAt: started.Add(2 * time.Second),
				NextRestartAt: started.Add(3 * time.Second), LastError: longError,
				PID: 123, SessionID: 9, SubscriptionGeneration: 11,
			},
		},
		configs: map[string]pluginapi.Config{"vendor.alpha": {Revision: 4, Data: json.RawMessage(`{"secret":true}`)}},
	}
	api := attachedPluginsAPI(t, context.Background(), fake)

	response := api.List()
	if response.Problem != nil || len(response.Plugins) != 2 || response.Plugins[0].ID != "vendor.alpha" || response.Plugins[1].ID != "vendor.beta" {
		t.Fatalf("List() = %+v", response)
	}
	alpha := response.Plugins[0]
	if !reflect.DeepEqual(alpha.Capabilities, []string{"eye", "expression", "lip"}) {
		t.Fatalf("capabilities = %v", alpha.Capabilities)
	}
	if alpha.FrameRate != 0 || response.Plugins[1].FrameRate != 0 {
		t.Fatalf("non-finite frame rates escaped: %v, %v", alpha.FrameRate, response.Plugins[1].FrameRate)
	}
	if len(alpha.LastError) != maxProblemMessageBytes || alpha.StartedAt == nil || !alpha.StartedAt.Equal(started) {
		t.Fatalf("bounded/timestamp conversion = %+v", alpha)
	}
	if len(response.Plugins[1].Capabilities) != 0 {
		t.Fatalf("unknown capability escaped: %v", response.Plugins[1].Capabilities)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"pid", "session", "subscription", "secret", "configData"} {
		if strings.Contains(strings.ToLower(string(encoded)), strings.ToLower(forbidden)) {
			t.Fatalf("List JSON leaked %q: %s", forbidden, encoded)
		}
	}
	response.Plugins[0].Capabilities[0] = "mutated"
	response.Plugins[0].StartedAt = timePointer(time.Unix(1, 0))
	if again := api.List(); again.Plugins[0].Capabilities[0] != "eye" || !again.Plugins[0].StartedAt.Equal(started) {
		t.Fatalf("List ownership leaked: %+v", again.Plugins[0])
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.configCalls != 0 {
		t.Fatalf("List construction called PluginConfig %d times", fake.configCalls)
	}
}

func TestPluginsAPIGetConfigIsOwnedAndBounded(t *testing.T) {
	data := json.RawMessage(`{"gain":2}`)
	fake := &fakePluginsBackend{
		snapshots: []plugins.RuntimeSnapshot{{ID: "vendor.alpha", ConfigRevision: 4}},
		configs:   map[string]pluginapi.Config{"vendor.alpha": {Revision: 4, Data: data}},
	}
	api := attachedPluginsAPI(t, context.Background(), fake)
	response := api.GetConfig("vendor.alpha")
	if response.Problem != nil || response.PluginID != "vendor.alpha" || response.ConfigRevision != 4 || response.Data != `{"gain":2}` {
		t.Fatalf("GetConfig() = %+v", response)
	}
	data[2] = 'X'
	if response.Data != `{"gain":2}` {
		t.Fatalf("backend mutation changed returned owned config: %+v", response)
	}

	fake.mu.Lock()
	fake.configs["vendor.alpha"] = pluginapi.Config{Revision: 5, Data: json.RawMessage(strings.Repeat("x", userconfig.MaxPluginConfigBytes+1))}
	fake.mu.Unlock()
	oversized := api.GetConfig("vendor.alpha")
	if oversized.Problem == nil || oversized.Problem.Code != ProblemInternal || oversized.Data != "" {
		t.Fatalf("oversized GetConfig = %+v", oversized)
	}
}

func TestPluginsAPIRejectsInvalidCallerPluginIDsBeforeAdmissionOrEcho(t *testing.T) {
	fake := &fakePluginsBackend{
		snapshots: []plugins.RuntimeSnapshot{{ID: "vendor.alpha"}},
		configs:   map[string]pluginapi.Config{"vendor.alpha": {Revision: 1, Data: json.RawMessage(`{}`)}},
	}
	api := attachedPluginsAPI(t, context.Background(), fake)
	for _, id := range []string{"", "Vendor.alpha", "vendor..alpha", strings.Repeat("a", 257)} {
		t.Run(fmt.Sprintf("%q", id), func(t *testing.T) {
			got := api.GetConfig(id)
			if got.Problem == nil || got.Problem.Code != ProblemValidation || got.Problem.Field != "pluginId" || got.PluginID != "" {
				t.Fatalf("GetConfig(%q) = %+v", id, got)
			}
			mutation := api.SetEnabled(id, true)
			if mutation.Problem == nil || mutation.Problem.Code != ProblemValidation || mutation.Problem.Field != "pluginId" || mutation.PluginID != "" {
				t.Fatalf("SetEnabled(%q) = %+v", id, mutation)
			}
			update := api.UpdateConfig(id, 1, `{}`)
			if update.Problem == nil || update.Problem.Code != ProblemValidation || update.Problem.Field != "pluginId" || update.PluginID != "" {
				t.Fatalf("UpdateConfig(%q) = %+v", id, update)
			}
		})
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.configCalls != 0 || len(fake.setCalls) != 0 || len(fake.updateCalls) != 0 {
		t.Fatalf("invalid caller ID reached backend: configs=%d set=%v update=%v", fake.configCalls, fake.setCalls, fake.updateCalls)
	}
}

func TestPluginsAPIConfigBoundaryRequiresValidUTF8AndPermitsExactCallerIDMaximum(t *testing.T) {
	maxID := strings.Repeat("a", 256)
	invalidUTF8JSON := string([]byte{'{', '"', 0xff, '"', '}'})
	fake := &fakePluginsBackend{
		snapshots: []plugins.RuntimeSnapshot{{ID: maxID}},
		configs:   map[string]pluginapi.Config{maxID: {Revision: 1, Data: json.RawMessage(`{}`)}},
	}
	api := attachedPluginsAPI(t, context.Background(), fake)
	if got := api.GetConfig(maxID); got.Problem != nil || got.PluginID != maxID {
		t.Fatalf("exact maximum caller ID = %+v", got)
	}
	if got := api.UpdateConfig(maxID, 1, invalidUTF8JSON); got.Problem == nil || got.Problem.Code != ProblemValidation || got.Problem.Field != "data" {
		t.Fatalf("invalid UTF-8 mutation = %+v", got)
	}
	fake.mu.Lock()
	fake.configs[maxID] = pluginapi.Config{Revision: 1, Data: json.RawMessage(invalidUTF8JSON)}
	fake.mu.Unlock()
	if got := api.GetConfig(maxID); got.Problem == nil || got.Problem.Code != ProblemInternal || got.Data != "" {
		t.Fatalf("invalid UTF-8 backend config = %+v", got)
	}
}

func TestPluginsAPIUnavailableUnknownTimeoutAndInternalProblemsAreSanitized(t *testing.T) {
	api := newPluginsAPI(nil)
	t.Cleanup(api.close)
	if got := api.GetConfig("vendor.alpha"); got.Problem == nil || got.Problem.Code != ProblemUnavailable {
		t.Fatalf("unavailable GetConfig = %+v", got)
	}
	if got := api.UpdateConfig("vendor.alpha", 1, "{"); got.Problem == nil || got.Problem.Code != ProblemUnavailable {
		t.Fatalf("unavailable UpdateConfig = %+v", got)
	}

	fake := &fakePluginsBackend{snapshots: []plugins.RuntimeSnapshot{{ID: "vendor.alpha"}}, configs: map[string]pluginapi.Config{}}
	api.attach(context.Background(), fake)
	if got := api.GetConfig("vendor.unknown"); got.Problem == nil || got.Problem.Code != ProblemNotFound {
		t.Fatalf("unknown GetConfig = %+v", got)
	}
	fake.setErr = errors.New("backend leaked credential=secret")
	if got := api.SetEnabled("vendor.alpha", true); got.Problem == nil || got.Problem.Code != ProblemInternal || strings.Contains(got.Problem.Message, "secret") {
		t.Fatalf("internal SetEnabled = %+v", got)
	}

	api.detach()
	rootCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	fake.setErr = nil
	fake.setFn = func(ctx context.Context, _ string, _ bool) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 2*time.Second {
			return errors.New("missing bounded command deadline")
		}
		<-ctx.Done()
		return ctx.Err()
	}
	api.attach(rootCtx, fake)
	if got := api.SetEnabled("vendor.alpha", true); got.Problem == nil || got.Problem.Code != ProblemTimeout {
		t.Fatalf("timed out SetEnabled = %+v", got)
	}
}

func TestPluginsAPISetEnabledIsIdempotentAndUpdatesModule(t *testing.T) {
	fake := &fakePluginsBackend{snapshots: []plugins.RuntimeSnapshot{{ID: "vendor.alpha", Enabled: true}}, configs: map[string]pluginapi.Config{}}
	api := attachedPluginsAPI(t, context.Background(), fake)
	before := api.List()

	if got := api.SetEnabled("vendor.alpha", true); got.Problem != nil || got.Revision != before.Revision {
		t.Fatalf("idempotent SetEnabled = %+v", got)
	}
	fake.mu.Lock()
	if len(fake.setCalls) != 0 {
		t.Fatalf("idempotent SetEnabled calls = %v", fake.setCalls)
	}
	fake.snapshots[0].Enabled = false
	fake.mu.Unlock()

	reconciled := api.SetEnabled("vendor.alpha", false)
	if reconciled.Problem != nil || reconciled.Revision != before.Revision+1 {
		t.Fatalf("idempotent reconcile = %+v", reconciled)
	}
	fake.mu.Lock()
	if len(fake.setCalls) != 0 {
		t.Fatalf("idempotent reconcile called backend mutation: %v", fake.setCalls)
	}
	fake.mu.Unlock()

	changed := api.SetEnabled("vendor.alpha", true)
	if changed.Problem != nil || changed.Revision != reconciled.Revision+1 {
		t.Fatalf("changed SetEnabled = %+v", changed)
	}
	if list := api.List(); !list.Plugins[0].Enabled || list.Revision != changed.Revision {
		t.Fatalf("module was not refreshed: %+v", list)
	}
	if got := api.SetEnabled("vendor.missing", true); got.Problem == nil || got.Problem.Code != ProblemNotFound {
		t.Fatalf("unknown SetEnabled = %+v", got)
	}
}

func TestPluginsAPIUpdateConfigValidatesBoundaryAndRevision(t *testing.T) {
	fake := &fakePluginsBackend{
		snapshots: []plugins.RuntimeSnapshot{{ID: "vendor.alpha", ConfigRevision: 4}},
		configs:   map[string]pluginapi.Config{"vendor.alpha": {Revision: 4, Data: json.RawMessage(`{"gain":1}`)}},
	}
	api := attachedPluginsAPI(t, context.Background(), fake)

	for _, test := range []struct {
		name string
		data string
	}{
		{name: "empty", data: ""},
		{name: "malformed", data: "{"},
		{name: "too large", data: strings.Repeat("x", userconfig.MaxPluginConfigBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := api.UpdateConfig("vendor.alpha", 4, test.data)
			if got.Problem == nil || got.Problem.Code != ProblemValidation || got.Problem.Field != "data" {
				t.Fatalf("UpdateConfig validation = %+v", got)
			}
		})
	}
	if got := api.UpdateConfig("vendor.alpha", 3, `{"gain":2}`); got.Problem == nil || got.Problem.Code != ProblemConflict || got.Problem.CurrentRevision != 4 {
		t.Fatalf("stale UpdateConfig = %+v", got)
	}

	fake.mu.Lock()
	fake.configs["vendor.alpha"] = pluginapi.Config{Revision: math.MaxUint64, Data: json.RawMessage(`{}`)}
	fake.mu.Unlock()
	if got := api.UpdateConfig("vendor.alpha", math.MaxUint64, `{}`); got.Problem == nil || got.Problem.Code != ProblemConflict || got.Problem.CurrentRevision != math.MaxUint64 {
		t.Fatalf("exhausted UpdateConfig = %+v", got)
	}
	fake.mu.Lock()
	if len(fake.updateCalls) != 0 {
		t.Fatalf("invalid updates reached backend: %+v", fake.updateCalls)
	}
	fake.configs["vendor.alpha"] = pluginapi.Config{Revision: 4, Data: json.RawMessage(`{"gain":1}`)}
	fake.mu.Unlock()

	response := api.UpdateConfig("vendor.alpha", 4, `{"gain":2}`)
	if response.Problem != nil {
		t.Fatalf("UpdateConfig success = %+v", response)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.updateCalls) != 1 || fake.updateCalls[0].config.Revision != 5 || string(fake.updateCalls[0].config.Data) != `{"gain":2}` {
		t.Fatalf("UpdatePluginConfig = %#v", fake.updateCalls)
	}
}

func TestPluginsAPIUpdateConfigSerializesSameIDAndRereadsInsideGate(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	fake := &fakePluginsBackend{
		snapshots: []plugins.RuntimeSnapshot{{ID: "vendor.alpha", ConfigRevision: 1}},
		configs:   map[string]pluginapi.Config{"vendor.alpha": {Revision: 1, Data: json.RawMessage(`{}`)}},
	}
	var once sync.Once
	fake.updateFn = func(_ context.Context, _ string, _ pluginapi.Config) error {
		once.Do(func() { close(entered) })
		<-release
		return nil
	}
	api := attachedPluginsAPI(t, context.Background(), fake)
	results := make(chan PluginMutationResponse, 2)
	go func() { results <- api.UpdateConfig("vendor.alpha", 1, `{"v":1}`) }()
	<-entered
	go func() { results <- api.UpdateConfig("vendor.alpha", 1, `{"v":2}`) }()
	time.Sleep(20 * time.Millisecond)
	fake.mu.Lock()
	if len(fake.updateCalls) != 1 {
		t.Fatalf("same-ID update was not serialized: %d calls", len(fake.updateCalls))
	}
	fake.mu.Unlock()
	close(release)
	first, second := <-results, <-results
	var successes, conflicts int
	for _, result := range []PluginMutationResponse{first, second} {
		if result.Problem == nil {
			successes++
		} else if result.Problem.Code == ProblemConflict && result.Problem.CurrentRevision == 2 {
			conflicts++
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("same-ID results = %+v, %+v", first, second)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.updateCalls) != 1 {
		t.Fatalf("stale queued update reached backend: %d calls", len(fake.updateCalls))
	}
}

func TestPluginsAPICommandsForDifferentIDsProceedConcurrently(t *testing.T) {
	entered := make(chan string, 2)
	release := make(chan struct{})
	fake := &fakePluginsBackend{
		snapshots: []plugins.RuntimeSnapshot{{ID: "vendor.alpha", ConfigRevision: 1}, {ID: "vendor.beta", ConfigRevision: 1}},
		configs: map[string]pluginapi.Config{
			"vendor.alpha": {Revision: 1, Data: json.RawMessage(`{}`)},
			"vendor.beta":  {Revision: 1, Data: json.RawMessage(`{}`)},
		},
		updateFn: func(_ context.Context, id string, _ pluginapi.Config) error {
			entered <- id
			<-release
			return nil
		},
	}
	api := attachedPluginsAPI(t, context.Background(), fake)
	results := make(chan PluginMutationResponse, 2)
	go func() { results <- api.UpdateConfig("vendor.alpha", 1, `{"v":1}`) }()
	go func() { results <- api.UpdateConfig("vendor.beta", 1, `{"v":2}`) }()
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case id := <-entered:
			seen[id] = true
		case <-time.After(time.Second):
			t.Fatalf("different IDs did not run concurrently: %v", seen)
		}
	}
	close(release)
	for range 2 {
		if got := <-results; got.Problem != nil {
			t.Fatalf("different-ID update = %+v", got)
		}
	}
}

func TestPluginsAPIManagerRemainsFinalConfigAuthority(t *testing.T) {
	fake := &fakePluginsBackend{
		snapshots: []plugins.RuntimeSnapshot{{ID: "vendor.alpha", ConfigRevision: 4}},
		configs:   map[string]pluginapi.Config{"vendor.alpha": {Revision: 4, Data: json.RawMessage(`{}`)}},
		updateErr: plugins.ErrConfigRevisionConflict,
	}
	api := attachedPluginsAPI(t, context.Background(), fake)
	before := api.List()
	got := api.UpdateConfig("vendor.alpha", 4, `{"v":2}`)
	if got.Problem == nil || got.Problem.Code != ProblemConflict || api.List().Revision != before.Revision {
		t.Fatalf("manager conflict = %+v", got)
	}
}

func TestPluginsAPISnapshotConsumerPublishesSemanticLatestOwnedLists(t *testing.T) {
	fake := &fakePluginsBackend{snapshots: []plugins.RuntimeSnapshot{{ID: "vendor.alpha", Name: "Alpha"}}, configs: map[string]pluginapi.Config{}}
	api := attachedPluginsAPI(t, context.Background(), fake)
	ctx, cancel := context.WithCancel(context.Background())
	updates := api.subscribe(ctx)
	t.Cleanup(cancel)
	initial := <-updates
	baselineRevision := initial.Revision

	sourceCtx, sourceCancel := context.WithCancel(context.Background())
	source := make(chan []plugins.RuntimeSnapshot, 1)
	api.consumeSnapshots(sourceCtx, source)
	source <- []plugins.RuntimeSnapshot{{ID: "vendor.alpha", Name: "Alpha"}}
	close(source)
	api.waitConsumers()
	if got := api.List(); got.Revision != baselineRevision {
		t.Fatalf("identical snapshot changed revision: %d -> %d", baselineRevision, got.Revision)
	}
	sourceCancel()

	burstCtx, burstCancel := context.WithCancel(context.Background())
	burst := make(chan []plugins.RuntimeSnapshot, 1)
	offerPluginSnapshots(burst, []plugins.RuntimeSnapshot{{ID: "vendor.alpha", Name: "stale"}})
	offerPluginSnapshots(burst, []plugins.RuntimeSnapshot{{ID: "vendor.beta", Name: "Beta", Capabilities: trackingmodel.CapabilityLip}, {ID: "vendor.alpha", Name: "Latest"}})
	api.consumeSnapshots(burstCtx, burst)
	var latest PluginListResponse
	select {
	case latest = <-updates:
	case <-time.After(time.Second):
		t.Fatal("snapshot consumer did not publish")
	}
	if latest.Revision != baselineRevision+1 || len(latest.Plugins) != 2 || latest.Plugins[0].Name != "Latest" || latest.Plugins[1].ID != "vendor.beta" {
		t.Fatalf("latest snapshot = %+v", latest)
	}
	latest.Plugins[1].Capabilities[0] = "mutated"
	if got := api.List(); got.Plugins[1].Capabilities[0] != "lip" {
		t.Fatalf("event ownership leaked: %+v", got)
	}

	burstCancel()
	api.waitConsumers()
	revision := api.List().Revision
	offerPluginSnapshots(burst, []plugins.RuntimeSnapshot{{ID: "vendor.after-cancel"}})
	if got := api.List(); got.Revision != revision {
		t.Fatalf("canceled consumer updated module: %+v", got)
	}
}

func TestPluginsAPIDetachRejectsAdmissionCancelsAndJoinsCommands(t *testing.T) {
	entered := make(chan struct{})
	fake := &fakePluginsBackend{
		snapshots: []plugins.RuntimeSnapshot{{ID: "vendor.alpha"}},
		configs:   map[string]pluginapi.Config{},
		setFn: func(ctx context.Context, _ string, _ bool) error {
			close(entered)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	api := attachedPluginsAPI(t, context.Background(), fake)
	result := make(chan PluginMutationResponse, 1)
	go func() { result <- api.SetEnabled("vendor.alpha", true) }()
	<-entered
	detached := make(chan struct{})
	go func() {
		api.detach()
		close(detached)
	}()
	select {
	case <-detached:
	case <-time.After(time.Second):
		t.Fatal("detach did not cancel and join command")
	}
	if got := <-result; got.Problem == nil || got.Problem.Code != ProblemUnavailable {
		t.Fatalf("canceled command = %+v", got)
	}
	started := time.Now()
	if got := api.SetEnabled("vendor.alpha", true); got.Problem == nil || got.Problem.Code != ProblemUnavailable {
		t.Fatalf("post-detach command = %+v", got)
	}
	if time.Since(started) > 100*time.Millisecond {
		t.Fatal("post-detach admission was not prompt")
	}
	api.close()
	api.close()
}

func attachedPluginsAPI(t *testing.T, ctx context.Context, fake *fakePluginsBackend) *PluginsAPI {
	t.Helper()
	api := newPluginsAPI(nil)
	api.attach(ctx, fake)
	t.Cleanup(api.close)
	return api
}

func offerPluginSnapshots(out chan []plugins.RuntimeSnapshot, snapshots []plugins.RuntimeSnapshot) {
	select {
	case <-out:
	default:
	}
	select {
	case out <- append([]plugins.RuntimeSnapshot(nil), snapshots...):
	default:
	}
}

func timePointer(value time.Time) *time.Time { return &value }
