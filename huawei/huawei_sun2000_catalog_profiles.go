package huawei

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/profiles"
)

// CatalogMonitoringProfile is the monitoring profile generated from one
// catalogue card. Catalogue facts are documentary, so their grade is
// Hypothesis. The profile sends nothing.
type CatalogMonitoringProfile struct {
	id      string
	card    string
	family  string
	prefix  string
	strings int
	groups  []catalogGroup
	fields  []catalogField
	states  map[uint16]string
	alarms  map[uint16]map[int]string
}

// CatalogMonitoringReading is one decoded catalogue profile.
type CatalogMonitoringReading struct {
	facts  []SUN2000Fact
	groups []SUN2000GroupReading
	losses []ProviderLoss
}

type catalogKind uint8

const (
	catalogString catalogKind = iota + 1
	catalogPower
	catalogEnergy
	catalogStatus
	catalogAlarm
	catalogTracker
	catalogRating
)

type catalogGroup struct {
	offset, quantity uint16
}

type catalogField struct {
	name     string
	index    int
	offset   uint16
	width    uint16
	signed   bool
	exponent int16
	unit     string
	kind     catalogKind
}

type catalogRegisterFile struct {
	Card string               `json:"card"`
	Rows []catalogRegisterRow `json:"rows"`
}

type catalogRegisterRow struct {
	Key             string       `json:"key"`
	Endpoint        string       `json:"endpoint"`
	Offset          *int         `json:"offset"`
	Quantity        int          `json:"quantity"`
	Type            string       `json:"type"`
	Access          string       `json:"access"`
	Class           string       `json:"class"`
	FunctionalClass string       `json:"functional_class"`
	Decode          *catalogCell `json:"decode"`
}

type catalogCell struct {
	Gain string `json:"gain"`
	Unit string `json:"unit"`
}

type catalogDictFile struct {
	Dictionaries []catalogDict `json:"dictionaries"`
}

type catalogDict struct {
	Kind    string            `json:"kind"`
	Card    string            `json:"card"`
	Offsets []int             `json:"offsets"`
	Values  []catalogDictCode `json:"values"`
	Bits    []catalogDictBit  `json:"bits"`
}

type catalogDictCode struct {
	Code    string `json:"code"`
	Meaning string `json:"meaning"`
}

type catalogDictBit struct {
	Offset *int   `json:"offset"`
	Bit    int    `json:"bit"`
	Label  string `json:"label"`
}

type catalogBundle struct {
	profiles []CatalogMonitoringProfile
	labelErr error
}

var (
	catalogOnce   sync.Once
	catalogLoaded catalogBundle

	errCatalogLabelsUnavailable = errors.New("catalogue status and alarm labels are unavailable")

	catalogStringNumber  = regexp.MustCompile(`(?i)pv_channel(\d+)`)
	catalogDirectChannel = regexp.MustCompile(`(?i)dc_channel_(\d+)`)
	catalogStringShort   = regexp.MustCompile(`(?i)pv(\d+)`)
	catalogTrackerNumber = regexp.MustCompile(`(?i)mppt_?(\d+)`)
)

