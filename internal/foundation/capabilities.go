package foundation

import "github.com/valter-silva-au/ai-dev-brain/internal/capability"

var (
	InitializeDescriptor = capability.Descriptor{
		Capability: "workspace.initialize",
		Version:    "v1",
		Command:    "init",
		Tool:       "adb_workspace_initialize",
		Summary:    "Plan or initialize a v3 workspace.",
		Mutating:   true,
	}
	DoctorDescriptor = capability.Descriptor{
		Capability: "workspace.doctor",
		Version:    "v1",
		Command:    "doctor",
		Tool:       "adb_workspace_doctor",
		Summary:    "Inspect a v3 workspace without modifying it.",
		Mutating:   false,
	}
)
