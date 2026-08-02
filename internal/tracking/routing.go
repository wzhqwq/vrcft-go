package tracking

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