// catalogLabelDocument is the status and alarm labels compiled into the module.
// Loading a profile reads this document and does not open the source tree.
var catalogLabelDocument = []byte(`{"dictionaries":[{"kind":"alarm_bits","card":"COMMERCIAL","offsets":[32008,32009,32010],"bits":[{"bit":0,"label":"alarm.highstringvolts","offset":32008},{"bit":1,"label":"alarm.dcarcfault","offset":32008},{"bit":2,"label":"alarm.stringreversed","offset":32008},{"bit":3,"label":"alarm.pvstringbackfeed","offset":32008},{"bit":4,"label":"alarm.faultstring","offset":32008},{"bit":5,"label":"alarm.afciself_testfault","offset":32008},{"bit":6,"label":"alarm.shortcircuitbetweenphasetope","offset":32008},{"bit":7,"label":"alarm.powergridfailure","offset":32008},{"bit":8,"label":"alarm.gridundervolts","offset":32008},{"bit":9,"label":"alarm.gridovervolts","offset":32008},{"bit":10,"label":"alarm.unbalancedgridvolts","offset":32008},{"bit":11,"label":"alarm.gridoverfrequency","offset":32008},{"bit":12,"label":"alarm.gridunderfrequency","offset":32008},{"bit":13,"label":"alarm.gridfrequencyinstability","offset":32008},{"bit":14,"label":"alarm.outputoveramps","offset":32008},{"bit":15,"label":"alarm.largedcofoutputamps","offset":32008},{"bit":0,"label":"alarm.faultleakageamps","offset":32009},{"bit":1,"label":"alarm.faultground","offset":32009},{"bit":2,"label":"alarm.lowinsulationres","offset":32009},{"bit":3,"label":"alarm.hightemp","offset":32009},{"bit":4,"label":"alarm.faultequipment","offset":32009},{"bit":5,"label":"alarm.upgradefailed","offset":32009},{"bit":6,"label":"alarm.licenseexpired","offset":32009},{"bit":7,"label":"alarm.faultmonitorunit","offset":32009},{"bit":2,"label":"alarm.highstringvoltstoground","offset":32010}]},{"kind":"value_enum","card":"COMMERCIAL","offsets":[32089],"values":[{"code":"0x0000","meaning":"standby_initializing"},{"code":"0x0001","meaning":"standby_insulation_check"},{"code":"0x0002","meaning":"standby_irradiance_check"},{"code":"0x0003","meaning":"standby_grid_check"},{"code":"0x0100","meaning":"starting"},{"code":"0x0200","meaning":"running_mode_dependent"},{"code":"0x0201","meaning":"running_power_limited"},{"code":"0x0202","meaning":"running_self_derated"},{"code":"0x0300","meaning":"fault_shutdown"},{"code":"0x0301","meaning":"command_shutdown"},{"code":"0x0302","meaning":"ovgr_shutdown"},{"code":"0x0303","meaning":"communication_shutdown"},{"code":"0x0304","meaning":"power_limit_shutdown"},{"code":"0x0305","meaning":"manual_start_required"},{"code":"0x0306","meaning":"dc_switch_off"},{"code":"0x0401","meaning":"cosphi_p_dispatch"},{"code":"0x0402","meaning":"q_u_dispatch"},{"code":"0xA000","meaning":"standby_no_irradiance"},{"code":"0x0500","meaning":"inspection_ready"},{"code":"0x0501","meaning":"inspection_running"},{"code":"0x0600","meaning":"checking"},{"code":"0X0700","meaning":"afci_self_test"},{"code":"0X0800","meaning":"iv_scan"},{"code":"0X0900","meaning":"dc_input_check"}]},{"kind":"alarm_bits","card":"HA","offsets":[32008,32009,32010,32011,32012],"bits":[{"bit":0,"label":"alarm.highstringinputvolts","offset":32008},{"bit":1,"label":"alarm.dcarcfault","offset":32008},{"bit":2,"label":"alarm.stringreverseconnection","offset":32008},{"bit":3,"label":"alarm.stringampsbackfeed","offset":32008},{"bit":4,"label":"alarm.faultstringpower","offset":32008},{"bit":5,"label":"alarm.afciself_checkfail","offset":32008},{"bit":6,"label":"alarm.phasewireshort_circuitedtope","offset":32008},{"bit":7,"label":"alarm.gridloss","offset":32008},{"bit":8,"label":"alarm.gridundervolts","offset":32008},{"bit":9,"label":"alarm.gridovervolts","offset":32008},{"bit":10,"label":"alarm.gridvolt_imbalance","offset":32008},{"bit":11,"label":"alarm.gridoverfrequency","offset":32008},{"bit":12,"label":"alarm.gridunderfrequency","offset":32008},{"bit":13,"label":"alarm.unstablegridfrequency","offset":32008},{"bit":14,"label":"alarm.outputoveramps","offset":32008},{"bit":15,"label":"alarm.outputdccomponentoverhigh","offset":32008},{"bit":0,"label":"alarm.faultresidualamps","offset":32009},{"bit":1,"label":"alarm.faultgrounding","offset":32009},{"bit":2,"label":"alarm.lowinsulationresistance","offset":32009},{"bit":3,"label":"alarm.overtemp","offset":32009},{"bit":4,"label":"alarm.devicefault","offset":32009},{"bit":5,"label":"alarm.upgradefailedorversionmismatch","offset":32009},{"bit":6,"label":"alarm.licenseexpired","offset":32009},{"bit":7,"label":"alarm.faultymonitoringunit","offset":32009},{"bit":8,"label":"alarm.faultypowercollector","offset":32009},{"bit":10,"label":"alarm.activeislanding","offset":32009},{"bit":11,"label":"alarm.passiveislanding","offset":32009},{"bit":12,"label":"alarm.transientacovervolts","offset":32009},{"bit":13,"label":"alarm.peripheralportshortcircuit","offset":32009},{"bit":14,"label":"alarm.churnoutputoverload","offset":32009},{"bit":15,"label":"alarm.faultpvmoduleconfiguration","offset":32009},{"bit":0,"label":"alarm.optimizerfault","offset":32010},{"bit":1,"label":"alarm.built_inpidoperationfault","offset":32010},{"bit":2,"label":"alarm.highinputstringvoltstoground","offset":32010},{"bit":3,"label":"alarm.externalfanfault","offset":32010},{"bit":5,"label":"alarm.on_grid_off_gridcontrollerfault","offset":32010},{"bit":6,"label":"alarm.pvstringloss","offset":32010},{"bit":7,"label":"alarm.internalfanfault","offset":32010},{"bit":8,"label":"alarm.dcprotectionunitfault","offset":32010},{"bit":9,"label":"alarm.elcellfaultity","offset":32010},{"bit":10,"label":"alarm.activeschedulinginstructionexception","offset":32010},{"bit":11,"label":"alarm.reactivepowerdispatchinstructionfault","offset":32010},{"bit":12,"label":"alarm.faultctwiring","offset":32010},{"bit":13,"label":"alarm.dcarcfault","offset":32010},{"bit":14,"label":"alarm.thedcswitchisfault","offset":32010},{"bit":0,"label":"alarm.dcovervolts","offset":32011},{"bit":1,"label":"alarm.dcreverseconnection","offset":32011},{"bit":2,"label":"alarm.dcshortcircuitorreverseconnection","offset":32011},{"bit":3,"label":"alarm.dcserialconnection","offset":32011},{"bit":4,"label":"alarm.dcunreliableconnection","offset":32011},{"bit":5,"label":"alarm.themanagementsystemcertificatedoesnottakeeffect","offset":32011},{"bit":6,"label":"alarm.managementsystemcertificateisabouttoexpire","offset":32011},{"bit":7,"label":"alarm.managementsystemcertificateexpired","offset":32011},{"bit":8,"label":"alarm.parallellinkexception","offset":32011},{"bit":9,"label":"alarm.thelocalaccesscertificatedoesnottakeeffect","offset":32011},{"bit":10,"label":"alarm.thelocalaccesscertificateisabouttoexpire","offset":32011},{"bit":11,"label":"alarm.thelocalaccesscertificatehasexpired","offset":32011},{"bit":12,"label":"alarm.ac_sidephasesequencereversal","offset":32011},{"bit":13,"label":"alarm.stringshortcircuittoground","offset":32011},{"bit":14,"label":"alarm.linklinkfailureprotection","offset":32011},{"bit":15,"label":"alarm.theacterminaltempisfault","offset":32011},{"bit":0,"label":"alarm.thedcterminaltempisfault","offset":32012}]},{"kind":"value_enum","card":"HA","offsets":[32089],"values":[{"code":"0x0000","meaning":"standby_initializing"},{"code":"0x0001","meaning":"standby_insulation_check"},{"code":"0x0002","meaning":"standby_irradiance_check"},{"code":"0x0003","meaning":"standby_grid_check"},{"code":"0x0100","meaning":"starting"},{"code":"0x0200","meaning":"running_mode_dependent"},{"code":"0x0201","meaning":"running_power_limited"},{"code":"0x0202","meaning":"running_self_derated"},{"code":"0x0300","meaning":"fault_shutdown"},{"code":"0x0301","meaning":"command_shutdown"},{"code":"0x0302","meaning":"ovgr_shutdown"},{"code":"0x0303","meaning":"communication_shutdown"},{"code":"0x0304","meaning":"power_limit_shutdown"},{"code":"0x0305","meaning":"manual_start_required"},{"code":"0x0306","meaning":"dc_switch_off"},{"code":"0x0307","meaning":"rapid_shutdown"},{"code":"0x0308","meaning":"input_underpower"},{"code":"0x0309","meaning":"ns_protection_shutdown"},{"code":"0x030A","meaning":"command_rapid_shutdown"},{"code":"0x0401","meaning":"cosphi_p_dispatch"},{"code":"0x0402","meaning":"q_u_dispatch"},{"code":"0x0403","meaning":"pf_u_dispatch"},{"code":"0x0404","meaning":"dry_contact_dispatch"},{"code":"0x0405","meaning":"q_p_dispatch"},{"code":"0x0500","meaning":"inspection_ready"},{"code":"0x0501","meaning":"inspection_running"},{"code":"0x0600","meaning":"checking"},{"code":"0X0700","meaning":"afci_self_test"},{"code":"0X0800","meaning":"iv_scan"},{"code":"0X0900","meaning":"dc_input_check"},{"code":"0X0A00","meaning":"off_grid_charge"},{"code":"0xA000","meaning":"standby_no_irradiance"}]},{"kind":"alarm_bits","card":"HB","offsets":[32008,32009,32010,32011],"bits":[{"bit":0,"label":"alarm.dcovervolts","offset":32011},{"bit":1,"label":"alarm.dcreverseconnection","offset":32011},{"bit":6,"label":"alarm.powergridphaselineisshort_circuitedtope","offset":32008},{"bit":7,"label":"alarm.powergridfailure","offset":32008},{"bit":8,"label":"alarm.powergridundervolts","offset":32008},{"bit":9,"label":"alarm.powergridovervolts","offset":32008},{"bit":10,"label":"alarm.gridvoltsimbalance","offset":32008},{"bit":11,"label":"alarm.powergridoverfrequency","offset":32008},{"bit":12,"label":"alarm.gridunderfrequency","offset":32008},{"bit":13,"label":"alarm.unstablepowergridfrequency","offset":32008},{"bit":14,"label":"alarm.acoveramps","offset":32008},{"bit":15,"label":"alarm.dccomponenttoolarge","offset":32008},{"bit":0,"label":"alarm.residualampsfault","offset":32009},{"bit":1,"label":"alarm.thesystemgroundingisfault","offset":32009},{"bit":2,"label":"alarm.lowinsulationimpedance","offset":32009},{"bit":3,"label":"alarm.temptoohigh","offset":32009},{"bit":4,"label":"alarm.deviceexception","offset":32009},{"bit":5,"label":"alarm.upgradefailed","offset":32009},{"bit":3,"label":"alarm.externalfanexception","offset":32010},{"bit":7,"label":"alarm.theinternalfanisfault","offset":32010},{"bit":5,"label":"alarm.thecertificatedoesnottakeeffect","offset":32011},{"bit":6,"label":"alarm.thecertificateisabouttoexpire","offset":32011},{"bit":7,"label":"alarm.certificateexpired","offset":32011},{"bit":8,"label":"alarm.theparallellinkisfault","offset":32011},{"bit":15,"label":"alarm.faultacterminaltemp","offset":32011},{"bit":0,"label":"alarm.dcterminaltempfault"},{"bit":7,"label":"alarm.monitoringunitfault","offset":32009}]},{"kind":"value_enum","card":"HB","offsets":[32089],"values":[{"code":"0x0000","meaning":"standby_initializing"},{"code":"0x0001","meaning":"standby_insulation_check"},{"code":"0x0002","meaning":"standby_irradiance_check"},{"code":"0x0003","meaning":"standby_grid_check"},{"code":"0x0100","meaning":"starting"},{"code":"0x0200","meaning":"running_mode_dependent"},{"code":"0x0201","meaning":"running_power_limited"},{"code":"0x0202","meaning":"running_self_derated"},{"code":"0x0203","meaning":"off_grid_running"},{"code":"0x0300","meaning":"fault_shutdown"},{"code":"0x0301","meaning":"command_shutdown"},{"code":"0x0302","meaning":"ovgr_shutdown"},{"code":"0x0303","meaning":"communication_shutdown"},{"code":"0x0304","meaning":"power_limit_shutdown"},{"code":"0x0305","meaning":"manual_start_required"},{"code":"0x0306","meaning":"dc_switch_off"},{"code":"0x0307","meaning":"rapid_shutdown"},{"code":"0x0308","meaning":"input_underpower"},{"code":"0x0309","meaning":"ns_protection_shutdown"},{"code":"0x030A","meaning":"command_rapid_shutdown"},{"code":"0x0401","meaning":"cosphi_p_dispatch"},{"code":"0x0402","meaning":"q_u_dispatch"},{"code":"0x0403","meaning":"pf_u_dispatch"},{"code":"0x0404","meaning":"dry_contact_dispatch"},{"code":"0x0405","meaning":"q_p_dispatch"},{"code":"0x0500","meaning":"inspection_ready"},{"code":"0x0501","meaning":"inspection_running"},{"code":"0x0600","meaning":"checking"},{"code":"0x0700","meaning":"afci_self_test"},{"code":"0x0800","meaning":"iv_scan"},{"code":"0x0900","meaning":"dc_input_check"},{"code":"0x0A00","meaning":"off_grid_charge"},{"code":"0xA000","meaning":"standby_no_irradiance"},{"code":"0xA001","meaning":"standby_no_dc_input"}]},{"kind":"alarm_bits","card":"HD","offsets":[32008,32009,32010,32011,32012],"bits":[{"bit":0,"label":"alarm.stringvoltshigh","offset":32008},{"bit":1,"label":"alarm.dcarcfault","offset":32008},{"bit":2,"label":"alarm.stringreverseconnection","offset":32008},{"bit":3,"label":"alarm.stringampsbackfeed","offset":32008},{"bit":4,"label":"alarm.faultstringpower","offset":32008},{"bit":5,"label":"alarm.afcicheckfailure","offset":32008},{"bit":6,"label":"alarm.phasewireshort_circuitedtope","offset":32008},{"bit":7,"label":"alarm.gridfailure","offset":32008},{"bit":8,"label":"alarm.gridundervolts","offset":32008},{"bit":9,"label":"alarm.gridovervolts","offset":32008},{"bit":10,"label":"alarm.gridvoltsimbalance","offset":32008},{"bit":11,"label":"alarm.gridoverfrequency","offset":32008},{"bit":12,"label":"alarm.gridunderfrequency","offset":32008},{"bit":13,"label":"alarm.gridfrequencyunstable","offset":32008},{"bit":14,"label":"alarm.acoveramps","offset":32008},{"bit":15,"label":"alarm.dccomponentoverhigh","offset":32008},{"bit":0,"label":"alarm.faultresidualamps","offset":32009},{"bit":1,"label":"alarm.faultgrounding","offset":32009},{"bit":2,"label":"alarm.lowinsulationresistance","offset":32009},{"bit":3,"label":"alarm.overtemp","offset":32009},{"bit":4,"label":"alarm.devicefault","offset":32009},{"bit":5,"label":"alarm.updatefailureorversionmismatch","offset":32009},{"bit":6,"label":"alarm.licenseexpired","offset":32009},{"bit":7,"label":"alarm.monitoringunitfaulty","offset":32009},{"bit":8,"label":"alarm.faultypowercollector","offset":32009},{"bit":10,"label":"alarm.activeislanding","offset":32009},{"bit":11,"label":"alarm.passiveislanding","offset":32009},{"bit":12,"label":"alarm.transientacovervolts","offset":32009},{"bit":13,"label":"alarm.peripheralportshortcircuit","offset":32009},{"bit":14,"label":"alarm.off_gridoutputoverload","offset":32009},{"bit":15,"label":"alarm.faultpvmoduleconfiguration","offset":32009},{"bit":1,"label":"alarm.built_inpidoperationfault","offset":32010},{"bit":2,"label":"alarm.highstringvoltstoground","offset":32010},{"bit":3,"label":"alarm.externalfanfault","offset":32010},{"bit":5,"label":"alarm.backupboxfault","offset":32010},{"bit":6,"label":"alarm.pvstringloss","offset":32010},{"bit":7,"label":"alarm.internalfanfault","offset":32010},{"bit":8,"label":"alarm.faultdcprotectionunit","offset":32010},{"bit":10,"label":"alarm.faultactivepowerschedulinginstruction","offset":32010},{"bit":11,"label":"alarm.faultreactivepowerschedulinginstruction","offset":32010},{"bit":13,"label":"alarm.dcarcfault","offset":32010},{"bit":14,"label":"alarm.dcswitchstatusfault","offset":32010},{"bit":0,"label":"alarm.dcovervolts","offset":32011},{"bit":1,"label":"alarm.dcinreversepolarity","offset":32011},{"bit":2,"label":"alarm.dcshort_circuitedorinreversepolarity","offset":32011},{"bit":3,"label":"alarm.dclinkedinseries","offset":32011},{"bit":8,"label":"alarm.parallelsystemlinkfailure","offset":32011},{"bit":12,"label":"alarm.reversephasesequenceonacside","offset":32011},{"bit":13,"label":"alarm.stringshort_circuitedtoground","offset":32011},{"bit":14,"label":"alarm.protectionuponlinkfailure","offset":32011},{"bit":15,"label":"alarm.acterminaltempfault","offset":32011},{"bit":0,"label":"alarm.dcterminaltempfault","offset":32012},{"bit":1,"label":"alarm.blackstartfailed","offset":32012},{"bit":2,"label":"alarm.incorrectblackstartinstructionsequence","offset":32012},{"bit":4,"label":"alarm.faultdcinput","offset":32012},{"bit":8,"label":"alarm.invalidregion","offset":32012}]},{"kind":"value_enum","card":"HD","offsets":[32089],"values":[{"code":"0x0000","meaning":"idle_init"},{"code":"0x0001","meaning":"idle_insulationresistancedetection"},{"code":"0x0002","meaning":"idle_irradiancedetection"},{"code":"0x0003","meaning":"idle_griddetection"},{"code":"0x0100","meaning":"starting"},{"code":"0x0200","meaning":"on_grid_off_gridmode_run"},{"code":"0x0201","meaning":"on_grid_powerlimited_off_gridmode_operatingatlimitedpower"},{"code":"0x0202","meaning":"on_grid_self_derating_off_gridmode_operatingatself_deratedpower"},{"code":"0x0300","meaning":"stopped_onfault"},{"code":"0x0301","meaning":"stopped_oncommand"},{"code":"0x0302","meaning":"stopped_ovgr"},{"code":"0x0303","meaning":"stopped_linkinterrupted"},{"code":"0x0304","meaning":"stopped_powerlimited"},{"code":"0x0305","meaning":"stopped_manualstartuprequired"},{"code":"0x0306","meaning":"stopped_dcswitchdislinked"},{"code":"0x0307","meaning":"stopped_rapidstopped"},{"code":"0x0308","meaning":"stopped_inputunderpower"},{"code":"0x0309","meaning":"stopped_nsprotection"},{"code":"0x030A","meaning":"stopped_rapidstoppedoncommand"},{"code":"0x0401","meaning":"griddispatch_cos_p_pcharacteristicncurve"},{"code":"0x0402","meaning":"griddispatch_q_ucharacteristiccurve"},{"code":"0x0403","meaning":"griddispatch_pf_ucharacteristiccurve"},{"code":"0x0404","meaning":"griddispatch_drycontact"},{"code":"0x0405","meaning":"griddispatch_q_pcharacteristiccurve"},{"code":"0x0500","meaning":"readyforterminaltest"},{"code":"0x0501","meaning":"terminaltesting"},{"code":"0x0600","meaning":"inspecting"},{"code":"0X0700","meaning":"afciself_check"},{"code":"0X0800","meaning":"i_vcurvescanning"},{"code":"0X0900","meaning":"dcinputdetection"},{"code":"0xA000","meaning":"idle_noirradiance"}]},{"kind":"alarm_bits","card":"L-LEGACY","offsets":[32008,32009,32010],"bits":[{"bit":0,"label":"alarm.highstringvolts","offset":32008},{"bit":1,"label":"alarm.dcarcfault","offset":32008},{"bit":2,"label":"alarm.stringreversed","offset":32008},{"bit":3,"label":"alarm.pvstringbackfeed","offset":32008},{"bit":4,"label":"alarm.faultstring","offset":32008},{"bit":5,"label":"alarm.afciself_testfault","offset":32008},{"bit":6,"label":"alarm.shortcircuitbetweenphasetope","offset":32008},{"bit":7,"label":"alarm.powergridfailure","offset":32008},{"bit":8,"label":"alarm.gridundervolts","offset":32008},{"bit":9,"label":"alarm.gridovervolts","offset":32008},{"bit":10,"label":"alarm.unbalancedgridvolts","offset":32008},{"bit":11,"label":"alarm.gridoverfrequency","offset":32008},{"bit":12,"label":"alarm.gridunderfrequency","offset":32008},{"bit":13,"label":"alarm.gridfrequencyinstability","offset":32008},{"bit":14,"label":"alarm.outputoveramps","offset":32008},{"bit":15,"label":"alarm.largedcofoutputamps","offset":32008},{"bit":0,"label":"alarm.faultleakageamps","offset":32009},{"bit":1,"label":"alarm.faultground","offset":32009},{"bit":2,"label":"alarm.lowinsulationres","offset":32009},{"bit":3,"label":"alarm.hightemp","offset":32009},{"bit":4,"label":"alarm.faultequipment","offset":32009},{"bit":5,"label":"alarm.upgradefailed","offset":32009},{"bit":6,"label":"alarm.licenseexpired","offset":32009},{"bit":7,"label":"alarm.faultmonitorunit","offset":32009},{"bit":8,"label":"alarm.powercollectorfault","offset":32009},{"bit":9,"label":"alarm.faultessdevice","offset":32009},{"bit":10,"label":"alarm.activeislanding","offset":32009},{"bit":11,"label":"alarm.passiveislanding","offset":32009},{"bit":12,"label":"alarm.transientacovervolts","offset":32009},{"bit":13,"label":"alarm.externaldeviceportshortcircuit","offset":32009},{"bit":14,"label":"alarm.off_gridoutputoverload","offset":32009},{"bit":15,"label":"alarm.faultpvmoduleconfiguration","offset":32009},{"bit":0,"label":"alarm.optimizerfault","offset":32010}]},{"kind":"value_enum","card":"L-LEGACY","offsets":[32089],"values":[{"code":"0x0000","meaning":"standby_initializing"},{"code":"0x0001","meaning":"standby_insulation_check"},{"code":"0x0002","meaning":"standby_irradiance_check"},{"code":"0x0003","meaning":"standby_grid_check"},{"code":"0x0100","meaning":"starting"},{"code":"0x0200","meaning":"running_mode_dependent"},{"code":"0x0201","meaning":"running_power_limited"},{"code":"0x0202","meaning":"running_self_derated"},{"code":"0x0300","meaning":"fault_shutdown"},{"code":"0x0301","meaning":"command_shutdown"},{"code":"0x0302","meaning":"ovgr_shutdown"},{"code":"0x0303","meaning":"communication_shutdown"},{"code":"0x0304","meaning":"power_limit_shutdown"},{"code":"0x0305","meaning":"manual_start_required"},{"code":"0x0306","meaning":"dc_switch_off"},{"code":"0x0401","meaning":"cosphi_p_dispatch"},{"code":"0x0402","meaning":"q_u_dispatch"},{"code":"0xA000","meaning":"standby_no_irradiance"},{"code":"0x0500","meaning":"inspection_ready"},{"code":"0x0501","meaning":"inspection_running"},{"code":"0x0600","meaning":"checking"},{"code":"0X0700","meaning":"afci_self_test"},{"code":"0X0800","meaning":"iv_scan"},{"code":"0X0900","meaning":"dc_input_check"}]},{"kind":"alarm_bits","card":"L-V200","offsets":[32008,32009,32010],"bits":[{"bit":0,"label":"alarm.highstringinputvolts","offset":32008},{"bit":1,"label":"alarm.dcarcfault","offset":32008},{"bit":2,"label":"alarm.stringreverseconnection","offset":32008},{"bit":3,"label":"alarm.stringampsbackfeed","offset":32008},{"bit":4,"label":"alarm.faultstringpower","offset":32008},{"bit":5,"label":"alarm.afciself_checkfail","offset":32008},{"bit":6,"label":"alarm.phasewireshort_circuitedtope","offset":32008},{"bit":7,"label":"alarm.gridloss","offset":32008},{"bit":8,"label":"alarm.gridundervolts","offset":32008},{"bit":9,"label":"alarm.gridovervolts","offset":32008},{"bit":10,"label":"alarm.gridvolt_imbalance","offset":32008},{"bit":11,"label":"alarm.gridoverfrequency","offset":32008},{"bit":12,"label":"alarm.gridunderfrequency","offset":32008},{"bit":13,"label":"alarm.unstablegridfrequency","offset":32008},{"bit":14,"label":"alarm.outputoveramps","offset":32008},{"bit":15,"label":"alarm.outputdccomponentoverhigh","offset":32008},{"bit":0,"label":"alarm.faultresidualamps","offset":32009},{"bit":1,"label":"alarm.faultgrounding","offset":32009},{"bit":2,"label":"alarm.lowinsulationresistance","offset":32009},{"bit":3,"label":"alarm.overtemp","offset":32009},{"bit":4,"label":"alarm.devicefault","offset":32009},{"bit":5,"label":"alarm.upgradefailedorversionmismatch","offset":32009},{"bit":6,"label":"alarm.licenseexpired","offset":32009},{"bit":7,"label":"alarm.faultymonitoringunit","offset":32009},{"bit":8,"label":"alarm.faultypowercollector","offset":32009},{"bit":9,"label":"alarm.packfault","offset":32009},{"bit":10,"label":"alarm.activeislanding","offset":32009},{"bit":11,"label":"alarm.passiveislanding","offset":32009},{"bit":12,"label":"alarm.transientacovervolts","offset":32009},{"bit":13,"label":"alarm.peripheralportshortcircuit","offset":32009},{"bit":14,"label":"alarm.churnoutputoverload","offset":32009},{"bit":15,"label":"alarm.faultpvmoduleconfiguration","offset":32009},{"bit":0,"label":"alarm.optimizerfault","offset":32010},{"bit":1,"label":"alarm.built_inpidoperationfault","offset":32010},{"bit":2,"label":"alarm.highinputstringvoltstoground","offset":32010},{"bit":3,"label":"alarm.externalfanfault","offset":32010},{"bit":4,"label":"alarm.packreverseconnection","offset":32010},{"bit":5,"label":"alarm.on_grid_off_gridcontrollerfault","offset":32010},{"bit":6,"label":"alarm.pvstringloss","offset":32010},{"bit":7,"label":"alarm.internalfanfault","offset":32010},{"bit":8,"label":"alarm.dcprotectionunitfault","offset":32010},{"bit":9,"label":"alarm.elunitfault","offset":32010},{"bit":10,"label":"alarm.activeadjustmentinstructionfault","offset":32010},{"bit":11,"label":"alarm.reactiveadjustmentinstructionfault","offset":32010},{"bit":12,"label":"alarm.ctwiringfault","offset":32010},{"bit":13,"label":"alarm.dcarcfault_admcalarmtobeclearmanually","offset":32010},{"bit":14,"label":"alarm.dcswitchfault","offset":32010},{"bit":15,"label":"alarm.allowableoutcapacityofthepackislow","offset":32010}]},{"kind":"value_enum","card":"L-V200","offsets":[32089],"values":[{"code":"0x0000","meaning":"standby_initializing"},{"code":"0x0001","meaning":"standby_insulation_check"},{"code":"0x0002","meaning":"standby_irradiance_check"},{"code":"0x0003","meaning":"standby_grid_check"},{"code":"0x0100","meaning":"starting"},{"code":"0x0200","meaning":"running_mode_dependent"},{"code":"0x0201","meaning":"running_power_limited"},{"code":"0x0202","meaning":"running_self_derated"},{"code":"0x0203","meaning":"off_grid_running"},{"code":"0x0300","meaning":"fault_shutdown"},{"code":"0x0301","meaning":"command_shutdown"},{"code":"0x0302","meaning":"ovgr_shutdown"},{"code":"0x0303","meaning":"communication_shutdown"},{"code":"0x0304","meaning":"power_limit_shutdown"},{"code":"0x0305","meaning":"manual_start_required"},{"code":"0x0306","meaning":"dc_switch_off"},{"code":"0x0307","meaning":"rapid_shutdown"},{"code":"0x0308","meaning":"input_underpower"},{"code":"0x0401","meaning":"cosphi_p_dispatch"},{"code":"0x0402","meaning":"q_u_dispatch"},{"code":"0x0403","meaning":"pf_u_dispatch"},{"code":"0x0404","meaning":"dry_contact_dispatch"},{"code":"0x0405","meaning":"q_p_dispatch"},{"code":"0x0500","meaning":"inspection_ready"},{"code":"0x0501","meaning":"inspection_running"},{"code":"0x0600","meaning":"checking"},{"code":"0X0700","meaning":"afci_self_test"},{"code":"0X0800","meaning":"iv_scan"},{"code":"0X0900","meaning":"dc_input_check"},{"code":"0X0A00","meaning":"off_grid_charge"},{"code":"0xA000","meaning":"standby_no_irradiance"},{"code":"0x0A01","meaning":"idle_backuppowersystemfaultity"},{"code":"0xA001","meaning":"standby_no_dc_input"},{"code":"0xB000","meaning":"linklinkbroken"},{"code":"0xC000","meaning":"loading"}]},{"kind":"alarm_bits","card":"LB-C00","offsets":[32008,32009,32010],"bits":[{"bit":0,"label":"alarm.highstringinputvolts","offset":32008},{"bit":1,"label":"alarm.dcarcfault","offset":32008},{"bit":2,"label":"alarm.stringreverseconnection","offset":32008},{"bit":3,"label":"alarm.stringampsbackfeed","offset":32008},{"bit":4,"label":"alarm.faultstringpower","offset":32008},{"bit":5,"label":"alarm.afciself_checkfail","offset":32008},{"bit":6,"label":"alarm.phasewireshort_circuitedtope","offset":32008},{"bit":7,"label":"alarm.gridloss","offset":32008},{"bit":8,"label":"alarm.gridundervolts","offset":32008},{"bit":9,"label":"alarm.gridovervolts","offset":32008},{"bit":10,"label":"alarm.gridvolt_imbalance","offset":32008},{"bit":11,"label":"alarm.gridoverfrequency","offset":32008},{"bit":12,"label":"alarm.gridunderfrequency","offset":32008},{"bit":13,"label":"alarm.unstablegridfrequency","offset":32008},{"bit":14,"label":"alarm.outputoveramps","offset":32008},{"bit":15,"label":"alarm.outputdccomponentoverhigh","offset":32008},{"bit":0,"label":"alarm.faultresidualamps","offset":32009},{"bit":1,"label":"alarm.faultgrounding","offset":32009},{"bit":2,"label":"alarm.lowinsulationresistance","offset":32009},{"bit":3,"label":"alarm.overtemp","offset":32009},{"bit":4,"label":"alarm.devicefault","offset":32009},{"bit":5,"label":"alarm.upgradefailedorversionmismatch","offset":32009},{"bit":6,"label":"alarm.licenseexpired","offset":32009},{"bit":7,"label":"alarm.faultymonitoringunit","offset":32009},{"bit":8,"label":"alarm.faultypowercollector","offset":32009},{"bit":9,"label":"alarm.packfault","offset":32009},{"bit":10,"label":"alarm.activeislanding","offset":32009},{"bit":11,"label":"alarm.passiveislanding","offset":32009},{"bit":12,"label":"alarm.transientacovervolts","offset":32009},{"bit":13,"label":"alarm.peripheralportshortcircuit","offset":32009},{"bit":14,"label":"alarm.churnoutputoverload","offset":32009},{"bit":15,"label":"alarm.faultpvmoduleconfiguration","offset":32009},{"bit":0,"label":"alarm.optimizerfault","offset":32010},{"bit":1,"label":"alarm.built_inpidoperationfault","offset":32010},{"bit":2,"label":"alarm.highinputstringvoltstoground","offset":32010},{"bit":3,"label":"alarm.externalfanfault","offset":32010},{"bit":4,"label":"alarm.packreverseconnection","offset":32010},{"bit":5,"label":"alarm.on_grid_off_gridcontrollerfault","offset":32010},{"bit":6,"label":"alarm.pvstringloss","offset":32010},{"bit":7,"label":"alarm.internalfanfault","offset":32010},{"bit":8,"label":"alarm.dcprotectionunitfault","offset":32010},{"bit":9,"label":"alarm.elunitfault","offset":32010},{"bit":10,"label":"alarm.activeadjustmentinstructionfault","offset":32010},{"bit":11,"label":"alarm.reactiveadjustmentinstructionfault","offset":32010},{"bit":12,"label":"alarm.ctwiringfault","offset":32010},{"bit":13,"label":"alarm.dcarcfault_admcalarmtobeclearmanually","offset":32010},{"bit":14,"label":"alarm.dcswitchfault","offset":32010},{"bit":15,"label":"alarm.allowableoutcapacityofthepackislow","offset":32010}]},{"kind":"value_enum","card":"LB-C00","offsets":[32089],"values":[{"code":"0x0000","meaning":"standby_initializing"},{"code":"0x0001","meaning":"standby_insulation_check"},{"code":"0x0002","meaning":"standby_irradiance_check"},{"code":"0x0003","meaning":"standby_grid_check"},{"code":"0x0100","meaning":"starting"},{"code":"0x0200","meaning":"running_mode_dependent"},{"code":"0x0201","meaning":"running_power_limited"},{"code":"0x0202","meaning":"running_self_derated"},{"code":"0x0203","meaning":"off_grid_running"},{"code":"0x0300","meaning":"fault_shutdown"},{"code":"0x0301","meaning":"command_shutdown"},{"code":"0x0302","meaning":"ovgr_shutdown"},{"code":"0x0303","meaning":"communication_shutdown"},{"code":"0x0304","meaning":"power_limit_shutdown"},{"code":"0x0305","meaning":"manual_start_required"},{"code":"0x0306","meaning":"dc_switch_off"},{"code":"0x0307","meaning":"rapid_shutdown"},{"code":"0x0308","meaning":"input_underpower"},{"code":"0x0401","meaning":"cosphi_p_dispatch"},{"code":"0x0402","meaning":"q_u_dispatch"},{"code":"0x0403","meaning":"pf_u_dispatch"},{"code":"0x0404","meaning":"dry_contact_dispatch"},{"code":"0x0405","meaning":"q_p_dispatch"},{"code":"0x0500","meaning":"inspection_ready"},{"code":"0x0501","meaning":"inspection_running"},{"code":"0x0600","meaning":"checking"},{"code":"0X0700","meaning":"afci_self_test"},{"code":"0X0800","meaning":"iv_scan"},{"code":"0X0900","meaning":"dc_input_check"},{"code":"0X0A00","meaning":"off_grid_charge"},{"code":"0xA000","meaning":"standby_no_irradiance"},{"code":"0x0A01","meaning":"idle_backuppowersystemfaultity"},{"code":"0xA001","meaning":"standby_no_dc_input"},{"code":"0xB000","meaning":"linklinkbroken"},{"code":"0xC000","meaning":"loading"}]},{"kind":"value_enum","card":"LB-C10","offsets":[32089],"values":[{"code":"0x0000","meaning":"standby_initializing"},{"code":"0x0001","meaning":"standby_insulation_check"},{"code":"0x0002","meaning":"standby_irradiance_check"},{"code":"0x0003","meaning":"standby_grid_check"},{"code":"0x0100","meaning":"starting"},{"code":"0x0200","meaning":"running_mode_dependent"},{"code":"0x0201","meaning":"running_power_limited"},{"code":"0x0202","meaning":"running_self_derated"},{"code":"0x0203","meaning":"off_grid_running"},{"code":"0x0300","meaning":"fault_shutdown"},{"code":"0x0301","meaning":"command_shutdown"},{"code":"0x0302","meaning":"ovgr_shutdown"},{"code":"0x0303","meaning":"communication_shutdown"},{"code":"0x0304","meaning":"power_limit_shutdown"},{"code":"0x0305","meaning":"manual_start_required"},{"code":"0x0306","meaning":"dc_switch_off"},{"code":"0x0307","meaning":"rapid_shutdown"},{"code":"0x0308","meaning":"input_underpower"},{"code":"0x0401","meaning":"cosphi_p_dispatch"},{"code":"0x0402","meaning":"q_u_dispatch"},{"code":"0x0403","meaning":"pf_u_dispatch"},{"code":"0x0404","meaning":"dry_contact_dispatch"},{"code":"0x0405","meaning":"q_p_dispatch"},{"code":"0x0500","meaning":"inspection_ready"},{"code":"0x0501","meaning":"inspection_running"},{"code":"0x0600","meaning":"checking"},{"code":"0X0700","meaning":"afci_self_test"},{"code":"0X0800","meaning":"iv_scan"},{"code":"0X0900","meaning":"dc_input_check"},{"code":"0X0A00","meaning":"off_grid_charge"},{"code":"0xA000","meaning":"standby_no_irradiance"}]},{"kind":"alarm_bits","card":"LC","offsets":[32008,32009,32010],"bits":[{"bit":0,"label":"alarm.highstringinputvolts","offset":32008},{"bit":1,"label":"alarm.dcarcfault","offset":32008},{"bit":2,"label":"alarm.stringreverseconnection","offset":32008},{"bit":3,"label":"alarm.stringampsbackfeed","offset":32008},{"bit":4,"label":"alarm.faultstringpower","offset":32008},{"bit":5,"label":"alarm.afciself_checkfail","offset":32008},{"bit":6,"label":"alarm.phasewireshort_circuitedtope","offset":32008},{"bit":7,"label":"alarm.gridloss","offset":32008},{"bit":8,"label":"alarm.gridundervolts","offset":32008},{"bit":9,"label":"alarm.gridovervolts","offset":32008},{"bit":10,"label":"alarm.gridvolt_imbalance","offset":32008},{"bit":11,"label":"alarm.gridoverfrequency","offset":32008},{"bit":12,"label":"alarm.gridunderfrequency","offset":32008},{"bit":13,"label":"alarm.unstablegridfrequency","offset":32008},{"bit":14,"label":"alarm.outputoveramps","offset":32008},{"bit":15,"label":"alarm.outputdccomponentoverhigh","offset":32008},{"bit":0,"label":"alarm.faultresidualamps","offset":32009},{"bit":1,"label":"alarm.faultgrounding","offset":32009},{"bit":2,"label":"alarm.lowinsulationresistance","offset":32009},{"bit":3,"label":"alarm.overtemp","offset":32009},{"bit":4,"label":"alarm.devicefault","offset":32009},{"bit":5,"label":"alarm.upgradefailedorversionmismatch","offset":32009},{"bit":6,"label":"alarm.licenseexpired","offset":32009},{"bit":7,"label":"alarm.faultymonitoringunit","offset":32009},{"bit":8,"label":"alarm.faultypowercollector","offset":32009},{"bit":9,"label":"alarm.packfault","offset":32009},{"bit":10,"label":"alarm.activeislanding","offset":32009},{"bit":11,"label":"alarm.passiveislanding","offset":32009},{"bit":12,"label":"alarm.transientacovervolts","offset":32009},{"bit":13,"label":"alarm.peripheralportshortcircuit","offset":32009},{"bit":14,"label":"alarm.churnoutputoverload","offset":32009},{"bit":15,"label":"alarm.faultpvmoduleconfiguration","offset":32009},{"bit":0,"label":"alarm.optimizerfault","offset":32010},{"bit":1,"label":"alarm.built_inpidoperationfault","offset":32010},{"bit":2,"label":"alarm.highinputstringvoltstoground","offset":32010},{"bit":3,"label":"alarm.externalfanfault","offset":32010},{"bit":4,"label":"alarm.packreverseconnection","offset":32010},{"bit":5,"label":"alarm.on_grid_off_gridcontrollerfault","offset":32010},{"bit":6,"label":"alarm.pvstringloss","offset":32010},{"bit":7,"label":"alarm.internalfanfault","offset":32010},{"bit":8,"label":"alarm.dcprotectionunitfault","offset":32010},{"bit":9,"label":"alarm.elunitfault","offset":32010},{"bit":10,"label":"alarm.activeadjustmentinstructionfault","offset":32010},{"bit":11,"label":"alarm.reactiveadjustmentinstructionfault","offset":32010},{"bit":12,"label":"alarm.ctwiringfault","offset":32010},{"bit":13,"label":"alarm.dcarcfault_admcalarmtobeclearmanually","offset":32010},{"bit":14,"label":"alarm.dcswitchfault","offset":32010},{"bit":15,"label":"alarm.allowableoutcapacityofthepackislow","offset":32010}]},{"kind":"value_enum","card":"LC","offsets":[32089],"values":[{"code":"0x0000","meaning":"standby_initializing"},{"code":"0x0001","meaning":"standby_insulation_check"},{"code":"0x0002","meaning":"standby_irradiance_check"},{"code":"0x0003","meaning":"standby_grid_check"},{"code":"0x0100","meaning":"starting"},{"code":"0x0200","meaning":"running_mode_dependent"},{"code":"0x0201","meaning":"running_power_limited"},{"code":"0x0202","meaning":"running_self_derated"},{"code":"0x0203","meaning":"off_grid_running"},{"code":"0x0300","meaning":"fault_shutdown"},{"code":"0x0301","meaning":"command_shutdown"},{"code":"0x0302","meaning":"ovgr_shutdown"},{"code":"0x0303","meaning":"communication_shutdown"},{"code":"0x0304","meaning":"power_limit_shutdown"},{"code":"0x0305","meaning":"manual_start_required"},{"code":"0x0306","meaning":"dc_switch_off"},{"code":"0x0307","meaning":"rapid_shutdown"},{"code":"0x0308","meaning":"input_underpower"},{"code":"0x0401","meaning":"cosphi_p_dispatch"},{"code":"0x0402","meaning":"q_u_dispatch"},{"code":"0x0403","meaning":"pf_u_dispatch"},{"code":"0x0404","meaning":"dry_contact_dispatch"},{"code":"0x0405","meaning":"q_p_dispatch"},{"code":"0x0500","meaning":"inspection_ready"},{"code":"0x0501","meaning":"inspection_running"},{"code":"0x0600","meaning":"checking"},{"code":"0X0700","meaning":"afci_self_test"},{"code":"0X0800","meaning":"iv_scan"},{"code":"0X0900","meaning":"dc_input_check"},{"code":"0X0A00","meaning":"off_grid_charge"},{"code":"0xA000","meaning":"standby_no_irradiance"},{"code":"0x0A01","meaning":"idle_backuppowersystemfaultity"},{"code":"0xA001","meaning":"standby_no_dc_input"},{"code":"0xB000","meaning":"linklinkbroken"},{"code":"0xC000","meaning":"loading"}]},{"kind":"alarm_bits","card":"MA-C00","offsets":[32008,32009,32010],"bits":[{"bit":0,"label":"alarm.highstringinputvolts","offset":32008},{"bit":1,"label":"alarm.dcarcfault","offset":32008},{"bit":2,"label":"alarm.stringreverseconnection","offset":32008},{"bit":3,"label":"alarm.stringampsbackfeed","offset":32008},{"bit":4,"label":"alarm.faultstringpower","offset":32008},{"bit":5,"label":"alarm.afciself_checkfail","offset":32008},{"bit":6,"label":"alarm.phasewireshort_circuitedtope","offset":32008},{"bit":7,"label":"alarm.gridloss","offset":32008},{"bit":8,"label":"alarm.gridundervolts","offset":32008},{"bit":9,"label":"alarm.gridovervolts","offset":32008},{"bit":10,"label":"alarm.gridvolt_imbalance","offset":32008},{"bit":11,"label":"alarm.gridoverfrequency","offset":32008},{"bit":12,"label":"alarm.gridunderfrequency","offset":32008},{"bit":13,"label":"alarm.unstablegridfrequency","offset":32008},{"bit":14,"label":"alarm.outputoveramps","offset":32008},{"bit":15,"label":"alarm.outputdccomponentoverhigh","offset":32008},{"bit":0,"label":"alarm.faultresidualamps","offset":32009},{"bit":1,"label":"alarm.faultgrounding","offset":32009},{"bit":2,"label":"alarm.lowinsulationresistance","offset":32009},{"bit":3,"label":"alarm.overtemp","offset":32009},{"bit":4,"label":"alarm.devicefault","offset":32009},{"bit":5,"label":"alarm.upgradefailedorversionmismatch","offset":32009},{"bit":6,"label":"alarm.licenseexpired","offset":32009},{"bit":7,"label":"alarm.faultymonitoringunit","offset":32009},{"bit":8,"label":"alarm.faultypowercollector","offset":32009},{"bit":9,"label":"alarm.packfault","offset":32009},{"bit":10,"label":"alarm.activeislanding","offset":32009},{"bit":11,"label":"alarm.passiveislanding","offset":32009},{"bit":12,"label":"alarm.transientacovervolts","offset":32009},{"bit":13,"label":"alarm.peripheralportshortcircuit","offset":32009},{"bit":14,"label":"alarm.churnoutputoverload","offset":32009},{"bit":15,"label":"alarm.faultpvmoduleconfiguration","offset":32009},{"bit":0,"label":"alarm.optimizerfault","offset":32010},{"bit":1,"label":"alarm.built_inpidoperationfault","offset":32010},{"bit":2,"label":"alarm.highinputstringvoltstoground","offset":32010},{"bit":3,"label":"alarm.externalfanfault","offset":32010},{"bit":4,"label":"alarm.packreverseconnection","offset":32010},{"bit":5,"label":"alarm.on_grid_off_gridcontrollerfault","offset":32010},{"bit":6,"label":"alarm.pvstringloss","offset":32010},{"bit":7,"label":"alarm.internalfanfault","offset":32010},{"bit":8,"label":"alarm.dcprotectionunitfault","offset":32010},{"bit":9,"label":"alarm.elunitfault","offset":32010},{"bit":10,"label":"alarm.activeadjustmentinstructionfault","offset":32010},{"bit":11,"label":"alarm.reactiveadjustmentinstructionfault","offset":32010},{"bit":12,"label":"alarm.ctwiringfault","offset":32010},{"bit":13,"label":"alarm.dcarcfault_admcalarmtobeclearmanually","offset":32010},{"bit":14,"label":"alarm.dcswitchfault","offset":32010},{"bit":15,"label":"alarm.allowableoutcapacityofthepackislow","offset":32010}]},{"kind":"value_enum","card":"MA-C00","offsets":[32089],"values":[{"code":"0x0000","meaning":"standby_initializing"},{"code":"0x0001","meaning":"standby_insulation_check"},{"code":"0x0002","meaning":"standby_irradiance_check"},{"code":"0x0003","meaning":"standby_grid_check"},{"code":"0x0100","meaning":"starting"},{"code":"0x0200","meaning":"running_mode_dependent"},{"code":"0x0201","meaning":"running_power_limited"},{"code":"0x0202","meaning":"running_self_derated"},{"code":"0x0203","meaning":"off_grid_running"},{"code":"0x0300","meaning":"fault_shutdown"},{"code":"0x0301","meaning":"command_shutdown"},{"code":"0x0302","meaning":"ovgr_shutdown"},{"code":"0x0303","meaning":"communication_shutdown"},{"code":"0x0304","meaning":"power_limit_shutdown"},{"code":"0x0305","meaning":"manual_start_required"},{"code":"0x0307","meaning":"rapid_shutdown"},{"code":"0x030A","meaning":"command_rapid_shutdown"},{"code":"0x030B","meaning":"stopped_thebackuppowersystemisfault"},{"code":"0x0401","meaning":"cosphi_p_dispatch"},{"code":"0x0402","meaning":"q_u_dispatch"},{"code":"0x0403","meaning":"pf_u_dispatch"},{"code":"0x0405","meaning":"q_p_dispatch"},{"code":"0x0600","meaning":"checking"},{"code":"0X0700","meaning":"afci_self_test"},{"code":"0X0800","meaning":"iv_scan"},{"code":"0X0A00","meaning":"off_grid_charge"},{"code":"0X0A01","meaning":"idle_thebackuppowersystemisfault"},{"code":"0xA000","meaning":"standby_no_irradiance"}]},{"kind":"alarm_bits","card":"MA-C20","offsets":[],"bits":[{"bit":0,"label":"alarm.highstringinputvolts"},{"bit":1,"label":"alarm.dcarcfault"},{"bit":2,"label":"alarm.stringreverseconnection"},{"bit":3,"label":"alarm.stringampsbackfeed"},{"bit":4,"label":"alarm.faultstringpower"},{"bit":5,"label":"alarm.afciself_checkfail"},{"bit":6,"label":"alarm.phasewireshort_circuitedtope"},{"bit":7,"label":"alarm.gridloss"},{"bit":8,"label":"alarm.gridundervolts"},{"bit":9,"label":"alarm.gridovervolts"},{"bit":10,"label":"alarm.gridvolt_imbalance"},{"bit":11,"label":"alarm.gridoverfrequency"},{"bit":12,"label":"alarm.gridunderfrequency"},{"bit":13,"label":"alarm.unstablegridfrequency"},{"bit":14,"label":"alarm.outputoveramps"},{"bit":15,"label":"alarm.outputdccomponentoverhigh"},{"bit":0,"label":"alarm.faultresidualamps"},{"bit":1,"label":"alarm.faultgrounding"},{"bit":2,"label":"alarm.lowinsulationresistance"},{"bit":3,"label":"alarm.overtemp"},{"bit":4,"label":"alarm.devicefault"},{"bit":5,"label":"alarm.upgradefailedorversionmismatch"},{"bit":6,"label":"alarm.licenseexpired"},{"bit":7,"label":"alarm.faultymonitoringunit"},{"bit":8,"label":"alarm.faultypowercollector"},{"bit":9,"label":"alarm.packfault"},{"bit":10,"label":"alarm.activeislanding"},{"bit":11,"label":"alarm.passiveislanding"},{"bit":12,"label":"alarm.transientacovervolts"},{"bit":13,"label":"alarm.peripheralportshortcircuit"},{"bit":14,"label":"alarm.churnoutputoverload"},{"bit":15,"label":"alarm.faultpvmoduleconfiguration"},{"bit":0,"label":"alarm.optimizerfault"},{"bit":1,"label":"alarm.built_inpidoperationfault"},{"bit":2,"label":"alarm.highinputstringvoltstoground"},{"bit":3,"label":"alarm.externalfanfault"},{"bit":4,"label":"alarm.packreverseconnection"},{"bit":5,"label":"alarm.on_grid_off_gridcontrollerfault"},{"bit":6,"label":"alarm.pvstringloss"},{"bit":7,"label":"alarm.internalfanfault"},{"bit":8,"label":"alarm.dcprotectionunitfault"}]},{"kind":"value_enum","card":"MA-C20","offsets":[32089],"values":[{"code":"0x0000","meaning":"standby_initializing"},{"code":"0x0001","meaning":"standby_insulation_check"},{"code":"0x0002","meaning":"standby_irradiance_check"},{"code":"0x0003","meaning":"standby_grid_check"},{"code":"0x0100","meaning":"starting"},{"code":"0x0200","meaning":"running_mode_dependent"},{"code":"0x0201","meaning":"running_power_limited"},{"code":"0x0202","meaning":"running_self_derated"},{"code":"0x0300","meaning":"fault_shutdown"},{"code":"0x0301","meaning":"command_shutdown"},{"code":"0x0302","meaning":"ovgr_shutdown"},{"code":"0x0303","meaning":"communication_shutdown"},{"code":"0x0304","meaning":"power_limit_shutdown"},{"code":"0x0305","meaning":"manual_start_required"},{"code":"0x0307","meaning":"rapid_shutdown"},{"code":"0x0401","meaning":"cosphi_p_dispatch"},{"code":"0x0402","meaning":"q_u_dispatch"},{"code":"0x0403","meaning":"pf_u_dispatch"},{"code":"0x0404","meaning":"dry_contact_dispatch"},{"code":"0x0405","meaning":"q_p_dispatch"},{"code":"0x0500","meaning":"inspection_ready"},{"code":"0x0501","meaning":"inspection_running"},{"code":"0x0600","meaning":"checking"},{"code":"0X0700","meaning":"afci_self_test"},{"code":"0X0800","meaning":"iv_scan"},{"code":"0xA000","meaning":"standby_no_irradiance"}]},{"kind":"alarm_bits","card":"MA-V200","offsets":[32008,32009,32010],"bits":[{"bit":0,"label":"string_overvoltage","offset":32008},{"bit":1,"label":"dc_arc","offset":32008},{"bit":2,"label":"string_reverse","offset":32008},{"bit":3,"label":"string_backfeed","offset":32008},{"bit":4,"label":"string_power","offset":32008},{"bit":5,"label":"afci_selftest","offset":32008},{"bit":6,"label":"phase_to_pe_short","offset":32008},{"bit":7,"label":"grid_lost","offset":32008},{"bit":8,"label":"grid_undervoltage","offset":32008},{"bit":9,"label":"grid_overvoltage","offset":32008},{"bit":10,"label":"grid_voltage_imbalance","offset":32008},{"bit":11,"label":"grid_overfrequency","offset":32008},{"bit":12,"label":"grid_underfrequency","offset":32008},{"bit":13,"label":"grid_frequency_unstable","offset":32008},{"bit":14,"label":"output_overcurrent","offset":32008},{"bit":15,"label":"output_dc_high","offset":32008},{"bit":0,"label":"residual_current_fault","offset":32009},{"bit":1,"label":"grounding_fault","offset":32009},{"bit":2,"label":"insulation_low","offset":32009},{"bit":3,"label":"temperature_high","offset":32009},{"bit":4,"label":"device_fault","offset":32009},{"bit":5,"label":"upgrade_or_version_fault","offset":32009},{"bit":6,"label":"licence_expired","offset":32009},{"bit":7,"label":"monitor_fault","offset":32009},{"bit":8,"label":"collector_or_meter_fault","offset":32009},{"bit":9,"label":"battery_fault","offset":32009},{"bit":10,"label":"active_islanding","offset":32009},{"bit":11,"label":"passive_islanding","offset":32009},{"bit":12,"label":"transient_ac_overvoltage","offset":32009},{"bit":13,"label":"external_port_short","offset":32009},{"bit":14,"label":"output_overload","offset":32009},{"bit":15,"label":"pv_configuration_fault","offset":32009},{"bit":0,"label":"optimizer_fault","offset":32010},{"bit":1,"label":"pid_fault","offset":32010},{"bit":2,"label":"string_ground_voltage_high","offset":32010},{"bit":3,"label":"external_fan_fault","offset":32010},{"bit":4,"label":"battery_reverse","offset":32010},{"bit":5,"label":"grid_island_controller_fault","offset":32010},{"bit":6,"label":"pv_string_lost","offset":32010},{"bit":7,"label":"internal_fan_fault","offset":32010},{"bit":8,"label":"dc_protection_fault","offset":32010}]},{"kind":"alarm_bits","card":"MB02","offsets":[32008,32009,32010],"bits":[{"bit":0,"label":"string_overvoltage","offset":32008},{"bit":1,"label":"dc_arc","offset":32008},{"bit":2,"label":"string_reverse","offset":32008},{"bit":3,"label":"string_backfeed","offset":32008},{"bit":4,"label":"string_power","offset":32008},{"bit":5,"label":"afci_selftest","offset":32008},{"bit":6,"label":"phase_to_pe_short","offset":32008},{"bit":7,"label":"grid_lost","offset":32008},{"bit":8,"label":"grid_undervoltage","offset":32008},{"bit":9,"label":"grid_overvoltage","offset":32008},{"bit":10,"label":"grid_voltage_imbalance","offset":32008},{"bit":11,"label":"grid_overfrequency","offset":32008},{"bit":12,"label":"grid_underfrequency","offset":32008},{"bit":13,"label":"grid_frequency_unstable","offset":32008},{"bit":14,"label":"output_overcurrent","offset":32008},{"bit":15,"label":"output_dc_high","offset":32008},{"bit":0,"label":"residual_current_fault","offset":32009},{"bit":1,"label":"grounding_fault","offset":32009},{"bit":2,"label":"insulation_low","offset":32009},{"bit":3,"label":"temperature_high","offset":32009},{"bit":4,"label":"device_fault","offset":32009},{"bit":5,"label":"upgrade_or_version_fault","offset":32009},{"bit":6,"label":"licence_expired","offset":32009},{"bit":7,"label":"monitor_fault","offset":32009},{"bit":8,"label":"collector_or_meter_fault","offset":32009},{"bit":9,"label":"battery_fault","offset":32009},{"bit":10,"label":"active_islanding","offset":32009},{"bit":11,"label":"passive_islanding","offset":32009},{"bit":12,"label":"transient_ac_overvoltage","offset":32009},{"bit":13,"label":"external_port_short","offset":32009},{"bit":14,"label":"output_overload","offset":32009},{"bit":15,"label":"pv_configuration_fault","offset":32009},{"bit":0,"label":"optimizer_fault","offset":32010},{"bit":1,"label":"pid_fault","offset":32010},{"bit":2,"label":"string_ground_voltage_high","offset":32010},{"bit":3,"label":"external_fan_fault","offset":32010},{"bit":4,"label":"battery_reverse","offset":32010},{"bit":5,"label":"grid_island_controller_fault","offset":32010},{"bit":6,"label":"pv_string_lost","offset":32010},{"bit":7,"label":"internal_fan_fault","offset":32010},{"bit":8,"label":"dc_protection_fault","offset":32010}]},{"kind":"alarm_bits","card":"MC","offsets":[32008,32009,32010],"bits":[{"bit":0,"label":"string_overvoltage","offset":32008},{"bit":1,"label":"dc_arc","offset":32008},{"bit":2,"label":"string_reverse","offset":32008},{"bit":3,"label":"string_backfeed","offset":32008},{"bit":4,"label":"string_power","offset":32008},{"bit":5,"label":"afci_selftest","offset":32008},{"bit":6,"label":"phase_to_pe_short","offset":32008},{"bit":7,"label":"grid_lost","offset":32008},{"bit":8,"label":"grid_undervoltage","offset":32008},{"bit":9,"label":"grid_overvoltage","offset":32008},{"bit":10,"label":"grid_voltage_imbalance","offset":32008},{"bit":11,"label":"grid_overfrequency","offset":32008},{"bit":12,"label":"grid_underfrequency","offset":32008},{"bit":13,"label":"grid_frequency_unstable","offset":32008},{"bit":14,"label":"output_overcurrent","offset":32008},{"bit":15,"label":"output_dc_high","offset":32008},{"bit":0,"label":"residual_current_fault","offset":32009},{"bit":1,"label":"grounding_fault","offset":32009},{"bit":2,"label":"insulation_low","offset":32009},{"bit":3,"label":"temperature_high","offset":32009},{"bit":4,"label":"device_fault","offset":32009},{"bit":5,"label":"upgrade_or_version_fault","offset":32009},{"bit":6,"label":"licence_expired","offset":32009},{"bit":7,"label":"monitor_fault","offset":32009},{"bit":8,"label":"collector_or_meter_fault","offset":32009},{"bit":9,"label":"battery_fault","offset":32009},{"bit":10,"label":"active_islanding","offset":32009},{"bit":11,"label":"passive_islanding","offset":32009},{"bit":12,"label":"transient_ac_overvoltage","offset":32009},{"bit":13,"label":"external_port_short","offset":32009},{"bit":14,"label":"output_overload","offset":32009},{"bit":15,"label":"pv_configuration_fault","offset":32009},{"bit":0,"label":"optimizer_fault","offset":32010},{"bit":1,"label":"pid_fault","offset":32010},{"bit":2,"label":"string_ground_voltage_high","offset":32010},{"bit":3,"label":"external_fan_fault","offset":32010},{"bit":4,"label":"battery_reverse","offset":32010},{"bit":5,"label":"grid_island_controller_fault","offset":32010},{"bit":6,"label":"pv_string_lost","offset":32010},{"bit":7,"label":"internal_fan_fault","offset":32010},{"bit":8,"label":"dc_protection_fault","offset":32010}]},{"kind":"status_enum","card":"MC","offsets":[32089],"values":[{"code":"0x0000","meaning":"standby_initializing"},{"code":"0x0001","meaning":"standby_insulation_check"},{"code":"0x0002","meaning":"standby_irradiation_check"},{"code":"0x0003","meaning":"standby_grid_check"},{"code":"0x0100","meaning":"starting"},{"code":"0x0200","meaning":"grid_operation"},{"code":"0x0201","meaning":"grid_power_limited"},{"code":"0x0202","meaning":"grid_derated"},{"code":"0x0300","meaning":"shutdown_fault"},{"code":"0x0301","meaning":"shutdown_command"},{"code":"0x0302","meaning":"shutdown_ovgr"},{"code":"0x0303","meaning":"shutdown_communication"},{"code":"0x0304","meaning":"shutdown_power_limit"},{"code":"0x0305","meaning":"manual_start_required"},{"code":"0x0307","meaning":"rapid_shutdown"},{"code":"0x0401","meaning":"cosphi_p_schedule"},{"code":"0x0402","meaning":"q_u_schedule"},{"code":"0x0403","meaning":"pf_u_schedule"},{"code":"0x0404","meaning":"dry_contact_schedule"},{"code":"0x0405","meaning":"q_p_schedule"},{"code":"0x0500","meaning":"test_ready"},{"code":"0x0501","meaning":"test_running"},{"code":"0x0600","meaning":"inspection"},{"code":"0x0700","meaning":"afci_self_check"},{"code":"0x0800","meaning":"iv_scan"},{"code":"0xA000","meaning":"standby_no_irradiation"}]},{"kind":"alarm_bits","card":"SUN-C81","offsets":[50000,50001,50002,50003,50004,50005,50006,50007,50008,50009,50016],"bits":[{"bit":1,"label":"alarm.faultstring1","offset":50000},{"bit":2,"label":"alarm.faultstring2","offset":50000},{"bit":3,"label":"alarm.faultstring3","offset":50000},{"bit":4,"label":"alarm.faultstring4","offset":50000},{"bit":5,"label":"alarm.faultstring5","offset":50000},{"bit":6,"label":"alarm.faultstring6","offset":50000},{"bit":10,"label":"alarm.swver_unmatch","offset":50000},{"bit":12,"label":"alarm.upgradefailed","offset":50000},{"bit":13,"label":"alarm.flashfault","offset":50000},{"bit":14,"label":"alarm.licenseexpired","offset":50000},{"bit":1,"label":"alarm.swver_unmatch","offset":50001},{"bit":2,"label":"alarm.swver_unmatch","offset":50001},{"bit":3,"label":"alarm.systemfault","offset":50001},{"bit":4,"label":"alarm.systemfault","offset":50001},{"bit":5,"label":"alarm.faultdccircuit","offset":50001},{"bit":6,"label":"alarm.faultinv_circuit","offset":50001},{"bit":7,"label":"alarm.abn_residualcurr","offset":50001},{"bit":8,"label":"alarm.overtemp","offset":50001},{"bit":10,"label":"alarm.systemfault","offset":50001},{"bit":11,"label":"alarm.fanfault","offset":50001},{"bit":12,"label":"alarm.faultspicomm","offset":50001},{"bit":14,"label":"alarm.systemfault","offset":50001},{"bit":0,"label":"alarm.lowinsulationresistance","offset":50002},{"bit":1,"label":"alarm.afciself_checkfailure","offset":50002},{"bit":2,"label":"alarm.dcarcfault","offset":50002},{"bit":3,"label":"alarm.afciself_checkfailure","offset":50002},{"bit":4,"label":"alarm.afciself_checkfailure","offset":50002},{"bit":5,"label":"alarm.faultinv_circuit","offset":50002},{"bit":7,"label":"alarm.systemfault","offset":50002},{"bit":8,"label":"alarm.systemfault","offset":50002},{"bit":9,"label":"alarm.string3reversed","offset":50002},{"bit":12,"label":"alarm.dcarcfault","offset":50002},{"bit":13,"label":"alarm.dcarcfault","offset":50002},{"bit":14,"label":"alarm.dcarcfault","offset":50002},{"bit":15,"label":"alarm.systemfault","offset":50002},{"bit":1,"label":"alarm.string1reversed","offset":50003},{"bit":2,"label":"alarm.string2reversed","offset":50003},{"bit":3,"label":"alarm.faultdccircuit","offset":50003},{"bit":6,"label":"alarm.string4reversed","offset":50003},{"bit":7,"label":"alarm.string5reversed","offset":50003},{"bit":8,"label":"alarm.string6reversed","offset":50003},{"bit":9,"label":"alarm.highdcinputvolts","offset":50003},{"bit":10,"label":"alarm.highdcinputvolts","offset":50003},{"bit":11,"label":"alarm.highdcinputvolts","offset":50003},{"bit":12,"label":"alarm.highdcinputvolts","offset":50003},{"bit":15,"label":"alarm.faultdccircuit","offset":50003},{"bit":2,"label":"alarm.string1reversed","offset":50004},{"bit":3,"label":"alarm.string2reversed","offset":50004},{"bit":4,"label":"alarm.string7reversed","offset":50004},{"bit":5,"label":"alarm.string7reversed","offset":50004},{"bit":6,"label":"alarm.string8reversed","offset":50004},{"bit":7,"label":"alarm.string8reversed","offset":50004},{"bit":8,"label":"alarm.faultpvstringconnection","offset":50004},{"bit":9,"label":"alarm.faultpvstringconnection","offset":50004},{"bit":10,"label":"alarm.faultpvstringconnection","offset":50004},{"bit":11,"label":"alarm.faultpvstringconnection","offset":50004},{"bit":12,"label":"alarm.string3reversed","offset":50004},{"bit":13,"label":"alarm.string4reversed","offset":50004},{"bit":14,"label":"alarm.string5reversed","offset":50004},{"bit":15,"label":"alarm.string6reversed","offset":50004},{"bit":1,"label":"alarm.faultdccircuit","offset":50005},{"bit":2,"label":"alarm.abn_auxiliarypower","offset":50005},{"bit":4,"label":"alarm.faultdccircuit","offset":50005},{"bit":5,"label":"alarm.faultdccircuit","offset":50005},{"bit":6,"label":"alarm.faultdccircuit","offset":50005},{"bit":7,"label":"alarm.bstinductorcableconnectionfault","offset":50005},{"bit":8,"label":"alarm.bstinductorcableconnectionfault","offset":50005},{"bit":9,"label":"alarm.bstinductorcableconnectionfault","offset":50005},{"bit":10,"label":"alarm.bstinductorcableconnectionfault","offset":50005},{"bit":6,"label":"alarm.systemfault","offset":50006},{"bit":10,"label":"alarm.faultinv_circuit","offset":50006},{"bit":12,"label":"alarm.faultinv_circuit","offset":50006},{"bit":1,"label":"alarm.faultinv_circuit","offset":50007},{"bit":5,"label":"alarm.systemfault","offset":50007},{"bit":0,"label":"alarm.faultgridvolts","offset":50008},{"bit":3,"label":"alarm.faultgridvolts","offset":50008},{"bit":6,"label":"alarm.faultgridfrequency","offset":50008},{"bit":7,"label":"alarm.faultgridfrequency","offset":50008},{"bit":8,"label":"alarm.faultgridvolts","offset":50008},{"bit":9,"label":"alarm.faultgridvolts","offset":50008},{"bit":10,"label":"alarm.faultgrounding","offset":50008},{"bit":11,"label":"alarm.faultgridvolts","offset":50008},{"bit":12,"label":"alarm.faultgridfrequency","offset":50008},{"bit":0,"label":"alarm.faultgridvolts","offset":50009},{"bit":1,"label":"alarm.faultgridvolts","offset":50009},{"bit":2,"label":"alarm.faultgridvolts","offset":50009},{"bit":8,"label":"alarm.faultgridvolts","offset":50009},{"bit":0,"label":"alarm.faultstring1","offset":50016},{"bit":1,"label":"alarm.faultstring2","offset":50016},{"bit":2,"label":"alarm.faultstring3","offset":50016},{"bit":3,"label":"alarm.faultstring4","offset":50016},{"bit":4,"label":"alarm.faultstring5","offset":50016},{"bit":5,"label":"alarm.faultstring6","offset":50016},{"bit":6,"label":"alarm.faultstring7","offset":50016},{"bit":7,"label":"alarm.faultstring8","offset":50016}]},{"kind":"alarm_bits","card":"SUN-V300","offsets":[32008,32009,32010],"bits":[{"bit":0,"label":"alarm.highstringinputvolts","offset":32008},{"bit":1,"label":"alarm.dcarcfault","offset":32008},{"bit":2,"label":"alarm.stringreverseconnection","offset":32008},{"bit":3,"label":"alarm.stringampsbackfeed","offset":32008},{"bit":4,"label":"alarm.faultstringpower","offset":32008},{"bit":5,"label":"alarm.afciself_checkfail","offset":32008},{"bit":6,"label":"alarm.phasewireshort_circuitedtope","offset":32008},{"bit":7,"label":"alarm.gridloss","offset":32008},{"bit":8,"label":"alarm.gridundervolts","offset":32008},{"bit":9,"label":"alarm.gridovervolts","offset":32008},{"bit":10,"label":"alarm.gridvolt_imbalance","offset":32008},{"bit":11,"label":"alarm.gridoverfrequency","offset":32008},{"bit":12,"label":"alarm.gridunderfrequency","offset":32008},{"bit":13,"label":"alarm.unstablegridfrequency","offset":32008},{"bit":14,"label":"alarm.outputoveramps","offset":32008},{"bit":15,"label":"alarm.outputdccomponentoverhigh","offset":32008},{"bit":0,"label":"alarm.faultresidualamps","offset":32009},{"bit":1,"label":"alarm.faultgrounding","offset":32009},{"bit":2,"label":"alarm.lowinsulationresistance","offset":32009},{"bit":3,"label":"alarm.overtemp","offset":32009},{"bit":4,"label":"alarm.devicefault","offset":32009},{"bit":5,"label":"alarm.upgradefailedorversionmismatch","offset":32009},{"bit":6,"label":"alarm.licenseexpired","offset":32009},{"bit":7,"label":"alarm.faultymonitoringunit","offset":32009},{"bit":8,"label":"alarm.faultypowercollector","offset":32009},{"bit":9,"label":"alarm.packfault","offset":32009},{"bit":10,"label":"alarm.activeislanding","offset":32009},{"bit":11,"label":"alarm.passiveislanding","offset":32009},{"bit":12,"label":"alarm.transientacovervolts","offset":32009},{"bit":13,"label":"alarm.peripheralportshortcircuit","offset":32009},{"bit":14,"label":"alarm.churnoutputoverload","offset":32009},{"bit":15,"label":"alarm.faultpvmoduleconfiguration","offset":32009},{"bit":0,"label":"alarm.optimizerfault","offset":32010},{"bit":1,"label":"alarm.built_inpidoperationfault","offset":32010},{"bit":2,"label":"alarm.highinputstringvoltstoground","offset":32010},{"bit":3,"label":"alarm.externalfanfault","offset":32010},{"bit":4,"label":"alarm.packreverseconnection","offset":32010},{"bit":5,"label":"alarm.on_grid_off_gridcontrollerfault","offset":32010},{"bit":6,"label":"alarm.pvstringloss","offset":32010},{"bit":7,"label":"alarm.internalfanfault","offset":32010},{"bit":8,"label":"alarm.dcprotectionunitfault","offset":32010},{"bit":10,"label":"alarm.managementsystemcertvalidtimeineffective"},{"bit":11,"label":"alarm.managementsystemcertvalidtimebeingoverdue"},{"bit":12,"label":"alarm.managementsystemcertvalidtimeoverdue"},{"bit":3,"label":"alarm.ctdisconnection"},{"bit":4,"label":"alarm.ptdisconnection"}]},{"kind":"value_enum","card":"SUN-V300","offsets":[32089],"values":[{"code":"0x0000","meaning":"standby_initializing"},{"code":"0x0001","meaning":"standby_insulation_check"},{"code":"0x0002","meaning":"standby_irradiance_check"},{"code":"0x0003","meaning":"standby_grid_check"},{"code":"0x0100","meaning":"starting"},{"code":"0x0200","meaning":"running_mode_dependent"},{"code":"0x0201","meaning":"running_power_limited"},{"code":"0x0202","meaning":"running_self_derated"},{"code":"0x0300","meaning":"fault_shutdown"},{"code":"0x0301","meaning":"command_shutdown"},{"code":"0x0302","meaning":"ovgr_shutdown"},{"code":"0x0303","meaning":"communication_shutdown"},{"code":"0x0304","meaning":"power_limit_shutdown"},{"code":"0x0305","meaning":"manual_start_required"},{"code":"0x0306","meaning":"dc_switch_off"},{"code":"0x0307","meaning":"rapid_shutdown"},{"code":"0x0308","meaning":"input_underpower"},{"code":"0x0401","meaning":"cosphi_p_dispatch"},{"code":"0x0402","meaning":"q_u_dispatch"},{"code":"0x0403","meaning":"pf_u_dispatch"},{"code":"0x0404","meaning":"dry_contact_dispatch"},{"code":"0x0405","meaning":"q_p_dispatch"},{"code":"0x0500","meaning":"inspection_ready"},{"code":"0x0501","meaning":"inspection_running"},{"code":"0x0600","meaning":"checking"},{"code":"0X0700","meaning":"afci_self_test"},{"code":"0X0800","meaning":"iv_scan"},{"code":"0X0900","meaning":"dc_input_check"},{"code":"0X0A00","meaning":"off_grid_charge"},{"code":"0xA000","meaning":"standby_no_irradiance"}]}]}`)

