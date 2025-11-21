package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"gowa-yourself/internal/model"
	"gowa-yourself/internal/service"
	"gowa-yourself/internal/ws"

	"github.com/labstack/echo/v4"
)

// Simpan cancel functions untuk setiap instance
var qrCancelFuncs = make(map[string]context.CancelFunc)
var qrCancelMutex sync.RWMutex

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

	// Cek apakah sudah ada QR generation yang sedang berjalan
	qrCancelMutex.RLock()
	_, exists := qrCancelFuncs[instanceID]
	qrCancelMutex.RUnlock()

	if exists {
		return ErrorResponse(c, 409, "QR generation already in progress, please wait", "QR_IN_PROGRESS", "Please wait or cancel the current QR generation first.")
	}

	// Ambil info instance dari DB
	inst, err := model.GetInstanceByInstanceID(instanceID)
	if err != nil {
		return ErrorResponse(c, 404, "Instance not found", "INSTANCE_NOT_FOUND", err.Error())
	}

	// Kalau sudah logged_out, jangan izinkan QR lagi untuk instance ini
	if inst.Status == "logged_out" {
		return ErrorResponse(c, 400,
			"This instance is logged out and cannot be reused. Please create a new instance for this number.",
			"INSTANCE_LOGGED_OUT",
			"",
		)
	}

	session, err := service.GetSession(instanceID)
	if err != nil {
		return ErrorResponse(c, 404, "Session not found. Please create a new instance first.", "SESSION_NOT_FOUND", "")
	}

	if session.IsConnected {
		return SuccessResponse(c, 200, "Already connected", map[string]interface{}{
			"status": "already_connected",
			"jid":    session.Client.Store.ID.String(),
		})
	}

	// Buat context dengan timeout 3 menit
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)

	// Simpan cancel function
	qrCancelMutex.Lock()
	qrCancelFuncs[instanceID] = cancel
	qrCancelMutex.Unlock()

	// Jalankan QR generation di goroutine (background process)
	go func() {
		// Cleanup setelah selesai
		defer func() {
			qrCancelMutex.Lock()
			delete(qrCancelFuncs, instanceID)
			qrCancelMutex.Unlock()
			cancel()
		}()

		// Get QR channel dengan context
		qrChan, err := session.Client.GetQRChannel(ctx)
		if err != nil {
			log.Printf("Failed to get QR channel for instance %s: %v", instanceID, err)

			// Broadcast error via WebSocket
			if service.Realtime != nil {
				errorEvt := ws.WsEvent{
					Event:     ws.EventInstanceError,
					Timestamp: time.Now().UTC(),
					Data: map[string]interface{}{
						"instance_id": instanceID,
						"error":       "Failed to get QR channel: " + err.Error(),
					},
				}
				service.Realtime.Publish(errorEvt)
			}
			return
		}

		// Connect client
		err = session.Client.Connect()
		if err != nil {
			log.Printf("Failed to connect client for instance %s: %v", instanceID, err)

			if service.Realtime != nil {
				errorEvt := ws.WsEvent{
					Event:     ws.EventInstanceError,
					Timestamp: time.Now().UTC(),
					Data: map[string]interface{}{
						"instance_id": instanceID,
						"error":       "Failed to connect: " + err.Error(),
					},
				}
				service.Realtime.Publish(errorEvt)
			}
			return
		}

		// Listen to QR events
		for evt := range qrChan {
			// Cek apakah context sudah dibatalkan atau timeout
			select {
			case <-ctx.Done():
				println("\n✗ QR Generation cancelled or timeout for instance:", instanceID)

				// Broadcast cancel/timeout event
				if service.Realtime != nil {
					cancelEvt := ws.WsEvent{
						Event:     ws.EventQRTimeout,
						Timestamp: time.Now().UTC(),
						Data: map[string]interface{}{
							"instance_id": instanceID,
							"status":      "cancelled",
							"reason":      ctx.Err().Error(),
						},
					}
					service.Realtime.Publish(cancelEvt)
				}
				return

			default:
				// Lanjut handle events
			}

			if evt.Event == "code" {
				// Print QR string untuk debugging
				println("\n=== QR Code String ===")
				println(evt.Code)
				println("Instance ID:", instanceID)

				// Simpan QR ke DB custom
				expiresAt := time.Now().Add(60 * time.Second)
				err := model.UpdateInstanceQR(instanceID, evt.Code, expiresAt)
				if err != nil {
					log.Printf("Failed to update QR info in database for instance %s: %v", instanceID, err)
				}

				// Broadcast QR via WebSocket
				if service.Realtime != nil {
					data := ws.QRGeneratedData{
						InstanceID:  instanceID,
						PhoneNumber: "",
						QRData:      evt.Code,
						ExpiresAt:   expiresAt,
					}

					evtWs := ws.WsEvent{
						Event:     ws.EventQRGenerated,
						Timestamp: time.Now().UTC(),
						Data:      data,
					}
					service.Realtime.Publish(evtWs)
				}

				println("QR sent via WebSocket. Waiting for scan or next QR refresh...")

			} else if evt.Event == "success" {
				println("\n✓ QR Scanned! Pairing successful for instance:", instanceID)

				// Broadcast success via WebSocket
				if service.Realtime != nil {
					successEvt := ws.WsEvent{
						Event:     ws.EventQRSuccess,
						Timestamp: time.Now().UTC(),
						Data: map[string]interface{}{
							"instance_id": instanceID,
							"status":      "connected",
						},
					}
					service.Realtime.Publish(successEvt)
				}
				return

			} else if evt.Event == "timeout" {
				println("\n✗ QR Timeout for instance:", instanceID)

				if service.Realtime != nil {
					timeoutEvt := ws.WsEvent{
						Event:     ws.EventQRTimeout,
						Timestamp: time.Now().UTC(),
						Data: map[string]interface{}{
							"instance_id": instanceID,
							"status":      "timeout",
						},
					}
					service.Realtime.Publish(timeoutEvt)
				}
				return

			} else if strings.HasPrefix(evt.Event, "err-") {
				println("\n✗ QR Error for instance:", instanceID, "->", evt.Event)

				if service.Realtime != nil {
					errorEvt := ws.WsEvent{
						Event:     ws.EventInstanceError,
						Timestamp: time.Now().UTC(),
						Data: map[string]interface{}{
							"instance_id": instanceID,
							"error":       evt.Event,
						},
					}
					service.Realtime.Publish(errorEvt)
				}
				return
			}
		}

		// Channel closed unexpectedly
		println("\n✗ QR channel closed for instance:", instanceID)

		if service.Realtime != nil {
			errorEvt := ws.WsEvent{
				Event:     ws.EventInstanceError,
				Timestamp: time.Now().UTC(),
				Data: map[string]interface{}{
					"instance_id": instanceID,
					"error":       "QR channel closed unexpectedly",
				},
			}
			service.Realtime.Publish(errorEvt)
		}
	}()

	// Return response LANGSUNG tanpa menunggu QR generation selesai
	return SuccessResponse(c, 200, "QR generation started", map[string]interface{}{
		"status":      "generating",
		"message":     "QR codes will be sent via WebSocket. Listen to QR_GENERATED event.",
		"instance_id": instanceID,
		"timeout":     "3 minutes",
	})
}

