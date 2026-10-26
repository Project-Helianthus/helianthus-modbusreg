package modbusreg

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// The gateway selection reads two files of the generated SmartLogger, S-Dongle
// and EMMA catalogue (see docs/huawei-catalogue.md): the editions, with their
// documented software minima, model scope and self unit, and the services,
// with the device-list attributes that each edition defines. It decides which
// documented floor, and so which editions, a gateway's own software text falls
// under. It decodes no register, reads no other catalogue file and grants no
// authority.
//
//go:embed profiles/vendor/huawei/catalogue/smartlogger-sdongle-emma/editions.json profiles/vendor/huawei/catalogue/smartlogger-sdongle-emma/services.json
var huaweiGatewayCatalogueFiles embed.FS

const huaweiGatewayCatalogueDirectory = "profiles/vendor/huawei/catalogue/smartlogger-sdongle-emma"

// HuaweiGatewayEndpoint is the endpoint class in which the identity was read.
// A gateway's own identity is read only in its self endpoint.
type HuaweiGatewayEndpoint string

const HuaweiGatewayEndpointSelf HuaweiGatewayEndpoint = "gateway-self"

// HuaweiGatewayScope is the endpoint class and the unit that the identity
// reads were addressed to.
type HuaweiGatewayScope struct {
	Endpoint HuaweiGatewayEndpoint
	Unit     uint8
}

// HuaweiGatewaySelectionInput is the gateway's own identity as already read.
// It carries no endpoint address, session or transport state.
type HuaweiGatewaySelectionInput struct {
	// Family is the canonical class of the card that asks: SmartLogger,
	// S-Dongle or EMMA.
	Family string
	Scope  HuaweiGatewayScope
	// Model is the gateway's model text, or empty when the card compares no
	// model with the catalogue rows.
	Model string
	// Software is the gateway's own software text, or empty when it was not
	// read.
	Software string
}

// HuaweiGatewaySelectionOutcome is the outcome of the gateway selection.
type HuaweiGatewaySelectionOutcome string

const (
	HuaweiGatewaySelected    HuaweiGatewaySelectionOutcome = "SELECTED"
	HuaweiGatewayNotSelected HuaweiGatewaySelectionOutcome = "NOT_SELECTED"
	HuaweiGatewayAmbiguous   HuaweiGatewaySelectionOutcome = "AMBIGUOUS"
)

// HuaweiGatewaySelectionReason is the closed reason of a gateway selection.
type HuaweiGatewaySelectionReason string

const (
	// Reasons of an admitting outcome.
	HuaweiGatewayAtOrAboveFloor       HuaweiGatewaySelectionReason = "at_or_above_floor"
	HuaweiGatewayNoDocumentedRelation HuaweiGatewaySelectionReason = "no_documented_branch_relation"
	HuaweiGatewayMultipleFloors       HuaweiGatewaySelectionReason = "multiple_floors"
	HuaweiGatewaySoftwareNotRead      HuaweiGatewaySelectionReason = "software_not_read"
	// Reasons of a refusing outcome.
	HuaweiGatewayFamilyMismatch    HuaweiGatewaySelectionReason = "family_mismatch"
	HuaweiGatewayBelowFloor        HuaweiGatewaySelectionReason = "below_floor"
	HuaweiGatewaySoftwareMalformed HuaweiGatewaySelectionReason = "software_malformed"
	HuaweiGatewayCatalogueInvalid  HuaweiGatewaySelectionReason = "catalogue_invalid"
)

// HuaweiGatewayFloorDomain is the extent within which a floor orders builds.
type HuaweiGatewayFloorDomain string

const (
	// HuaweiGatewayFloorLine: the floor is a minimum of its line (V). The
	// family documents an order across the releases of that line, so builds
	// of one line are ordered by release, branch and build number.
	HuaweiGatewayFloorLine HuaweiGatewayFloorDomain = "LINE"
	// HuaweiGatewayFloorBranch: the floor is a minimum of its branch (V, R,
	// C). Builds of the branch are ordered by build number, and no two
	// branches are ordered.
	HuaweiGatewayFloorBranch HuaweiGatewayFloorDomain = "BRANCH"
)

