package tesla

import (
	"context"
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// StateSyncRequest carries an optional report. Presence replaces the stored
// report, even if the nested message is empty (RS-485 Site Controller Protocol §7).
type StateSyncRequest struct{ Report *SiteControllerReport }

// SiteControllerReport carries reserved values by wire field order; nil omits a scalar.
type SiteControllerReport struct {
	FieldOne     *uint32
	Measurements *SiteControllerMeasurements
	ScalarGroup  *SiteControllerScalarGroup
	FieldFour    *uint32
	FieldFive    *uint32
	FieldSix     *uint32
}

// SiteControllerMeasurements carries the reserved fields of the measurement block.
type SiteControllerMeasurements struct {
	FieldOne, FieldTwo                         *uint32
	FieldThree, FieldFour, FieldFive, FieldSix *uint32 // fixed32
	FieldSeven, FieldEight, FieldNine          *bool
}

// SiteControllerScalarGroup carries the reserved fixed32 field.
type SiteControllerScalarGroup struct{ FieldOne *uint32 }

// StateSyncReply carries the stored vehicle snapshot and its age (nominally ms).
// The age is not the time at which the vehicle sampled the data.
type StateSyncReply struct {
	Snapshot         *VehicleSnapshot
	SnapshotAgeTicks *uint32
	native           HSCNativeRecord
}

// Payload returns a copy of the native response payload, including unknown fields.
func (reply StateSyncReply) Payload() []byte { return reply.native.Payload() }

// VehicleSnapshot carries reserved values without assigning them quantities.
// The group arrays are indexed by wire field number minus one.
type VehicleSnapshot struct {
	FieldOne   *uint32
	FourValues *[4]*NativeValueAndCode
	SixValues  *[6]*NativeValueAndCode
	Scalars    *VehicleSnapshotScalars
	FieldFive  *uint32 // Documented on 26.34.0 only.
}

// VehicleSnapshotScalars carries the two reserved fixed32 values.
type VehicleSnapshotScalars struct{ FieldThree, FieldFour *uint32 }

// NativeValueAndCode is the specified fixed32 and uint32 pair.
type NativeValueAndCode struct{ Value, Code *uint32 }

func decodeTeslaNativeValueAndCode(data []byte, pair *NativeValueAndCode) error {
	return decodeTeslaCommandFields(data, teslaCommandFixed(&pair.Value), teslaCommandUint(&pair.Code))
}

// EncodeTeslaStateSync builds a typed HscStateSync request (§7 and Appendix A).
func EncodeTeslaStateSync(firmware string, input StateSyncRequest) (modbus.PrivateFunctionRequest, error) {
	_, _, supported := teslaNativeHSCLayout(firmware)
	if !supported {
		return modbus.PrivateFunctionRequest{}, fmt.Errorf("HscStateSync layout not specified for firmware %q", firmware)
	}
	var body []byte
	if report := input.Report; report != nil {
		fields, err := encodeTeslaSiteControllerReport(report)
		if err != nil {
			return modbus.PrivateFunctionRequest{}, err
		}
		body = appendTeslaCommandMessage(nil, 10, fields)
	}
	return BuildTeslaHSCNativeRequest(teslaHSCFunction102, body)
}

func encodeTeslaSiteControllerReport(report *SiteControllerReport) ([]byte, error) {
	for _, field := range []*uint32{report.FieldOne, report.FieldFour, report.FieldFive, report.FieldSix} {
		if field != nil && *field > 255 {
			return nil, fmt.Errorf("HscStateSync report uint32 is outside 0–255")
		}
	}
	body := appendTeslaCommandUint(nil, 8, report.FieldOne)
	if m := report.Measurements; m != nil {
		for _, field := range []*uint32{m.FieldOne, m.FieldTwo} {
			if field != nil && *field > 255 {
				return nil, fmt.Errorf("HscStateSync measurement uint32 is outside 0–255")
			}
		}
		fields := appendTeslaCommandUint(nil, 8, m.FieldOne)
		fields = appendTeslaCommandUint(fields, 16, m.FieldTwo)
		fields = appendTeslaCommandFixed(fields, 29, m.FieldThree)
		fields = appendTeslaCommandFixed(fields, 37, m.FieldFour)
		fields = appendTeslaCommandFixed(fields, 45, m.FieldFive)
		fields = appendTeslaCommandFixed(fields, 53, m.FieldSix)
		fields = appendTeslaCommandBool(fields, 56, m.FieldSeven)
		fields = appendTeslaCommandBool(fields, 64, m.FieldEight)
		fields = appendTeslaCommandBool(fields, 72, m.FieldNine)
		body = appendTeslaCommandMessage(body, 18, fields)
	}
	if group := report.ScalarGroup; group != nil {
		body = appendTeslaCommandMessage(body, 26, appendTeslaCommandFixed(nil, 13, group.FieldOne))
	}
	body = appendTeslaCommandUint(body, 32, report.FieldFour)
	body = appendTeslaCommandUint(body, 40, report.FieldFive)
	return appendTeslaCommandUint(body, 48, report.FieldSix), nil
}

// DecodeTeslaStateSync decodes one normal terminal; the transport owns exceptions.
func DecodeTeslaStateSync(firmware string, payload []byte) (StateSyncReply, error) {
	_, latest, supported := teslaNativeHSCLayout(firmware)
	if !supported {
		return StateSyncReply{}, fmt.Errorf("HscStateSync layout not specified for firmware %q", firmware)
	}
	envelope, err := decodeTeslaHSCExactEnvelope(teslaHSCFunction102, payload)
	if err != nil {
		return StateSyncReply{}, err
	}
	reply := StateSyncReply{native: HSCNativeRecord{function: teslaHSCFunction102, payload: cloneTeslaGen3Bytes(payload)}}
	err = decodeTeslaCommandFields(envelope.Payload(), teslaCommandNested(&reply.Snapshot, func(data []byte, snapshot *VehicleSnapshot) error {
		return decodeTeslaVehicleSnapshot(data, snapshot, latest)
	}), teslaCommandUint(&reply.SnapshotAgeTicks))
	if err != nil {
		return StateSyncReply{}, err
	}
	return reply, nil
}

func decodeTeslaVehicleSnapshot(data []byte, snapshot *VehicleSnapshot, latest bool) error {
	fields := []teslaCommandField{teslaCommandUint(&snapshot.FieldOne),
		teslaCommandNested(&snapshot.FourValues, func(data []byte, group *[4]*NativeValueAndCode) error { return decodeTeslaVehiclePairs(data, group[:]) }),
		teslaCommandNested(&snapshot.SixValues, func(data []byte, group *[6]*NativeValueAndCode) error { return decodeTeslaVehiclePairs(data, group[:]) }),
		teslaCommandNested(&snapshot.Scalars, func(data []byte, group *VehicleSnapshotScalars) error {
			return decodeTeslaCommandFields(data, teslaCommandField{}, teslaCommandField{}, teslaCommandFixed(&group.FieldThree), teslaCommandFixed(&group.FieldFour))
		}),
	}
	if latest {
		fields = append(fields, teslaCommandUint(&snapshot.FieldFive))
	}
	return decodeTeslaCommandFields(data, fields...)
}

func decodeTeslaVehiclePairs(data []byte, group []*NativeValueAndCode) error {
	fields := make([]teslaCommandField, len(group))
	for index := range group {
		fields[index] = teslaCommandNested(&group[index], decodeTeslaNativeValueAndCode)
	}
	return decodeTeslaCommandFields(data, fields...)
}

// ExchangeStateSync sends exactly one explicit HscStateSync request.
func (acquisition *Gen3ReadAcquisition) ExchangeStateSync(ctx context.Context, input StateSyncRequest) (Gen3ReadRecord, StateSyncReply, error) {
	firmware := acquisition.config.Gen3.Version()
	request, err := EncodeTeslaStateSync(firmware, input)
	var reply StateSyncReply
	receipt, err := acquisition.runNativeCommand(ctx, teslaNativeCommand{name: "hsc.state_sync", decodeHSC: func(payload []byte) error {
		var decodeErr error
		reply, decodeErr = DecodeTeslaStateSync(firmware, payload)
		return decodeErr
	}}, request, err)
	if receipt.State != Gen3ReadEvidenceAck {
		reply = StateSyncReply{}
	}
	return receipt, reply, err
}