// ID returns the profile identifier. The two hand families keep theirs.
func (p CatalogMonitoringProfile) ID() string { return p.id }

// Card returns the catalogue card key.
func (p CatalogMonitoringProfile) Card() string { return p.card }

// Family returns the family name the profile serves.
func (p CatalogMonitoringProfile) Family() string { return p.family }

// IDPrefix returns the decoder-key prefix, including its trailing dot.
func (p CatalogMonitoringProfile) IDPrefix() string { return p.prefix }

// StringCount returns the number of documented string voltage channels.
func (p CatalogMonitoringProfile) StringCount() int { return p.strings }

// Plan returns the FC03 reads of the profile at one unit.
func (p CatalogMonitoringProfile) Plan(unit uint8) []SUN2000ReadStep {
	steps := make([]SUN2000ReadStep, 0, len(p.groups))
	for _, group := range p.groups {
		steps = append(steps, SUN2000ReadStep{Function: uint8(modbusreg.FunctionReadHoldingRegisters), UnitID: unit, Offset: group.offset, Quantity: group.quantity})
	}
	return steps
}

// Facts returns a copy of the decoded facts.
func (r CatalogMonitoringReading) Facts() []SUN2000Fact {
	return append([]SUN2000Fact(nil), r.facts...)
}

// Fact returns the fact with the decoder key and index.
func (r CatalogMonitoringReading) Fact(key string, index int) (SUN2000Fact, bool) {
	for _, fact := range r.facts {
		if fact.Key == key && fact.Index == index {
			return fact, true
		}
	}
	return SUN2000Fact{}, false
}

