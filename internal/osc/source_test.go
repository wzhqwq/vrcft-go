package osc

import (
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
)

func TestSnapshotSourceUsesParameterIDs(t *testing.T) {
	source := NewSnapshotSource()
	source.SetFloat(parameters.ParameterJawOpen, 0.75)
	source.SetBool(parameters.ParameterExpressionTrackingActive, true)

	if value, ok := source.Float(parameters.ParameterJawOpen); !ok || value != 0.75 {
		t.Fatalf("Float(JawOpen) = %v, %v", value, ok)
	}
	if value, ok := source.Bool(parameters.ParameterExpressionTrackingActive); !ok || !value {
		t.Fatalf("Bool(ExpressionTrackingActive) = %v, %v", value, ok)
	}
	if _, ok := source.Float(parameters.ParameterJawX); ok {
		t.Fatal("unset parameter reported as valid")
	}
}
