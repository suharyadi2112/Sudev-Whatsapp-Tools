package helper

import (
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// FormatPhoneNumber converts phone number to WhatsApp JID format
// Supports formats: 0812xxx, 62812xxx, +62812xxx
func FormatPhoneNumber(phone string) (types.JID, error) {
	// Hapus karakter non-digit
	cleaned := ""
	for _, char := range phone {
		if char >= '0' && char <= '9' {
			cleaned += string(char)
		}
	}

	// Tambahkan 62 jika diawali 0
	if len(cleaned) > 0 && cleaned[0] == '0' {
		cleaned = "62" + cleaned[1:]
	}

	// Validate minimal length
	if len(cleaned) < 10 {
		return types.JID{}, fmt.Errorf("invalid phone number: %s", phone)
	}

	// Format ke JID WhatsApp
	jid := types.JID{
		User:   cleaned,
		Server: types.DefaultUserServer, // s.whatsapp.net
	}

	return jid, nil
}

func ExtractPhoneFromJID(jid string) string {
	// "6285148107612:43@s.whatsapp.net" -> "6285148107612"
	atSplit := strings.SplitN(jid, "@", 2)
	if len(atSplit) == 0 {
		return jid
	}
	beforeAt := atSplit[0]
	colonSplit := strings.SplitN(beforeAt, ":", 2)
	return colonSplit[0]
}
