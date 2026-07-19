package tracking

type SourceSelection struct {
	Auto     bool   `json:"auto"`
	PluginID string `json:"pluginId,omitempty"`
}

type RoutingConfig struct {
	Eye        SourceSelection `json:"eye"`
	Expression SourceSelection `json:"expression"`
	Head       SourceSelection `json:"head"`
}