// HuaweiGatewayFloorRelation is how a software text relates to a floor.
type HuaweiGatewayFloorRelation string

const (
	HuaweiGatewayRelationAtOrAbove            HuaweiGatewayFloorRelation = "AT_OR_ABOVE"
	HuaweiGatewayRelationBelow                HuaweiGatewayFloorRelation = "BELOW"
	HuaweiGatewayRelationNoDocumentedRelation HuaweiGatewayFloorRelation = "NO_DOCUMENTED_RELATION"
	HuaweiGatewayRelationNotEvaluated         HuaweiGatewayFloorRelation = "NOT_EVALUATED"
)

// HuaweiGatewayEdition is one catalogue edition as the selection saw it: its
// identity as the edition states it and the device-list attributes that its
// services entry defines.
type HuaweiGatewayEdition struct {
	ID     string
	Title  string
	Issue  string
	Date   string
	Branch string
	// DescriptorKeys are the device-list attribute numbers the edition defines.
	DescriptorKeys []int
	// AttributeSix is the edition's grammar of attribute 6. It is recorded and
	// never decoded.
	AttributeSix string
	// Quarantine names the quarantined topics of the edition's services entry.
	Quarantine []string
}

func (e HuaweiGatewayEdition) clone() HuaweiGatewayEdition {
	e.DescriptorKeys = append([]int(nil), e.DescriptorKeys...)
	e.Quarantine = append([]string(nil), e.Quarantine...)
	return e
}

// HuaweiGatewayFloor is one documented software minimum of a family together
// with every edition that states it. Editions that state one floor document
// one applicability and are one candidate.
type HuaweiGatewayFloor struct {
	Component string
	Minimum   string
	Domain    HuaweiGatewayFloorDomain
	Relation  HuaweiGatewayFloorRelation
	Editions  []HuaweiGatewayEdition
}

func (f HuaweiGatewayFloor) clone() HuaweiGatewayFloor {
	editions := make([]HuaweiGatewayEdition, len(f.Editions))
	for index, edition := range f.Editions {
		editions[index] = edition.clone()
	}
	f.Editions = editions
	return f
}

// EditionIDs lists the catalogue keys of the editions that state the floor.
func (f HuaweiGatewayFloor) EditionIDs() []string {
	ids := make([]string, len(f.Editions))
	for index, edition := range f.Editions {
		ids[index] = edition.ID
	}
	return ids
}

// HuaweiGatewaySelection is the result of the gateway selection. A floor is
// selected only when exactly one candidate floor admits the software text;
// order, priority and edition name never choose a winner. A software text
// below every floor of its own domain is not selected; a text of a domain that
// no floor documents is admitted with grade Hypothesis and never refused.
type HuaweiGatewaySelection struct {
	outcome    HuaweiGatewaySelectionOutcome
	reason     HuaweiGatewaySelectionReason
	family     string
	scope      HuaweiGatewayScope
	model      string
	software   string
	build      string
	grade      HuaweiEvidenceGrade
	selected   *HuaweiGatewayFloor
	candidates []HuaweiGatewayFloor
}

func (s HuaweiGatewaySelection) Outcome() HuaweiGatewaySelectionOutcome { return s.outcome }
func (s HuaweiGatewaySelection) Reason() HuaweiGatewaySelectionReason   { return s.reason }
func (s HuaweiGatewaySelection) Family() string                         { return s.family }
func (s HuaweiGatewaySelection) Scope() HuaweiGatewayScope              { return s.scope }
func (s HuaweiGatewaySelection) Model() string                          { return s.model }
func (s HuaweiGatewaySelection) Software() string                       { return s.software }

// Build is the version part of the software text that was compared, without
// a component label.
func (s HuaweiGatewaySelection) Build() string { return s.build }

// Grade is the grade of the facts that the selection lets the card apply:
// Hypothesis for an admitted build until an observation of that build grades
// it Proven, and Unknown when nothing was compared or nothing is admitted.
func (s HuaweiGatewaySelection) Grade() HuaweiEvidenceGrade {
	if s.grade == "" {
		return HuaweiGradeUnknown
	}
	return s.grade
}

