package gowa

type WebhookEvent struct {
	Event    string         `json:"event"`
	DeviceID string         `json:"device_id"`
	Payload  MessagePayload `json:"payload"`
}

type MessagePayload struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	ChatID    string `json:"chat_id"`
	IsFromMe  bool   `json:"is_from_me"`
	Type      string `json:"type"`
	Body      string `json:"body"`
	Caption   string `json:"caption"`
	MediaType string `json:"media_type"`
	IsGroup   bool   `json:"is_group"`
}

func NormalizeSenderJID(jid string) string {
	for _, suffix := range []string{"@s.whatsapp.net", "@c.us", "@lid"} {
		if len(jid) > len(suffix) && jid[len(jid)-len(suffix):] == suffix {
			return jid[:len(jid)-len(suffix)]
		}
	}
	return jid
}
