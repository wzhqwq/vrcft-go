package osc

import (
	"context"
)

type OSCService interface {
	Start(ctx context.Context) error
	Close(ctx context.Context) error
	Events() <-chan ControllerEvent
	AvatarChanges(context.Context) <-chan AvatarChange
	ClearRuntime()
	InstallCatalog(*Catalog) error
	Publish(uint64, ValueSource) error
	Status() OSCStatus
}

type OSCTarget struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type OSCStatus struct {
	Running   bool
	Connected bool
	HasTarget bool
	Target    OSCTarget
	LastError string
}

type baseOSCService struct {
	controller *Controller
}

func NewOSCService(config ControllerConfig) (OSCService, error) {
	source := NewSnapshotSource()
	if config.Sender == (SenderConfig{}) {
		config.Sender = SenderConfig{
			FloatEpsilon: 0.001,
			MaxDatagram:  1200,
			UseBundles:   true,
		}
	}
	controller, err := NewController(config, source, nil)
	if err != nil {
		return nil, err
	}
	return &baseOSCService{controller: controller}, nil
}

func (s *baseOSCService) Start(ctx context.Context) error {
	return s.controller.Start(ctx)
}

func (s *baseOSCService) Close(ctx context.Context) error {
	return s.controller.Close(ctx)
}

func (s *baseOSCService) Events() <-chan ControllerEvent { return s.controller.Events() }

func (s *baseOSCService) AvatarChanges(ctx context.Context) <-chan AvatarChange {
	return s.controller.AvatarChanges(ctx)
}

func (s *baseOSCService) ClearRuntime() { s.controller.ClearRuntime() }

func (s *baseOSCService) InstallCatalog(catalog *Catalog) error {
	return s.controller.InstallCatalog(catalog)
}

func (s *baseOSCService) Publish(generation uint64, source ValueSource) error {
	return s.controller.Publish(generation, source)
}

func (s *baseOSCService) Status() OSCStatus {
	return s.controller.Status()
}

var _ OSCService = (*baseOSCService)(nil)
