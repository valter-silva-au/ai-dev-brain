package v3cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/foundation"
)

var ErrAttention = errors.New("doctor requires attention")

func newDoctorCommand(service Foundation) *cobra.Command {
	var (
		workspacePath string
		format        string
	)

	command := &cobra.Command{
		Use:   foundation.DoctorDescriptor.Command,
		Short: foundation.DoctorDescriptor.Summary,
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			v3Annotation: "true",
		},
		RunE: func(command *cobra.Command, _ []string) error {
			if service == nil {
				return errors.New("foundation service is not configured")
			}
			if workspacePath == "" {
				return errors.New("--workspace is required")
			}
			root, err := filepath.Abs(workspacePath)
			if err != nil {
				return fmt.Errorf("resolve workspace path: %w", err)
			}

			result, err := service.Doctor(
				command.Context(),
				foundation.DoctorRequest{Root: root},
			)
			if err != nil {
				return err
			}
			if err := writeResult(
				command,
				format,
				result,
				renderDoctorHuman,
			); err != nil {
				return err
			}
			if result.Outcome == capability.OutcomeAttention {
				command.SilenceErrors = true
				return ErrAttention
			}
			return nil
		},
	}

	command.Flags().StringVar(
		&workspacePath,
		"workspace",
		"",
		"Explicit v3 workspace path",
	)
	command.Flags().StringVar(
		&format,
		"format",
		"human",
		"Output format: human or json",
	)

	return command
}

func renderDoctorHuman(
	writer io.Writer,
	result capability.Result[foundation.DoctorData],
) error {
	if _, err := fmt.Fprintf(
		writer,
		"%s/%s: %s\n",
		result.Capability,
		result.Version,
		result.Outcome,
	); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "Root: %s\n", result.Data.Root); err != nil {
		return err
	}
	if result.Data.WorkspaceID != "" {
		if _, err := fmt.Fprintf(
			writer,
			"Workspace: %s\n",
			result.Data.WorkspaceID,
		); err != nil {
			return err
		}
	}
	for _, finding := range result.Data.Findings {
		if _, err := fmt.Fprintf(
			writer,
			"%s %s: %s\n",
			finding.Severity,
			finding.ID,
			finding.Summary,
		); err != nil {
			return err
		}
		for _, evidence := range finding.Evidence {
			if _, err := fmt.Fprintf(writer, "  Evidence: %s\n", evidence); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(
			writer,
			"  Remediation: %s\n",
			finding.Remediation,
		); err != nil {
			return err
		}
	}
	return renderCommonHuman(
		writer,
		result.Warnings,
		result.NextActions,
		result.Recovery,
	)
}
