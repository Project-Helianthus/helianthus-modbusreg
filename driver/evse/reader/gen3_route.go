package reader

import (
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

// BindingIdentifier is the identifier of the native binding the provider
// publishes for the configured charger.
func (p ChargerPublication) BindingIdentifier() semreg.NativeBindingID { return p.bindingID() }

// ServiceInstanceIdentifier is the identifier of the service instance the
// provider publishes for the service a capability binds to.
func (p ChargerPublication) ServiceInstanceIdentifier(c Capability) semreg.ServiceInstanceID {
	return p.service(c).InstanceID
}

// CapabilityInstanceIdentifier is the identifier of the capability instance the
// provider names for a capability.
func (p ChargerPublication) CapabilityInstanceIdentifier(c Capability) semreg.CapabilityInstanceID {
	return semreg.CapabilityInstanceID(opaque("capability:tesla-wc3", p.scoped(c.Capability)...))
}

// ConnectorCandidateIdentifier is the identifier of the candidate the provider
// publishes for an element on the configured connector.
func (p ChargerPublication) ConnectorCandidateIdentifier(element string) semreg.CandidateID {
	return p.candidateID(element, RoleConnector)
}
