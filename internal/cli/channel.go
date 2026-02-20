package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"github.com/spf13/cobra"
)

// ChannelReg is set during app initialization in app.go.
var ChannelReg core.ChannelRegistry

var channelCmd = &cobra.Command{
	Use:   "channel",
	Short: "Manage input/output channels",
	Long:  `Commands for listing channels, viewing inbox items, and sending output.`,
}

var channelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered channel adapters",
	RunE: func(cmd *cobra.Command, args []string) error {
		if ChannelReg == nil {
			return fmt.Errorf("channel registry not initialized")
		}

		adapters := ChannelReg.ListAdapters()
		if len(adapters) == 0 {
			fmt.Println("No channel adapters registered.")
			return nil
		}

		fmt.Printf("%-20s %-10s\n", "NAME", "TYPE")
		fmt.Printf("%-20s %-10s\n", strings.Repeat("-", 20), strings.Repeat("-", 10))
		for _, a := range adapters {
			fmt.Printf("%-20s %-10s\n", a.Name(), string(a.Type()))
		}
		return nil
	},
}

var channelInboxCmd = &cobra.Command{
	Use:   "inbox [adapter-name]",
	Short: "Show pending items from a channel or all channels",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if ChannelReg == nil {
			return fmt.Errorf("channel registry not initialized")
		}

		var items []models.ChannelItem
		var err error

		if len(args) > 0 {
			adapter, adapterErr := ChannelReg.GetAdapter(args[0])
			if adapterErr != nil {
				return adapterErr
			}
			items, err = adapter.Fetch()
		} else {
			items, err = ChannelReg.FetchAll()
		}
		if err != nil {
			return err
		}

		if len(items) == 0 {
			fmt.Println("No pending items.")
			return nil
		}

		fmt.Printf("%-20s %-10s %-10s %-20s %s\n", "ID", "CHANNEL", "PRIORITY", "FROM", "SUBJECT")
		fmt.Printf("%-20s %-10s %-10s %-20s %s\n",
			strings.Repeat("-", 20), strings.Repeat("-", 10),
			strings.Repeat("-", 10), strings.Repeat("-", 20),
			strings.Repeat("-", 30))
		for _, item := range items {
			subject := item.Subject
			if len(subject) > 50 {
				subject = subject[:47] + "..."
			}
			fmt.Printf("%-20s %-10s %-10s %-20s %s\n",
				truncate(item.ID, 20),
				string(item.Channel),
				string(item.Priority),
				truncate(item.From, 20),
				subject)
		}
		return nil
	},
}

var channelSendCmd = &cobra.Command{
	Use:   "send <adapter-name> <destination> <subject> <content>",
	Short: "Send an output item to a channel",
	Args:  cobra.ExactArgs(4),
	RunE: func(cmd *cobra.Command, args []string) error {
		if ChannelReg == nil {
			return fmt.Errorf("channel registry not initialized")
		}

		adapterName := args[0]
		adapter, err := ChannelReg.GetAdapter(adapterName)
		if err != nil {
			return err
		}

		item := models.OutputItem{
			ID:          fmt.Sprintf("out-%d", time.Now().UTC().UnixMilli()),
			Channel:     adapter.Type(),
			Destination: args[1],
			Subject:     args[2],
			Content:     args[3],
		}

		if err := adapter.Send(item); err != nil {
			return fmt.Errorf("sending to channel %s: %w", adapterName, err)
		}

		fmt.Printf("Sent item %s to channel %s.\n", item.ID, adapterName)
		return nil
	},
}

func init() {
	channelCmd.AddCommand(channelListCmd)
	channelCmd.AddCommand(channelInboxCmd)
	channelCmd.AddCommand(channelSendCmd)
	rootCmd.AddCommand(channelCmd)
}
