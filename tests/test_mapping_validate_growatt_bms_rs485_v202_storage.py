#!/usr/bin/env python3
"""Focused mutation controls for every Issue #25 mapping acceptance dimension."""
import copy
import importlib.util
import sys
from fractions import Fraction
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location('m',ROOT/'scripts/mapping/validate_growatt_bms_rs485_v202_storage_mapping.py')
m=importlib.util.module_from_spec(spec); sys.modules[spec.name]=m; spec.loader.exec_module(m)
def reject(name,path,value,error):
 d=copy.deepcopy(m.load()); m.mutate(d,{'path':path,'value':value})
 try: m.validate_contract(d)
 except ValueError as e:
  if error not in str(e): raise AssertionError(e)
 else: raise AssertionError(name+' accepted')
 print(name+': REJECTED')
ONE,ZERO={'coefficient':'1','exponent10':0},{'coefficient':'0','exponent10':0}
# (native unit, canonical SI unit) -> exact factor and offset; the SI value is native x factor + offset.
CONVERSIONS={('%','unit.ratio'):{'factor':{'coefficient':'1','exponent10':-2},'offset':ZERO},('V','unit.volt'):{'factor':ONE,'offset':ZERO},('A','unit.ampere'):{'factor':ONE,'offset':ZERO},('degC','unit.kelvin'):{'factor':ONE,'offset':{'coefficient':'27315','exponent10':-2}},('Ah','unit.ampere_hour'):{'factor':ONE,'offset':ZERO},('mV','unit.volt'):{'factor':{'coefficient':'1','exponent10':-3},'offset':ZERO}}
# Rule index by target, so that a mutation names the rule it varies.
RULE={rule['target']:index for index,rule in enumerate(m.load()['field_rules'])}
def fraction_of(number):
    return Fraction(int(number['coefficient'])) * Fraction(10) ** number['exponent10']


def kernel(value):
    """Exact kernel Decimal of a terminating Fraction, computed independently of the validator."""
    exponent = 0
    while value.denominator != 1:
        value *= 10
        exponent -= 1
    coefficient = value.numerator
    if coefficient == 0:
        return dict(ZERO)
    while coefficient % 10 == 0:
        coefficient //= 10
        exponent += 1
    return {'coefficient': str(coefficient), 'exponent10': exponent}


def reject_change(name, change, error):
    document = copy.deepcopy(m.load())
    change(document)
    try:
        m.validate_contract(document)
    except ValueError as e:
        if error not in str(e):
            raise AssertionError(name + ': ' + str(e)) from e
    else:
        raise AssertionError(name + ' accepted')
    print(name + ': REJECTED')


def check_rule_conversions(contract):
    """Each quantity rule names its pack field's SI unit and the exact conversion of its unit pair, with a unit loss exactly when that is not the identity."""
    if m.UNIT_CONVERSION != CONVERSIONS or hasattr(m, 'UNIT_SHIFT'):
        raise AssertionError('conversion table differs from the unit rule')
    fields = m.pack_fields()
    for rule in contract['field_rules']:
        if 'unit' not in rule:
            continue
        if rule['unit'] != fields[rule['target']]['unit'] or rule['conversion'] != CONVERSIONS[(rule['native_unit'], rule['unit'])]:
            raise AssertionError(rule['target'] + ' does not state its SI conversion')
        identity = rule['conversion'] == {'factor': ONE, 'offset': ZERO}
        if identity == ('unit' in [loss['kind'] for loss in rule['disposition']['loss']]):
            raise AssertionError(rule['target'] + ' unit loss')
    units = {rule['target']: rule['unit'] for rule in contract['field_rules'] if 'unit' in rule}
    if units['storage.state.soc'] != 'unit.ratio' or units['storage.temperature.pack'] != 'unit.kelvin':
        raise AssertionError('state of charge and temperature are not in SI units')
    if {units['storage.capacity.charge'], units['storage.capacity.discharge']} != {'unit.ampere_hour'}:
        raise AssertionError('both capacities stay in ampere-hours')


