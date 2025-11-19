package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gowa-yourself/database"
	"gowa-yourself/internal/helper"
	"gowa-yourself/internal/model"

	"gowa-yourself/internal/ws"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

var (
	sessions     = make(map[string]*model.Session)
	sessionsLock sync.RWMutex

	// Track instances yang sedang logout
	loggingOut     = make(map[string]bool)
	loggingOutLock sync.RWMutex
	Realtime       ws.RealtimePublisher
)

// Event handler untuk handle connection events
func eventHandler(instanceID string) func(evt interface{}) {
	return func(evt interface{}) {
		switch evt.(type) {

		case *events.Connected:
			loggingOutLock.RLock()
			isLoggingOut := loggingOut[instanceID]
			loggingOutLock.RUnlock()
			if isLoggingOut {
				fmt.Println("⚠ Ignoring reconnect during logout:", instanceID)
				return
			}

			sessionsLock.Lock()
			session, exists := sessions[instanceID]
			if exists {
				session.IsConnected = true
				if session.Client.Store.ID != nil {
					session.JID = session.Client.Store.ID.String()
				}
				fmt.Println("✓ Connected! Instance:", instanceID, "JID:", session.JID)
			}
			sessionsLock.Unlock()

			if exists && session.Client.Store.ID != nil {
				// Ambil phoneNumber dari JID (mis. "6285148107612:38@s.whatsapp.net")
				jid := session.Client.Store.ID
				phoneNumber := jid.User // biasanya sudah format 6285xxxx

				platform := "" // kalau ada field ini; kalau tidak bisa kosong
				if err := model.UpdateInstanceOnConnected(
					instanceID,
					jid.String(),
					phoneNumber,
					platform,
				); err != nil {
					fmt.Println("Warning: failed to update instance on connected:", err)
				}

				// Setelah DB update, kirim event WS
				if Realtime != nil {
					now := time.Now().UTC()
					data := ws.InstanceStatusChangedData{
						InstanceID:     instanceID,
						PhoneNumber:    phoneNumber,
						Status:         "online",
						IsConnected:    true,
						ConnectedAt:    &now,
						DisconnectedAt: nil,
					}

					evt := ws.WsEvent{
						Event:     ws.EventInstanceStatusChanged,
						Timestamp: now,
						Data:      data,
					}

					Realtime.Publish(evt)
				}

			}

		case *events.PairSuccess:
			fmt.Println("✓ Pair Success! Instance:", instanceID)

		case *events.LoggedOut:
			sessionsLock.Lock()
			if session, exists := sessions[instanceID]; exists {
				session.IsConnected = false
				fmt.Println("✗ Logged out! Instance:", instanceID)
			}
			sessionsLock.Unlock()

			if err := model.UpdateInstanceOnLoggedOut(instanceID); err != nil {
				fmt.Println("Warning: failed to update instance on logged out:", err)
			} else {
				// Setelah DB update, kirim event WS status logged_out
				if Realtime != nil {
					now := time.Now().UTC()

					inst, err := model.GetInstanceByInstanceID(instanceID)
					if err != nil {
						fmt.Printf("Failed to get instance by instance ID %s: %v\n", instanceID, err)
					}

					data := ws.InstanceStatusChangedData{
						InstanceID:     instanceID,
						PhoneNumber:    inst.PhoneNumber.String,
						Status:         "logged_out",
						IsConnected:    false,
						ConnectedAt:    &inst.ConnectedAt.Time,
						DisconnectedAt: &now,
					}

					evt := ws.WsEvent{
						Event:     ws.EventInstanceStatusChanged,
						Timestamp: now,
						Data:      data,
					}

					Realtime.Publish(evt)
				}
			}

		case *events.StreamReplaced:
			fmt.Println("⚠ Stream replaced! Instance:", instanceID)

		case *events.Disconnected:
			loggingOutLock.RLock()
			isLoggingOut := loggingOut[instanceID]
			loggingOutLock.RUnlock()
			if !isLoggingOut {
				fmt.Println("⚠ Disconnected! Instance:", instanceID)

				sessionsLock.Lock()
				if session, exists := sessions[instanceID]; exists {
					session.IsConnected = false
				}
				sessionsLock.Unlock()

				if err := model.UpdateInstanceOnDisconnected(instanceID); err != nil {
					fmt.Println("Warning: failed to update instance on disconnected:", err)
				}
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

		// 1) Ambil instanceID dari DB custom, JANGAN generate baru dari JID
		inst, err := model.GetInstanceByJID(jid)
		if err != nil {
			fmt.Printf("Failed to get instance for jid %s: %v\n", jid, err)
			continue
		}

		instanceID := inst.InstanceID
		if instanceID == "" {
			fmt.Printf("Empty instanceID for jid %s, skipping\n", jid)
			continue
		}

		// 2) Buat client WhatsMeow dan attach event handler dengan instanceID yang benar
		client := whatsmeow.NewClient(device, nil)
		client.AddEventHandler(eventHandler(instanceID))

		if err := client.Connect(); err != nil {
			fmt.Printf("Failed to connect device %s: %v\n", jid, err)
			continue
		}

		// 3) Simpan ke sessions map dengan key instanceID yang konsisten
		sessionsLock.Lock()
		sessions[instanceID] = &model.Session{
			ID:          instanceID,
			JID:         jid,
			Client:      client,
			IsConnected: client.IsConnected(),
		}
		sessionsLock.Unlock()

		// 4) Update status di DB bahwa instance ini berhasil re-connect
		//    (kalau client.IsConnected() == true)
		if client.IsConnected() {
			phoneNumber := helper.ExtractPhoneFromJID(jid) // mis. "6285148107612"

			if err := model.UpdateInstanceOnConnected(
				instanceID,
				jid,
				phoneNumber,
				"", // platform sementara kosong
			); err != nil {
				fmt.Printf("Warning: failed to update instance on reconnect %s: %v\n", instanceID, err)
			}
		}

		fmt.Printf("✓ Loaded and connected: %s (instance: %s)\n", jid, instanceID)
	}

	return nil
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

	// Hapus dari map sessions (memory)
	delete(sessions, instanceID)
	sessionsLock.Unlock()

	// LOGOUT: Unlink device dari WhatsApp
	if session.Client != nil {
		err := session.Client.Logout(context.Background())
		if err != nil {
			fmt.Printf("Warning: Failed to logout from WhatsApp: %v\n", err)
		}
		session.Client.Disconnect()
	}

	// Update status instance di DB custom (tidak dihapus, hanya update status)
	err := model.UpdateInstanceStatus(instanceID, "logged_out", false, time.Now())
	if err != nil {
		fmt.Printf("Warning: Failed to update instance status in DB: %v\n", err)
	} else {
		if Realtime != nil {
			now := time.Now().UTC()

			inst, err := model.GetInstanceByInstanceID(instanceID)
			if err != nil {
				fmt.Printf("Failed to get instance by instance ID %s: %v\n", instanceID, err)
			}

			data := ws.InstanceStatusChangedData{
				InstanceID:     instanceID,
				PhoneNumber:    inst.PhoneNumber.String,
				Status:         "logged_out",
				IsConnected:    false,
				ConnectedAt:    &inst.ConnectedAt.Time,
				DisconnectedAt: &now,
			}

			evt := ws.WsEvent{
				Event:     ws.EventInstanceStatusChanged,
				Timestamp: now,
				Data:      data,
			}

			Realtime.Publish(evt)
		}
	}

	// Clean up flag
	loggingOutLock.Lock()
	delete(loggingOut, instanceID)
	loggingOutLock.Unlock()

	fmt.Println("✓ Device logged out, session cleared. Instance kept in DB:", instanceID)
	return nil
}
