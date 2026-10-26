package main

import (
	"fmt"
	"strconv"
	"strings"
)

// The gateway catalogue (SmartLogger, S-Dongle and EMMA interface editions)
// is a second set of documents of the same schema family, written to a
// subdirectory of the inverter catalogue. It reuses the editions, registers,
// dictionaries and quarantines documents and adds one services document.
const (
	gatewayDirectory   = "smartlogger-sdongle-emma"
	defaultInverterOut = "profiles/vendor/huawei/catalogue"
	defaultGatewayOut  = defaultInverterOut + "/" + gatewayDirectory
	servicesSchema     = "helianthus-modbusreg-huawei-catalogue-services/v1"
	servicesFile       = "services.json"
)

// Endpoint classes of a gateway register row. The forwarded inverter is a
// namespace of the editions without rows: its registers belong to the
// separate inverter definition.
const (
	endpointGatewaySelf           = "gateway-self"
	endpointAttachedMeter         = "attached-meter"
	endpointAttachedCharger       = "attached-charger"
	endpointAttachedEnvironmental = "attached-environmental-instrument"
	endpointChildAlias            = "child-alias"
	endpointInventoryCounter      = "inventory-counter"
	endpointRemappedInverter      = "remapped-inverter"
	endpointRemappedEnvironmental = "remapped-environmental-instrument"
	endpointForwardedInverter     = "forwarded-inverter"
)

var gatewayEndpoints = map[string]bool{
	endpointGatewaySelf: true, endpointAttachedMeter: true, endpointAttachedCharger: true, endpointAttachedEnvironmental: true,
	endpointChildAlias: true, endpointInventoryCounter: true, endpointRemappedInverter: true, endpointRemappedEnvironmental: true,
}

// Services vocabulary.
const (
	functionListed         = "listed"
	functionNotEstablished = "not established"
	limitStandard          = "standard"
	limitVendor            = "vendor"
	limitClientPolicy      = "client_policy"
	basisRequired          = "required"
	basisAdvisory          = "advisory"
)

type softwareFloor struct {
	Component string `json:"component"`
	Minimum   string `json:"minimum"`
	Inclusive bool   `json:"inclusive"`
}

type namespaceRef struct {
	Endpoint string `json:"endpoint"`
	Page     int    `json:"page"`
	Rows     int    `json:"rows"`
}