def check_published_values(contract):
    """The published value is the native value through the exact factor and offset, computed here with fractions."""
    typed = contract['scenarios'][0]['observation']['typed']
    outputs = [scenario['expect']['output'] for scenario in contract['scenarios'] if 'output' in scenario.get('expect', {})]
    for fact in [fact for output in outputs for fact in output['facts']]:
        rule = contract['field_rules'][RULE[fact['key']['fact_id']]]
        if 'unit' not in rule:
            continue
        native = Fraction(str(typed[rule['source']]))
        expected = kernel(native * fraction_of(rule['conversion']['factor']) + fraction_of(rule['conversion']['offset']))
        if fact['value']['quantity'] != {'number': expected, 'unit': rule['unit']}:
            raise AssertionError(rule['target'] + ' published value')
    if kernel(Fraction(75, 100)) != {'coefficient': '75', 'exponent10': -2} or kernel(Fraction(25) + Fraction(27315, 100)) != {'coefficient': '29815', 'exponent10': -2}:
        raise AssertionError('worked values')


def check_si_conversions():
    contract = m.load()
    check_rule_conversions(contract)
    check_published_values(contract)
    print('si-conversions-and-published-values: PASS')


PACK_KIND = 'resource.battery_pack'
ASSET_ID = m.load()['scenarios'][0]['identity']['asset_id']
SCOPE = {'profile': 'synthetic', 'version': 'synthetic', 'rule_scope': 'synthetic'}


def relation(definition, from_kind, to_kind, evidence):
    return {'definition': definition, 'basis_class': 'gate_rule', 'from_kind': from_kind, 'to_kind': to_kind,
            'evidence': evidence, 'validity_scope': dict(SCOPE), 'absence_sides': []}


def cell_by_index(document):
    """A module-to-cell relation keyed by the cell index, with no module published."""
    document['graph']['relations'].append(relation('storage.relation.module_cell', 'resource.battery_module', 'resource.battery_cell',
                                                   ['native_position:cell_index']))


def collect(cases, run):
    """Run every case; report each one accepted or rejected for another reason, so that a RED run lists them all."""
    problems = []
    for name, change, error in cases:
        try:
            run(change)
        except (ValueError, KeyError, TypeError, IndexError) as e:
            if error not in str(e):
                problems.append(f'{name}: rejected as {e!r}, expected {error}')
                continue
        else:
            problems.append(f'{name}: accepted')
            continue
        print(name + ': REJECTED')
    if problems:
        raise AssertionError('; '.join(problems))
    return len(cases)


def run_contract(change):
    document = copy.deepcopy(m.load())
    change(document)
    m.validate_contract(document)


def res(document):
    return document['graph']['resources'][0]


def join(document):
    return document['graph']['joins'][0]


def selector(document, name):
    return next(s for s in document['graph']['selectors'] if s['selector'] == name)


def row(document, name):
    return next(r for r in document['unstated_relations'] if r['definition'] == name)


def pack(document):
    return document['identity']['resources']['pack']


def ids(case):
    return case['identity']


def key(output):
    return output['facts'][0]['key']