// Admitted reports that the selection does not refuse the identity: the
// family matched and the software text is not below the floors of its own
// domain. An ambiguous selection admits the family and binds no edition.
func (s HuaweiGatewaySelection) Admitted() bool {
	return s.outcome == HuaweiGatewaySelected || s.outcome == HuaweiGatewayAmbiguous
}

// Floor is the selected floor, or nothing.
func (s HuaweiGatewaySelection) Floor() (HuaweiGatewayFloor, bool) {
	if s.selected == nil {
		return HuaweiGatewayFloor{}, false
	}
	return s.selected.clone(), true
}

// Candidates are the floors the selection named: the selected floor, every
// admitting floor of an ambiguous selection, or the floors that refused.
func (s HuaweiGatewaySelection) Candidates() []HuaweiGatewayFloor {
	out := make([]HuaweiGatewayFloor, len(s.candidates))
	for index, floor := range s.candidates {
		out[index] = floor.clone()
	}
	return out
}

// DescriptorKeys is the device-list attribute dictionary of the editions that
// the selection admits: the union of the attributes they define, because a
// fact documented by an applicable edition applies to the build.
func (s HuaweiGatewaySelection) DescriptorKeys() []int {
	if !s.Admitted() {
		return nil
	}
	seen := map[int]struct{}{}
	for _, floor := range s.candidates {
		for _, edition := range floor.Editions {
			for _, key := range edition.DescriptorKeys {
				seen[key] = struct{}{}
			}
		}
	}
	keys := make([]int, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}

// DefinesAttribute reports whether an admitted edition defines the
// device-list attribute named by key.
func (s HuaweiGatewaySelection) DefinesAttribute(key string) bool {
	number, ok := huaweiDescriptorKeyNumber(key)
	if !ok {
		return false
	}
	for _, defined := range s.DescriptorKeys() {
		if defined == number {
			return true
		}
	}
	return false
}

// AttributeSixGrammars lists the distinct grammars of attribute 6 among the
// admitted editions. Attribute 6 is never decoded.
func (s HuaweiGatewaySelection) AttributeSixGrammars() []string {
	if !s.Admitted() {
		return nil
	}
	seen := map[string]struct{}{}
	for _, floor := range s.candidates {
		for _, edition := range floor.Editions {
			seen[edition.AttributeSix] = struct{}{}
		}
	}
	grammars := make([]string, 0, len(seen))
	for grammar := range seen {
		grammars = append(grammars, grammar)
	}
	sort.Strings(grammars)
	return grammars
}

func (HuaweiGatewaySelection) LiveQualified() bool  { return false }
func (HuaweiGatewaySelection) SupportClaim() bool   { return false }
func (HuaweiGatewaySelection) WriteAuthority() bool { return false }

// withObservedBuild grades an admitted selection Proven when its build is the
// build a card observed. Every other build keeps its grade.
func (s HuaweiGatewaySelection) withObservedBuild(observed string) HuaweiGatewaySelection {
	if s.Admitted() && s.build != "" && s.build == observed {
		s.grade = HuaweiGradeProven
	}
	return s
}

// huaweiGatewayFamilyRule is the registry's reading of a family's own software
// text: the component that names the family's own floors in the catalogue,
// and whether the text carries that component label and a space before the
// version.
type huaweiGatewayFamilyRule struct {
	family    string
	component string
	labelled  bool
}

const huaweiGatewayComponentNotStated = "not stated"

var huaweiGatewayFamilyRules = []huaweiGatewayFamilyRule{
	{family: HuaweiSmartLoggerCanonicalClass, component: "SmartLogger"},
	{family: HuaweiSDongleCanonicalClass, component: huaweiGatewayComponentNotStated},
	{family: HuaweiEMMACanonicalClass, component: "SmartHEMS", labelled: true},
}

type huaweiGatewayVersion struct {
	line, release, branch int
	build                 int
	hasBuild              bool
}

func (v huaweiGatewayVersion) text() string {
	out := fmt.Sprintf("V%03dR%03dC%02d", v.line, v.release, v.branch)
	if v.hasBuild {
		out += fmt.Sprintf("SPC%03d", v.build)
	}
	return out
}

var (
	huaweiGatewayVersionForm  = regexp.MustCompile(`^V([0-9]{3})R([0-9]{3})C([0-9]{2})(?:SPC([0-9]{3}))?$`)
	huaweiGatewaySoftwareForm = regexp.MustCompile(`^(?:([A-Za-z][A-Za-z0-9]*) )?(V[0-9]{3}R[0-9]{3}C[0-9]{2}(?:SPC[0-9]{3})?)$`)
	huaweiGatewayModelToken   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

	errHuaweiGatewayCatalogueInvalid = errors.New("invalid SmartLogger, S-Dongle and EMMA catalogue")
)

func parseHuaweiGatewayVersion(text string) (huaweiGatewayVersion, bool) {
	parts := huaweiGatewayVersionForm.FindStringSubmatch(text)
	if parts == nil {
		return huaweiGatewayVersion{}, false
	}
	line, _ := strconv.Atoi(parts[1])
	release, _ := strconv.Atoi(parts[2])
	branch, _ := strconv.Atoi(parts[3])
	version := huaweiGatewayVersion{line: line, release: release, branch: branch}
	if parts[4] != "" {
		version.build, _ = strconv.Atoi(parts[4])
		version.hasBuild = true
	}
	return version, true
}

type huaweiGatewayFloorRecord struct {
	component string
	minimum   string
	version   huaweiGatewayVersion
}

type huaweiGatewayFamily struct {
	rule     huaweiGatewayFamilyRule
	selfUnit uint8
	editions []huaweiGatewayFamilyEdition
	// orderedLines are the lines across whose releases the family documents
	// an order.
	orderedLines map[int]bool
	// buildRequired is set when every own floor of the family names a build
	// number, so a software text must carry one to be compared.
	buildRequired bool
}

type huaweiGatewayFamilyEdition struct {
	edition    HuaweiGatewayEdition
	models     []string
	exhaustive bool
	floors     []huaweiGatewayFloorRecord
}

type huaweiGatewayCatalogue struct {
	families map[string]huaweiGatewayFamily
	err      error
}

var (
	huaweiGatewayCatalogueOnce   sync.Once
	huaweiGatewayCatalogueLoaded huaweiGatewayCatalogue
)

func huaweiGatewayLoadedCatalogue() huaweiGatewayCatalogue {
	huaweiGatewayCatalogueOnce.Do(func() {
		families, err := loadHuaweiGatewayCatalogue(func(name string) ([]byte, error) {
			return huaweiGatewayCatalogueFiles.ReadFile(huaweiGatewayCatalogueDirectory + "/" + name)
		})
		huaweiGatewayCatalogueLoaded = huaweiGatewayCatalogue{families: families, err: err}
	})
	return huaweiGatewayCatalogueLoaded
}

func decodeHuaweiGatewayCatalogueFile(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return errHuaweiGatewayCatalogueInvalid
	}
	return nil
}

