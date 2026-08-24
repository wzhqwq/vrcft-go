package application

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wzhqwq/vrcft-go/internal/avatar"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

type planPluginControls interface {
	List() []plugins.RuntimeSnapshot
	SetActive(context.Context, string, bool) error
	UpdateSubscription(context.Context, string, pluginapi.Subscription) error
}

type generationControl interface {
	SetGeneration(uint64) error
}

type catalogControl interface {
	ClearRuntime()
	InstallCatalog(*osc.Catalog) error
}

type planInstaller struct {
	plugins              planPluginControls
	tracking             generationControl
	osc                  catalogControl
	pluginControlTimeout time.Duration
}

type installOutcome struct {
	plan           planView
	planErr        error
	runtimeErr     error
	pluginFailures []PluginControlFailure
	exhausted      bool
	catalogReady   bool
	outputBlocked  bool
}

// maxPluginControlFailureMessageBytes bounds each status diagnostic while
// leaving enough room for actionable plugin and operation context.
const maxPluginControlFailureMessageBytes = 512

func (installer *planInstaller) install(ctx context.Context, current activation) installOutcome {
	outcome := installOutcome{
		plan:    current.plan,
		planErr: current.err,
	}
	if ctx == nil {
		ctx = context.Background()
	}

	installer.osc.ClearRuntime()
	plugins := installer.sortedPlugins()
	if current.plan == nil {
		outcome.exhausted = true
		installer.deactivateAll(ctx, plugins, nil, &outcome)
		return outcome
	}

	generation := current.plan.Generation()
	if err := installer.tracking.SetGeneration(generation); err != nil {
		outcome.runtimeErr = fmt.Errorf("advance tracking generation to %d: %w", generation, err)
		installer.deactivateAll(ctx, plugins, nil, &outcome)
		return outcome
	}

	failedDeactivation := make(map[string]bool)
	installer.deactivateAll(ctx, plugins, failedDeactivation, &outcome)
	if current.plan.Status() != avatar.StatusReady {
		return outcome
	}

	catalog := current.plan.Catalog()
	if catalog == nil || len(catalog.Bindings) == 0 {
		return outcome
	}

	for _, plugin := range plugins {
		if failedDeactivation[plugin.ID] {
			continue
		}
		subscription, matches := current.plan.SubscriptionFor(plugin.Capabilities)
		if !matches {
			continue
		}
		if err := installer.control(ctx, func(controlCtx context.Context) error {
			return installer.plugins.UpdateSubscription(controlCtx, plugin.ID, subscription)
		}); err != nil {
			outcome.addPluginFailure(plugin.ID, "subscription", err)
			continue
		}
		if err := installer.control(ctx, func(controlCtx context.Context) error {
			return installer.plugins.SetActive(controlCtx, plugin.ID, true)
		}); err != nil {
			outcome.addPluginFailure(plugin.ID, "activate", err)
			if compensationErr := installer.control(ctx, func(controlCtx context.Context) error {
				return installer.plugins.SetActive(controlCtx, plugin.ID, false)
			}); compensationErr != nil {
				outcome.addPluginFailure(plugin.ID, "deactivate", compensationErr)
				outcome.outputBlocked = true
			}
		}
	}

	if outcome.outputBlocked {
		return outcome
	}
	if err := installer.osc.InstallCatalog(catalog); err != nil {
		outcome.runtimeErr = fmt.Errorf("install OSC catalog generation %d: %w", generation, err)
		return outcome
	}
	outcome.catalogReady = true
	return outcome
}

func (installer *planInstaller) sortedPlugins() []plugins.RuntimeSnapshot {
	snapshots := append([]plugins.RuntimeSnapshot(nil), installer.plugins.List()...)
	sort.SliceStable(snapshots, func(left, right int) bool {
		return snapshots[left].ID < snapshots[right].ID
	})
	return snapshots
}

func (installer *planInstaller) deactivateAll(
	ctx context.Context,
	plugins []plugins.RuntimeSnapshot,
	failed map[string]bool,
	outcome *installOutcome,
) {
	for _, plugin := range plugins {
		err := installer.control(ctx, func(controlCtx context.Context) error {
			return installer.plugins.SetActive(controlCtx, plugin.ID, false)
		})
		if err == nil {
			continue
		}
		if failed != nil {
			failed[plugin.ID] = true
		}
		outcome.addPluginFailure(plugin.ID, "deactivate", err)
	}
}

func (installer *planInstaller) control(parent context.Context, call func(context.Context) error) error {
	timeout := installer.pluginControlTimeout
	if timeout <= 0 {
		timeout = DefaultPluginControlTimeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return call(ctx)
}

func (outcome *installOutcome) addPluginFailure(pluginID, operation string, err error) {
	outcome.pluginFailures = append(outcome.pluginFailures, PluginControlFailure{
		PluginID:  pluginID,
		Operation: operation,
		Message:   sanitizedPluginControlFailure(err),
	})
}

func sanitizedPluginControlFailure(err error) string {
	message := strings.ToValidUTF8(err.Error(), "\uFFFD")
	message = strings.Join(strings.Fields(message), " ")
	if len(message) <= maxPluginControlFailureMessageBytes {
		return message
	}
	message = message[:maxPluginControlFailureMessageBytes]
	for !utf8.ValidString(message) {
		message = message[:len(message)-1]
	}
	return message
}