def graph_member_cases():
    """Section 15 items 1 to 6: every member of each graph entry varied, through the shared checker or the gate's own rules."""
    return [
        # Resource: kind, anchor class, anchor basis, anchor source, scheme, parent kind, evidence.
        ('resource-kind', lambda d: res(d).update(kind='resource.battery'), 'duplicate_entry'),
        ('resource-kind-not-member', lambda d: res(d).update(kind='resource.storage_bank'), 'kind_not_member'),
        ('resource-anchor-class', lambda d: res(d).update(anchor_class='function'), 'anchor_class_mismatch'),
        ('resource-anchor-basis', lambda d: res(d).update(anchor_basis='native_identity'), 'anchor_basis_mismatch'),
        ('resource-anchor-source-address', lambda d: res(d).update(anchor_source='unit_id'), 'anchor_source_not_allowed'),
        ('resource-pack-on-operator-label', lambda d: res(d).update(anchor_basis='operator_slot', anchor_source='operator_label'), 'graph_anchor'),
        ('resource-battery-on-native-position', lambda d: d['graph']['resources'][2].update(anchor_basis='native_slot', anchor_source='native_position'), 'graph_anchor'),
        ('resource-interface-on-label', lambda d: d['graph']['resources'][4].update(anchor_basis='operator_slot', anchor_source='operator_label'), 'graph_anchor'),
        ('resource-interface-parent-battery', lambda d: d['graph']['resources'][4].update(parent_kind='resource.battery'), 'graph_parent'),
        ('resource-system-dropped', lambda d: d['graph']['resources'].pop(3), 'dangling_endpoint $.graph.relations[2].from_kind'),
        ('resource-scheme-not-listed', lambda d: res(d).update(identification_scheme={'id': 'scheme.pack_serial', 'issuer_registry': 'none'}), 'scheme_not_listed'),
        ('resource-parent-kind', lambda d: res(d).update(parent_kind='resource.battery'), 'graph_parent'),
        ('resource-token-stability-unstated', lambda d: res(d).update(evidence=[e for e in res(d)['evidence'] if not e.startswith('native_owner_token_stability:')]), 'graph_evidence'),
        ('resource-native-contract-unstated', lambda d: res(d).update(evidence=[e for e in res(d)['evidence'] if not e.startswith('native_contract:')]), 'graph_evidence'),
        ('resource-second-kind', lambda d: d['graph']['resources'].append(dict(res(d), kind='resource.battery')), 'duplicate_entry'),
        # Relations: the named negative, a dangling battery endpoint, a stated relation the evidence does not support.
        ('cell-relation-by-index-without-module', cell_by_index, 'dangling_endpoint $.graph.relations[3].from_kind'),
        ('battery-pack-relation-duplicated', lambda d: d['graph']['relations'].append(relation('storage.relation.battery_pack', 'resource.battery', PACK_KIND, ['gate_rule:one_pack'])), 'duplicate_entry'),
        # The three required storage relations are stated, each with evidence and scope.
        ('battery-pack-dropped', lambda d: d['graph']['relations'].pop(0), 'graph_relations'),
        ('pack-interface-dropped', lambda d: d['graph']['relations'].pop(1), 'graph_relations'),
        ('system-battery-dropped', lambda d: d['graph']['relations'].pop(2), 'graph_relations'),
        ('system-battery-by-gate-rule', lambda d: d['graph']['relations'][2].update(basis_class='gate_rule'), 'basis_class_not_admitted'),
        ('pack-interface-by-configuration', lambda d: d['graph']['relations'][1].update(basis_class='configuration'), 'basis_class_not_admitted'),
        ('battery-pack-evidence-empty', lambda d: d['graph']['relations'][0].update(evidence=[]), 'invalid_value'),
        ('pack-interface-scope-widened', lambda d: d['graph']['relations'][1]['validity_scope'].update(rule_scope='every_interface'), 'graph_relations'),
        ('system-battery-scope-profile', lambda d: d['graph']['relations'][2]['validity_scope'].update(profile='other'), 'graph_relations'),
        ('battery-pack-absence-claimed', lambda d: d['graph']['relations'][0].update(absence_sides=['to']), 'graph_relations'),
        ('relation-not-supported-by-evidence', lambda d: d['graph']['relations'].append(dict(relation('relation.relays', PACK_KIND, PACK_KIND, ['gate_rule:x']), basis_class='configuration')), 'graph_relations'),
        # Relayed values: the pack is read directly.
        ('relayed-stated', lambda d: d['graph']['relayed'].append({'relaying_kind': PACK_KIND, 'relayed_kind': PACK_KIND, 'direct_anchor_key_source': 'none'}), 'graph_relayed'),
        ('relayed-address-key', lambda d: d['graph']['relayed'].append({'relaying_kind': PACK_KIND, 'relayed_kind': PACK_KIND, 'direct_anchor_key_source': 'unit_id'}), 'anchor_source_not_allowed'),
        # Two paths: kind, observations, basis, native tokens.
        ('join-kind', lambda d: join(d).update(kind='resource.battery'), 'graph_joins'),
        ('join-kind-unpublished', lambda d: join(d).update(kind='resource.meter'), 'kind_not_published'),
        ('join-observations', lambda d: join(d).update(observations=['read_directly_over_rs485', 'seen_by_label']), 'graph_joins'),
        ('join-by-label', lambda d: join(d).update(basis='label'), 'join_basis_not_allowed'),
        ('join-false-native-tokens', lambda d: join(d).update(native_tokens='differ'), 'false_join'),
        ('join-native-tokens-equal-without-token', lambda d: join(d).update(native_tokens='equal'), 'graph_joins'),
        ('join-removed', lambda d: d['graph'].update(joins=[]), 'graph_joins'),
        # Aggregates: no fact of this gate is an aggregate.
        ('aggregate-stated', lambda d: d['graph']['aggregates'].append({'fact': 'storage.pack.voltage', 'subject_kind': PACK_KIND, 'coverage': []}), 'not_an_aggregate'),
        # Selectors: selector, classification, evidence; the address selector is never identity.
        ('address-selector-as-identity', lambda d: selector(d, 'unit_id').update(classification='identity_evidence'), 'graph_selectors'),
        ('address-selector-as-attribute', lambda d: selector(d, 'unit_id').update(classification='resource_attribute'), 'graph_selectors'),
        ('selector-classification-unknown', lambda d: selector(d, 'revision').update(classification='address'), 'invalid_value'),
        ('selector-renamed', lambda d: selector(d, 'profile').update(selector='profile_id'), 'graph_selectors'),
        ('selector-evidence', lambda d: selector(d, 'unit_id').update(evidence=['synthetic:other']), 'graph_selectors'),
        ('selector-removed', lambda d: d['graph'].update(selectors=[s for s in d['graph']['selectors'] if s['selector'] != 'unit_id']), 'graph_selectors'),
        ('address-as-identity-evidence', lambda d: d['identity']['native_identity_evidence'].append('unit_id'), 'identity contract'),
        ('revision-as-identity-evidence', lambda d: d['identity']['native_identity_evidence'].append('revision'), 'identity contract'),
        # Section 15 item 6 and PLN-1: no plane membership anywhere in the gate.
        ('plane-on-resource', lambda d: res(d).update(planes=['physical']), 'plane_membership_stated'),
        ('plane-on-gate', lambda d: d.update(plane='physical'), 'plane_membership_stated'),
        ('graph-not-stated', lambda d: d.pop('graph'), 'graph_not_stated'),
        ('graph-member-missing', lambda d: d['graph'].pop('selectors'), 'missing_member'),
    ]


