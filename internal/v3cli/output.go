package v3cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
)

func writeResult[T any](
	command *cobra.Command,
	format string,
	result T,
	human func(io.Writer, T) error,
) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(command.OutOrStdout())
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			return fmt.Errorf("encode JSON result: %w", err)
		}
		return nil
	case "human":
		if human == nil {
			return errors.New("human result renderer is not configured")
		}
		if err := human(command.OutOrStdout(), result); err != nil {
			return fmt.Errorf("render human result: %w", err)
		}
		return nil
	default:
		return fmt.Errorf(
			"unsupported output format %q: use human or json",
			format,
		)
	}
}

func renderEffects(
	writer io.Writer,
	effects []capability.Effect,
) error {
	for _, effect := range effects {
		if _, err := fmt.Fprintf(
			writer,
			"%s %s %s\n",
			effect.Status,
			effect.Action,
			effect.Target,
		); err != nil {
			return err
		}
	}
	return nil
}

func renderCommonHuman(
	writer io.Writer,
	warnings []capability.Notice,
	actions []capability.Action,
	recovery capability.Recovery,
) error {
	for _, warning := range warnings {
		if _, err := fmt.Fprintf(
			writer,
			"Warning: %s\n",
			warning.Message,
		); err != nil {
			return err
		}
	}
	for _, action := range actions {
		if _, err := fmt.Fprintf(writer, "Next: %s\n", action.Message); err != nil {
			return err
		}
	}
	for _, guidance := range recovery.Guidance {
		if _, err := fmt.Fprintf(writer, "Recovery: %s\n", guidance); err != nil {
			return err
		}
	}
	return nil
}
