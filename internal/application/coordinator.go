package application

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/wzhqwq/vrcft-go/internal/avatar"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/internal/tracking"
)

type coordinatorInputs struct {
	avatarChanges <-chan osc.AvatarChange
	oscEvents     <-chan osc.ControllerEvent
	pluginEvents  <-chan plugins.Event
	merged        <-chan tracking.MergedFrame
	ticks         <-chan time.Time
}

type framePipeline interface {
	ProcessAt(tracking.MergedFrame, int64) (processing.CanonicalFrame, error)
}

type sourceRemover interface {
	RemoveSource(string)
}

type runtimePublisher interface {
	catalogControl
	Publish(uint64, osc.ValueSource) error
	Status() osc.OSCStatus
}

type coordinator struct {
	planner          activationPlanner
	installer        *planInstaller
	pipeline         framePipeline
	tracking         sourceRemover
	runtime          runtimePublisher
	status           *statusStore
	clock            *monotonicClock
	current          planView
	latest           tracking.MergedFrame
	hasLatest        bool
	suspended        bool
	outputBlocked    bool
	pluginSessionsMu sync.Mutex
	pluginSessions   map[string]uint64
}

func (c *coordinator) run(ctx context.Context, inputs coordinatorInputs, ready chan<- struct{}) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.clock == nil {
		c.clock = newMonotonicClock(nil)
	}
	if ready != nil {
		close(ready)
	}

	avatarChanges := inputs.avatarChanges
	oscEvents := inputs.oscEvents
	pluginEvents := inputs.pluginEvents
	merged := inputs.merged
	ticks := inputs.ticks

	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case change, ok := <-avatarChanges:
			if !ok {
				avatarChanges = nil
				continue
			}
			c.activate(ctx, change)
		case event, ok := <-oscEvents:
			if !ok {
				oscEvents = nil
				continue
			}
			c.observeOSC(event)
		case event, ok := <-pluginEvents:
			if !ok {
				pluginEvents = nil
				continue
			}
			c.observePlugin(event)
		case frame, ok := <-merged:
			if !ok {
				merged = nil
				continue
			}
			c.latest = frame
			c.hasLatest = true
			c.process(ctx, frame, c.clock.now(true))
		case tick, ok := <-ticks:
			if !ok {
				ticks = nil
				continue
			}
			if c.hasLatest {
				c.process(ctx, c.latest, c.clock.advance(tick))
			}
		}
	}
}

func (c *coordinator) activate(ctx context.Context, change osc.AvatarChange) {
	if ctx.Err() != nil {
		return
	}
	current := c.planner.Activate(change.AvatarID)
	if ctx.Err() != nil {
		return
	}
	outcome := c.installer.install(ctx, current)
	c.current = outcome.plan
	c.outputBlocked = outcome.outputBlocked
	c.suspended = !c.outputBlocked && outcome.runtimeErr != nil && usablePlan(outcome.plan)
	c.publishInstallStatus(change.AvatarID, outcome)
}

func (c *coordinator) process(ctx context.Context, frame tracking.MergedFrame, nowNS int64) {
	if ctx.Err() != nil || !c.hasUsablePlan(frame.Generation) {
		return
	}

	generation := c.current.Generation()
	canonical, err := c.pipeline.ProcessAt(frame, nowNS)
	if err != nil {
		c.failRuntime(fmt.Errorf("process generation %d: %w", generation, err))
		return
	}
	if canonical.Generation != generation {
		c.failRuntime(fmt.Errorf("processed generation %d does not match plan generation %d", canonical.Generation, generation))
		return
	}

	evaluatorPlan := c.current.Evaluator()
	if evaluatorPlan == nil {
		c.failRuntime(fmt.Errorf("plan generation %d has no evaluator", generation))
		return
	}
	snapshot := evaluatorPlan.Evaluate(canonical)
	if c.suspended {
		if err := c.runtime.InstallCatalog(c.current.Catalog()); err != nil {
			c.failRuntime(fmt.Errorf("recover OSC catalog generation %d: %w", generation, err))
			return
		}
	}
	if err := c.runtime.Publish(generation, snapshot); err != nil {
		c.failRuntime(fmt.Errorf("publish OSC generation %d: %w", generation, err))
		return
	}
	c.suspended = false
	c.status.update(func(status *Status) {
		status.OSC = c.runtime.Status()
		status.RuntimeError = ""
		status.Lifecycle = lifecycleForStatus(status)
	})
}

func (c *coordinator) hasUsablePlan(generation uint64) bool {
	return !c.outputBlocked && usablePlan(c.current) && generation == c.current.Generation()
}