// Groups returns copies of the group readings in plan order.
func (r CatalogMonitoringReading) Groups() []SUN2000GroupReading {
	out := make([]SUN2000GroupReading, len(r.groups))
	for index, group := range r.groups {
		out[index] = group.clone()
	}
	return out
}

// Losses returns the unit losses recorded for this reading.
func (r CatalogMonitoringReading) Losses() []ProviderLoss {
	return append([]ProviderLoss(nil), r.losses...)
}

// CatalogMonitoringProfiles returns one profile per catalogue card, ordered
// by card key. Status names and alarm labels come from the document compiled
// into the module. A missing or unreadable document is an error, and no
// profile is returned with empty names.
func CatalogMonitoringProfiles() ([]CatalogMonitoringProfile, error) {
	catalogOnce.Do(func() {
		catalogLoaded = catalogLoad()
	})
	if catalogLoaded.labelErr != nil {
		return nil, catalogLoaded.labelErr
	}
	return append([]CatalogMonitoringProfile(nil), catalogLoaded.profiles...), nil
}

// Decode validates the group readings and decodes the fields they carry.
// A group the unit refuses is a loss for this unit. The other groups still
// decode. A group outside the plan, a repeated group, or a short reply is
// refused as a whole.
func (p CatalogMonitoringProfile) Decode(unit uint8, readings []SUN2000GroupReading) (CatalogMonitoringReading, error) {
	if p.id == "" || unit == 0 || unit > huaweiSUN2000MaxUnit {
		return CatalogMonitoringReading{}, errHuaweiSUN2000ReadingInvalid
	}
	groups := make([]SUN2000GroupReading, len(p.groups))
	for index, group := range p.groups {
		groups[index] = SUN2000GroupReading{Offset: group.offset, Quantity: group.quantity, Outcome: SUN2000GroupNotRead}
	}
	seen := make([]bool, len(p.groups))
	var losses []ProviderLoss
	for _, reading := range readings {
		position := catalogGroupPosition(p.groups, reading)
		if position < 0 || seen[position] || !validHuaweiSUN2000GroupReading(reading) {
			return CatalogMonitoringReading{}, errHuaweiSUN2000ReadingInvalid
		}
		seen[position] = true
		groups[position] = reading.clone()
		if reading.Outcome == SUN2000GroupException {
			losses = append(losses, ProviderLoss{Kind: ProviderLossRefusedGroup, HasUnit: true, UnitID: unit,
				Offset: reading.Offset, Quantity: reading.Quantity, Detail: huaweiRuntimeRefusalDetail(reading.ExceptionCode)})
		}
	}
	return CatalogMonitoringReading{facts: p.catalogFacts(groups), groups: groups, losses: losses}, nil
}