def unstated_cases():
    """Section 7.2: each storage relation the gate does not state is unknown, never absent, with what withholds it."""
    return [
        ('unstated-row-removed', lambda d: d['unstated_relations'].pop(), 'unstated_relations'),
        ('unstated-definition', lambda d: row(d, 'storage.relation.pack_module').update(definition='storage.relation.pack_modules'), 'unstated_relations'),
        ('unstated-from-kind', lambda d: row(d, 'storage.relation.module_cell').update(from_kind=PACK_KIND), 'unstated_relations'),
        ('unstated-to-kind', lambda d: row(d, 'storage.relation.module_cell').update(to_kind='resource.battery_module'), 'unstated_relations'),
        ('unstated-absent', lambda d: row(d, 'storage.relation.pack_string').update(outcome='absent'), 'unstated_relations'),
        ('unstated-withheld-by', lambda d: row(d, 'storage.relation.pack_string').update(withheld_by='unsupported:not_in_the_list'), 'unstated_relations'),
        ('unstated-system-battery-returned', lambda d: d['unstated_relations'].append({'definition': 'storage.relation.system_battery', 'from_kind': 'resource.storage_system', 'to_kind': 'resource.battery', 'outcome': 'unknown_not_absent', 'withheld_by': 'configuration:none'}), 'unstated_relations'),
        ('unstated-pack-interface-returned', lambda d: d['unstated_relations'].append({'definition': 'storage.relation.pack_interface', 'from_kind': PACK_KIND, 'to_kind': 'resource.storage_interface', 'outcome': 'unknown_not_absent', 'withheld_by': 'native_contract:none'}), 'unstated_relations'),
    ]


def subject_cases():
    """EN-1: each field rule names its catalog subject kind and its measurement point."""
    cases = []
    for target, index in RULE.items():
        cases.append((f'subject-kind-{target}', lambda d, i=index: d['field_rules'][i].update(subject_kind='resource.battery'), 'field rule subject'))
        cases.append((f'measurement-point-{target}', lambda d, i=index: d['field_rules'][i].update(measurement_point='reporting_device'), 'field rule subject'))
        cases.append((f'subject-missing-{target}', lambda d, i=index: d['field_rules'][i].pop('subject_kind'), 'field rule subject'))
    # EN-4: the catalog keeps the native sign convention of pack current, so the rule keeps it as a provenance loss.
    cases.append(('sign-convention-dropped', lambda d: d['field_rules'][RULE['storage.pack.current']]['disposition']['loss'][0].update(kind='policy'), 'field rule sign'))
    return cases


