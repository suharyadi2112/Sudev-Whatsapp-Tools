package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"gowa-yourself/internal/model"
	"gowa-yourself/internal/service"

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

	session, err := service.GetSession(instanceID)
	if err != nil {
		return ErrorResponse(c, 404, "Session not found", "SESSION_NOT_FOUND", "")
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

// GET /instances atau GET /instances?all=true
func GetAllInstances(c echo.Context) error {
	sessions := service.GetAllSessions()
	showAll := c.QueryParam("all") == "true"

	instances := make([]map[string]interface{}, 0)

	for id, session := range sessions {
		// Skip jika tidak connected dan tidak request all
		if !showAll && !session.IsConnected {
			continue
		}

		instance := map[string]interface{}{
			"instanceId":  id,
			"isConnected": session.IsConnected,
			"jid":         session.JID,
		}
		instances = append(instances, instance)
	}

	return SuccessResponse(c, 200, "Instances retrieved", map[string]interface{}{
		"total":     len(instances),
		"instances": instances,
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
