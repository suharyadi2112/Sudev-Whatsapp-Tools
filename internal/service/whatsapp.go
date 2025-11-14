package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"gowa-yourself/database"
	"gowa-yourself/internal/model"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

var (
	sessions     = make(map[string]*model.Session)
	sessionsLock sync.RWMutex

	// Track instances yang sedang logout
	loggingOut     = make(map[string]bool)
	loggingOutLock sync.RWMutex
)

// Event handler untuk handle connection events
func eventHandler(instanceID string) func(evt interface{}) {
	return func(evt interface{}) {
		switch evt.(type) {
		case *events.Connected:
			// Cek apakah sedang proses logout
			loggingOutLock.RLock()
			isLoggingOut := loggingOut[instanceID]
			loggingOutLock.RUnlock()

			if isLoggingOut {
				fmt.Println("⚠ Ignoring reconnect during logout:", instanceID)
				return
			}

			// Update session status ketika connected
			sessionsLock.Lock()
			if session, exists := sessions[instanceID]; exists {
				session.IsConnected = true
				if session.Client.Store.ID != nil {
					session.JID = session.Client.Store.ID.String()
				}
				fmt.Println("✓ Connected! Instance:", instanceID, "JID:", session.JID)
			}
			sessionsLock.Unlock()

		case *events.PairSuccess:
			fmt.Println("✓ Pair Success! Instance:", instanceID)

		case *events.LoggedOut:
			// Handle logout
			sessionsLock.Lock()
			if session, exists := sessions[instanceID]; exists {
				session.IsConnected = false
				fmt.Println("✗ Logged out! Instance:", instanceID)
			}
			sessionsLock.Unlock()

		case *events.StreamReplaced:
			fmt.Println("⚠ Stream replaced! Instance:", instanceID)

		case *events.Disconnected:
			// Cek apakah sedang logout
			loggingOutLock.RLock()
			isLoggingOut := loggingOut[instanceID]
			loggingOutLock.RUnlock()

			if !isLoggingOut {
				fmt.Println("⚠ Disconnected! Instance:", instanceID)

				// Mark session as disconnected
				sessionsLock.Lock()
				if session, exists := sessions[instanceID]; exists {
					session.IsConnected = false
				}
				sessionsLock.Unlock()
			}
		}
	}
}

// Load all devices from database and reconnect
func LoadAllDevices() error {
	devices, err := database.Container.GetAllDevices(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get devices: %w", err)
	}

	fmt.Printf("Found %d saved devices in database\n", len(devices))

	for _, device := range devices {
		if device.ID == nil {
			continue
		}

		jid := device.ID.String()
		instanceID := generateInstanceIDFromJID(jid)

		client := whatsmeow.NewClient(device, nil)
		client.AddEventHandler(eventHandler(instanceID))

		err := client.Connect()
		if err != nil {
			fmt.Printf("Failed to connect device %s: %v\n", jid, err)
			continue
		}

		sessionsLock.Lock()
		sessions[instanceID] = &model.Session{
			ID:          instanceID,
			JID:         jid,
			Client:      client,
			IsConnected: client.IsConnected(),
		}
		sessionsLock.Unlock()

		fmt.Printf("✓ Loaded and connected: %s (instance: %s)\n", jid, instanceID)
	}

	return nil
}

// Helper function untuk generate instance ID dari JID
func generateInstanceIDFromJID(jid string) string {
	// Ambil phone number dari JID (sebelum ':')
	// Contoh: 6285148104468:6@s.whatsapp.net -> 6285148104468_6
	parts := strings.Split(jid, "@")
	if len(parts) > 0 {
		phoneAndDevice := strings.ReplaceAll(parts[0], ":", "_")
		return "instance_" + phoneAndDevice
	}
	return jid
}

// Generate random instance ID untuk login baru
func generateInstanceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func CreateSession(instanceID string) (*model.Session, error) {
	sessionsLock.Lock()
	defer sessionsLock.Unlock()

	// Cek apakah session sudah ada
	if _, exists := sessions[instanceID]; exists {
		return nil, fmt.Errorf("session already exists")
	}

	// Buat device baru
	deviceStore := database.Container.NewDevice()

	// Create whatsmeow client
	client := whatsmeow.NewClient(deviceStore, nil)

	// Add event handler
	client.AddEventHandler(eventHandler(instanceID))

	// Simpan session
	session := &model.Session{
		ID:          instanceID,
		Client:      client,
		IsConnected: false,
	}

	sessions[instanceID] = session
	return session, nil
}

func GetSession(instanceID string) (*model.Session, error) {
	sessionsLock.RLock()
	defer sessionsLock.RUnlock()

	session, exists := sessions[instanceID]
	if !exists {
		return nil, fmt.Errorf("session not found")
	}

	return session, nil
}

// ambil semua session dalam instance
func GetAllSessions() map[string]*model.Session {
	sessionsLock.RLock()
	defer sessionsLock.RUnlock()

	result := make(map[string]*model.Session)
	for k, v := range sessions {
		result[k] = v
	}

	return result
}

func DeleteSession(instanceID string) error {
	// Mark sebagai sedang logout untuk prevent auto-reconnect
	loggingOutLock.Lock()
	loggingOut[instanceID] = true
	loggingOutLock.Unlock()

	// Ambil session
	sessionsLock.Lock()
	session, exists := sessions[instanceID]
	if !exists {
		sessionsLock.Unlock()

		// Clean up flag
		loggingOutLock.Lock()
		delete(loggingOut, instanceID)
		loggingOutLock.Unlock()

		return fmt.Errorf("session not found")
	}

	// Hapus dari map SEBELUM logout
	delete(sessions, instanceID)
	sessionsLock.Unlock()

	// PERMANENT LOGOUT: Unlink device + delete from database
	if session.Client != nil {
		err := session.Client.Logout(context.Background())
		if err != nil {
			fmt.Printf("Warning: Failed to logout from WhatsApp: %v\n", err)
		}
		session.Client.Disconnect()
	}

	// Clean up flag
	loggingOutLock.Lock()
	delete(loggingOut, instanceID)
	loggingOutLock.Unlock()

	fmt.Println("✓ Device unlinked and deleted from database:", instanceID)
	return nil
}