def resource_identity_cases():
    """Section 6.3: the pack's ResourceID is gateway-allocated per its pack dimension; each member varied."""
    return [
        ('pack-role-dimension', lambda d: pack(d).update(dimension='storage.dimension.battery'), 'identity contract'),
        ('pack-role-source-asset', lambda d: pack(d).update(source='gateway_configured_semantic_asset_id'), 'identity contract'),
        ('pack-role-not-required', lambda d: pack(d).update(required=False), 'identity contract'),
        ('pack-role-extra-member', lambda d: pack(d).update(kind=PACK_KIND), 'identity contract'),
        ('pack-role-missing', lambda d: d['identity'].pop('resources'), 'identity contract'),
        ('cell-role-without-fact', lambda d: d['identity']['resources'].update(cells={'dimension': 'storage.dimension.cell', 'source': 'gateway_allocated_resource_id', 'required': True}), 'identity contract'),
    ]


def run_case(change):
    contract = m.load()
    case = copy.deepcopy(contract['scenarios'][0])
    for key in ('expect', 'id', 'polarity'):
        case.pop(key)
    change(case)
    m.project(case, contract)


def observation_cases():
    """RES-5 and REL-8: the pack dimension carries the pack's ResourceID, never the asset, source or binding identifier."""
    return [
        ('asset-id-as-pack-dimension-value', lambda c: ids(c)['resources'].update(pack=ids(c)['asset_id']), 'identity_missing_or_invalid'),
        ('source-id-as-pack-dimension-value', lambda c: ids(c)['resources'].update(pack=c['source']['source_id']), 'identity_missing_or_invalid'),
        ('binding-id-as-pack-dimension-value', lambda c: ids(c)['resources'].update(pack=ids(c)['binding_id']), 'identity_missing_or_invalid'),
        ('pack-resource-missing', lambda c: ids(c)['resources'].pop('pack'), 'identity_missing_or_invalid'),
        ('pack-resources-missing', lambda c: ids(c).pop('resources'), 'identity_missing_or_invalid'),
        ('pack-resource-invalid', lambda c: ids(c)['resources'].update(pack='!'), 'identity_missing_or_invalid'),
        ('pack-resource-number', lambda c: ids(c)['resources'].update(pack=7), 'identity_missing_or_invalid'),
        ('pack-resource-extra-role', lambda c: ids(c)['resources'].update(string='resource:demo-string-1'), 'identity_missing_or_invalid'),
    ]


def run_document(change):
    contract = m.load()
    change(contract['scenarios'][0]['expect']['output'])
    m.document(contract)


def fact_key_cases():
    """Every member of the published fact key varied in the expected output; each drift is a scenario output mismatch."""
    return [
        ('key-pack-id', lambda o: key(o).update(pack_id='helianthus.pack.other'), 'scenario output'),
        ('key-pack-version', lambda o: key(o).update(pack_version='1.0.0'), 'scenario output'),
        ('key-fact-id', lambda o: key(o).update(fact_id='storage.pack.power'), 'scenario output'),
        ('key-dimension-id', lambda o: key(o)['dimensions'][0].update(id='storage.dimension.battery'), 'scenario output'),
        ('key-dimension-value-asset-id', lambda o: key(o)['dimensions'][0]['value'].update(text=ASSET_ID), 'scenario output'),
        ('key-dimension-value-kind', lambda o: key(o)['dimensions'][0]['value'].update(kind='resource'), 'scenario output'),
        ('key-second-dimension', lambda o: key(o)['dimensions'].append({'id': 'storage.dimension.cell', 'value': {'kind': 'text', 'text': 'resource:demo-cell-1'}}), 'scenario output'),
        ('disposition-source-key', lambda o: o['dispositions'][0]['source_keys'][0]['dimensions'][0]['value'].update(text=ASSET_ID), 'scenario output'),
    ]


def check_published_subjects():
    """Each published key's resource dimension is its fact's catalog subject dimension, valued by the pack's ResourceID."""
    catalog = {e['ref']['id']: e['attributes'] for e in m.graph_rules.load_catalog()['elements'] if e['element_kind'] == 'fact'}
    contract = m.load()
    baseline = contract['scenarios'][0]
    resources = baseline['identity']['resources']
    role_of = {spec['dimension']: role for role, spec in m.RESOURCE_ROLES.items()}
    identifiers = [resources[role] for role, spec in m.RESOURCE_ROLES.items() if spec['source'] == m.ALLOCATED]
    if len(set(identifiers + [baseline['identity']['asset_id'], baseline['identity']['binding_id'], baseline['source']['source_id']])) != len(identifiers) + 3:
        raise AssertionError('a ResourceID repeats another identifier')
    case = copy.deepcopy(baseline)
    for member in ('expect', 'id', 'polarity'):
        case.pop(member)
    projected = m.project(case, contract)
    m.document(contract)
    for output in (baseline['expect']['output'], projected):
        keys = [fact['key'] for fact in output['facts']] + [k for d in output['dispositions'] for k in d['source_keys']]
        for fact_key in keys:
            dimension = catalog[fact_key['fact_id']]['subject']['dimension']['id']
            text = resources[role_of[dimension]]
            if fact_key['dimensions'] != [{'id': dimension, 'value': {'kind': 'text', 'text': text}}]:
                raise AssertionError(fact_key['fact_id'] + ' key is not its catalog subject ResourceID')
    print('published-subjects: PASS')