func catalogGroupPosition(groups []catalogGroup, reading SUN2000GroupReading) int {
	for index, group := range groups {
		if reading.Offset == group.offset && reading.Quantity == group.quantity {
			return index
		}
	}
	return -1
}

func (p CatalogMonitoringProfile) catalogFacts(groups []SUN2000GroupReading) []SUN2000Fact {
	facts := make([]SUN2000Fact, 0, len(p.fields))
	for _, field := range p.fields {
		if field.kind == catalogAlarm {
			continue
		}
		facts = append(facts, p.catalogFact(field, groups))
	}
	facts = append(facts, p.catalogAlarmFacts(groups)...)
	return facts
}

func (p CatalogMonitoringProfile) catalogFact(field catalogField, groups []SUN2000GroupReading) SUN2000Fact {
	fact := SUN2000Fact{Key: p.prefix + field.name, Index: field.index, Unit: field.unit, Availability: FactUnknown, Grade: GradeUnknown}
	group, words, ok := catalogFieldWords(field, groups)
	if !ok {
		return fact
	}
	raw, ok := catalogDecodeWord(field, words)
	if !ok {
		return fact
	}
	if field.kind == catalogStatus {
		meaning, named := p.states[uint16(raw)]
		if !named || meaning == "" {
			return fact
		}
		fact.Availability, fact.Value, fact.State = FactValue, Decimal{Coefficient: raw}, meaning
		fact.Grade, fact.Cycle, fact.Retained = GradeHypothesis, group.Cycle, group.Retained
		return fact
	}
	fact.Availability, fact.Value = FactValue, Decimal{Coefficient: raw, Exponent: field.exponent}
	fact.Grade, fact.Cycle, fact.Retained = GradeHypothesis, group.Cycle, group.Retained
	return fact
}

