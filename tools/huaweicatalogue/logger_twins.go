package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// gatewayTwin is the deterministic JSON twin of one gateway interface
// edition. It is decoded strictly: a field the generator does not know stops
// the run, so a reference change is reviewed rather than silently dropped.
type gatewayTwin struct {
	Schema            string                 `json:"schema"`
	Title             string                 `json:"title"`
	SourceID          string                 `json:"source_id"`
	Baseline          string                 `json:"baseline"`
	Rights            string                 `json:"rights"`
	RegisterCount     int                    `json:"register_count"`
	Edition           twinEdition            `json:"edition"`
	ModelScope        twinModelScope         `json:"model_scope"`
	IntegrationPolicy twinIntegrationPolicy  `json:"integration_policy"`
	Protocol          twinProtocol           `json:"protocol"`
	Registers         []gatewayRegister      `json:"registers"`
	Dictionaries      []gatewayFactSummary   `json:"dictionaries"`
	AlarmDictionary   []gatewayAlarm         `json:"alarm_dictionary"`
	AlarmQuarantines  gatewayAlarmQuarantine `json:"alarm_quarantines"`
}

type twinEdition struct {
	CoverVersion string `json:"cover_version"`
	Date         string `json:"date"`
	Issue        string `json:"issue"`
}

type twinModelScope struct {
	Exhaustive           bool     `json:"exhaustive_exact_model_list"`
	Entries              []string `json:"model_entries"`
	Models               string   `json:"models"`
	RequirementPages     []int    `json:"requirements_physical_pages"`
	SoftwareRequirements string   `json:"software_requirements"`
	Minima               []struct {
		Interpretation string `json:"interpretation"`
		Relation       string `json:"relation"`
		Requirement    string `json:"source_requirement"`
	} `json:"software_minimum_entries"`
}

type twinIntegrationPolicy struct {
	ConflictScope        string `json:"conflict_scope"`
	CitationKeyOnly      bool   `json:"edition_is_citation_key_not_runtime_gate"`
	Kind                 string `json:"kind"`
	SilenceRetracts      bool   `json:"later_partial_silence_retracts_earlier_fact"`
	MinimumIsFloor       bool   `json:"minimum_is_floor"`
	Reference            string `json:"reference"`
	UndocumentedBranches string `json:"undocumented_branch_relationship_confidence"`
}

type twinSection struct {
	Pages     []int  `json:"physical_pages_inclusive"`
	SectionID string `json:"section_id"`
	Title     string `json:"title"`
}

type twinProtocol struct {
	ExceptionCodes []string `json:"exception_codes"`
	ExceptionPages []int    `json:"exception_physical_pages"`
	PortCheck      struct {
		Pages     []int `json:"physical_pages"`
		PriorFlag bool  `json:"prior_audit_flag"`
	} `json:"port_author_check"`
	RequestObject string                  `json:"private_request_object"`
	Sections      map[string]*twinSection `json:"section_evidence"`
	SourceID      string                  `json:"source_id"`
	Transport     struct {
		ByteOrder     string `json:"byte_order"`
		Carrier       string `json:"carrier"`
		FrameConflict string `json:"frame_limit_conflict"`
		Port502       bool   `json:"port_502_explicit_in_physical_section"`
		Timeout5s     bool   `json:"response_timeout_5s_explicit"`
		Units         struct {
			Basis      string   `json:"basis"`
			Conflicts  []string `json:"conflicts"`
			ChildRange string   `json:"forwarded_child_range_literal"`
			Reserved   string   `json:"reserved_literal"`
			Self       *int     `json:"self"`
		} `json:"unit_id_contract"`
		FrameLimit int `json:"vendor_frame_limit_bytes"`
	} `json:"transport"`
	Wraps bool `json:"wrap_printed"`
}

type gatewayPredicate struct {
	Kind     string `json:"kind"`
	Operator string `json:"operator"`
	Page     int    `json:"physical_page"`
	SourceID string `json:"source_id"`
	Scope    struct {
		Namespace string `json:"namespace"`
		Maximum   int    `json:"ordinal_maximum"`
		Minimum   int    `json:"ordinal_minimum"`
		Ordinal   int    `json:"this_row_ordinal"`
	} `json:"source_scope"`
	Summary string `json:"summary"`
	Value   string `json:"value"`
}