def check_relation_dispositions():
    """Each relation definition of the storage pack is stated in the graph or in the unknown statements, exactly once."""
    pack = m.load()['pack']
    names = sorted(e['ref']['id'] for e in m.graph_rules.load_catalog()['elements']
                   if e['element_kind'] == 'relation' and e.get('disposition') == 'member' and e['ref']['owner'].get('pack') == pack)
    contract = m.load()
    stated = [r['definition'] for r in contract['graph']['relations']] + [r['definition'] for r in contract['unstated_relations']]
    if sorted(stated) != names or len(names) != 6:
        raise AssertionError('storage relation definitions are not each disposed once')
    print('relation-dispositions: PASS')


def check_graph():
    total = collect(graph_member_cases() + unstated_cases() + subject_cases() + resource_identity_cases(), run_contract)
    total += collect(observation_cases(), run_case)
    total += collect(fact_key_cases(), run_document)
    check_published_subjects()
    check_relation_dispositions()
    return total


def wired_rows():
    """Cell voltage extremes are mapped, and each withheld row names the element it waits for."""
    contract = m.load()
    targets = {rule['target'] for rule in contract['field_rules']}
    if {'storage.cell.voltage_minimum', 'storage.cell.voltage_maximum'} - targets or 'cell_voltage_extrema' in contract.get('unsupported', []):
        raise AssertionError('cell voltage extremes are not mapped')
    waits = {row['source']: row.get('waits_for') for row in contract['withheld']}
    if waits.get('remaining_capacity_amp_hours') != 'storage.energy.remaining' or waits.get('full_charge_capacity_amp_hours') != 'storage.energy.usable_capacity':
        raise AssertionError('withheld capacity rows do not name the element they wait for')
    if any(not row.get('waits_for') for row in contract['withheld']):
        raise AssertionError('a withheld row does not name what it waits for')
    print('wired-rows: PASS')


