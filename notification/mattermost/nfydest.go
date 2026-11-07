package mattermost

import (
	"context"
	"net/url"
	"strings"

	"github.com/target/goalert/config"
	"github.com/target/goalert/gadb"
	"github.com/target/goalert/notification/nfydest"
	"github.com/target/goalert/validation"
	"github.com/target/goalert/validation/validate"
)

const (
	DestTypeMattermost = "builtin-mattermost"
	FieldWebhookURL    = "mattermost_webhook_url"
	IconURL            = "builtin://mattermost"
)

// NewMattermostDest returns a destination for the given Mattermost incoming webhook URL.
func NewMattermostDest(webhookURL string) gadb.DestV1 {
	return gadb.NewDestV1(DestTypeMattermost, FieldWebhookURL, webhookURL)
}

var _ nfydest.Provider = (*Sender)(nil)

func (Sender) ID() string { return DestTypeMattermost }

func (Sender) TypeInfo(ctx context.Context) (*nfydest.TypeInfo, error) {
	cfg := config.FromContext(ctx)
	return &nfydest.TypeInfo{
		Type:                       DestTypeMattermost,
		Name:                       "Mattermost",
		IconURL:                    IconURL,
		IconAltText:                "Mattermost",
		Enabled:                    cfg.Mattermost.Enable,
		SupportsUserVerification:   true,
		SupportsOnCallNotify:       true,
		SupportsStatusUpdates:      true,
		SupportsAlertNotifications: true,
		RequiredFields: []nfydest.FieldConfig{{
			FieldID:            FieldWebhookURL,
			Label:              "Mattermost Webhook URL",
			PlaceholderText:    "https://mattermost.example.com/hooks/xxxxxxxx",
			InputType:          "url",
			Hint:               "Mattermost Documentation",
			HintURL:            "/docs#mattermost",
			SupportsValidation: true,
		}},
	}, nil
}

func (s *Sender) ValidateField(ctx context.Context, fieldID, value string) error {
	cfg := config.FromContext(ctx)
	switch fieldID {
	case FieldWebhookURL:
		err := validate.AbsoluteURL(FieldWebhookURL, value)
		if err != nil {
			return err
		}
		u, err := url.Parse(value)
		if err != nil {
			return validation.WrapError(err)
		}
		if !strings.Contains(u.Path, "/hooks/") {
			return validation.NewGenericError("must be a Mattermost incoming webhook URL (containing /hooks/)")
		}
		if !cfg.ValidMattermostURL(value) {
			return validation.NewGenericError("url is not allowed by administrator")
		}

		return nil
	}

	return validation.NewGenericError("unknown field ID")
}

func (s *Sender) DisplayInfo(ctx context.Context, args map[string]string) (*nfydest.DisplayInfo, error) {
	if args == nil {
		args = make(map[string]string)
	}

	u, err := url.Parse(args[FieldWebhookURL])
	if err != nil {
		return nil, validation.WrapError(err)
	}
	return &nfydest.DisplayInfo{
		IconURL:     IconURL,
		IconAltText: "Mattermost",
		Text:        u.Hostname(),
	}, nil
}
