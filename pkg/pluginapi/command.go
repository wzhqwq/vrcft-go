package pluginapi

type ControlEvent interface {
	controlEvent()
}

type ActiveChanged struct {
	Active bool
}

func (ActiveChanged) controlEvent() {}

type ConfigChanged struct {
	Config Config
}

func (ConfigChanged) controlEvent() {}

type SubscriptionChanged struct {
	Subscription Subscription
}

func (SubscriptionChanged) controlEvent() {}

type ShutdownRequested struct{}

func (ShutdownRequested) controlEvent() {}
