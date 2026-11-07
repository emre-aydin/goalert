# Using Mattermost

GoAlert can send notifications to Mattermost using [incoming webhooks](https://developers.mattermost.com/integrate/webhooks/incoming/). Messages are posted as Markdown and include links back to GoAlert. Webhook calls must complete within 3 seconds.

### Creating an Incoming Webhook

1. In Mattermost, open **Product menu → Integrations → Incoming Webhooks** and select **Add Incoming Webhook**.
2. Pick the channel that should receive notifications. Optionally, enable **Lock to this channel**.
3. Copy the generated URL (it looks like `https://mattermost.example.com/hooks/xxxxxxxx`) and paste it into GoAlert as a **Mattermost** destination.

The same webhook URL can be used as an escalation policy step, a schedule on-call notification, or a user contact method. For a personal contact method, use a webhook that posts to a channel you watch (for example, a private channel with only yourself), since incoming webhooks cannot send direct messages unless the Mattermost administrator allows channel overrides.

### Messages

- **Verification**: sent when a Mattermost contact method is created, containing the verification code.
- **Alert**: the alert number, summary, service, and details.
- **Alert Status**: posted when an alert is acknowledged, escalated, or closed (if status updates are enabled).
- **On-Call Notification**: the list of users currently on-call for a schedule.

### Restricting Destinations (Administrators)

Administrators can restrict Mattermost destinations from the Admin page under **Mattermost**:

- **Allowed URLs**: if set, only webhook URLs matching one of the listed URL prefixes (e.g., `https://mattermost.example.com/hooks/`) are accepted.
- **Block Private Addresses**: if enabled, requests to private, loopback, and link-local addresses are rejected. Do not enable this if your Mattermost server is on a private network.
