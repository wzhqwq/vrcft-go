package osc

import (
	"context"
	"log"
)

type OSCService interface {
	Start(ctx context.Context) error
	Close(ctx context.Context) error

	SetEnabled(enabled bool)
	SetTarget(target OSCTarget)

	Status() OSCStatus
}

type OSCTarget struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type OSCStatus struct {
	Enabled   bool
	Running   bool
	LastError string
}

type baseOSCService struct {
	source     *SnapshotSource
	controller *Controller
}

func NewOSCService() OSCService {
	source := NewSnapshotSource()

	controller, err := NewController(ControllerConfig{
		ServiceName: "VRCFaceTracking-Go",
		HTTPBind:    "0.0.0.0:0",
		OSCBind:     "0.0.0.0:0",
		Sender: SenderConfig{
			FloatEpsilon: 0.001,
			MaxDatagram:  1200,
			UseBundles:   true,
		},
	}, source, nil)

	if err != nil {
		log.Fatal(err)
	}

	return &baseOSCService{
		source:     source,
		controller: controller,
	}
}

func (s *baseOSCService) Start(ctx context.Context) error {
	return s.controller.Start(ctx)
}

func (s *baseOSCService) Close(ctx context.Context) error {
	return s.controller.Close(ctx)
}

func (s *baseOSCService) SetEnabled(enabled bool) {
	panic("unimplemented")
}

func (s *baseOSCService) SetTarget(target OSCTarget) {
	panic("unimplemented")
}

func (s *baseOSCService) Status() OSCStatus {
	panic("unimplemented")
}

var _ OSCService = (*baseOSCService)(nil)
