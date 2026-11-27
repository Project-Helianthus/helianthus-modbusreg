package pv

import (
	"context"

	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

// withPendingLinks upserts the pending operator links of the batch's own
// bindings, by binding id. The link of a matched binding is replaced; every
// other link stays. An empty set leaves the batch as it is.
func withPendingLinks(batch semreg.PublicationBatch, pending []semreg.IdentityLink) semreg.PublicationBatch {
	if len(pending) == 0 {
		return batch
	}
	replaced := map[semreg.NativeBindingID]bool{}
	for _, l := range pending {
		replaced[l.BindingID] = true
	}
	links := make([]semreg.IdentityLink, 0, len(batch.IdentityLinkUpserts)+len(pending))
	for _, l := range batch.IdentityLinkUpserts {
		if !replaced[l.BindingID] {
			links = append(links, l)
		}
	}
	batch.IdentityLinkUpserts = append(links, pending...)
	return batch
}

// StageSunSpec stages the SunSpec part of a publication in a transaction: the
// manufacturer and serial claim of each device, the relation claims, and the
// prepared batch, which carries the pending operator links of its own bindings.
// It does not commit.
func (p *Publisher) StageSunSpec(ctx context.Context, tx driver.PublicationTransaction, batch semreg.PublicationBatch, pub Publication) error {
	for _, claim := range pub.Claims {
		if err := tx.AdmitManufacturerSerial(claim); err != nil {
			return err
		}
	}
	for _, link := range pub.Links {
		if err := tx.StageLink(link); err != nil {
			return err
		}
	}
	return tx.Stage(withPendingLinks(batch, tx.OperatorLinks(batch.BindingUpserts)))
}

// StageDongle stages the dongle-family part of a publication. The logger batch
// keeps its behavior: the prepared batch is staged as it is.
func (p *Publisher) StageDongle(ctx context.Context, tx driver.PublicationTransaction, batch semreg.PublicationBatch, pub Publication) error {
	for _, link := range pub.DongleLinks {
		if err := tx.StageLink(link); err != nil {
			return err
		}
	}
	return tx.Stage(batch)
}
