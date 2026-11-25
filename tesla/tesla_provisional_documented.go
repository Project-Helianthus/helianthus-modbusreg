package tesla

import (
	"context"
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// The installation query is reserved to the setter, not a catalog operation
// or a poll. TEDAPI Specification §§6.4.3, 6.4.10 and 7.8.
const teslaProvisionalInstallationQuery FC100Operation = "wc.get_config"

var teslaInstallationQuerySpec = teslaFC100OperationSpec{6, 5, 6}

func (acquisition *Gen3ReadAcquisition) exchangeSpec(operation FC100Operation) teslaFC100OperationSpec {
	if operation == teslaProvisionalInstallationQuery {
		return teslaInstallationQuerySpec
	}
	spec := teslaFC100OperationSpecs[operation]
	if operation == FC100OperationWCSetProvisional && acquisition.config.Gen3.documentedCurrentLimitLayout() {
		// RS-485 Site Controller Protocol §5.4: the reply can equal the request.
		spec.responseTag = spec.requestTag
	}
	return spec
}

func validateTeslaProvisionalTerminal(record Gen3ReadRecord) error {
	body := record.TerminalBody()
	if record.TerminalTag == record.RequestTag {
		_, err := decodeTeslaProvisionalFields(body, false)
		return err
	}
	if len(body) != 0 {
		return fmt.Errorf("t26 body is not empty")
	}
	return nil
}

func (acquisition *Gen3ReadAcquisition) applyDocumentedProvisionalLimit(ctx context.Context, input ProvisionalCurrentLimitRequest, body []byte, result ProvisionalCurrentLimitResult) (ProvisionalCurrentLimitResult, error) {
	request, err := buildTeslaOperationRequest(teslaInstallationQuerySpec, nil)
	if err != nil {
		return result.reject(err)
	}
	result.Prerequisite = acquisition.mutationExchange(ctx, teslaProvisionalInstallationQuery, request, modbus.DefaultPrivateFunctionResponsePolicy())
	result.Bound = result.Prerequisite
	if result.Bound.State != Gen3ReadEvidenceAck {
		return result.reject(fmt.Errorf("%w: installation current query did not succeed", modbusreg.ErrQualifiedFunctionNoSend))
	}
	// The queried installation maximum is a product ceiling. The station
	// checks its own stored capacity; the t28 scalar is not that maximum.
	amps, present, err := decodeTeslaPersistentSettingsBody(result.Bound.TerminalBody())
	if err != nil {
		return result.reject(err)
	}
	if !present || amps <= 0 || input.LimitCurrentMaxAmps > uint32(amps) {
		return result.reject(fmt.Errorf("%w: installation settings lack a usable bound for the requested current", modbusreg.ErrQualifiedFunctionNoSend))
	}
	return acquisition.exchangeProvisionalSet(ctx, input, body, result)
}