func catalogFieldWords(field catalogField, groups []SUN2000GroupReading) (SUN2000GroupReading, []uint16, bool) {
	for _, group := range groups {
		if group.Outcome != SUN2000GroupAnswered || field.offset < group.Offset {
			continue
		}
		if field.offset >= group.Offset+group.Quantity {
			continue
		}
		return group, group.Words[field.offset-group.Offset:], true
	}
	return SUN2000GroupReading{}, nil, false
}

func catalogDecodeWord(field catalogField, words []uint16) (int64, bool) {
	if len(words) < int(field.width) || field.width == 0 {
		return 0, false
	}
	if field.width == 1 {
		if field.signed {
			return int64(int16(words[0])), true
		}
		return int64(words[0]), true
	}
	combined := uint32(words[0])<<16 | uint32(words[1])
	if field.signed {
		return int64(int32(combined)), true
	}
	return int64(combined), true
}

func (p CatalogMonitoringProfile) catalogAlarmFacts(groups []SUN2000GroupReading) []SUN2000Fact {
	var facts []SUN2000Fact
	for _, field := range p.fields {
		if field.kind != catalogAlarm {
			continue
		}
		group, words, ok := catalogFieldWords(field, groups)
		if !ok || len(words) == 0 {
			continue
		}
		facts = append(facts, catalogBits(p.prefix, field.offset, words[0], p.alarms[field.offset], group)...)
	}
	return facts
}

