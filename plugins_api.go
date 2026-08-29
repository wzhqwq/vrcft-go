package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/application"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/userconfig"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

const pluginCommandTimeout = 2 * time.Second

type pluginsBackend interface {
	Plugins() []plugins.RuntimeSnapshot
	PluginConfig(string) (pluginapi.Config, bool)
	SetPluginEnabled(context.Context, string, bool) error
	UpdatePluginConfig(context.Context, string, pluginapi.Config) error
	SubscribePlugins(context.Context) <-chan []plugins.RuntimeSnapshot
}

type pluginCommandGate struct {
	token chan struct{}
	refs  int
}

// PluginsAPI is the Wails-safe plugin operations surface. The backend and all
// lifecycle/event seams stay package-private so only the four approved methods
// are bound.
type PluginsAPI struct {
	store *moduleStore[[]PluginDTO]

	lifecycleMu sync.Mutex
	mu          sync.Mutex
	backend     pluginsBackend
	session     context.Context
	cancel      context.CancelFunc
	generation  uint64
	accepting   bool
	closed      bool
	gates       map[string]*pluginCommandGate
	commandsWG  sync.WaitGroup
	consumersWG sync.WaitGroup

	snapshotMu sync.Mutex

	apiContext context.Context
	apiCancel  context.CancelFunc
	closeOnce  sync.Once
	subsWG     sync.WaitGroup
}

func newPluginsAPI(now func() time.Time) *PluginsAPI {
	apiContext, apiCancel := context.WithCancel(context.Background())
	return &PluginsAPI{
		store:      newModuleStore([]PluginDTO{}, clonePluginDTOs, now),
		gates:      make(map[string]*pluginCommandGate),
		apiContext: apiContext,
		apiCancel:  apiCancel,
	}
}

// List returns the last complete owned plugin module snapshot without calling
// the backend or exposing plugin-private configuration.
func (api *PluginsAPI) List() PluginListResponse {
	envelope := api.store.snapshot()
	if problem := api.unavailableProblem(envelope.Revision); problem != nil {
		return pluginListResponse(envelope, problem)
	}
	return pluginListResponse(envelope, envelope.Problem)
}

// GetConfig is the sole plugin API method that returns plugin-owned JSON.
func (api *PluginsAPI) GetConfig(pluginID string) PluginConfigResponse {
	command, err := api.admit(pluginID)
	if err != nil {
		return api.pluginConfigFailure(pluginID, err)
	}
	defer command.release()

	config, ok := command.backend.PluginConfig(pluginID)
	if !ok {
		return api.pluginConfigFailure(pluginID, plugins.ErrUnknownPlugin)
	}
	if len(config.Data) > userconfig.MaxPluginConfigBytes || (len(config.Data) != 0 && !json.Valid(config.Data)) {
		return api.pluginConfigFailure(pluginID, errors.New("plugin configuration violates the public boundary"))
	}
	envelope := api.store.snapshot()
	return PluginConfigResponse{
		Revision:       envelope.Revision,
		UpdatedAt:      envelope.UpdatedAt,
		PluginID:       pluginID,
		ConfigRevision: config.Revision,
		Data:           string(append([]byte(nil), config.Data...)),
	}
}

// SetEnabled changes a plugin's durable desired state. Equal desired state is
// an idempotent success and never calls the backend mutation method.
func (api *PluginsAPI) SetEnabled(pluginID string, enabled bool) PluginMutationResponse {
	command, err := api.admit(pluginID)
	if err != nil {
		return api.pluginMutationFailure(pluginID, err, 0)
	}
	defer command.release()

	snapshots := command.backend.Plugins()
	snapshot, ok := findPluginSnapshot(snapshots, pluginID)
	if !ok {
		return api.pluginMutationFailure(pluginID, plugins.ErrUnknownPlugin, 0)
	}
	if snapshot.Enabled == enabled {
		envelope := api.publishSnapshots(snapshots, command.generation, false)
		return pluginMutationResponse(envelope, pluginID, nil)
	}
	if err := command.backend.SetPluginEnabled(command.ctx, pluginID, enabled); err != nil {
		return api.pluginMutationFailure(pluginID, api.commandError(command, err), 0)
	}
	envelope := api.refreshBackend(command.backend, command.generation, false)
	return pluginMutationResponse(envelope, pluginID, nil)
}

