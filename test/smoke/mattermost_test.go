package smoke

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/target/goalert/test/smoke/harness"
)

// TestMattermostAlert checks that an alert is posted to a Mattermost incoming webhook contact method.
func TestMattermostAlert(t *testing.T) {
	t.Parallel()

	type mmPayload struct {
		Text     string `json:"text"`
		Username string `json:"username"`
	}
	ch := make(chan mmPayload, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.Equal(t, "/hooks/smoketest", r.URL.Path) {
			return
		}

		var p mmPayload
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&p)) {
			return
		}
		ch <- p
	}))
	defer ts.Close()

	sql := `
	insert into users (id, name, email)
	values
		({{uuid "user"}}, 'bob', 'joe');
	insert into user_contact_methods (id, user_id, name, dest)
	values
		({{uuid "cm1"}}, {{uuid "user"}}, 'personal', jsonb_build_object('Type', 'builtin-mattermost', 'Args', jsonb_build_object('mattermost_webhook_url', '` + ts.URL + `/hooks/smoketest')));

	insert into user_notification_rules (user_id, contact_method_id, delay_minutes)
	values
		({{uuid "user"}}, {{uuid "cm1"}}, 0);

	insert into escalation_policies (id, name)
	values
		({{uuid "eid"}}, 'esc policy');
	insert into escalation_policy_steps (id, escalation_policy_id)
	values
		({{uuid "esid"}}, {{uuid "eid"}});
	insert into escalation_policy_actions (escalation_policy_step_id, user_id)
	values
		({{uuid "esid"}}, {{uuid "user"}});

	insert into services (id, escalation_policy_id, name, description)
	values
		({{uuid "sid"}}, {{uuid "eid"}}, 'service', 'testing');

	insert into alerts (service_id, summary, details, status, dedup_key)
	values
		({{uuid "sid"}}, 'testing summary', 'testing details', 'triggered', 'auto:1:foo');
`

	h := harness.NewHarness(t, sql, "")
	defer h.Close()

	p := <-ch
	assert.Equal(t, "GoAlert", p.Username)
	assert.Contains(t, p.Text, "testing summary")
	assert.Contains(t, p.Text, "testing details")
	assert.Contains(t, p.Text, "service")
}