func catalogBits(prefix string, offset, word uint16, labels map[int]string, group SUN2000GroupReading) []SUN2000Fact {
	var facts []SUN2000Fact
	for bit := 0; bit < 16; bit++ {
		if word&(1<<uint(bit)) == 0 {
			continue
		}
		label := ""
		if labels != nil {
			label = labels[bit]
		}
		fact := SUN2000Fact{Availability: FactValue, Value: Decimal{Coefficient: 1}, Cycle: group.Cycle, Retained: group.Retained}
		if label != "" {
			fact.Key, fact.State, fact.Grade = prefix+label, label, GradeHypothesis
		} else {
			fact.Key = prefix + "bit_" + strconv.Itoa(int(offset)) + "_" + strconv.Itoa(bit)
			fact.Index, fact.Grade = bit, GradeUnknown
		}
		facts = append(facts, fact)
	}
	return facts
}

func catalogLoad() catalogBundle {
	states, alarms, err := catalogDictionaries(catalogLabelDocument)
	if err != nil {
		return catalogBundle{labelErr: err}
	}
	names, err := catalogRegisterNames()
	if err != nil {
		return catalogBundle{}
	}
	var built []CatalogMonitoringProfile
	for _, name := range names {
		data, err := profiles.HuaweiInverterCatalog().ReadFile(huaweiSUN2000CatalogDirectory + "/" + name)
		if err != nil {
			return catalogBundle{}
		}
		var file catalogRegisterFile
		if err := decodeHuaweiSUN2000CatalogFile(data, &file); err != nil || file.Card == "" {
			return catalogBundle{}
		}
		built = append(built, catalogBuild(file, states[file.Card], alarms[file.Card]))
	}
	sort.Slice(built, func(i, j int) bool { return built[i].card < built[j].card })
	return catalogBundle{profiles: built}
}

