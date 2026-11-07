package mattermost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/target/goalert/config"
	"github.com/target/goalert/notification"
	"github.com/target/goalert/notification/nfydest"
	"github.com/target/goalert/util/privnet"
)

// maxDetailsLen limits alert details included in a post; Mattermost
// rejects very large posts and long details are hard to read in chat.
const maxDetailsLen = 3000

// Sender posts notifications to Mattermost incoming webhooks.
type Sender struct {
	Client *http.Client
}

// payload is the body of a Mattermost incoming webhook request.
type payload struct {
	Text     string `json:"text"`
	Username string `json:"username,omitempty"`
}

// NewSender returns a new Mattermost Sender using the provided HTTP client.
func NewSender(ctx context.Context, client *http.Client) *Sender {
	return &Sender{Client: client}
}

var _ nfydest.MessageSender = &Sender{}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func quote(s string) string {
	return "> " + strings.ReplaceAll(s, "\n", "\n> ")
}

// formatMessage renders msg as Mattermost-flavored Markdown.
func formatMessage(cfg config.Config, msg notification.Message) (string, error) {
	switch m := msg.(type) {
	case notification.Test:
		return fmt.Sprintf("This is a test message from %s.", cfg.ApplicationName()), nil
	case notification.Verification:
		return fmt.Sprintf("Your %s verification code is `%s`.", cfg.ApplicationName(), m.Code), nil
	case notification.Alert:
		var b strings.Builder
		fmt.Fprintf(&b, "#### :rotating_light: [Alert #%d](%s): %s\n", m.AlertID, cfg.CallbackURL(fmt.Sprintf("/alerts/%d", m.AlertID)), m.Summary)
		fmt.Fprintf(&b, "**Service:** [%s](%s)", m.ServiceName, cfg.CallbackURL("/services/"+m.ServiceID))
		if d := strings.TrimSpace(m.Details); d != "" {
			b.WriteString("\n\n")
			b.WriteString(quote(truncate(d, maxDetailsLen)))
		}
		return b.String(), nil
	case notification.AlertBundle:
		return fmt.Sprintf(":rotating_light: %d unacknowledged alerts for [%s](%s)",
			m.Count, m.ServiceName, cfg.CallbackURL(fmt.Sprintf("/services/%s/alerts", m.ServiceID))), nil
	case notification.AlertStatus:
		return fmt.Sprintf("[Alert #%d](%s): %s\n%s",
			m.AlertID, cfg.CallbackURL(fmt.Sprintf("/alerts/%d", m.AlertID)), m.Summary, m.LogEntry), nil
	case notification.ScheduleOnCallUsers:
		if len(m.Users) == 0 {
			return fmt.Sprintf("No one is on-call for [%s](%s).", m.ScheduleName, m.ScheduleURL), nil
		}
		users := make([]string, len(m.Users))
		for i, u := range m.Users {
			users[i] = fmt.Sprintf("[%s](%s)", u.Name, u.URL)
		}
		return fmt.Sprintf("Now on-call for [%s](%s): %s", m.ScheduleName, m.ScheduleURL, strings.Join(users, ", ")), nil
	}

	return "", fmt.Errorf("message type '%T' not supported", msg)
}

// SendMessage posts the message to the destination's Mattermost incoming webhook.
func (s *Sender) SendMessage(ctx context.Context, msg notification.Message) (*notification.SentMessage, error) {
	cfg := config.FromContext(ctx)

	text, err := formatMessage(cfg, msg)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(payload{Text: text, Username: cfg.ApplicationName()})
	if err != nil {
		return nil, err
	}

	hookURL := msg.DestArg(FieldWebhookURL)
	if !cfg.ValidMattermostURL(hookURL) {
		// fail permanently if the URL is not currently valid/allowed
		return &notification.SentMessage{
			State:        notification.StateFailedPerm,
			StateDetails: "invalid or not allowed URL",
		}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if cfg.Mattermost.BlockPrivateAddresses {
		ctx = privnet.WithBlockPrivate(ctx)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hookURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.Client.Do(req)
	if errors.Is(err, privnet.ErrPrivateAddress) {
		// fail permanently; the destination is blocked by the administrator
		return &notification.SentMessage{
			State:        notification.StateFailedPerm,
			StateDetails: "destination address is not allowed by administrator",
		}, nil
	}
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return &notification.SentMessage{State: notification.StateSent}, nil
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		// temporary failure, retry
		return nil, fmt.Errorf("mattermost webhook: unexpected status %s", resp.Status)
	}

	// Other 4xx (e.g., invalid or disabled webhook) will not succeed on retry.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	details := resp.Status
	if b := strings.TrimSpace(string(body)); b != "" {
		details += ": " + truncate(b, 256)
	}
	return &notification.SentMessage{
		State:        notification.StateFailedPerm,
		StateDetails: details,
	}, nil
}
