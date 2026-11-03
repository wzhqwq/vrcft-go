package steamlink

import (
	"errors"
	"math"
	"strings"

	"github.com/wzhqwq/vrcft-go/pkg/osc"
)

const (
	maxDatagramBytes    = 65507
	maxDatagramMessages = 512
	maxAddressBytes     = 256
	maxUnknownAddresses = 32
)

var errPacketLimit = errors.New("steamlink OSC packet limit exceeded")

type observation struct {
	ID     rawID
	Values [3]float32
}

type inputReport struct {
	InvalidMessages  int
	UnknownMessages  int
	UnknownAddresses []string
}

type inputField struct {
	id       rawID
	argCount int
}

var inputRegistry = map[string]inputField{
	"/sl/eyeTrackedGazePoint":            {id: rawGazePoint, argCount: 3},
	"/sl/xrfb/facew/EyesClosedL":         {id: rawEyesClosedL, argCount: 1},
	"/sl/xrfb/facew/EyesClosedR":         {id: rawEyesClosedR, argCount: 1},
	"/sl/xrfb/facew/UpperLidRaiserL":     {id: rawUpperLidRaiserL, argCount: 1},
	"/sl/xrfb/facew/UpperLidRaiserR":     {id: rawUpperLidRaiserR, argCount: 1},
	"/sl/xrfb/facew/LidTightenerL":       {id: rawLidTightenerL, argCount: 1},
	"/sl/xrfb/facew/LidTightenerR":       {id: rawLidTightenerR, argCount: 1},
	"/sl/xrfb/facew/InnerBrowRaiserL":    {id: rawInnerBrowRaiserL, argCount: 1},
	"/sl/xrfb/facew/InnerBrowRaiserR":    {id: rawInnerBrowRaiserR, argCount: 1},
	"/sl/xrfb/facew/OuterBrowRaiserL":    {id: rawOuterBrowRaiserL, argCount: 1},
	"/sl/xrfb/facew/OuterBrowRaiserR":    {id: rawOuterBrowRaiserR, argCount: 1},
	"/sl/xrfb/facew/BrowLowererL":        {id: rawBrowLowererL, argCount: 1},
	"/sl/xrfb/facew/BrowLowererR":        {id: rawBrowLowererR, argCount: 1},
	"/sl/xrfb/facew/NoseWrinklerL":       {id: rawNoseWrinklerL, argCount: 1},
	"/sl/xrfb/facew/NoseWrinklerR":       {id: rawNoseWrinklerR, argCount: 1},
	"/sl/xrfb/facew/CheekRaiserL":        {id: rawCheekRaiserL, argCount: 1},
	"/sl/xrfb/facew/CheekRaiserR":        {id: rawCheekRaiserR, argCount: 1},
	"/sl/xrfb/facew/CheekPuffL":          {id: rawCheekPuffL, argCount: 1},
	"/sl/xrfb/facew/CheekPuffR":          {id: rawCheekPuffR, argCount: 1},
	"/sl/xrfb/facew/CheekSuckL":          {id: rawCheekSuckL, argCount: 1},
	"/sl/xrfb/facew/CheekSuckR":          {id: rawCheekSuckR, argCount: 1},
	"/sl/xrfb/facew/JawDrop":             {id: rawJawDrop, argCount: 1},
	"/sl/xrfb/facew/LipsToward":          {id: rawLipsToward, argCount: 1},
	"/sl/xrfb/facew/JawSidewaysRight":    {id: rawJawSidewaysRight, argCount: 1},
	"/sl/xrfb/facew/JawSidewaysLeft":     {id: rawJawSidewaysLeft, argCount: 1},
	"/sl/xrfb/facew/JawThrust":           {id: rawJawThrust, argCount: 1},
	"/sl/xrfb/facew/MouthRight":          {id: rawMouthRight, argCount: 1},
	"/sl/xrfb/facew/MouthLeft":           {id: rawMouthLeft, argCount: 1},
	"/sl/xrfb/facew/ChinRaiserT":         {id: rawChinRaiserT, argCount: 1},
	"/sl/xrfb/facew/ChinRaiserB":         {id: rawChinRaiserB, argCount: 1},
	"/sl/xrfb/facew/DimplerL":            {id: rawDimplerL, argCount: 1},
	"/sl/xrfb/facew/DimplerR":            {id: rawDimplerR, argCount: 1},
	"/sl/xrfb/facew/LipCornerPullerL":    {id: rawLipCornerPullerL, argCount: 1},
	"/sl/xrfb/facew/LipCornerPullerR":    {id: rawLipCornerPullerR, argCount: 1},
	"/sl/xrfb/facew/LipCornerDepressorL": {id: rawLipCornerDepressorL, argCount: 1},
	"/sl/xrfb/facew/LipCornerDepressorR": {id: rawLipCornerDepressorR, argCount: 1},
	"/sl/xrfb/facew/LowerLipDepressorL":  {id: rawLowerLipDepressorL, argCount: 1},
	"/sl/xrfb/facew/LowerLipDepressorR":  {id: rawLowerLipDepressorR, argCount: 1},
	"/sl/xrfb/facew/UpperLipRaiserL":     {id: rawUpperLipRaiserL, argCount: 1},
	"/sl/xrfb/facew/UpperLipRaiserR":     {id: rawUpperLipRaiserR, argCount: 1},
	"/sl/xrfb/facew/LipTightenerL":       {id: rawLipTightenerL, argCount: 1},
	"/sl/xrfb/facew/LipTightenerR":       {id: rawLipTightenerR, argCount: 1},
	"/sl/xrfb/facew/LipPressorL":         {id: rawLipPressorL, argCount: 1},
	"/sl/xrfb/facew/LipPressorR":         {id: rawLipPressorR, argCount: 1},
	"/sl/xrfb/facew/LipStretcherL":       {id: rawLipStretcherL, argCount: 1},
	"/sl/xrfb/facew/LipStretcherR":       {id: rawLipStretcherR, argCount: 1},
	"/sl/xrfb/facew/LipPuckerL":          {id: rawLipPuckerL, argCount: 1},
	"/sl/xrfb/facew/LipPuckerR":          {id: rawLipPuckerR, argCount: 1},
	"/sl/xrfb/facew/LipFunnelerLT":       {id: rawLipFunnelerLT, argCount: 1},
	"/sl/xrfb/facew/LipFunnelerRT":       {id: rawLipFunnelerRT, argCount: 1},
	"/sl/xrfb/facew/LipFunnelerLB":       {id: rawLipFunnelerLB, argCount: 1},
	"/sl/xrfb/facew/LipFunnelerRB":       {id: rawLipFunnelerRB, argCount: 1},
	"/sl/xrfb/facew/LipSuckLT":           {id: rawLipSuckLT, argCount: 1},
	"/sl/xrfb/facew/LipSuckRT":           {id: rawLipSuckRT, argCount: 1},
	"/sl/xrfb/facew/LipSuckLB":           {id: rawLipSuckLB, argCount: 1},
	"/sl/xrfb/facew/LipSuckRB":           {id: rawLipSuckRB, argCount: 1},
	"/sl/xrfb/facew/TongueOut":           {id: rawTongueOut, argCount: 1},
	"/sl/xrfb/facew/ToungeOut":           {id: rawTongueOut, argCount: 1},
}