// UpdateConfig validates public JSON, performs a per-plugin optimistic check,
// creates the next revision, and leaves the Manager as final conflict authority.
func (api *PluginsAPI) UpdateConfig(pluginID string, expectedConfigRevision uint64, data string) PluginMutationResponse {
	command, err := api.admit(pluginID)
	if err != nil {
		return api.pluginMutationFailure(pluginID, err, 0)
	}
	defer command.release()

	if len(data) == 0 {
		return api.pluginMutationFailure(pluginID, pluginDataValidation("required"), 0)
	}
	if len(data) > userconfig.MaxPluginConfigBytes {
		return api.pluginMutationFailure(pluginID, pluginDataValidation("exceeds 64 KiB"), 0)
	}
	encoded := []byte(data)
	if !json.Valid(encoded) {
		return api.pluginMutationFailure(pluginID, pluginDataValidation("must contain valid JSON"), 0)
	}

	current, ok := command.backend.PluginConfig(pluginID)
	if !ok {
		return api.pluginMutationFailure(pluginID, plugins.ErrUnknownPlugin, 0)
	}
	if expectedConfigRevision != current.Revision {
		return api.pluginMutationFailure(pluginID, plugins.ErrConfigRevisionConflict, current.Revision)
	}
	if current.Revision == math.MaxUint64 {
		return api.pluginMutationFailure(pluginID, userconfig.ErrRevisionExhausted, current.Revision)
	}
	next := pluginapi.Config{Revision: current.Revision + 1, Data: append(json.RawMessage(nil), encoded...)}
	if err := command.backend.UpdatePluginConfig(command.ctx, pluginID, next.Clone()); err != nil {
		currentRevision := current.Revision
		if latest, exists := command.backend.PluginConfig(pluginID); exists {
			currentRevision = latest.Revision
		}
		return api.pluginMutationFailure(pluginID, api.commandError(command, err), currentRevision)
	}
	envelope := api.refreshBackend(command.backend, command.generation, false)
	return pluginMutationResponse(envelope, pluginID, nil)
}

// attach enables commands against one running backend. It is intentionally
// unexported; root lifecycle wiring owns when a backend becomes available.
func (api *PluginsAPI) attach(root context.Context, backend pluginsBackend) {
	api.lifecycleMu.Lock()
	defer api.lifecycleMu.Unlock()
	api.stopSession(false)
	if root == nil {
		root = context.Background()
	}
	if backend == nil || root.Err() != nil {
		return
	}
	session, cancel := context.WithCancel(root)
	api.mu.Lock()
	if api.closed {
		api.mu.Unlock()
		cancel()
		return
	}
	api.generation++
	generation := api.generation
	api.backend = backend
	api.session = session
	api.cancel = cancel
	api.accepting = true
	api.mu.Unlock()
	api.refreshBackend(backend, generation, true)
}

// detach rejects new commands, cancels admitted work and snapshot consumers,
// and joins API-owned goroutines before returning.
func (api *PluginsAPI) detach() {
	api.lifecycleMu.Lock()
	defer api.lifecycleMu.Unlock()
	api.stopSession(true)
}

func (api *PluginsAPI) stopSession(publishUnavailable bool) {
	api.mu.Lock()
	wasAttached := api.backend != nil || api.accepting
	api.accepting = false
	api.backend = nil
	api.session = nil
	cancel := api.cancel
	api.cancel = nil
	api.generation++
	api.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	api.commandsWG.Wait()
	api.consumersWG.Wait()
	if publishUnavailable && wasAttached {
		envelope := api.store.snapshot()
		problem := sanitizeProblem(application.ErrInvalidLifecycle, envelope.Revision)
		api.store.update(envelope.Value, &problem)
	}
}

// consumeSnapshots owns one backend snapshot consumer until its parent,
// current backend session, or the API closes.
func (api *PluginsAPI) consumeSnapshots(parent context.Context, source <-chan []plugins.RuntimeSnapshot) {
	if parent == nil {
		parent = context.Background()
	}
	api.mu.Lock()
	if api.closed || !api.accepting || source == nil {
		api.mu.Unlock()
		return
	}
	generation := api.generation
	session := api.session
	api.consumersWG.Add(1)
	api.mu.Unlock()

	ctx, cancel := context.WithCancel(parent)
	stopSession := context.AfterFunc(session, cancel)
	stopAPI := context.AfterFunc(api.apiContext, cancel)
	go func() {
		defer api.consumersWG.Done()
		defer cancel()
		defer stopSession()
		defer stopAPI()
		for {
			select {
			case <-ctx.Done():
				return
			case snapshots, ok := <-source:
				if !ok {
					return
				}
				if ctx.Err() == nil {
					api.publishSnapshots(snapshots, generation, false)
				}
			}
		}
	}()
}