type alarmNote struct {
	AlarmID     string `json:"alarm_id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Impact      string `json:"impact"`
	Page        int    `json:"page"`
}

type servicesDoc struct {
	Schema            string             `json:"schema"`
	DeviceListGrammar string             `json:"device_list_grammar"`
	Functions         []serviceFunction  `json:"functions"`
	Exceptions        []exceptionMeaning `json:"exceptions"`
	ListObjects       []listObject       `json:"list_objects"`
	Attributes        []listAttribute    `json:"attributes"`
	Limits            []frameLimit       `json:"limits"`
	Editions          []serviceEdition   `json:"editions"`
}

type serviceFunction struct {
	Code     string `json:"code"`
	Purpose  string `json:"purpose"`
	Status   string `json:"status"`
	Boundary string `json:"boundary,omitempty"`
}

type exceptionMeaning struct {
	Code     string   `json:"code"`
	Meaning  string   `json:"meaning"`
	Families []string `json:"families"`
}

type listObject struct {
	Object  string `json:"object"`
	Meaning string `json:"meaning"`
}

type listAttribute struct {
	Number      int    `json:"number"`
	Meaning     string `json:"meaning"`
	Differences string `json:"differences,omitempty"`
}

type frameLimit struct {
	ID     string `json:"id"`
	Origin string `json:"origin"`
	Rule   string `json:"rule"`
	Words  int    `json:"words,omitempty"`
	Bytes  int    `json:"bytes,omitempty"`
}

type serviceEdition struct {
	Edition              string          `json:"edition"`
	SelfUnit             int             `json:"self_unit"`
	ChildUnits           string          `json:"child_units"`
	ReservedUnits        string          `json:"reserved_units"`
	UnitNotes            []string        `json:"unit_notes,omitempty"`
	ByteOrder            string          `json:"byte_order"`
	Port502Stated        bool            `json:"port_502_stated"`
	ResponseTimeout      int             `json:"response_timeout_s"`
	FrameLimitBytes      int             `json:"frame_limit_bytes"`
	ReadQuantity         string          `json:"read_quantity"`
	WriteQuantity        string          `json:"write_quantity"`
	Functions            []string        `json:"functions"`
	ExceptionCodes       []string        `json:"exception_codes"`
	ExceptionPages       []int           `json:"exception_pages"`
	RequestObject        string          `json:"request_object"`
	WrapsBeyondFF        bool            `json:"wraps_beyond_ff"`
	Attribute6           string          `json:"attribute_6"`
	DescriptorAttributes string          `json:"descriptor_attributes"`
	Spacing              []spacingRule   `json:"spacing,omitempty"`
	QueryPeriods         []queryPeriod   `json:"query_periods,omitempty"`
	Pages                []contractPages `json:"pages"`
	Quarantine           []string        `json:"quarantine,omitempty"`
}

type spacingRule struct {
	Between   string `json:"between"`
	MinimumMS int    `json:"minimum_ms"`
	Basis     string `json:"basis"`
	Page      int    `json:"page"`
}

type queryPeriod struct {
	Equipment string `json:"equipment"`
	Seconds   string `json:"seconds"`
	Basis     string `json:"basis"`
	Page      int    `json:"page"`
}

type contractPages struct {
	Contract string `json:"contract"`
	First    int    `json:"first"`
	Last     int    `json:"last"`
}

// gatewayCatalogue is the full set of committed gateway documents.
type gatewayCatalogue struct {
	Editions     editionsDoc
	Registers    map[string]*registersDoc
	Dictionaries dictionariesDoc
	Quarantines  quarantinesDoc
	Services     servicesDoc
}

// gatewayCounts is the accounting the gateway mode asserts: editions, read-
// only rows, control rows, all register rows and alarm records.
type gatewayCounts struct {
	Editions int
	RO       int
	Controls int
	Rows     int
	Alarms   int
}

var defaultGatewayExpect = gatewayCounts{Editions: 18, RO: 2564, Controls: 829, Rows: 3393, Alarms: 1034}

func (c gatewayCounts) String() string {
	return fmt.Sprintf("editions=%d,ro=%d,controls=%d,rows=%d,alarms=%d", c.Editions, c.RO, c.Controls, c.Rows, c.Alarms)
}

func gatewayCountLine(c gatewayCounts) string {
	return "editions " + strconv.Itoa(c.Editions) + ", read-only rows " + strconv.Itoa(c.RO) + ", control rows " + strconv.Itoa(c.Controls) +
		", register rows " + strconv.Itoa(c.Rows) + ", alarm records " + strconv.Itoa(c.Alarms)
}

func parseGatewayExpect(text string) (gatewayCounts, error) {
	expect := defaultGatewayExpect
	if strings.TrimSpace(text) == "" {
		return expect, nil
	}
	for _, part := range strings.Split(text, ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		number, err := strconv.Atoi(value)
		if !ok || err != nil || number < 0 {
			return gatewayCounts{}, fmt.Errorf("invalid -expect entry %q", part)
		}
		switch name {
		case "editions":
			expect.Editions = number
		case "ro":
			expect.RO = number
		case "controls":
			expect.Controls = number
		case "rows":
			expect.Rows = number
		case "alarms":
			expect.Alarms = number
		default:
			return gatewayCounts{}, fmt.Errorf("unknown -expect count %q", name)
		}
	}
	return expect, nil
}

func countGatewayCatalogue(c *gatewayCatalogue) gatewayCounts {
	result := gatewayCounts{Editions: len(c.Editions.Editions)}
	for _, doc := range c.Registers {
		result.RO += len(doc.Rows)
		result.Controls += len(doc.Controls)
	}
	result.Rows = result.RO + result.Controls
	for _, item := range c.Dictionaries.Dictionaries {
		if item.Kind == kindAlarmBits {
			result.Alarms += len(item.Bits) + len(item.Narratives)
		}
	}
	return result
}

// slug lowers text and joins its letter and digit runs with hyphens.
func slug(text string) string {
	var out strings.Builder
	pending := false
	for _, r := range strings.ToLower(text) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if pending && out.Len() > 0 {
				out.WriteByte('-')
			}
			pending = false
			out.WriteRune(r)
			continue
		}
		pending = true
	}
	return out.String()
}