type gatewayRegister struct {
	Access          string  `json:"access"`
	Address         int     `json:"address"`
	AddressKind     string  `json:"address_kind"`
	AliasTargetKind *string `json:"alias_target_kind"`
	Class           string  `json:"class"`
	Conditions      struct {
		DetailID    string             `json:"detail_dictionary_id"`
		Namespace   []string           `json:"namespace"`
		RowSpecific []gatewayPredicate `json:"row_specific"`
		Summary     string             `json:"summary"`
	} `json:"conditions"`
	DefaultLiteral    *string           `json:"default_literal"`
	DisplayName       string            `json:"display_name"`
	ExecutionStatus   string            `json:"execution_status"`
	GainLiteral       *string           `json:"gain_literal"`
	Namespace         string            `json:"namespace"`
	OperationalMarker *string           `json:"operational_marker"`
	OriginalNamespace string            `json:"original_reconstruction_namespace"`
	Quantity          int               `json:"quantity_words"`
	Quarantine        []json.RawMessage `json:"quarantine"`
	ReaderPath        string            `json:"reader_path"`
	RecordID          string            `json:"record_id"`
	SourceCategory    *string           `json:"source_category"`
	SourceID          string            `json:"source_id"`
	SourceOrdinal     int               `json:"source_ordinal"`
	References        []struct {
		Page  int `json:"physical_page"`
		Row   int `json:"row_index"`
		Table int `json:"table_index"`
	} `json:"source_references"`
	SourceSection string  `json:"source_section"`
	StatusLiteral *string `json:"source_status_literal"`
	TypeLiteral   string  `json:"type_literal"`
	UnitLiteral   *string `json:"unit_literal"`
}

type gatewayFactSummary struct {
	ID         string   `json:"dictionary_id"`
	Summary    string   `json:"factual_summary"`
	RecordIDs  []string `json:"record_ids"`
	References []struct {
		Page     int    `json:"physical_page"`
		Row      int    `json:"row_index"`
		SourceID string `json:"source_id"`
		Table    int    `json:"table_index"`
	} `json:"source_references"`
}

type gatewaySemantic struct {
	DetailID string `json:"detail_id"`
	Summary  string `json:"summary"`
}

type gatewayAlarm struct {
	Address      *int              `json:"address"`
	AlarmID      *int              `json:"alarm_id"`
	AlarmLiteral string            `json:"alarm_id_literal"`
	Bit          *int              `json:"bit"`
	Cause        *int              `json:"cause_id"`
	Inherited    map[string]string `json:"inherited_fields"`
	Kind         string            `json:"kind"`
	Label        string            `json:"label"`
	Locator      struct {
		Page  int `json:"physical_page"`
		Row   int `json:"row_index"`
		Table int `json:"table_index"`
	} `json:"locator"`
	Namespace string                     `json:"namespace"`
	RecordID  string                     `json:"record_id"`
	SectionID string                     `json:"section_id"`
	Semantic  map[string]gatewaySemantic `json:"semantic_fields"`
	Severity  *string                    `json:"severity_literal"`
	SourceID  string                     `json:"source_id"`
}

type gatewayAlarmQuarantine struct {
	Colliding []struct {
		Address    int      `json:"address"`
		Bit        int      `json:"bit"`
		Candidates [][2]int `json:"candidates"`
		RecordIDs  []string `json:"record_ids"`
		SourceID   string   `json:"source_id"`
	} `json:"colliding_bit_keys"`
	DictionaryOnly []struct {
		Address  int    `json:"address"`
		SourceID string `json:"source_id"`
	} `json:"dictionary_only_addresses"`
	MultipleLocations []struct {
		AlarmID   int      `json:"alarm_id"`
		CauseID   int      `json:"cause_id"`
		Locations [][2]int `json:"locations"`
		RecordIDs []string `json:"record_ids"`
		SourceID  string   `json:"source_id"`
	} `json:"multiple_locations_for_alarm_cause"`
}