def main():
 wired_rows()
 graph_total = check_graph()
 check_si_conversions()
 soc,temperature,voltage,charge=RULE['storage.state.soc'],RULE['storage.temperature.pack'],RULE['storage.pack.voltage'],RULE['storage.capacity.charge']
 conversion_cases=[
  # Key field native unit: a native unit with no entry for the canonical unit.
  ('native-unit-soc',lambda d:d['field_rules'][soc].update(native_unit='ratio'),'unit conversion'),
  ('native-unit-temperature',lambda d:d['field_rules'][temperature].update(native_unit='K'),'unit conversion'),
  ('native-unit-capacity-energy',lambda d:d['field_rules'][charge].update(native_unit='kWh'),'unit conversion'),
  ('native-unit-missing',lambda d:d['field_rules'][voltage].pop('native_unit'),'unit conversion'),
  # Key field canonical unit: a customary unit, or an SI unit other than the pack field's.
  ('canonical-unit-percent',lambda d:d['field_rules'][soc].update(unit='unit.percent'),'unit conversion'),
  ('canonical-unit-celsius',lambda d:d['field_rules'][temperature].update(unit='unit.celsius'),'unit conversion'),
  ('canonical-unit-capacity-joule',lambda d:d['field_rules'][charge].update(unit='unit.joule'),'unit conversion'),
  ('canonical-unit-other-si',lambda d:d['field_rules'][voltage].update(unit='unit.ampere'),'unit conversion'),
  # Value field factor: coefficient and exponent each varied.
  ('factor-coefficient',lambda d:d['field_rules'][soc]['conversion'].update(factor={'coefficient':'2','exponent10':-2}),'unit conversion'),
  ('factor-exponent',lambda d:d['field_rules'][soc]['conversion'].update(factor={'coefficient':'1','exponent10':-1}),'unit conversion'),
  ('factor-identity-dropped',lambda d:d['field_rules'][voltage]['conversion'].update(factor={'coefficient':'1','exponent10':-3}),'unit conversion'),
  # Value field offset: coefficient and exponent each varied.
  ('offset-coefficient',lambda d:d['field_rules'][temperature]['conversion'].update(offset={'coefficient':'273','exponent10':0}),'unit conversion'),
  ('offset-exponent',lambda d:d['field_rules'][temperature]['conversion'].update(offset={'coefficient':'27315','exponent10':-3}),'unit conversion'),
  ('offset-on-ratio',lambda d:d['field_rules'][soc]['conversion'].update(offset={'coefficient':'27315','exponent10':-2}),'unit conversion'),
  ('conversion-missing',lambda d:d['field_rules'][charge].pop('conversion'),'unit conversion'),
  ('conversion-extra-member',lambda d:d['field_rules'][soc]['conversion'].update(rounding='half_even'),'unit conversion'),
  ('rule-extra-member',lambda d:d['field_rules'][soc].update(scale=-2),'unit conversion'),
  ('symbol-rule-with-unit',lambda d:d['field_rules'][RULE['storage.status.operating']].update(unit='unit.ratio'),'unit conversion'),
  # Loss: a conversion that is not the identity declares a unit loss, and the identity declares none.
  ('silent-ratio-conversion',lambda d:d['field_rules'][soc].update(disposition={'outcome':'exact','loss':[]}),'silent unit conversion'),
  ('silent-kelvin-conversion',lambda d:d['field_rules'][temperature]['disposition']['loss'][0].update(kind='provenance'),'silent unit conversion'),
  ('identity-with-unit-loss',lambda d:d['field_rules'][RULE['storage.pack.current']]['disposition']['loss'][0].update(kind='unit'),'silent unit conversion'),
 ]
 for c in conversion_cases:
  reject_change(*c)
 cases=[('pin','pins.gateway_tree','0'*40,'contract'),('revision','native_contract.revision.cumulative_revision','2.03','native contract'),('unit','native_contract.unit.allowed','0..247','native contract'),('slice','native_contract.slices.0.quantity',8,'native contract'),('physical','native_contract.qualification.physical_qualified',True,'native contract'),('asset-derivation','identity.asset_id.forbidden_derivations.0','serial','identity contract'),('source-distinction','identity.source_id.distinct_from','unit_id','identity contract'),('binding-input','identity.binding_id.stable_inputs.0','unit_id','identity contract'),('observation-axis','lifecycle.required_observation_evidence.0','bad','lifecycle contract'),('exact-loss','field_rules.2.disposition.loss',[{'kind':'policy'}],'exact loss'),('invalid-loss-kind','field_rules.0.disposition.loss.0.kind','native_enum_bitfield','transformed loss'),('missing-withheld-reason','withheld.0.reason','','withheld unsupported'),('waits-remaining','withheld.0.waits_for','storage.energy.usable_capacity','withheld unsupported'),('waits-full-charge','withheld.1.waits_for','storage.energy.remaining','withheld unsupported'),('waits-cycle','withheld.2.waits_for','storage.energy.remaining','withheld unsupported'),('waits-charge-time','withheld.3.waits_for','storage.energy.remaining','withheld unsupported'),('waits-cycle-charge','withheld.4.waits_for','storage.energy.remaining','withheld unsupported'),('waits-average-cell','withheld.5.waits_for','storage.cell.voltage_minimum','withheld unsupported'),('waits-floating','withheld.6.waits_for','storage.pack.voltage','withheld unsupported'),('unsupported-removed','unsupported.0','x','withheld unsupported'),('requested-drift','projection.requested_items.0.item_id','storage.x','requested items'),('precedence','projection.error_precedence.0','x','projection'),('operation','projection.operations','allowed','projection'),('consumer','consumer_cutover.required_removals.1','allowed','consumer cutover')]
 for c in cases: reject(*c)
 data=m.load(); data['scenarios'][0]['expect']['output']['facts'][0]['value']['quantity']['number']['coefficient']='999'
 try: m.document(data)
 except ValueError as error:
  if str(error)!='scenario output': raise AssertionError(error)
 else: raise AssertionError('scenario output drift accepted')
 print('scenario-output-drift: REJECTED')
 print('baseline: PASS; %d focused mutations rejected'%(len(cases)+1+len(conversion_cases)+graph_total))
if __name__=='__main__': main()
