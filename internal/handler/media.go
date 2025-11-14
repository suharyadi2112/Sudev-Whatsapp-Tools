package handler

import (
	"context"
	"fmt"
	"io"

	"gowa-yourself/internal/helper"
	"gowa-yourself/internal/service"

	"github.com/labstack/echo/v4"
	"go.mau.fi/whatsmeow"
)

// Request body untuk send media from URL
type SendMediaRequest struct {
	To        string `json:"to" validate:"required"`
	MediaURL  string `json:"mediaUrl" validate:"required"`
	Caption   string `json:"caption"`
	MediaType string `json:"mediaType"` // image, video, document, audio
}

// POST /send/:instanceId/media (upload file)
func SendMediaFile(c echo.Context) error {
	instanceID := c.Param("instanceId")

	to := c.FormValue("to")
	caption := c.FormValue("caption")

	if to == "" {
		return ErrorResponse(c, 400, "Field 'to' is required", "VALIDATION_ERROR", "")
	}

	// 1. CEK SESSION EXISTS
	session, err := service.GetSession(instanceID)
	if err != nil {
		return ErrorResponse(c, 404, "Session not found", "SESSION_NOT_FOUND", "Please login first")
	}

	// 2. CEK CONNECTION FLAG (dari memory/database)
	if !session.IsConnected {
		return ErrorResponse(c, 400, "Session is not connected", "NOT_CONNECTED", "Please check /status endpoint")
	}

	// 3. CEK REAL WHATSAPP CONNECTION (websocket)
	if !session.Client.IsConnected() {
		return ErrorResponse(c, 400, "WhatsApp connection lost", "CONNECTION_LOST", "Please reconnect")
	}

	// 4. CEK SUDAH LOGIN (punya JID)
	if session.Client.Store.ID == nil {
		return ErrorResponse(c, 400, "Not logged in", "NOT_LOGGED_IN", "Please scan QR code first")
	}

	// 5. FORMAT & VALIDATE PHONE NUMBER
	recipient, err := helper.FormatPhoneNumber(to)
	if err != nil {
		return ErrorResponse(c, 400, "Invalid phone number", "INVALID_PHONE", err.Error())
	}

	// 6. CEK NOMOR TERDAFTAR DI WHATSAPP
	isRegistered, err := session.Client.IsOnWhatsApp(context.Background(), []string{recipient.User})
	if err != nil {
		return ErrorResponse(c, 500, "Failed to verify phone number", "VERIFICATION_FAILED", err.Error())
	}

	if len(isRegistered) == 0 || !isRegistered[0].IsIn {
		return ErrorResponse(c, 400, "Phone number is not registered on WhatsApp", "PHONE_NOT_REGISTERED",
			"Please check the number or ask recipient to install WhatsApp")
	}

	// 7. GET & VALIDATE FILE
	file, err := c.FormFile("file")
	if err != nil {
		return ErrorResponse(c, 400, "File is required", "FILE_REQUIRED", err.Error())
	}

	src, err := file.Open()
	if err != nil {
		return ErrorResponse(c, 500, "Failed to open file", "FILE_OPEN_FAILED", err.Error())
	}
	defer src.Close()

	fileData, err := io.ReadAll(src)
	if err != nil {
		return ErrorResponse(c, 500, "Failed to read file", "FILE_READ_FAILED", err.Error())
	}

	// 8. DETECT MEDIA TYPE
	mediaType := helper.DetectMediaType(file.Filename)

	// 9. VALIDATE FILE SIZE
	maxSize := getMaxFileSize(mediaType)
	if len(fileData) > maxSize {
		return ErrorResponse(c, 400, "File too large", "FILE_TOO_LARGE",
			fmt.Sprintf("File size: %d bytes, Max: %d bytes (%s)", len(fileData), maxSize, mediaType))
	}

	// 10. CONVERT MEDIA TYPE
	var whatsmeowMediaType whatsmeow.MediaType
	switch mediaType {
	case "image":
		whatsmeowMediaType = whatsmeow.MediaImage
	case "video":
		whatsmeowMediaType = whatsmeow.MediaVideo
	case "audio":
		whatsmeowMediaType = whatsmeow.MediaAudio
	default:
		whatsmeowMediaType = whatsmeow.MediaDocument
	}

	// 11. UPLOAD TO WHATSAPP
	uploaded, err := session.Client.Upload(context.Background(), fileData, whatsmeowMediaType)
	if err != nil {
		return ErrorResponse(c, 500, "Failed to upload media", "UPLOAD_FAILED",
			fmt.Sprintf("Type: %s, Size: %d bytes, Error: %v", mediaType, len(fileData), err))
	}

	// 12. CREATE MESSAGE
	msg := helper.CreateMediaMessage(uploaded, caption, file.Filename, mediaType)

	// 13. SEND MESSAGE
	resp, err := session.Client.SendMessage(context.Background(), recipient, msg)
	if err != nil {
		return ErrorResponse(c, 500, "Failed to send media", "SEND_FAILED", err.Error())
	}

	// 14. SUCCESS RESPONSE
	return SuccessResponse(c, 200, "Media sent successfully", map[string]interface{}{
		"messageId": resp.ID,
		"timestamp": resp.Timestamp.Unix(),
		"to":        to,
		"mediaType": mediaType,
		"fileName":  file.Filename,
		"fileSize":  len(fileData),
		"verified":  true,
	})
}