func catalogRegisterNames() ([]string, error) {
	found, err := profiles.HuaweiInverterCatalog().ReadDir(huaweiSUN2000CatalogDirectory)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range found {
		name := entry.Name()
		if strings.HasPrefix(name, "registers-") && strings.HasSuffix(name, ".json") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func catalogDictionaries(data []byte) (map[string]map[uint16]string, map[string]map[uint16]map[int]string, error) {
	if len(data) == 0 {
		return nil, nil, errCatalogLabelsUnavailable
	}
	var file catalogDictFile
	if err := decodeHuaweiSUN2000CatalogFile(data, &file); err != nil {
		return nil, nil, err
	}
	states := map[string]map[uint16]string{}
	alarms := map[string]map[uint16]map[int]string{}
	for _, item := range file.Dictionaries {
		catalogTakeDict(item, states, alarms)
	}
	return states, alarms, nil
}

func catalogTakeDict(item catalogDict, states map[string]map[uint16]string, alarms map[string]map[uint16]map[int]string) {
	switch item.Kind {
	case "status_enum", "value_enum":
		if !catalogCovers(item.Offsets, 32089) {
			return
		}
		for _, value := range item.Values {
			code, ok := catalogCode(value.Code)
			if ok && value.Meaning != "" {
				catalogPutState(states, item.Card, code, value.Meaning)
			}
		}
	case "alarm_bits":
		for _, bit := range item.Bits {
			if bit.Offset == nil || bit.Bit < 0 || bit.Bit > 15 || bit.Label == "" {
				continue
			}
			if !catalogCovers(item.Offsets, *bit.Offset) {
				continue
			}
			catalogPutAlarm(alarms, item.Card, uint16(*bit.Offset), bit.Bit, bit.Label)
		}
	}
}

func catalogCovers(offsets []int, want int) bool {
	for _, offset := range offsets {
		if offset == want {
			return true
		}
	}
	return false
}

func catalogCode(text string) (uint16, bool) {
	text = strings.TrimSpace(strings.ToLower(text))
	base := 10
	if strings.HasPrefix(text, "0x") {
		text = text[2:]
		base = 16
	}
	value, err := strconv.ParseUint(text, base, 16)
	if err != nil {
		return 0, false
	}
	return uint16(value), true
}

func catalogPutState(states map[string]map[uint16]string, card string, code uint16, meaning string) {
	if states[card] == nil {
		states[card] = map[uint16]string{}
	}
	if previous, ok := states[card][code]; ok && previous != meaning {
		delete(states[card], code)
		return
	}
	states[card][code] = meaning
}

func catalogPutAlarm(alarms map[string]map[uint16]map[int]string, card string, offset uint16, bit int, label string) {
	if alarms[card] == nil {
		alarms[card] = map[uint16]map[int]string{}
	}
	if alarms[card][offset] == nil {
		alarms[card][offset] = map[int]string{}
	}
	alarms[card][offset][bit] = label
}

func catalogBuild(file catalogRegisterFile, states map[uint16]string, alarms map[uint16]map[int]string) CatalogMonitoringProfile {
	fields := catalogFields(file.Rows, alarms)
	if (file.Card == "MC" || file.Card == "MA-C00") && !catalogHasOffset(fields, 32108) {
		fields = append(fields, catalogField{name: "energy_dc_input", offset: 32108, width: 2, exponent: -2, unit: "kWh", kind: catalogEnergy})
	}
	fields = catalogDropOverlaps(fields)
	groups := catalogGroups(file.Card, fields)
	profile := CatalogMonitoringProfile{
		id: catalogIdentity(file.Card), card: file.Card, family: catalogFamily(file.Card),
		prefix: catalogPrefix(file.Card), groups: groups, fields: catalogKeepCovered(fields, groups),
		states: states, alarms: alarms,
	}
	for _, field := range profile.fields {
		if field.kind == catalogString && field.unit == "V" {
			profile.strings++
		}
	}
	return profile
}

func catalogIdentity(card string) string {
	switch card {
	case "MC":
		return SUN2000M3MonitoringProfileID
	case "MA-C00":
		return SUN2000M1MonitoringProfileID
	default:
		return "huawei.sun2000." + strings.ToLower(card) + ".monitoring-readonly@1.0.0"
	}
}

func catalogFamily(card string) string {
	switch card {
	case "MC":
		return huaweiSUN2000M3Family
	case "MA-C00":
		return huaweiSUN2000M1Family
	default:
		return "SUN2000 " + card
	}
}

func catalogPrefix(card string) string {
	switch card {
	case "MC":
		return "huawei.sun2000.m3."
	case "MA-C00":
		return "huawei.sun2000.m1."
	default:
		return "huawei.sun2000." + strings.ToLower(card) + "."
	}
}

func catalogHasOffset(fields []catalogField, offset uint16) bool {
	for _, field := range fields {
		if field.offset == offset {
			return true
		}
	}
	return false
}

func catalogFields(rows []catalogRegisterRow, alarms map[uint16]map[int]string) []catalogField {
	var fields []catalogField
	seen := map[int]int{}
	for _, row := range rows {
		field, ok := catalogClassify(row, alarms)
		if !ok || row.Offset == nil {
			continue
		}
		seen[*row.Offset]++
		fields = append(fields, field)
	}
	var kept []catalogField
	for _, field := range fields {
		if seen[int(field.offset)] == 1 {
			kept = append(kept, field)
		}
	}
	return kept
}

func catalogClassify(row catalogRegisterRow, alarms map[uint16]map[int]string) (catalogField, bool) {
	if row.Offset == nil || row.Endpoint != "inverter" || row.Access == "WO" {
		return catalogField{}, false
	}
	if row.Class != "reviewed" && row.Class != "conditional" {
		return catalogField{}, false
	}
	width, signed, word := catalogWordShape(row.Type)
	if !word || row.Quantity != width {
		return catalogField{}, false
	}
	gain, unit := "", ""
	if row.Decode != nil {
		gain, unit = row.Decode.Gain, row.Decode.Unit
	}
	field := catalogField{offset: uint16(*row.Offset), width: uint16(width), signed: signed, unit: catalogPhysicalUnit(unit)}
	exponent, scaled := catalogScale(gain)
	field.exponent = exponent
	switch {
	case catalogStringClass(row.FunctionalClass) && width == 1 && (unit == "V" || unit == "A") && scaled:
		number, ok := catalogChannel(row.Key, *row.Offset)
		if !ok {
			return catalogField{}, false
		}
		field.kind, field.index = catalogString, number
		if unit == "V" {
			field.name = "pv_string_voltage"
		} else {
			field.name = "pv_string_current"
		}
	case catalogTrackerClass(row.FunctionalClass) && *row.Offset < 40000 && !strings.Contains(strings.ToLower(row.Key), "predict") && scaled && (unit == "kW" || unit == "kWh"):
		number, ok := catalogNumber(catalogTrackerNumber, row.Key)
		if !ok {
			return catalogField{}, false
		}
		field.kind, field.index = catalogTracker, number
		if unit == "kW" {
			field.name = "mppt_power"
		} else {
			field.name = "mppt_energy"
		}
	case catalogRatingOffset(*row.Offset) && catalogRatingUnit(unit) && scaled:
		name, ok := catalogKnownName(*row.Offset)
		if !ok {
			return catalogField{}, false
		}
		field.kind, field.name = catalogRating, name
	case catalogEnergyClass(row.FunctionalClass) && unit == "kWh" && *row.Offset < 37000 && scaled:
		field.kind = catalogEnergy
		if name, ok := catalogKnownName(*row.Offset); ok {
			field.name = name
		} else {
			field.name = "energy_" + strconv.Itoa(*row.Offset)
		}
	case *row.Offset == 32089:
		field.kind, field.name, field.unit, field.exponent = catalogStatus, "status", "", 0
	case alarms != nil && alarms[uint16(*row.Offset)] != nil && width == 1:
		field.kind, field.name = catalogAlarm, "alarm_word"
	case *row.Offset >= 32064 && *row.Offset < 32106 && !catalogSkipsPower(row.FunctionalClass) && scaled:
		field.kind = catalogPower
		if name, ok := catalogKnownName(*row.Offset); ok {
			field.name = name
		} else {
			field.name = "register_" + strconv.Itoa(*row.Offset)
		}
	default:
		return catalogField{}, false
	}
	return field, true
}

func catalogStringClass(class string) bool {
	switch class {
	case "pv.channel", "dc_channel", "pv12", "pv36":
		return true
	default:
		return false
	}
}

func catalogTrackerClass(class string) bool {
	return class == "mppt" || class == "pv.mppt"
}

func catalogEnergyClass(class string) bool {
	return class == "energy" || class == "energy.period_or_total"
}

func catalogSkipsPower(class string) bool {
	switch class {
	case "epoch", "epoch_times", "fault_code", "alarm.priority_code", "time.event_or_collection":
		return true
	default:
		return false
	}
}

func catalogRatingOffset(offset int) bool {
	switch offset {
	case 30073, 30075, 30077, 30079, 30081, 30083, 30085:
		return true
	default:
		return false
	}
}

func catalogRatingUnit(unit string) bool {
	switch unit {
	case "kW", "kVA", "kVar":
		return true
	default:
		return false
	}
}

func catalogWordShape(typeName string) (int, bool, bool) {
	switch strings.ToUpper(strings.TrimSpace(typeName)) {
	case "I16", "INT16":
		return 1, true, true
	case "U16", "UINT16", "ENUM16", "E16", "BITFIELD16":
		return 1, false, true
	case "I32", "INT32":
		return 2, true, true
	case "U32", "UINT32":
		return 2, false, true
	default:
		return 0, false, false
	}
}

func catalogScale(gain string) (int16, bool) {
	switch strings.TrimSpace(gain) {
	case "1":
		return 0, true
	case "10":
		return -1, true
	case "100":
		return -2, true
	case "1000":
		return -3, true
	default:
		return 0, false
	}
}

func catalogPhysicalUnit(unit string) string {
	switch strings.TrimSpace(unit) {
	case "", "N/A", "n/a", "not stated", "-":
		return ""
	default:
		return strings.TrimSpace(unit)
	}
}

func catalogKnownName(offset int) (string, bool) {
	switch offset {
	case 30073:
		return "rated_power", true
	case 30075:
		return "active_limit", true
	case 30077:
		return "apparent_limit", true
	case 30079:
		return "reactive_export_limit", true
	case 30081:
		return "reactive_import_limit", true
	case 30083:
		return "nameplate_active_capacity", true
	case 30085:
		return "nameplate_apparent_capacity", true
	case 32064:
		return "input_power", true
	case 32066:
		return "line_voltage_ab", true
	case 32067:
		return "line_voltage_bc", true
	case 32068:
		return "line_voltage_ca", true
	case 32069:
		return "phase_voltage_a", true
	case 32070:
		return "phase_voltage_b", true
	case 32071:
		return "phase_voltage_c", true
	case 32072:
		return "phase_current_a", true
	case 32074:
		return "phase_current_b", true
	case 32076:
		return "phase_current_c", true
	case 32078:
		return "peak_active_power", true
	case 32080:
		return "active_power", true
	case 32082:
		return "reactive_power", true
	case 32084:
		return "power_factor", true
	case 32085:
		return "frequency", true
	case 32086:
		return "efficiency", true
	case 32087:
		return "internal_temperature", true
	case 32088:
		return "insulation_resistance", true
	case 32106:
		return "energy_accumulated", true
	case 32108:
		return "energy_dc_input", true
	case 32112:
		return "energy_current_hour", true
	case 32114:
		return "energy_current_day", true
	case 32116:
		return "energy_current_month", true
	case 32118:
		return "energy_current_year", true
	default:
		return "", false
	}
}

func catalogChannel(key string, offset int) (int, bool) {
	if number, ok := catalogNumber(catalogStringNumber, key); ok {
		return number, true
	}
	if number, ok := catalogNumber(catalogDirectChannel, key); ok {
		return number, true
	}
	if number, ok := catalogNumber(catalogStringShort, key); ok {
		return number, true
	}
	// A channel pair keeps its place when the card names the word only by
	// address. The low block starts at 32016. The high block starts at
	// 32344, which the cards that name it call channel 25. Aggregate
	// voltages outside both blocks are not channels.
	if offset >= 32016 && offset < 32064 {
		return (offset-32016)/2 + 1, true
	}
	if offset >= 32344 && offset < 32400 {
		return 25 + (offset-32344)/2, true
	}
	return 0, false
}

func catalogNumber(expression *regexp.Regexp, text string) (int, bool) {
	match := expression.FindStringSubmatch(text)
	if match == nil {
		return 0, false
	}
	for _, part := range match[1:] {
		if part == "" {
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil || value <= 0 {
			return 0, false
		}
		return value, true
	}
	return 0, false
}

func catalogDropOverlaps(fields []catalogField) []catalogField {
	sort.Slice(fields, func(i, j int) bool {
		if fields[i].offset != fields[j].offset {
			return fields[i].offset < fields[j].offset
		}
		return fields[i].name < fields[j].name
	})
	var kept []catalogField
	var end uint16
	for _, field := range fields {
		if len(kept) > 0 && field.offset < end {
			continue
		}
		kept = append(kept, field)
		end = field.offset + field.width
	}
	return kept
}

func catalogGroups(card string, fields []catalogField) []catalogGroup {
	rest := fields
	var forced []catalogField
	if catalogHasOffset(fields, 32064) {
		quantity := catalogWindowQuantity(fields, catalogPower, catalogStatus, 32064, 32106, 33)
		forced, rest = catalogTakeRange(fields, catalogPower, 32064, 32064+quantity, catalogStatus)
	}
	energyStart := uint16(32106)
	energyWidth := uint16(14)
	hand := card == "MC" || card == "MA-C00"
	var energy []catalogField
	if hand {
		energy, rest = catalogTakeRange(rest, catalogEnergy, energyStart, energyStart+energyWidth)
	}
	groups := catalogPack(rest)
	if len(forced) > 0 {
		groups = append(groups, catalogGroup{offset: 32064, quantity: catalogWindowQuantity(forced, catalogPower, catalogStatus, 32064, 32106, 33)})
	}
	if hand && len(energy) > 0 {
		groups = append(groups, catalogGroup{offset: energyStart, quantity: energyWidth})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].offset < groups[j].offset })
	return groups
}

func catalogWindowQuantity(fields []catalogField, left, right catalogKind, start, limit, minimum uint16) uint16 {
	end := start + minimum
	if end > limit {
		end = limit
	}
	for _, field := range fields {
		if field.kind != left && field.kind != right {
			continue
		}
		fieldEnd := field.offset + field.width
		if field.offset >= start && fieldEnd <= limit && fieldEnd > end {
			end = fieldEnd
		}
	}
	return end - start
}

func catalogTakeRange(fields []catalogField, kind catalogKind, start, end uint16, extra ...catalogKind) (inside, rest []catalogField) {
	for _, field := range fields {
		matches := field.kind == kind
		for _, other := range extra {
			if field.kind == other {
				matches = true
			}
		}
		if matches && field.offset >= start && field.offset < end {
			inside = append(inside, field)
			continue
		}
		rest = append(rest, field)
	}
	return inside, rest
}

func catalogPack(fields []catalogField) []catalogGroup {
	if len(fields) == 0 {
		return nil
	}
	sorted := append([]catalogField(nil), fields...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].offset < sorted[j].offset })
	var groups []catalogGroup
	start := sorted[0].offset
	end := start + sorted[0].width
	for _, field := range sorted[1:] {
		fieldEnd := field.offset + field.width
		if field.offset > end || fieldEnd-start > huaweiSUN2000MaxReadWords {
			groups = append(groups, catalogGroup{offset: start, quantity: end - start})
			start, end = field.offset, fieldEnd
			continue
		}
		end = fieldEnd
	}
	return append(groups, catalogGroup{offset: start, quantity: end - start})
}

func catalogKeepCovered(fields []catalogField, groups []catalogGroup) []catalogField {
	var kept []catalogField
	for _, field := range fields {
		for _, group := range groups {
			if field.offset >= group.offset && field.offset+field.width <= group.offset+group.quantity {
				kept = append(kept, field)
				break
			}
		}
	}
	return kept
}