// Literal values of the twins the generator relies on.
const (
	gatewayTwinSchema    = "gateway-edition-reference-v1"
	gatewayIndexSchema   = "gateway-reference-index-v1"
	documentaryOnly      = "documentary only; runtime access and field-specific write function not established by a successful unrelated read"
	noInterpretation     = "No additional interpretation is specified in this cell."
	coverNotStated       = "not stated in source"
	classMeasurement     = "measurement_or_status"
	classConfiguration   = "configuration_or_control"
	addressLiteral       = "literal_register_address"
	addressBlockOffset   = "offset_in_25_register_alias_block"
	addressBlockConflict = "offset_cell_conflicts_with_25_register_block"
	alarmTuple           = "alarm_bit_tuple"
	alarmNarrative       = "alarm_narrative"
	remappedNamespace    = "gateway_self_remapped_child_alias"
	counterNamespace     = "public_inventory_counter_route_unspecified"
	childAliasNamespace  = "child_public_alias_gateway_supplied"
	opaqueLayoutType     = "MLD"
)

// Twin row quarantine codes.
const (
	codeWidth            = "literal_type_quantity_mismatch"
	codeOutsideBlock     = "offset_outside_documented_25_register_device_block"
	codeRemapLayout      = "source_remap_layout_conflict_25_word_block_vs_38000_series_offsets"
	codeEnvApplicability = "source_preamble_inverter_only_but_environmental_table_present"
)

var gatewayNamespaces = map[string]string{
	"gateway_self":                    endpointGatewaySelf,
	"forwarded_external_meter":        endpointAttachedMeter,
	"forwarded_charger":               endpointAttachedCharger,
	"forwarded_environmental_monitor": endpointAttachedEnvironmental,
	childAliasNamespace:               endpointChildAlias,
	counterNamespace:                  endpointInventoryCounter,
}

var remapTargets = map[string]string{"inverter": endpointRemappedInverter, "environmental_monitor": endpointRemappedEnvironmental}

var (
	floorStatementPattern = regexp.MustCompile(`^(?:(\S+) )?(V\d{3}R\d{3}C\d{2}(?:SPC\d{3})?) or later$`)
	twoDigitIssuePattern  = regexp.MustCompile(`^\d{2}$`)
	codeItemPattern       = regexp.MustCompile(`^(0x[0-9A-Fa-f]+|\d+) (?:means |is |= )?(\S.*)$`)
	gatewayFamilies       = []string{"S-Dongle", "EMMA", "SmartLogger"}
)

// locatorKey is the key under which the index and every twin name the
// reference's locator file. The generator reads that file only for the
// sidecar, so the key is checked and removed before strict decoding.
var locatorKey = "source_" + "led" + "ger"

const locatorFile = "source-locators.json"

// withoutLocatorKey checks that a reference document names the locator file
// and returns the document without that key.
func withoutLocatorKey(data []byte) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	var name string
	if err := json.Unmarshal(fields[locatorKey], &name); err != nil || name != locatorFile {
		return nil, fmt.Errorf("the document does not name the locator file")
	}
	delete(fields, locatorKey)
	return json.Marshal(fields)
}

// decodeGatewayTwin strictly decodes one twin.
func decodeGatewayTwin(data []byte) (*gatewayTwin, error) {
	data, err := withoutLocatorKey(data)
	if err != nil {
		return nil, err
	}
	twin := &gatewayTwin{}
	if err := decodeStrict(data, twin); err != nil {
		return nil, err
	}
	return twin, nil
}

