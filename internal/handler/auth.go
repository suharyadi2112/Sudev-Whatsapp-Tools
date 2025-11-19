package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"time"

	"gowa-yourself/internal/model"
	"gowa-yourself/internal/service"
	"gowa-yourself/internal/ws"

	"github.com/labstack/echo/v4"
)

// Generate random instance ID
func generateInstanceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// POST /login
func Login(c echo.Context) error {
	instanceID := generateInstanceID()

	session, err := service.CreateSession(instanceID)
	if err != nil {
		return ErrorResponse(c, 400, "Failed to create session", "CREATE_SESSION_FAILED", err.Error())
	}

	// Cek apakah sudah login sebelumnya
	if session.Client.Store.ID != nil {
		err = session.Client.Connect()
		if err != nil {
			return ErrorResponse(c, 500, "Failed to connect", "CONNECT_FAILED", err.Error())
		}

		session.IsConnected = true
		return SuccessResponse(c, 200, "Session reconnected successfully", map[string]interface{}{
			"instanceId": instanceID,
			"status":     "connected",
			"jid":        session.Client.Store.ID.String(),
		})
	}

	// Insert ke custom DB sudevwa
	instance := &model.Instance{
		InstanceID:  instanceID,
		Status:      "qr_required",
		IsConnected: false,
		CreatedAt:   time.Now(),
	}
	err = model.InsertInstance(instance)
	if err != nil {
		return ErrorResponse(c, 500, "Failed to insert instance", "DB_INSERT_FAILED", err.Error())
	}

	return SuccessResponse(c, 200, "Instance created, QR code required", map[string]interface{}{
		"instanceId": instanceID,
		"status":     "qr_required",
		"nextStep":   "Call GET /qr/:instanceId to get QR code",
	})
}

// GET /qr/:instanceId
func GetQR(c echo.Context) error {
	instanceID := c.Param("instanceId")

	//Ambil info instance dari DB
	inst, err := model.GetInstanceByInstanceID(instanceID)
	if err != nil {
		return ErrorResponse(c, 404, "Instance not found", "INSTANCE_NOT_FOUND", err.Error())
	}
	//Kalau sudah logged_out, jangan izinkan QR lagi untuk instance ini
	if inst.Status == "logged_out" {
		return ErrorResponse(c, 400,
			"This instance is logged out and cannot be reused. Please create a new instance for this number.",
			"INSTANCE_LOGGED_OUT",
			"",
		)
	}

	session, err := service.GetSession(instanceID)
	if err != nil {
		// Session tidak ada, berikan instruksi buat session baru dulu
		return ErrorResponse(c, 404, "Session not found. Please create a new instance first.", "SESSION_NOT_FOUND", "")
	}

	if session.IsConnected {
		return SuccessResponse(c, 200, "Already connected", map[string]interface{}{
			"status": "already_connected",
			"jid":    session.Client.Store.ID.String(),
		})
	}

	// Get QR channel
	qrChan, err := session.Client.GetQRChannel(context.Background())
	if err != nil {
		return ErrorResponse(c, 500, "Failed to get QR channel", "QR_CHANNEL_FAILED", err.Error())
	}

	// Connect client
	err = session.Client.Connect()
	if err != nil {
		return ErrorResponse(c, 500, "Failed to connect", "CONNECT_FAILED", err.Error())
	}

	// Listen to QR events
	for evt := range qrChan {
		if evt.Event == "code" {
			// Print QR string untuk debugging
			println("\n=== QR Code String ===")
			println(evt.Code)
			println("\nGenerate QR at: https://www.qr-code-generator.com/")

			// Simpan QR ke DB custom (misal update field qr_code, qr_expires_at, status)
			expiresAt := time.Now().Add(60 * time.Second)
			err := model.UpdateInstanceQR(instanceID, evt.Code, expiresAt)
			if err != nil {
				log.Printf("Failed to update QR info in database for instance %s: %v", instanceID, err)
			} else if service.Realtime != nil {

				now := time.Now().UTC()
				data := ws.QRGeneratedData{
					InstanceID:  instanceID,
					PhoneNumber: "", // kalau ada di struct inst
					QRData:      evt.Code,
					ExpiresAt:   expiresAt,
				}

				evtWs := ws.WsEvent{
					Event:     ws.EventQRGenerated,
					Timestamp: now,
					Data:      data,
				}

				service.Realtime.Publish(evtWs)
			}

			return SuccessResponse(c, 200, "QR code generated", map[string]interface{}{
				"qr":      evt.Code,
				"status":  "qr_ready",
				"message": "Scan with WhatsApp. Status will auto-update.",
			})
		} else if evt.Event == "success" {
			println("\n✓ QR Scanned! Waiting for connection...")

			return SuccessResponse(c, 200, "QR code scanned", map[string]interface{}{
				"status":  "pairing",
				"message": "QR scanned! Check /status endpoint for connection status",
			})
		}
	}

	// Kalau keluar dari loop, QR dianggap expired
	if service.Realtime != nil {
		now := time.Now().UTC()
		data := ws.QRExpiredData{
			InstanceID:  instanceID,
			PhoneNumber: "", // kalau ada
		}
		evtWs := ws.WsEvent{
			Event:     ws.EventQRExpired,
			Timestamp: now,
			Data:      data,
		}
		service.Realtime.Publish(evtWs)
	}
	return ErrorResponse(c, 400, "QR code expired", "QR_EXPIRED", "Please try again")
}

// GET /status/:instanceId
func GetStatus(c echo.Context) error {
	instanceID := c.Param("instanceId")

	session, err := service.GetSession(instanceID)
	if err != nil {
		return ErrorResponse(c, 404, "Session not found", "SESSION_NOT_FOUND", "")
	}

	return SuccessResponse(c, 200, "Status retrieved", map[string]interface{}{
		"instanceId":  instanceID,
		"isConnected": session.IsConnected,
		"jid":         session.JID,
	})
}

// GET /instances?all=true
func GetAllInstances(c echo.Context) error {
	showAll := c.QueryParam("all") == "true"

	// Ambil semua instance dari table custom
	dbInstances, err := model.GetAllInstances()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to get instances from DB",
			"error":   err.Error(),
		})
	}

	// Ambil semua session memory (active sessions)
	sessions := service.GetAllSessions()

	var instances []model.InstanceResp

	for _, inst := range dbInstances {
		// Convert dari model.Instance ke model.InstanceResp (string primitif)
		resp := model.ToResponse(inst)

		// Cek apakah ada session aktif untuk instance ini
		session, found := sessions[inst.InstanceID]

		if found {
			resp.IsConnected = session.IsConnected
			resp.JID = session.JID

			if resp.IsConnected {
				resp.Status = "online"
			}
		}
		// Tambahkan info apakah session ada di Whatsmeow memory
		resp.ExistsInWhatsmeow = found

		// Kalau tidak show all, dan instance offline, skip
		if !showAll && !resp.IsConnected {
			continue
		}

		instances = append(instances, resp)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Instances retrieved",
		"data": map[string]interface{}{
			"total":     len(instances),
			"instances": instances,
		},
	})
}

// POST /logout/:instanceId
func Logout(c echo.Context) error {
	instanceID := c.Param("instanceId")

	err := service.DeleteSession(instanceID)
	if err != nil {
		return ErrorResponse(c, 404, "Session not found", "SESSION_NOT_FOUND", err.Error())
	}

	return SuccessResponse(c, 200, "Logged out successfully", map[string]interface{}{
		"instanceId": instanceID,
	})
}
