package osc

import pkgosc "github.com/wzhqwq/vrcft-go/pkg/osc"

type ValueKind = pkgosc.ValueKind

const (
	ValueInt32   = pkgosc.ValueInt32
	ValueFloat32 = pkgosc.ValueFloat32
	ValueString  = pkgosc.ValueString
	ValueBool    = pkgosc.ValueBool
)

type Value = pkgosc.Value
type Message = pkgosc.Message
type Bundle = pkgosc.Bundle

var (
	ErrMalformedPacket = pkgosc.ErrMalformedPacket
	ErrUnsupportedType = pkgosc.ErrUnsupportedType
	Int32              = pkgosc.Int32
	Float32            = pkgosc.Float32
	String             = pkgosc.String
	Bool               = pkgosc.Bool
	MarshalMessage     = pkgosc.MarshalMessage
	MarshalBundle      = pkgosc.MarshalBundle
	NTPTime            = pkgosc.NTPTime
	UnmarshalPacket    = pkgosc.UnmarshalPacket
)

func validAddress(address string) bool { return pkgosc.ValidAddress(address) }
