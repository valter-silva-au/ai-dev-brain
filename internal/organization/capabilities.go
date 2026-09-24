package organization

import "github.com/valter-silva-au/ai-dev-brain/internal/capability"

var (
	InitializeDescriptor = capability.Descriptor{
		Capability: "organization.initialize",
		Version:    "v1",
		Command:    "init",
		Tool:       "adb_organization_initialize",
		Summary:    "Plan or initialize an organization trust scope.",
		Mutating:   true,
	}
	ListDescriptor = capability.Descriptor{
		Capability: "organization.list",
		Version:    "v1",
		Command:    "list",
		Tool:       "adb_organization_list",
		Summary:    "List registered organizations.",
		Mutating:   false,
	}
	ShowDescriptor = capability.Descriptor{
		Capability: "organization.show",
		Version:    "v1",
		Command:    "show",
		Tool:       "adb_organization_show",
		Summary:    "Show one registered organization.",
		Mutating:   false,
	}
	ValidateDescriptor = capability.Descriptor{
		Capability: "organization.validate",
		Version:    "v1",
		Command:    "validate",
		Tool:       "adb_organization_validate",
		Summary:    "Validate an organization without repairing it.",
		Mutating:   false,
	}
	UpdateDescriptor = capability.Descriptor{
		Capability: "organization.update",
		Version:    "v1",
		Command:    "update",
		Tool:       "adb_organization_update",
		Summary:    "Plan or update organization metadata.",
		Mutating:   true,
	}
	AdoptDescriptor = capability.Descriptor{
		Capability: "organization.adopt",
		Version:    "v1",
		Command:    "adopt",
		Tool:       "adb_organization_adopt",
		Summary:    "Plan or adopt an unmanaged organization directory.",
		Mutating:   true,
	}
	MoveDescriptor = capability.Descriptor{
		Capability: "organization.move",
		Version:    "v1",
		Command:    "move",
		Tool:       "adb_organization_move",
		Summary:    "Plan or move an organization while preserving identity.",
		Mutating:   true,
	}
	ArchiveDescriptor = capability.Descriptor{
		Capability: "organization.archive",
		Version:    "v1",
		Command:    "archive",
		Tool:       "adb_organization_archive",
		Summary:    "Plan or change an organization's archive state.",
		Mutating:   true,
	}
)
