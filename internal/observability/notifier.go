package observability

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Notifier sends alert notifications to external channels.
type Notifier interface {
	Notify(alerts []Alert) error
}

// slackNotifier sends alert notifications to a Slack webhook.
type slackNotifier struct {
	webhookURL string
	client     *http.Client
}

// NewSlackNotifier creates a Notifier that sends alerts to the given Slack webhook URL.
func NewSlackNotifier(webhookURL string) Notifier {
	return &slackNotifier{
		webhookURL: webhookURL,
		client:     &http.Client{},
	}
}

type slackMessage struct {
	Blocks []slackBlock `json:"blocks"`
}

type slackBlock struct {
	Type string     `json:"type"`
	Text *slackText `json:"text,omitempty"`
}

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Notify sends the given alerts to the configured Slack webhook.
// It returns nil without making a request if the alerts slice is empty.
func (s *slackNotifier) Notify(alerts []Alert) error {
	if len(alerts) == 0 {
		return nil
	}

	msg := s.buildMessage(alerts)

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling slack message: %w", err)
	}

	resp, err := s.client.Post(s.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("posting to slack webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}

	return nil
}

func (s *slackNotifier) buildMessage(alerts []Alert) slackMessage {
	blocks := []slackBlock{
		{
			Type: "header",
			Text: &slackText{Type: "plain_text", Text: "adb Alert Summary"},
		},
	}

	for i, alert := range alerts {
		if i > 0 {
			blocks = append(blocks, slackBlock{Type: "divider"})
		}
		emoji := severityEmoji(alert.Severity)
		text := fmt.Sprintf("%s *[%s]* %s\n_%s_",
			emoji,
			strings.ToUpper(string(alert.Severity)),
			alert.Message,
			alert.TriggeredAt.Format("2006-01-02 15:04 UTC"),
		)
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackText{Type: "mrkdwn", Text: text},
		})
	}

	return slackMessage{Blocks: blocks}
}

func severityEmoji(severity AlertSeverity) string {
	switch severity {
	case SeverityHigh:
		return "\U0001f534"
	case SeverityMedium:
		return "\U0001f7e1"
	case SeverityLow:
		return "\U0001f535"
	default:
		return "\u2753"
	}
}
