package ws

import "time"

// Nama event (konstanta) supaya konsisten antara BE dan FE.
const (
	EventQRGenerated           = "QR_GENERATED"
	EventQRExpired             = "QR_EXPIRED"
	EventInstanceStatusChanged = "INSTANCE_STATUS_CHANGED"
	EventInstanceError         = "INSTANCE_ERROR"

	EventQRSuccess   = "QR_SUCCESS" // Pairing berhasil
	EventQRTimeout   = "QR_TIMEOUT"
	EventQRCancelled = "QR_CANCELLED" // Tambahkan ini
	// Kalau nanti mau dipakai:
	// EventQRScanned = "QR_SCANNED"
)

// WsEvent adalah envelope umum setiap pesan yang dikirim via WebSocket.
// FE cukup switch berdasarkan field Event, lalu cast Data ke bentuk yang sesuai.
type WsEvent struct {
	Event     string      `json:"event"`     // Nama event, salah satu dari konstanta di atas
	Timestamp time.Time   `json:"timestamp"` // Waktu event dibuat (UTC)
	Data      interface{} `json:"data"`      // Payload spesifik event
}

// =====================
// Payload per jenis event
// =====================

// QRGeneratedData dikirim ketika QR baru berhasil digenerate
// dan siap discan untuk sebuah instance.
type QRGeneratedData struct {
	InstanceID  string    `json:"instance_id"`
	PhoneNumber string    `json:"phone_number,omitempty"` // boleh kosong kalau belum tahu
	QRData      string    `json:"qr_data"`                // raw string QR (atau URL/base64 sesuai kebutuhan FE)
	ExpiresAt   time.Time `json:"expires_at"`             // waktu kadaluarsa QR
}

// QRExpiredData dikirim ketika QR untuk instance tertentu dianggap kadaluarsa.
type QRExpiredData struct {
	InstanceID  string `json:"instance_id"`
	PhoneNumber string `json:"phone_number,omitempty"`
}

// InstanceStatusChangedData dikirim ketika status koneksi instance berubah,
// misalnya akibat events.Connected, events.Disconnected, events.LoggedOut.
type InstanceStatusChangedData struct {
	InstanceID     string     `json:"instance_id"`
	PhoneNumber    string     `json:"phone_number,omitempty"`
	Status         string     `json:"status"`                    // "online", "disconnected", "logged_out", dll
	IsConnected    bool       `json:"is_connected"`              // true kalau koneksi aktif
	ConnectedAt    *time.Time `json:"connected_at,omitempty"`    // bisa nil jika belum pernah connect
	DisconnectedAt *time.Time `json:"disconnected_at,omitempty"` // bisa nil jika belum pernah disconnect
}

// InstanceErrorData opsional, untuk mengirim error penting terkait instance,
// misalnya login gagal, forced logout karena unofficial app, dll.
type InstanceErrorData struct {
	InstanceID  string `json:"instance_id"`
	PhoneNumber string `json:"phone_number,omitempty"`
	Code        string `json:"code"`    // contoh: "LOGIN_FAILED", "UNOFFICIAL_APP", "QR_CHANNEL_FAILED"
	Message     string `json:"message"` // human readable message
}