// parseHuaweiDescriptorKeys reads the defined attribute numbers from a
// services entry: the list before the first semicolon, of single numbers and
// inclusive ranges. The notes after the semicolon define no attribute.
func parseHuaweiDescriptorKeys(text string) ([]int, bool) {
	defined, _, _ := strings.Cut(text, ";")
	seen := map[int]struct{}{}
	for _, item := range strings.Split(defined, ",") {
		item = strings.TrimSpace(item)
		low, high, ranged := strings.Cut(item, "–")
		if !ranged {
			low, high, ranged = strings.Cut(item, "-")
		}
		if !ranged {
			high = low
		}
		first, ok := huaweiDescriptorKeyNumber(strings.TrimSpace(low))
		if !ok {
			return nil, false
		}
		last, ok := huaweiDescriptorKeyNumber(strings.TrimSpace(high))
		if !ok || last < first {
			return nil, false
		}
		for key := first; key <= last; key++ {
			seen[key] = struct{}{}
		}
	}
	keys := make([]int, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys, len(keys) > 0
}

// huaweiDescriptorKeyNumber accepts the canonical decimal form of an
// attribute number from 1 through 255: no sign, no space and no leading zero.
func huaweiDescriptorKeyNumber(key string) (int, bool) {
	if key == "" || len(key) > 3 || key[0] == '0' {
		return 0, false
	}
	number := 0
	for _, digit := range []byte(key) {
		if digit < '0' || digit > '9' {
			return 0, false
		}
		number = number*10 + int(digit-'0')
	}
	return number, number <= 255
}

// loadHuaweiGatewayCatalogue reads the editions and the services entries and
// binds them per family. An edition's own floors are the floors whose
// component is the family's own component. A family orders the releases of a
// line when one of its editions, issued for a release of that line, states a
// floor in another release or branch of the same line: the edition applies to
// its own release, so it documents that its own release follows the floor.
func loadHuaweiGatewayCatalogue(read func(string) ([]byte, error)) (map[string]huaweiGatewayFamily, error) {
	var editions struct {
		Editions []struct {
			ID         string   `json:"id"`
			Title      string   `json:"title"`
			Issue      string   `json:"issue"`
			Date       string   `json:"date"`
			Family     string   `json:"family"`
			Branch     string   `json:"branch"`
			Models     []string `json:"models"`
			Exhaustive bool     `json:"model_list_exhaustive"`
			Floors     []struct {
				Component string `json:"component"`
				Minimum   string `json:"minimum"`
				Inclusive bool   `json:"inclusive"`
			} `json:"floors"`
			SelfUnit *int `json:"self_unit"`
		} `json:"editions"`
	}
	var services struct {
		Editions []struct {
			Edition      string   `json:"edition"`
			SelfUnit     *int     `json:"self_unit"`
			AttributeSix string   `json:"attribute_6"`
			Descriptor   string   `json:"descriptor_attributes"`
			Quarantine   []string `json:"quarantine"`
		} `json:"editions"`
	}
	for name, target := range map[string]any{"editions.json": &editions, "services.json": &services} {
		data, err := read(name)
		if err != nil {
			return nil, err
		}
		if err := decodeHuaweiGatewayCatalogueFile(data, target); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	type serviceEntry struct {
		selfUnit     int
		keys         []int
		attributeSix string
		quarantine   []string
	}
	entries := map[string]serviceEntry{}
	for _, entry := range services.Editions {
		keys, ok := parseHuaweiDescriptorKeys(entry.Descriptor)
		if _, repeated := entries[entry.Edition]; repeated || !ok || entry.SelfUnit == nil || entry.AttributeSix == "" {
			return nil, fmt.Errorf("%w: services entry %q", errHuaweiGatewayCatalogueInvalid, entry.Edition)
		}
		entries[entry.Edition] = serviceEntry{selfUnit: *entry.SelfUnit, keys: keys, attributeSix: entry.AttributeSix, quarantine: append([]string(nil), entry.Quarantine...)}
	}
	rules := map[string]huaweiGatewayFamilyRule{}
	for _, rule := range huaweiGatewayFamilyRules {
		rules[rule.family] = rule
	}
	families := map[string]huaweiGatewayFamily{}
	for _, source := range editions.Editions {
		rule, known := rules[source.Family]
		if !known {
			continue
		}
		service, ok := entries[source.ID]
		if !ok || source.SelfUnit == nil || *source.SelfUnit != service.selfUnit || service.selfUnit < 0 || service.selfUnit > 247 || len(source.Models) == 0 {
			return nil, fmt.Errorf("%w: edition %q", errHuaweiGatewayCatalogueInvalid, source.ID)
		}
		edition := huaweiGatewayFamilyEdition{
			edition: HuaweiGatewayEdition{
				ID: source.ID, Title: source.Title, Issue: source.Issue, Date: source.Date, Branch: source.Branch,
				DescriptorKeys: service.keys, AttributeSix: service.attributeSix, Quarantine: service.quarantine,
			},
			models:     append([]string(nil), source.Models...),
			exhaustive: source.Exhaustive,
		}
		for _, floor := range source.Floors {
			if !floor.Inclusive {
				return nil, fmt.Errorf("%w: edition %q states a floor that is not inclusive", errHuaweiGatewayCatalogueInvalid, source.ID)
			}
			if floor.Component != rule.component {
				continue
			}
			version, ok := parseHuaweiGatewayVersion(floor.Minimum)
			if !ok {
				return nil, fmt.Errorf("%w: edition %q floor %q", errHuaweiGatewayCatalogueInvalid, source.ID, floor.Minimum)
			}
			edition.floors = append(edition.floors, huaweiGatewayFloorRecord{component: floor.Component, minimum: floor.Minimum, version: version})
		}
		if len(edition.floors) == 0 {
			return nil, fmt.Errorf("%w: edition %q states no floor of its family", errHuaweiGatewayCatalogueInvalid, source.ID)
		}
		family, seen := families[rule.family]
		if !seen {
			family = huaweiGatewayFamily{rule: rule, selfUnit: uint8(service.selfUnit), orderedLines: map[int]bool{}, buildRequired: true}
		} else if int(family.selfUnit) != service.selfUnit {
			return nil, fmt.Errorf("%w: family %s has two self units", errHuaweiGatewayCatalogueInvalid, rule.family)
		}
		if cover, ok := parseHuaweiGatewayVersion(source.Branch); ok {
			for _, floor := range edition.floors {
				if floor.version.line == cover.line && (floor.version.release != cover.release || floor.version.branch != cover.branch) {
					family.orderedLines[cover.line] = true
				}
			}
		}
		for _, floor := range edition.floors {
			family.buildRequired = family.buildRequired && floor.version.hasBuild
		}
		family.editions = append(family.editions, edition)
		families[rule.family] = family
	}
	for _, rule := range huaweiGatewayFamilyRules {
		if len(families[rule.family].editions) == 0 {
			return nil, fmt.Errorf("%w: family %s has no edition", errHuaweiGatewayCatalogueInvalid, rule.family)
		}
	}
	return families, nil
}

// DecideHuaweiGatewaySelection decides which documented floor of a gateway
// family the gateway's own software text falls under. The family must be one
// of the catalogue's, the scope its self endpoint at its self unit, and a
// model, when given, one that the family's model rows hold. The software text
// is compared with every floor of the family in the floor's own domain: a
// floor of the text's line or branch admits it at or above the minimum and
// refuses it below; a floor of another line or branch has no documented
// relation to the text and neither admits nor refuses it. A text that no floor
// relates to is admitted with grade Hypothesis. Exactly one admitting floor is
// selected; two or more are ambiguous and none is selected.
func DecideHuaweiGatewaySelection(input HuaweiGatewaySelectionInput) HuaweiGatewaySelection {
	return decideHuaweiGatewaySelection(huaweiGatewayLoadedCatalogue(), input)
}

func decideHuaweiGatewaySelection(catalogue huaweiGatewayCatalogue, input HuaweiGatewaySelectionInput) HuaweiGatewaySelection {
	decision := HuaweiGatewaySelection{
		outcome: HuaweiGatewayNotSelected, family: input.Family, scope: input.Scope, model: input.Model, software: input.Software,
		grade: HuaweiGradeUnknown,
	}
	refuse := func(reason HuaweiGatewaySelectionReason, candidates []HuaweiGatewayFloor) HuaweiGatewaySelection {
		decision.outcome, decision.reason, decision.grade, decision.candidates = HuaweiGatewayNotSelected, reason, HuaweiGradeUnknown, candidates
		return decision
	}
	if catalogue.err != nil || len(catalogue.families) == 0 {
		return refuse(HuaweiGatewayCatalogueInvalid, nil)
	}
	family, known := catalogue.families[input.Family]
	if !known || input.Scope.Endpoint != HuaweiGatewayEndpointSelf || input.Scope.Unit != family.selfUnit {
		return refuse(HuaweiGatewayFamilyMismatch, nil)
	}
	applicable := family.editions
	if input.Model != "" {
		applicable = nil
		for _, edition := range family.editions {
			if huaweiGatewayModelInRows(input.Model, edition.models, edition.exhaustive) {
				applicable = append(applicable, edition)
			}
		}
		if len(applicable) == 0 {
			return refuse(HuaweiGatewayFamilyMismatch, nil)
		}
	}
	floors := huaweiGatewayFloors(family, applicable)
	if len(floors) == 0 {
		return refuse(HuaweiGatewayFamilyMismatch, nil)
	}
	if input.Software == "" {
		decision.outcome, decision.reason, decision.candidates = HuaweiGatewayAmbiguous, HuaweiGatewaySoftwareNotRead, floors
		return decision
	}
	build, reason := parseHuaweiGatewaySoftware(catalogue, family, input.Software)
	if reason != "" {
		return refuse(reason, nil)
	}
	decision.build = build.text()
	var admitted, below, unrelated []HuaweiGatewayFloor
	for _, floor := range floors {
		floor.Relation = huaweiGatewayRelationOf(family, build, floor.Minimum)
		switch floor.Relation {
		case HuaweiGatewayRelationAtOrAbove:
			admitted = append(admitted, floor)
		case HuaweiGatewayRelationBelow:
			below = append(below, floor)
		default:
			unrelated = append(unrelated, floor)
		}
	}
	switch {
	case len(admitted) == 1:
		selected := admitted[0]
		decision.outcome, decision.reason, decision.grade = HuaweiGatewaySelected, HuaweiGatewayAtOrAboveFloor, HuaweiGradeHypothesis
		decision.selected, decision.candidates = &selected, []HuaweiGatewayFloor{selected.clone()}
	case len(admitted) > 1:
		decision.outcome, decision.reason, decision.grade, decision.candidates = HuaweiGatewayAmbiguous, HuaweiGatewayMultipleFloors, HuaweiGradeHypothesis, admitted
	case len(below) > 0:
		return refuse(HuaweiGatewayBelowFloor, below)
	case len(unrelated) == 1:
		selected := unrelated[0]
		decision.outcome, decision.reason, decision.grade = HuaweiGatewaySelected, HuaweiGatewayNoDocumentedRelation, HuaweiGradeHypothesis
		decision.selected, decision.candidates = &selected, []HuaweiGatewayFloor{selected.clone()}
	default:
		decision.outcome, decision.reason, decision.grade, decision.candidates = HuaweiGatewayAmbiguous, HuaweiGatewayNoDocumentedRelation, HuaweiGradeHypothesis, unrelated
	}
	return decision
}

// huaweiGatewayRelationOfFloor compares a software text with a floor that a
// card states on its own, such as an observed build, under the domain rules
// of the family's catalogue floors.
func huaweiGatewayRelationOfFloor(familyName, software, minimum string) HuaweiGatewayFloorRelation {
	catalogue := huaweiGatewayLoadedCatalogue()
	family, known := catalogue.families[familyName]
	if catalogue.err != nil || !known {
		return HuaweiGatewayRelationNotEvaluated
	}
	build, reason := parseHuaweiGatewaySoftware(catalogue, family, software)
	if reason != "" {
		return HuaweiGatewayRelationNotEvaluated
	}
	return huaweiGatewayRelationOf(family, build, minimum)
}

// huaweiGatewayFloors groups the own floors of the applicable editions by
// their minimum. The list is sorted by minimum for a stable record; the order
// selects nothing.
func huaweiGatewayFloors(family huaweiGatewayFamily, editions []huaweiGatewayFamilyEdition) []HuaweiGatewayFloor {
	byMinimum := map[string]*HuaweiGatewayFloor{}
	for _, edition := range editions {
		for _, floor := range edition.floors {
			record, seen := byMinimum[floor.minimum]
			if !seen {
				domain := HuaweiGatewayFloorBranch
				if family.orderedLines[floor.version.line] {
					domain = HuaweiGatewayFloorLine
				}
				record = &HuaweiGatewayFloor{Component: floor.component, Minimum: floor.minimum, Domain: domain, Relation: HuaweiGatewayRelationNotEvaluated}
				byMinimum[floor.minimum] = record
			}
			record.Editions = append(record.Editions, edition.edition.clone())
		}
	}
	floors := make([]HuaweiGatewayFloor, 0, len(byMinimum))
	for _, record := range byMinimum {
		sort.Slice(record.Editions, func(i, j int) bool { return record.Editions[i].ID < record.Editions[j].ID })
		floors = append(floors, *record)
	}
	sort.Slice(floors, func(i, j int) bool { return floors[i].Minimum < floors[j].Minimum })
	return floors
}

// parseHuaweiGatewaySoftware reads a family's software text. A labelled family
// requires its component label and a space; a label of another family's
// component is a family mismatch; any other label, or a version outside the
// V, R, C and optional SPC form, is malformed. A family whose every floor
// names a build number requires the text to name one.
func parseHuaweiGatewaySoftware(catalogue huaweiGatewayCatalogue, family huaweiGatewayFamily, text string) (huaweiGatewayVersion, HuaweiGatewaySelectionReason) {
	parts := huaweiGatewaySoftwareForm.FindStringSubmatch(text)
	if parts == nil {
		return huaweiGatewayVersion{}, HuaweiGatewaySoftwareMalformed
	}
	label := parts[1]
	switch {
	case label == "" && family.rule.labelled:
		return huaweiGatewayVersion{}, HuaweiGatewaySoftwareMalformed
	case label != "" && label != family.rule.component:
		for name, other := range catalogue.families {
			if name != family.rule.family && label == other.rule.component {
				return huaweiGatewayVersion{}, HuaweiGatewayFamilyMismatch
			}
		}
		return huaweiGatewayVersion{}, HuaweiGatewaySoftwareMalformed
	}
	version, ok := parseHuaweiGatewayVersion(parts[2])
	if !ok || (family.buildRequired && !version.hasBuild) {
		return huaweiGatewayVersion{}, HuaweiGatewaySoftwareMalformed
	}
	return version, ""
}

// huaweiGatewayRelationOf compares a build with a floor in the floor's
// domain: within the line when the family orders the line's releases, and
// within the branch otherwise. A build without a build number is the base of
// its branch, which precedes every build number of that branch.
func huaweiGatewayRelationOf(family huaweiGatewayFamily, build huaweiGatewayVersion, minimum string) HuaweiGatewayFloorRelation {
	floor, ok := parseHuaweiGatewayVersion(minimum)
	if !ok || floor.line != build.line {
		return HuaweiGatewayRelationNoDocumentedRelation
	}
	if !family.orderedLines[floor.line] && (floor.release != build.release || floor.branch != build.branch) {
		return HuaweiGatewayRelationNoDocumentedRelation
	}
	switch {
	case build.release != floor.release:
		return huaweiGatewayOrdered(build.release > floor.release)
	case build.branch != floor.branch:
		return huaweiGatewayOrdered(build.branch > floor.branch)
	case !floor.hasBuild:
		return HuaweiGatewayRelationAtOrAbove
	case !build.hasBuild:
		return HuaweiGatewayRelationBelow
	}
	return huaweiGatewayOrdered(build.build >= floor.build)
}

func huaweiGatewayOrdered(atOrAbove bool) HuaweiGatewayFloorRelation {
	if atOrAbove {
		return HuaweiGatewayRelationAtOrAbove
	}
	return HuaweiGatewayRelationBelow
}

// huaweiGatewayFamilyHoldsModel reports whether the model rows of any edition
// of a family hold a model text.
func huaweiGatewayFamilyHoldsModel(familyName, model string) bool {
	catalogue := huaweiGatewayLoadedCatalogue()
	family, known := catalogue.families[familyName]
	if catalogue.err != nil || !known {
		return false
	}
	for _, edition := range family.editions {
		if huaweiGatewayModelInRows(model, edition.models, edition.exhaustive) {
			return true
		}
	}
	return false
}

// huaweiGatewayModelInRows reports whether a model text is held by an
// edition's model rows. An exhaustive list holds exactly its rows. A list that
// is not exhaustive names a family, not models, so it holds a model token of
// that family: the family name, a hyphen and a non-empty token. A row may carry an explanatory
// parenthetical after the family name; only the name before it is compared. The family
// name alone is not a model.
func huaweiGatewayModelInRows(model string, rows []string, exhaustive bool) bool {
	if exhaustive {
		for _, row := range rows {
			if model == row {
				return true
			}
		}
		return false
	}
	if !huaweiGatewayModelToken.MatchString(model) {
		return false
	}
	family, rest, found := strings.Cut(model, "-")
	if !found || rest == "" {
		return false
	}
	for _, row := range rows {
		name, _, _ := strings.Cut(row, " (")
		if family == strings.TrimSpace(name) {
			return true
		}
	}
	return false
}