func usablePlan(plan planView) bool {
	if plan == nil || plan.Status() != avatar.StatusReady || plan.Generation() == 0 || plan.Evaluator() == nil {
		return false
	}
	catalog := plan.Catalog()
	return catalog != nil && len(catalog.Bindings) != 0 && catalog.Generation == plan.Generation()
}

func (c *coordinator) failRuntime(err error) {
	c.runtime.ClearRuntime()
	c.suspended = true
	c.status.update(func(status *Status) {
		status.OSC = c.runtime.Status()
		status.RuntimeError = coordinatorErrorMessage(err)
		status.Lifecycle = lifecycleForStatus(status)
	})
}

func (c *coordinator) publishInstallStatus(requestedAvatarID string, outcome installOutcome) {
	c.status.update(func(status *Status) {
		status.AvatarID = requestedAvatarID
		status.PlanGeneration = 0
		status.PlanStatus = 0
		status.PlanSource = 0
		status.ConfigPath = ""
		status.ConfigID = ""
		if outcome.plan != nil {
			status.AvatarID = outcome.plan.AvatarID()
			status.PlanGeneration = outcome.plan.Generation()
			status.PlanStatus = outcome.plan.Status()
			status.PlanSource = outcome.plan.Source()
			status.ConfigPath = outcome.plan.ConfigPath()
			status.ConfigID = outcome.plan.ConfigID()
		}
		status.GenerationExhausted = outcome.exhausted
		status.PluginFailures = append([]PluginControlFailure(nil), outcome.pluginFailures...)
		status.PlanError = coordinatorErrorMessage(outcome.planErr)
		status.RuntimeError = coordinatorErrorMessage(outcome.runtimeErr)
		status.OSC = c.runtime.Status()
		status.Lifecycle = lifecycleForStatus(status)
	})
}

func (c *coordinator) observePlugin(event plugins.Event) {
	pluginID := event.PluginID
	if pluginID == "" && event.Snapshot != nil {
		pluginID = event.Snapshot.ID
	}
	if event.Type == plugins.EventPluginRemoved {
		c.pluginSessionsMu.Lock()
		delete(c.pluginSessions, pluginID)
		c.pluginSessionsMu.Unlock()
		c.tracking.RemoveSource(pluginID)
		return
	}

	sessionChanged := false
	staleSession := false
	if event.Snapshot != nil && event.Snapshot.SessionID != 0 {
		c.pluginSessionsMu.Lock()
		if c.pluginSessions == nil {
			c.pluginSessions = make(map[string]uint64)
		}
		previous, observed := c.pluginSessions[pluginID]
		switch {
		case observed && event.Snapshot.SessionID < previous:
			staleSession = true
		case observed && event.Snapshot.SessionID > previous:
			sessionChanged = true
			c.pluginSessions[pluginID] = event.Snapshot.SessionID
		case !observed:
			c.pluginSessions[pluginID] = event.Snapshot.SessionID
		}
		c.pluginSessionsMu.Unlock()
	}
	if staleSession {
		return
	}
	if sessionChanged || event.Snapshot != nil &&
		(event.Snapshot.State != plugins.StateRunning || !event.Snapshot.Active) {
		c.tracking.RemoveSource(pluginID)
	}
}

func (c *coordinator) observeOSC(event osc.ControllerEvent) {
	c.status.update(func(status *Status) {
		status.OSC = c.runtime.Status()
		if event.Kind == osc.EventError {
			if event.Err != nil {
				status.RuntimeError = coordinatorErrorMessage(event.Err)
			} else {
				status.RuntimeError = coordinatorErrorMessage(fmt.Errorf("%s", event.Message))
			}
			status.Lifecycle = lifecycleForStatus(status)
		}
	})
}

func lifecycleForStatus(status *Status) LifecycleState {
	if status.Lifecycle == LifecycleClosing || status.Lifecycle == LifecycleClosed {
		return status.Lifecycle
	}
	if status.GenerationExhausted || status.PlanError != "" || status.RuntimeError != "" || len(status.PluginFailures) != 0 {
		return LifecycleDegraded
	}
	return LifecycleRunning
}

func coordinatorErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToValidUTF8(err.Error(), "\uFFFD")
	message = strings.Join(strings.Fields(message), " ")
	const maxErrorBytes = 512
	if len(message) > maxErrorBytes {
		message = message[:maxErrorBytes]
		for !utf8.ValidString(message) {
			message = message[:len(message)-1]
		}
	}
	return message
}
