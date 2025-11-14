package helper

import (
	"fmt"

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