func (api *PluginsAPI) waitConsumers() { api.consumersWG.Wait() }

// subscribe exposes the module's owned capacity-one stream to the root event
// forwarder without binding generic store values through Wails.
func (api *PluginsAPI) subscribe(parent context.Context) <-chan PluginListResponse {
	updates := make(chan PluginListResponse, 1)
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	stopAPI := context.AfterFunc(api.apiContext, cancel)
	api.mu.Lock()
	if api.closed {
		api.mu.Unlock()
		stopAPI()
		cancel()
		close(updates)
		return updates
	}
	api.subsWG.Add(1)
	api.mu.Unlock()
	source := api.store.subscribe(ctx)
	go func() {
		defer api.subsWG.Done()
		defer stopAPI()
		defer cancel()
		defer close(updates)
		for {
			select {
			case <-ctx.Done():
				return
			case envelope, ok := <-source:
				if !ok {
					return
				}
				problem := envelope.Problem
				if unavailable := api.unavailableProblem(envelope.Revision); unavailable != nil {
					problem = unavailable
				}
				offerPluginListResponse(updates, pluginListResponse(envelope, problem))
			}
		}
	}()
	return updates
}

func (api *PluginsAPI) close() {
	api.closeOnce.Do(func() {
		api.lifecycleMu.Lock()
		api.mu.Lock()
		api.closed = true
		api.mu.Unlock()
		api.apiCancel()
		api.stopSession(false)
		api.lifecycleMu.Unlock()
		api.subsWG.Wait()
	})
}

type admittedPluginCommand struct {
	api        *PluginsAPI
	backend    pluginsBackend
	ctx        context.Context
	cancel     context.CancelFunc
	gate       *pluginCommandGate
	generation uint64
	held       bool
}

func (api *PluginsAPI) admit(pluginID string) (*admittedPluginCommand, error) {
	api.mu.Lock()
	if api.closed || !api.accepting || api.backend == nil || api.session == nil {
		api.mu.Unlock()
		return nil, application.ErrInvalidLifecycle
	}
	backend := api.backend
	session := api.session
	generation := api.generation
	gate := api.gates[pluginID]
	if gate == nil {
		gate = &pluginCommandGate{token: make(chan struct{}, 1)}
		gate.token <- struct{}{}
		api.gates[pluginID] = gate
	}
	gate.refs++
	api.commandsWG.Add(1)
	api.mu.Unlock()

	ctx, cancel := context.WithTimeout(session, pluginCommandTimeout)
	command := &admittedPluginCommand{api: api, backend: backend, ctx: ctx, cancel: cancel, gate: gate, generation: generation}
	select {
	case <-ctx.Done():
		command.release()
		return nil, ctx.Err()
	case <-gate.token:
		command.held = true
	}
	api.mu.Lock()
	valid := !api.closed && api.accepting && api.backend == backend && api.generation == generation
	api.mu.Unlock()
	if !valid {
		command.release()
		return nil, application.ErrInvalidLifecycle
	}
	if err := ctx.Err(); err != nil {
		command.release()
		return nil, err
	}
	return command, nil
}

func (command *admittedPluginCommand) release() {
	if command == nil || command.api == nil {
		return
	}
	api := command.api
	if command.held {
		command.gate.token <- struct{}{}
	}
	command.cancel()
	api.mu.Lock()
	command.gate.refs--
	if command.gate.refs == 0 {
		for id, gate := range api.gates {
			if gate == command.gate {
				delete(api.gates, id)
				break
			}
		}
	}
	command.api = nil
	api.mu.Unlock()
	api.commandsWG.Done()
}

