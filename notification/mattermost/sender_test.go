package mattermost

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/goalert/config"
	"github.com/target/goalert/notification"
	"github.com/target/goalert/notification/nfymsg"
	"github.com/target/goalert/util/privnet"
)

func testCtx(fn func(*config.Config)) context.Context {
	var cfg config.Config
	cfg.Mattermost.Enable = true
	cfg.General.PublicURL = "https://goalert.example.com"
	if fn != nil {
		fn(&cfg)
	}
	return cfg.Context(context.Background())
}

func TestSender_SendMessage_Payload(t *testing.T) {
	var got payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, &got))
	}))
	defer srv.Close()

	s := NewSender(context.Background(), srv.Client())
	msg := notification.Alert{
		Base:        nfymsg.Base{Dest: NewMattermostDest(srv.URL + "/hooks/abc")},
		AlertID:     42,
		Summary:     "disk full",
		Details:     "line1\nline2",
		ServiceID:   "svc-id",
		ServiceName: "Storage",
	}
	res, err := s.SendMessage(testCtx(nil), msg)
	require.NoError(t, err)
	assert.Equal(t, notification.StateSent, res.State)

	assert.Equal(t, "GoAlert", got.Username)
	assert.Contains(t, got.Text, "[Alert #42](https://goalert.example.com/alerts/42): disk full")
	assert.Contains(t, got.Text, "[Storage](https://goalert.example.com/services/svc-id)")
	assert.Contains(t, got.Text, "> line1\n> line2")
}

func TestFormatMessage(t *testing.T) {
	var cfg config.Config
	cfg.General.PublicURL = "https://goalert.example.com"

	text, err := formatMessage(cfg, notification.Verification{Code: "123456"})
	require.NoError(t, err)
	assert.Contains(t, text, "`123456`")

	text, err = formatMessage(cfg, notification.ScheduleOnCallUsers{
		ScheduleName: "Primary",
		ScheduleURL:  "https://goalert.example.com/schedules/1",
		Users: []nfymsg.User{
			{Name: "Alice", URL: "https://goalert.example.com/users/a"},
			{Name: "Bob", URL: "https://goalert.example.com/users/b"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Now on-call for [Primary](https://goalert.example.com/schedules/1): [Alice](https://goalert.example.com/users/a), [Bob](https://goalert.example.com/users/b)", text)

	text, err = formatMessage(cfg, notification.ScheduleOnCallUsers{ScheduleName: "Primary", ScheduleURL: "u"})
	require.NoError(t, err)
	assert.Contains(t, text, "No one is on-call")

	text, err = formatMessage(cfg, notification.AlertStatus{AlertID: 7, Summary: "s", LogEntry: "Acknowledged by Bob"})
	require.NoError(t, err)
	assert.Contains(t, text, "Acknowledged by Bob")

	_, err = formatMessage(cfg, notification.SignalMessage{})
	assert.Error(t, err)
}

func TestSender_SendMessage_Status(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		state   notification.State
		wantErr bool
	}{
		{"ok", http.StatusOK, notification.StateSent, false},
		{"bad request", http.StatusBadRequest, notification.StateFailedPerm, false},
		{"not found", http.StatusNotFound, notification.StateFailedPerm, false},
		{"rate limited", http.StatusTooManyRequests, 0, true},
		{"server error", http.StatusInternalServerError, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, "some error")
			}))
			defer srv.Close()

			s := NewSender(context.Background(), srv.Client())
			msg := notification.Test{Base: nfymsg.Base{Dest: NewMattermostDest(srv.URL + "/hooks/abc")}}
			res, err := s.SendMessage(testCtx(nil), msg)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.state, res.State)
			if tc.state == notification.StateFailedPerm {
				assert.Contains(t, res.StateDetails, "some error")
			}
		})
	}
}

func TestSender_SendMessage_Restrictions(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	defer srv.Close()

	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	s := NewSender(context.Background(), &http.Client{Transport: privnet.RoundTripper(base)})
	msg := notification.Test{Base: nfymsg.Base{Dest: NewMattermostDest(srv.URL + "/hooks/abc")}}

	res, err := s.SendMessage(testCtx(nil), msg)
	require.NoError(t, err)
	assert.Equal(t, notification.StateSent, res.State)
	assert.Equal(t, 1, calls)

	// blocked when enabled (test server listens on 127.0.0.1)
	res, err = s.SendMessage(testCtx(func(c *config.Config) { c.Mattermost.BlockPrivateAddresses = true }), msg)
	require.NoError(t, err)
	assert.Equal(t, notification.StateFailedPerm, res.State)
	assert.Equal(t, 1, calls, "request should not have been sent")

	// not in allowed URLs
	res, err = s.SendMessage(testCtx(func(c *config.Config) {
		c.Mattermost.AllowedURLs = []string{"https://mattermost.example.com/hooks/"}
	}), msg)
	require.NoError(t, err)
	assert.Equal(t, notification.StateFailedPerm, res.State)
	assert.Equal(t, 1, calls, "request should not have been sent")
}

func TestSender_ValidateField(t *testing.T) {
	var s Sender
	ctx := testCtx(nil)

	assert.NoError(t, s.ValidateField(ctx, FieldWebhookURL, "https://mattermost.example.com/hooks/abc123"))
	assert.Error(t, s.ValidateField(ctx, FieldWebhookURL, "https://mattermost.example.com/team/channels/town-square"))
	assert.Error(t, s.ValidateField(ctx, FieldWebhookURL, "not a url"))
	assert.Error(t, s.ValidateField(ctx, "unknown", "https://mattermost.example.com/hooks/abc123"))

	restricted := testCtx(func(c *config.Config) {
		c.Mattermost.AllowedURLs = []string{"https://chat.corp.example.com/hooks/"}
	})
	assert.NoError(t, s.ValidateField(restricted, FieldWebhookURL, "https://chat.corp.example.com/hooks/abc"))
	assert.Error(t, s.ValidateField(restricted, FieldWebhookURL, "https://mattermost.example.com/hooks/abc"))
}
