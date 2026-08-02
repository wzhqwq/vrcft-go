package tracking

import "github.com/wzhqwq/vrcft-go/pkg/trackingmodel"

type SourceSelection struct {
	Auto     bool   `json:"auto"`
	PluginID string `json:"pluginId,omitempty"`
}

type RoutingConfig struct {
	Eye        SourceSelection `json:"eye"`
	Expression SourceSelection `json:"expression"`
}

func defaultRouting() RoutingConfig {
	return RoutingConfig{
		Eye:        SourceSelection{Auto: true},
		Expression: SourceSelection{Auto: true},
	}
}

func (selection SourceSelection) validate() error {
	if selection.Auto == (selection.PluginID != "") {
		return ErrInvalidRouting
	}

	return nil
}

func (routing RoutingConfig) validate() error {
	if err := routing.Eye.validate(); err != nil {
		return err
	}

	return routing.Expression.validate()
}

func chooseAutoSource(current string, sources map[string]sourceState, capability trackingmodel.Capability) string {
	if source, ok := sources[current]; ok && source.frame.Capabilities.Has(capability) {
		return current
	}

	selected := ""
	for pluginID, source := range sources {
		if !source.frame.Capabilities.Has(capability) {
			continue
		}
		if selected == "" || pluginID < selected {
			selected = pluginID
		}
	}
	return selected
}

func resolveSource(selection SourceSelection, current string, sources map[string]sourceState, capability trackingmodel.Capability) string {
	if selection.Auto {
		return chooseAutoSource(current, sources, capability)
	}

	source, ok := sources[selection.PluginID]
	if ok && source.frame.Capabilities.Has(capability) {
		return selection.PluginID
	}
	return ""
}

func (s *service) SetRouting(config RoutingConfig) error {
	if err := config.validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if config == s.routing {
		return nil
	}
	s.routing = config
	if s.generation == 0 {
		return nil
	}
	s.recomputeMergedLocked(true)
	return nil
}

func (s *service) Routing() RoutingConfig {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.routing
}

func (s *service) RemoveSource(pluginID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if pluginID == "" {
		return
	}
	if _, ok := s.sources[pluginID]; !ok {
		return
	}

	delete(s.sources, pluginID)
	s.recomputeMergedLocked(false)
}