// DELETE /qr/:instanceId - Cancel QR generation
func CancelQR(c echo.Context) error {
	instanceID := c.Param("instanceId")

	qrCancelMutex.RLock()
	cancel, exists := qrCancelFuncs[instanceID]
	qrCancelMutex.RUnlock()

	if !exists {
		return ErrorResponse(c, 404, "No active QR generation", "NO_QR_SESSION", "No QR generation in progress for this instance.")
	}

	println("\n✗ User cancelled QR generation for instance:", instanceID)
	// Cancel QR generation
	cancel()

	// Broadcast cancel event via WebSocket
	if service.Realtime != nil {
		cancelEvt := ws.WsEvent{
			Event:     ws.EventQRCancelled,
			Timestamp: time.Now().UTC(),
			Data: map[string]interface{}{
				"instance_id": instanceID,
				"status":      "cancelled",
				"message":     "User cancelled QR generation",
			},
		}
		service.Realtime.Publish(cancelEvt)
	}

	return SuccessResponse(c, 200, "QR generation cancelled successfully", map[string]interface{}{
		"instance_id": instanceID,
		"status":      "cancelled",
	})
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

// DELETE /instances/:instanceId
func DeleteInstance(c echo.Context) error {
	instanceID := c.Param("instanceId")

	err := service.DeleteInstance(instanceID)
	if err != nil {
		// Instance tidak ditemukan
		if errors.Is(err, service.ErrInstanceNotFound) {
			return ErrorResponse(c, 404,
				"Instance not found",
				"INSTANCE_NOT_FOUND",
				err.Error(),
			)
		}

		// Instance masih terkoneksi / belum logout
		if errors.Is(err, service.ErrInstanceStillConnected) {
			return ErrorResponse(c, 400,
				"Instance is still connected. Please logout first.",
				"INSTANCE_STILL_CONNECTED",
				err.Error(),
			)
		}

		// Error lain (DB / internal)
		return ErrorResponse(c, 500,
			"Failed to delete instance",
			"DELETE_INSTANCE_FAILED",
			err.Error(),
		)
	}

	return SuccessResponse(c, 200, "Instance deleted successfully", map[string]interface{}{
		"instanceId": instanceID,
	})
}