// POST /send/:instanceId/media-url (from URL)
func SendMediaURL(c echo.Context) error {
	instanceID := c.Param("instanceId")

	var req SendMediaRequest
	if err := c.Bind(&req); err != nil {
		return ErrorResponse(c, 400, "Invalid request body", "INVALID_REQUEST", err.Error())
	}

	if req.To == "" || req.MediaURL == "" {
		return ErrorResponse(c, 400, "Fields 'to' and 'mediaUrl' are required", "VALIDATION_ERROR", "")
	}

	// 1. CEK SESSION EXISTS
	session, err := service.GetSession(instanceID)
	if err != nil {
		return ErrorResponse(c, 404, "Session not found", "SESSION_NOT_FOUND", "Please login first")
	}

	// 2. CEK CONNECTION FLAG (dari memory/database)
	if !session.IsConnected {
		return ErrorResponse(c, 400, "Session is not connected", "NOT_CONNECTED", "Please check /status endpoint")
	}

	// 3. CEK REAL WHATSAPP CONNECTION (websocket)
	if !session.Client.IsConnected() {
		return ErrorResponse(c, 400, "WhatsApp connection lost", "CONNECTION_LOST", "Please reconnect")
	}

	// 4. CEK SUDAH LOGIN (punya JID)
	if session.Client.Store.ID == nil {
		return ErrorResponse(c, 400, "Not logged in", "NOT_LOGGED_IN", "Please scan QR code first")
	}

	// 5. FORMAT & VALIDATE PHONE NUMBER
	recipient, err := helper.FormatPhoneNumber(req.To)
	if err != nil {
		return ErrorResponse(c, 400, "Invalid phone number", "INVALID_PHONE", err.Error())
	}

	// 6. CEK NOMOR TERDAFTAR DI WHATSAPP
	isRegistered, err := session.Client.IsOnWhatsApp(context.Background(), []string{recipient.User})
	if err != nil {
		return ErrorResponse(c, 500, "Failed to verify phone number", "VERIFICATION_FAILED", err.Error())
	}

	if len(isRegistered) == 0 || !isRegistered[0].IsIn {
		return ErrorResponse(c, 400, "Phone number is not registered on WhatsApp", "PHONE_NOT_REGISTERED",
			"Please check the number or ask recipient to install WhatsApp")
	}

	// 7. DOWNLOAD FILE FROM URL
	fmt.Printf("Downloading from: %s\n", req.MediaURL)
	fileData, filename, err := helper.DownloadFile(req.MediaURL)
	if err != nil {
		return ErrorResponse(c, 500, "Failed to download file", "DOWNLOAD_FAILED", err.Error())
	}
	fmt.Printf("Downloaded: %s (%d bytes)\n", filename, len(fileData))

	// 8. DETECT MEDIA TYPE
	mediaType := req.MediaType
	if mediaType == "" {
		mediaType = helper.DetectMediaType(filename)
	}
	fmt.Printf("Detected media type: %s\n", mediaType)

	// 9. VALIDATE FILE SIZE
	maxSize := getMaxFileSize(mediaType)
	if len(fileData) > maxSize {
		return ErrorResponse(c, 400, "File too large", "FILE_TOO_LARGE",
			fmt.Sprintf("File size: %d bytes, Max allowed: %d bytes (%s)", len(fileData), maxSize, mediaType))
	}

	// 10. CONVERT MEDIA TYPE
	var whatsmeowMediaType whatsmeow.MediaType
	switch mediaType {
	case "image":
		whatsmeowMediaType = whatsmeow.MediaImage
	case "video":
		whatsmeowMediaType = whatsmeow.MediaVideo
	case "audio":
		whatsmeowMediaType = whatsmeow.MediaAudio
	default:
		whatsmeowMediaType = whatsmeow.MediaDocument
	}

	// 11. UPLOAD TO WHATSAPP
	fmt.Printf("Uploading to WhatsApp as: %s\n", whatsmeowMediaType)
	uploaded, err := session.Client.Upload(context.Background(), fileData, whatsmeowMediaType)
	if err != nil {
		return ErrorResponse(c, 500, "Failed to upload media to WhatsApp", "UPLOAD_FAILED",
			fmt.Sprintf("File: %s, Size: %d bytes, Type: %s, Error: %v", filename, len(fileData), mediaType, err))
	}

	// 12. CREATE MESSAGE
	msg := helper.CreateMediaMessage(uploaded, req.Caption, filename, mediaType)

	// 13. SEND MESSAGE
	resp, err := session.Client.SendMessage(context.Background(), recipient, msg)
	if err != nil {
		return ErrorResponse(c, 500, "Failed to send media", "SEND_FAILED", err.Error())
	}

	// 14. SUCCESS RESPONSE
	return SuccessResponse(c, 200, "Media sent successfully", map[string]interface{}{
		"messageId": resp.ID,
		"timestamp": resp.Timestamp.Unix(),
		"to":        req.To,
		"mediaType": mediaType,
		"fileName":  filename,
		"fileSize":  len(fileData),
		"verified":  true,
	})
}

// Helper: Get max file size per media type (WhatsApp limits)
func getMaxFileSize(mediaType string) int {
	switch mediaType {
	case "image":
		return 5 * 1024 * 1024 // 5MB for images
	case "video":
		return 16 * 1024 * 1024 // 16MB for videos
	case "audio":
		return 16 * 1024 * 1024 // 16MB for audio
	default: // document
		return 100 * 1024 * 1024 // 100MB for documents
	}
}
