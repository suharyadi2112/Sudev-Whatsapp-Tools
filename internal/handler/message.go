package handler

import (
	"context"

	"gowa-yourself/internal/helper"
	"gowa-yourself/internal/service"

	"github.com/labstack/echo/v4"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

// Request body untuk send message
type SendMessageRequest struct {
	To      string `json:"to" validate:"required"`
	Message string `json:"message" validate:"required"`
}

type CheckNumberRequest struct {
	Phone string `json:"phone" validate:"required"`
}

// POST /send/:instanceId
func SendMessage(c echo.Context) error {
	instanceID := c.Param("instanceId")

	var req SendMessageRequest
	if err := c.Bind(&req); err != nil {
		return ErrorResponse(c, 400, "Invalid request body", "INVALID_REQUEST", err.Error())
	}

	if req.To == "" || req.Message == "" {
		return ErrorResponse(c, 400, "Field 'to' and 'message' are required", "VALIDATION_ERROR", "")
	}

	session, err := service.GetSession(instanceID)
	if err != nil {
		return ErrorResponse(c, 404, "Session not found", "SESSION_NOT_FOUND", "Please login first")
	}

	if !session.IsConnected {
		return ErrorResponse(c, 400, "Session is not connected", "NOT_CONNECTED", "Please check /status endpoint")
	}

	if !session.Client.IsConnected() {
		return ErrorResponse(c, 400, "WhatsApp connection lost", "CONNECTION_LOST", "Please reconnect")
	}

	if session.Client.Store.ID == nil {
		return ErrorResponse(c, 400, "Not logged in", "NOT_LOGGED_IN", "Please scan QR code first")
	}

	recipient, err := helper.FormatPhoneNumber(req.To)
	if err != nil {
		return ErrorResponse(c, 400, "Invalid phone number", "INVALID_PHONE", err.Error())
	}

	isRegistered, err := session.Client.IsOnWhatsApp(context.Background(), []string{recipient.User})
	if err != nil {
		return ErrorResponse(c, 500, "Failed to verify phone number", "VERIFICATION_FAILED", err.Error())
	}

	if len(isRegistered) == 0 || !isRegistered[0].IsIn {
		return ErrorResponse(c, 400, "Phone number is not registered on WhatsApp", "PHONE_NOT_REGISTERED",
			"Please check the number or ask recipient to install WhatsApp")
	}

	msg := &waE2E.Message{
		Conversation: &req.Message,
	}

	resp, err := session.Client.SendMessage(context.Background(), recipient, msg)
	if err != nil {
		return ErrorResponse(c, 500, "Failed to send message", "SEND_FAILED", err.Error())
	}

	return SuccessResponse(c, 200, "Message sent successfully", map[string]interface{}{
		"messageId": resp.ID,
		"timestamp": resp.Timestamp.Unix(),
		"to":        req.To,
		"verified":  true,
	})
}

// POST /check/:instanceId
func CheckNumber(c echo.Context) error {
	instanceID := c.Param("instanceId")

	var req CheckNumberRequest
	if err := c.Bind(&req); err != nil {
		return ErrorResponse(c, 400, "Invalid request body", "INVALID_REQUEST", err.Error())
	}

	session, err := service.GetSession(instanceID)
	if err != nil {
		return ErrorResponse(c, 404, "Session not found", "SESSION_NOT_FOUND", "")
	}

	if !session.Client.IsConnected() {
		return ErrorResponse(c, 400, "Session is not connected", "NOT_CONNECTED", "")
	}

	recipient, err := helper.FormatPhoneNumber(req.Phone)
	if err != nil {
		return ErrorResponse(c, 400, "Invalid phone number", "INVALID_PHONE", err.Error())
	}

	isRegistered, err := session.Client.IsOnWhatsApp(context.Background(), []string{recipient.User})
	if err != nil {
		return ErrorResponse(c, 500, "Failed to check phone number", "CHECK_FAILED", err.Error())
	}

	if len(isRegistered) == 0 {
		return ErrorResponse(c, 400, "Unable to verify number", "VERIFICATION_ERROR", "")
	}

	return SuccessResponse(c, 200, "Phone number checked", map[string]interface{}{
		"phone":        req.Phone,
		"isRegistered": isRegistered[0].IsIn,
		"jid":          isRegistered[0].JID.String(),
	})
}