func (api *PluginsAPI) commandError(command *admittedPluginCommand, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(command.ctx.Err(), context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(err, context.Canceled) || errors.Is(command.ctx.Err(), context.Canceled) {
		return application.ErrInvalidLifecycle
	}
	return err
}

func (api *PluginsAPI) refreshBackend(backend pluginsBackend, generation uint64, force bool) moduleEnvelope[[]PluginDTO] {
	api.snapshotMu.Lock()
	defer api.snapshotMu.Unlock()
	return api.publishSnapshotsLocked(backend.Plugins(), generation, force)
}

func (api *PluginsAPI) publishSnapshots(snapshots []plugins.RuntimeSnapshot, generation uint64, force bool) moduleEnvelope[[]PluginDTO] {
	api.snapshotMu.Lock()
	defer api.snapshotMu.Unlock()
	return api.publishSnapshotsLocked(snapshots, generation, force)
}

func (api *PluginsAPI) publishSnapshotsLocked(snapshots []plugins.RuntimeSnapshot, generation uint64, force bool) moduleEnvelope[[]PluginDTO] {
	api.mu.Lock()
	valid := !api.closed && api.accepting && api.generation == generation
	api.mu.Unlock()
	current := api.store.snapshot()
	if !valid {
		return current
	}
	converted := pluginDTOList(snapshots)
	if !force && current.Problem == nil && reflect.DeepEqual(current.Value, converted) {
		return current
	}
	return api.store.update(converted, nil)
}

func (api *PluginsAPI) unavailableProblem(currentRevision uint64) *Problem {
	api.mu.Lock()
	unavailable := api.closed || !api.accepting || api.backend == nil || api.session == nil || api.session.Err() != nil
	api.mu.Unlock()
	if !unavailable {
		return nil
	}
	problem := sanitizeProblem(application.ErrInvalidLifecycle, currentRevision)
	return &problem
}

func (api *PluginsAPI) pluginConfigFailure(pluginID string, err error) PluginConfigResponse {
	envelope := api.store.snapshot()
	problem := api.problem(err, envelope.Revision, 0)
	return PluginConfigResponse{Revision: envelope.Revision, UpdatedAt: envelope.UpdatedAt, PluginID: pluginID, Problem: &problem}
}

func (api *PluginsAPI) pluginMutationFailure(pluginID string, err error, currentConfigRevision uint64) PluginMutationResponse {
	envelope := api.store.snapshot()
	problem := api.problem(err, envelope.Revision, currentConfigRevision)
	return pluginMutationResponse(envelope, pluginID, &problem)
}

func (api *PluginsAPI) problem(err error, moduleRevision, currentConfigRevision uint64) Problem {
	if errors.Is(err, context.Canceled) {
		err = application.ErrInvalidLifecycle
	}
	current := moduleRevision
	if errors.Is(err, userconfig.ErrRevisionExhausted) || errors.Is(err, plugins.ErrConfigRevisionConflict) || errors.Is(err, plugins.ErrConfigRevisionRegression) {
		current = currentConfigRevision
	}
	return sanitizeProblem(err, current)
}

func pluginDataValidation(message string) error {
	return &userconfig.ValidationError{Field: "data", Err: errors.New(message)}
}

func findPluginSnapshot(snapshots []plugins.RuntimeSnapshot, id string) (plugins.RuntimeSnapshot, bool) {
	for _, snapshot := range snapshots {
		if snapshot.ID == id {
			return snapshot, true
		}
	}
	return plugins.RuntimeSnapshot{}, false
}

func pluginDTOList(snapshots []plugins.RuntimeSnapshot) []PluginDTO {
	result := make([]PluginDTO, len(snapshots))
	for index, snapshot := range snapshots {
		result[index] = pluginDTO(snapshot)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result
}

func clonePluginDTOs(values []PluginDTO) []PluginDTO {
	result := make([]PluginDTO, len(values))
	for index, value := range values {
		result[index] = value
		result[index].Capabilities = make([]string, len(value.Capabilities))
		copy(result[index].Capabilities, value.Capabilities)
		result[index].StartedAt = cloneTimePointer(value.StartedAt)
		result[index].LastHeartbeatAt = cloneTimePointer(value.LastHeartbeatAt)
		result[index].LastFrameAt = cloneTimePointer(value.LastFrameAt)
		result[index].NextRestartAt = cloneTimePointer(value.NextRestartAt)
	}
	return result
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func pluginListResponse(envelope moduleEnvelope[[]PluginDTO], problem *Problem) PluginListResponse {
	return PluginListResponse{
		Revision: envelope.Revision, UpdatedAt: envelope.UpdatedAt,
		Plugins: clonePluginDTOs(envelope.Value), Problem: cloneProblem(problem),
	}
}

func pluginMutationResponse(envelope moduleEnvelope[[]PluginDTO], pluginID string, problem *Problem) PluginMutationResponse {
	return PluginMutationResponse{
		Revision: envelope.Revision, UpdatedAt: envelope.UpdatedAt,
		PluginID: pluginID, Problem: cloneProblem(problem),
	}
}

func offerPluginListResponse(out chan PluginListResponse, response PluginListResponse) {
	select {
	case <-out:
	default:
	}
	select {
	case out <- response:
	default:
	}
}
