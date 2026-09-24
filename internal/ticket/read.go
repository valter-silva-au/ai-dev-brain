package ticket

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
)

func (service *Service) List(
	_ context.Context,
	request ListRequest,
) (capability.Result[ListData], error) {
	located, err := ReadScopeManifests(request.Scope)
	if err != nil {
		return emptyListResult(), err
	}
	tickets := make([]TicketData, 0, len(located))
	for _, candidate := range located {
		if !request.IncludeArchived &&
			candidate.Manifest.ArchiveState == ArchiveStateArchived {
			continue
		}
		tickets = append(
			tickets,
			ticketData(candidate.Layout, candidate.Manifest),
		)
	}
	sort.Slice(tickets, func(left int, right int) bool {
		if tickets[left].Manifest.VisibleKey !=
			tickets[right].Manifest.VisibleKey {
			return tickets[left].Manifest.VisibleKey <
				tickets[right].Manifest.VisibleKey
		}
		return tickets[left].Manifest.ID < tickets[right].Manifest.ID
	})
	return capability.Result[ListData]{
		Capability:  ListDescriptor.Capability,
		Version:     ListDescriptor.Version,
		Outcome:     capability.OutcomeHealthy,
		Data:        ListData{Tickets: tickets},
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}, nil
}

func (service *Service) Show(
	_ context.Context,
	request ShowRequest,
) (capability.Result[ShowData], error) {
	located, err := findTicket(request.Scope, request.Selector)
	if err != nil {
		return emptyShowResult(), err
	}
	return capability.Result[ShowData]{
		Capability:  ShowDescriptor.Capability,
		Version:     ShowDescriptor.Version,
		Outcome:     capability.OutcomeHealthy,
		Data:        ShowData{Ticket: ticketData(located.Layout, located.Manifest)},
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}, nil
}

func findTicket(
	scope ScopeLayout,
	selector string,
) (LocatedManifest, error) {
	if selector == "" {
		return LocatedManifest{}, fmt.Errorf("ticket selector is required")
	}
	located, err := ReadScopeManifests(scope)
	if err != nil {
		return LocatedManifest{}, err
	}
	folded := foldSelector(selector)
	for _, candidate := range located {
		selectors := append(
			[]string{
				candidate.Manifest.ID,
				candidate.Manifest.LocalKey,
				candidate.Manifest.VisibleKey,
				candidate.Layout.RelativePath(),
				candidate.Layout.OwnerRelativePath(),
				candidate.Layout.Root(),
			},
			candidate.Manifest.Aliases...,
		)
		for _, value := range selectors {
			if value == selector ||
				(!filepath.IsAbs(value) &&
					foldSelector(value) == folded) {
				return candidate, nil
			}
		}
	}
	return LocatedManifest{}, fmt.Errorf(
		"read ticket %q: %w",
		selector,
		sql.ErrNoRows,
	)
}

func ticketData(layout Layout, manifest Manifest) TicketData {
	return TicketData{
		Manifest: manifest,
		Layout:   layout,
		Path:     layout.Root(),
	}
}

func emptyListResult() capability.Result[ListData] {
	return capability.Result[ListData]{
		Capability:  ListDescriptor.Capability,
		Version:     ListDescriptor.Version,
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}

func emptyShowResult() capability.Result[ShowData] {
	return capability.Result[ShowData]{
		Capability:  ShowDescriptor.Capability,
		Version:     ShowDescriptor.Version,
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}