// These observed addresses are intentionally outside steamlink-osc-v1. Keep
// unexpected addresses visible in diagnostics without warning on every frame
// of the known, unmapped telemetry stream.
var ignoredInputAddresses = map[string]struct{}{
	"/sl/xrfb/facec/UpperFace":            {},
	"/sl/xrfb/facec/LowerFace":            {},
	"/sl/xrfb/facew/BackDorsalVelar":      {},
	"/sl/xrfb/facew/FrontDorsalPalate":    {},
	"/sl/xrfb/facew/MidDorsalPalate":      {},
	"/sl/xrfb/facew/TongueRetreat":        {},
	"/sl/xrfb/facew/TongueTipAlveolar":    {},
	"/sl/xrfb/facew/TongueTipInterdental": {},
	"/sl/xrfb/facew/ToungeRetreat":        {},
	"/sl/xrfb/facew/ToungeTipAlveolar":    {},
	"/sl/xrfb/facew/ToungeTipInterdental": {},
	"/tracking/eye/CenterVecFull":         {},
	"/tracking/eye/EyesClosedAmount":      {},
}

func ignoredInputAddress(address string) bool {
	if strings.HasPrefix(address, "/avatar/parameters/") {
		return true
	}
	if strings.HasPrefix(address, "/sl/xrfb/facew/EyesLook") {
		switch strings.TrimPrefix(address, "/sl/xrfb/facew/EyesLook") {
		case "DownL", "DownR", "LeftL", "LeftR", "RightL", "RightR", "UpL", "UpR":
			return true
		}
	}
	_, ignored := ignoredInputAddresses[address]
	return ignored
}

func decodeDatagram(packet []byte) ([]observation, inputReport, error) {
	if len(packet) > maxDatagramBytes {
		return nil, inputReport{}, errPacketLimit
	}
	messages, err := osc.UnmarshalPacket(packet)
	if err != nil {
		return nil, inputReport{}, err
	}
	if len(messages) > maxDatagramMessages {
		return nil, inputReport{}, errPacketLimit
	}
	for _, message := range messages {
		if len(message.Address) > maxAddressBytes {
			return nil, inputReport{}, errPacketLimit
		}
	}

	observations := make([]observation, 0, len(messages))
	var report inputReport
	for _, message := range messages {
		field, ok := inputRegistry[message.Address]
		if !ok {
			if ignoredInputAddress(message.Address) {
				continue
			}
			report.UnknownMessages++
			if len(report.UnknownAddresses) < maxUnknownAddresses {
				report.UnknownAddresses = append(report.UnknownAddresses, message.Address)
			}
			continue
		}

		values, valid := validateInputMessage(message, field)
		if !valid {
			report.InvalidMessages++
			continue
		}
		observations = append(observations, observation{ID: field.id, Values: values})
	}
	return observations, report, nil
}

func validateInputMessage(message osc.Message, field inputField) ([3]float32, bool) {
	if len(message.Args) != field.argCount {
		return [3]float32{}, false
	}
	var values [3]float32
	for index, arg := range message.Args {
		if arg.Kind != osc.ValueFloat32 || math.IsNaN(float64(arg.F32)) || math.IsInf(float64(arg.F32), 0) {
			return [3]float32{}, false
		}
		values[index] = arg.F32
	}
	if field.id == rawGazePoint {
		return values, values[2] < 0
	}
	return values, values[0] >= 0 && values[0] <= 1
}
