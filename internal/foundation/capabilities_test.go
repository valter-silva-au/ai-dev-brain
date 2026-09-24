package foundation

import "testing"

func TestFoundationCapabilityDescriptors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		capability string
		version    string
		command    string
		tool       string
		mutating   bool
	}{
		{
			name:       "initialize",
			capability: "workspace.initialize",
			version:    "v1",
			command:    "init",
			tool:       "adb_workspace_initialize",
			mutating:   true,
		},
		{
			name:       "doctor",
			capability: "workspace.doctor",
			version:    "v1",
			command:    "doctor",
			tool:       "adb_workspace_doctor",
			mutating:   false,
		},
	}

	descriptors := map[string]struct {
		capability string
		version    string
		command    string
		tool       string
		mutating   bool
	}{
		"initialize": {
			InitializeDescriptor.Capability,
			InitializeDescriptor.Version,
			InitializeDescriptor.Command,
			InitializeDescriptor.Tool,
			InitializeDescriptor.Mutating,
		},
		"doctor": {
			DoctorDescriptor.Capability,
			DoctorDescriptor.Version,
			DoctorDescriptor.Command,
			DoctorDescriptor.Tool,
			DoctorDescriptor.Mutating,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := descriptors[test.name]
			if got.capability != test.capability ||
				got.version != test.version ||
				got.command != test.command ||
				got.tool != test.tool ||
				got.mutating != test.mutating {
				t.Fatalf("descriptor = %#v, want %#v", got, test)
			}
		})
	}
}