// quarantineCodes returns the codes of a twin row quarantine list, whose
// entries are either a code string or an object with a code field.
func quarantineCodes(entries []json.RawMessage) ([]string, error) {
	var codes []string
	for _, raw := range entries {
		var code string
		if err := json.Unmarshal(raw, &code); err == nil {
			codes = append(codes, code)
			continue
		}
		var object struct {
			Code   string `json:"code"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(raw, &object); err != nil || object.Code == "" || object.Status != "unresolved_vendor_source" {
			return nil, fmt.Errorf("quarantine entry %s is neither a code nor an unresolved code object", string(raw))
		}
		codes = append(codes, object.Code)
	}
	sort.Strings(codes)
	return codes, nil
}

// gatewayEndpointOf maps a twin namespace and alias target to an endpoint.
func gatewayEndpointOf(register gatewayRegister) (string, error) {
	if register.Namespace == remappedNamespace {
		if register.AliasTargetKind == nil {
			return "", fmt.Errorf("remapped row without a target kind")
		}
		endpoint, ok := remapTargets[*register.AliasTargetKind]
		if !ok {
			return "", fmt.Errorf("unknown remap target %q", *register.AliasTargetKind)
		}
		if register.AddressKind != addressBlockOffset && register.AddressKind != addressBlockConflict {
			return "", fmt.Errorf("remapped row with address kind %q", register.AddressKind)
		}
		return endpoint, nil
	}
	endpoint, ok := gatewayNamespaces[register.Namespace]
	if !ok {
		return "", fmt.Errorf("unknown namespace %q", register.Namespace)
	}
	if register.AliasTargetKind != nil || register.AddressKind != addressLiteral {
		return "", fmt.Errorf("literal-namespace row with a target kind or address kind %q", register.AddressKind)
	}
	return endpoint, nil
}

// gatewayRow is one converted register row with its private locator.
type gatewayRow struct {
	parsedRow
	Codes       []string
	RowSpecific []gatewayPredicate
	DetailID    string
	Summary     string
}

// gatewayClass applies the gateway class rules. Read-write rows are readable
// configuration and write-only rows are no-send; a quarantined control is
// no-send. A read-only row the twin quarantines is a conflict row; an opaque
// multi-register layout without a decode type is raw only; a row with a row
// predicate or a cell note is conditional; any other row is reviewed.
func gatewayClass(access, typeLiteral string, codes []string, predicates int, note bool) (string, error) {
	switch access {
	case "RW":
		if len(codes) > 0 {
			return classNoSend, nil
		}
		return classReadableConfiguration, nil
	case "WO":
		return classNoSend, nil
	case "RO":
	default:
		return "", fmt.Errorf("unknown access %q", access)
	}
	switch {
	case len(codes) > 0:
		return classConflict, nil
	case typeLiteral == opaqueLayoutType:
		return classRawOnly, nil
	case predicates > 0 || note:
		return classConditional, nil
	}
	return classReviewed, nil
}

// convertGatewayRegister checks the twin invariants of one row and converts
// it. The display name, ordinal, table position, section and reader path stay
// private.
func convertGatewayRegister(register gatewayRegister, sourceID string, order int) (gatewayRow, error) {
	label := register.RecordID
	switch {
	case register.SourceID != sourceID:
		return gatewayRow{}, fmt.Errorf("%s: not a row of the edition", label)
	case register.ExecutionStatus != documentaryOnly:
		return gatewayRow{}, fmt.Errorf("%s: unexpected execution status", label)
	case len(register.References) != 1:
		return gatewayRow{}, fmt.Errorf("%s: %d source references", label, len(register.References))
	case (register.Class == classMeasurement) != (register.Access == "RO"), register.Class != classMeasurement && register.Class != classConfiguration:
		return gatewayRow{}, fmt.Errorf("%s: class %q with access %q", label, register.Class, register.Access)
	case (register.OperationalMarker != nil) != (register.Class == classConfiguration):
		return gatewayRow{}, fmt.Errorf("%s: operational marker out of place", label)
	case register.Namespace != register.OriginalNamespace && (register.Namespace != counterNamespace || register.OriginalNamespace != childAliasNamespace):
		return gatewayRow{}, fmt.Errorf("%s: namespace %q from %q", label, register.Namespace, register.OriginalNamespace)
	case len(register.Conditions.Namespace) == 0:
		return gatewayRow{}, fmt.Errorf("%s: no namespace condition", label)
	}
	endpoint, err := gatewayEndpointOf(register)
	if err != nil {
		return gatewayRow{}, fmt.Errorf("%s: %w", label, err)
	}
	codes, err := quarantineCodes(register.Quarantine)
	if err != nil {
		return gatewayRow{}, fmt.Errorf("%s: %w", label, err)
	}
	for _, predicate := range register.Conditions.RowSpecific {
		if predicate.SourceID != sourceID || predicate.Kind == "" || predicate.Operator == "" || predicate.Value == "" || predicate.Summary == "" || predicate.Page < 1 {
			return gatewayRow{}, fmt.Errorf("%s: incomplete row predicate", label)
		}
	}
	summary := register.Conditions.Summary
	note := summary != noInterpretation
	class, err := gatewayClass(register.Access, register.TypeLiteral, codes, len(register.Conditions.RowSpecific), note)
	if err != nil {
		return gatewayRow{}, fmt.Errorf("%s: %w", label, err)
	}
	gain, unit := floorNotStated, floorNotStated
	if register.GainLiteral != nil {
		gain = literalCell(*register.GainLiteral)
	}
	if register.UnitLiteral != nil {
		unit = literalCell(*register.UnitLiteral)
	}
	reference := register.References[0]
	row := registerRow{
		Key: endpoint + "." + strconv.Itoa(register.Address), Endpoint: endpoint, Offset: intPointer(register.Address),
		Quantity: register.Quantity, Type: register.TypeLiteral, Access: register.Access, Class: class, Page: reference.Page,
	}
	if note {
		row.Condition = gatewayProse(summary)
	}
	applyScale(&row, gain, unit)
	private := map[string]string{
		"record": register.RecordID, "ordinal": strconv.Itoa(register.SourceOrdinal), "section": register.SourceSection,
		"table": strconv.Itoa(reference.Table), "row": strconv.Itoa(reference.Row), "reader": register.ReaderPath,
		"name": register.DisplayName, "detail": register.Conditions.DetailID, "address_kind": register.AddressKind,
	}
	if len(codes) > 0 {
		private["quarantine"] = strings.Join(codes, ",")
	}
	return gatewayRow{
		parsedRow:   parsedRow{Row: row, Evidence: register.RecordID, Private: private, Order: order},
		Codes:       codes,
		RowSpecific: register.Conditions.RowSpecific,
		DetailID:    register.Conditions.DetailID,
		Summary:     summary,
	}, nil
}

// codeList reads an enumeration from the first sentence of a cell note: an
// optional label ending in a colon, then two or more distinct "code meaning"
// items separated by semicolons. Any other form yields no enumeration; the
// note stays the row condition.
func codeList(summary string) []dictValue {
	sentences := splitSentences(cleanText(summary))
	if len(sentences) == 0 {
		return nil
	}
	first := strings.TrimSuffix(sentences[0], ".")
	if index := strings.Index(first, ": "); index >= 0 && codeItemPattern.FindStringSubmatch(first) == nil {
		first = first[index+2:]
	}
	items := strings.Split(first, "; ")
	if len(items) < 2 {
		return nil
	}
	seen := map[string]bool{}
	values := make([]dictValue, 0, len(items))
	for _, item := range items {
		match := codeItemPattern.FindStringSubmatch(item)
		if match == nil {
			return nil
		}
		meaning := strings.TrimSpace(match[2])
		lower := strings.ToLower(meaning)
		for _, prefix := range []string{"and ", "or ", "to ", "also ", "through "} {
			if strings.HasPrefix(lower, prefix) {
				return nil
			}
		}
		if meaning == "" || strings.HasPrefix(meaning, "…") || strings.ContainsAny(meaning, ";:") || seen[match[1]] {
			return nil
		}
		seen[match[1]] = true
		values = append(values, dictValue{Code: match[1], Meaning: meaning})
	}
	return values
}

// gatewayFamily returns the family that a reference title starts with.
func gatewayFamily(title string) (string, error) {
	for _, family := range gatewayFamilies {
		if strings.HasPrefix(title, family+" ") {
			return family, nil
		}
	}
	return "", fmt.Errorf("title %q names no known family", title)
}

// parseFloors converts the documented software minima into inclusive floors.
// A requirement without a component keeps the component "not stated".
func parseFloors(scope twinModelScope) ([]softwareFloor, error) {
	var floors []softwareFloor
	var statements []string
	for _, entry := range scope.Minima {
		match := floorStatementPattern.FindStringSubmatch(entry.Requirement)
		if entry.Relation != "or later" || match == nil {
			return nil, fmt.Errorf("software requirement %q is not an inclusive floor", entry.Requirement)
		}
		component := match[1]
		if component == "" {
			component = floorNotStated
		}
		floors = append(floors, softwareFloor{Component: component, Minimum: match[2], Inclusive: true})
		statements = append(statements, entry.Requirement)
	}
	if len(floors) == 0 || strings.Join(statements, "; ")+"." != scope.SoftwareRequirements {
		return nil, fmt.Errorf("software requirements %q disagree with their entries", scope.SoftwareRequirements)
	}
	return floors, nil
}
