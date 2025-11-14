package handler

import (
	"context"
	"fmt"
	"io"

	"gowa-yourself/internal/helper"
	"gowa-yourself/internal/service"

	"github.com/labstack/echo/v4"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// Request untuk send group message
type SendGroupMessageRequest struct {
	GroupJID string `json:"groupJid" validate:"required"`
	Message  string `json:"message" validate:"required"`
}

// GET /groups/:instanceId - List all groups
func GetGroups(c echo.Context) error {
	instanceID := c.Param("instanceId")

	session, err := service.GetSession(instanceID)
	if err != nil {
		return ErrorResponse(c, 404, "Session not found", "SESSION_NOT_FOUND", "")
	}

	if !session.IsConnected {
		return ErrorResponse(c, 400, "Session is not connected", "NOT_CONNECTED", "")
	}

	if !session.Client.IsConnected() {
		return ErrorResponse(c, 400, "WhatsApp connection lost", "CONNECTION_LOST", "")
	}

	// Get all groups dengan context
	groups, err := session.Client.GetJoinedGroups(context.Background())
	if err != nil {
		return ErrorResponse(c, 500, "Failed to get groups", "GET_GROUPS_FAILED", err.Error())
	}

	groupList := make([]map[string]interface{}, 0)
	for _, groupInfo := range groups {
		groupList = append(groupList, map[string]interface{}{
			"jid":          groupInfo.JID.String(),
			"name":         groupInfo.Name,
			"topic":        groupInfo.Topic,
			"participants": len(groupInfo.Participants),
			"ownerJid":     groupInfo.OwnerJID.String(),
			"createdAt":    groupInfo.GroupCreated.Unix(),
		})
	}

	return SuccessResponse(c, 200, "Groups retrieved", map[string]interface{}{
		"total":  len(groupList),
		"groups": groupList,
	})
}

// POST /send-group/:instanceId - Send text to group
func SendGroupMessage(c echo.Context) error {
	instanceID := c.Param("instanceId")

	var req SendGroupMessageRequest
	if err := c.Bind(&req); err != nil {
		return ErrorResponse(c, 400, "Invalid request body", "INVALID_REQUEST", err.Error())
	}

	if req.GroupJID == "" || req.Message == "" {
		return ErrorResponse(c, 400, "Fields 'groupJid' and 'message' are required", "VALIDATION_ERROR", "")
	}

	session, err := service.GetSession(instanceID)
	if err != nil {
		return ErrorResponse(c, 404, "Session not found", "SESSION_NOT_FOUND", "")
	}

	if !session.IsConnected {
		return ErrorResponse(c, 400, "Session is not connected", "NOT_CONNECTED", "")
	}

	if !session.Client.IsConnected() {
		return ErrorResponse(c, 400, "WhatsApp connection lost", "CONNECTION_LOST", "")
	}

	if session.Client.Store.ID == nil {
		return ErrorResponse(c, 400, "Not logged in", "NOT_LOGGED_IN", "")
	}

	// Parse group JID
	groupJID, err := types.ParseJID(req.GroupJID)
	if err != nil {
		return ErrorResponse(c, 400, "Invalid group JID", "INVALID_GROUP_JID", err.Error())
	}

	// Validate it's a group JID (ends with @g.us)
	if groupJID.Server != types.GroupServer {
		return ErrorResponse(c, 400, "Not a group JID", "NOT_GROUP_JID", "Group JID must end with @g.us")
	}

	// Create message
	msg := &waE2E.Message{
		Conversation: &req.Message,
	}

	// Send to group
	resp, err := session.Client.SendMessage(context.Background(), groupJID, msg)
	if err != nil {
		return ErrorResponse(c, 500, "Failed to send message", "SEND_FAILED", err.Error())
	}

	return SuccessResponse(c, 200, "Message sent to group", map[string]interface{}{
		"messageId": resp.ID,
		"timestamp": resp.Timestamp.Unix(),
		"groupJid":  req.GroupJID,
	})
}

// POST /send-group/:instanceId/media - Send media to group
func SendGroupMedia(c echo.Context) error {
	instanceID := c.Param("instanceId")

	groupJid := c.FormValue("groupJid")
	caption := c.FormValue("caption")

	if groupJid == "" {
		return ErrorResponse(c, 400, "Field 'groupJid' is required", "VALIDATION_ERROR", "")
	}

	session, err := service.GetSession(instanceID)
	if err != nil {
		return ErrorResponse(c, 404, "Session not found", "SESSION_NOT_FOUND", "")
	}

	if !session.IsConnected {
		return ErrorResponse(c, 400, "Session is not connected", "NOT_CONNECTED", "")
	}

	if !session.Client.IsConnected() {
		return ErrorResponse(c, 400, "WhatsApp connection lost", "CONNECTION_LOST", "")
	}

	if session.Client.Store.ID == nil {
		return ErrorResponse(c, 400, "Not logged in", "NOT_LOGGED_IN", "")
	}

	// Parse group JID
	groupJID, err := types.ParseJID(groupJid)
	if err != nil {
		return ErrorResponse(c, 400, "Invalid group JID", "INVALID_GROUP_JID", err.Error())
	}

	if groupJID.Server != types.GroupServer {
		return ErrorResponse(c, 400, "Not a group JID", "NOT_GROUP_JID", "Group JID must end with @g.us")
	}

	// Get file
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

	mediaType := helper.DetectMediaType(file.Filename)

	maxSize := getMaxFileSize(mediaType)
	if len(fileData) > maxSize {
		return ErrorResponse(c, 400, "File too large", "FILE_TOO_LARGE",
			fmt.Sprintf("File: %d bytes, Max: %d bytes", len(fileData), maxSize))
	}

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

	uploaded, err := session.Client.Upload(context.Background(), fileData, whatsmeowMediaType)
	if err != nil {
		return ErrorResponse(c, 500, "Failed to upload media", "UPLOAD_FAILED", err.Error())
	}

	msg := helper.CreateMediaMessage(uploaded, caption, file.Filename, mediaType)

	resp, err := session.Client.SendMessage(context.Background(), groupJID, msg)
	if err != nil {
		return ErrorResponse(c, 500, "Failed to send media", "SEND_FAILED", err.Error())
	}

	return SuccessResponse(c, 200, "Media sent to group", map[string]interface{}{
		"messageId": resp.ID,
		"timestamp": resp.Timestamp.Unix(),
		"groupJid":  groupJid,
		"mediaType": mediaType,
		"fileName":  file.Filename,
		"fileSize":  len(fileData),
	})
}

// POST /send-group/:instanceId/media-url - Send media from URL to group
func SendGroupMediaURL(c echo.Context) error {
	instanceID := c.Param("instanceId")

	var req struct {
		GroupJID  string `json:"groupJid" validate:"required"`
		MediaURL  string `json:"mediaUrl" validate:"required"`
		Caption   string `json:"caption"`
		MediaType string `json:"mediaType"`
	}

	if err := c.Bind(&req); err != nil {
		return ErrorResponse(c, 400, "Invalid request body", "INVALID_REQUEST", err.Error())
	}

	if req.GroupJID == "" || req.MediaURL == "" {
		return ErrorResponse(c, 400, "Fields 'groupJid' and 'mediaUrl' are required", "VALIDATION_ERROR", "")
	}

	session, err := service.GetSession(instanceID)
	if err != nil {
		return ErrorResponse(c, 404, "Session not found", "SESSION_NOT_FOUND", "")
	}

	if !session.IsConnected {
		return ErrorResponse(c, 400, "Session is not connected", "NOT_CONNECTED", "")
	}

	if !session.Client.IsConnected() {
		return ErrorResponse(c, 400, "WhatsApp connection lost", "CONNECTION_LOST", "")
	}

	if session.Client.Store.ID == nil {
		return ErrorResponse(c, 400, "Not logged in", "NOT_LOGGED_IN", "")
	}

	groupJID, err := types.ParseJID(req.GroupJID)
	if err != nil {
		return ErrorResponse(c, 400, "Invalid group JID", "INVALID_GROUP_JID", err.Error())
	}

	if groupJID.Server != types.GroupServer {
		return ErrorResponse(c, 400, "Not a group JID", "NOT_GROUP_JID", "Group JID must end with @g.us")
	}

	fileData, filename, err := helper.DownloadFile(req.MediaURL)
	if err != nil {
		return ErrorResponse(c, 500, "Failed to download file", "DOWNLOAD_FAILED", err.Error())
	}

	mediaType := req.MediaType
	if mediaType == "" {
		mediaType = helper.DetectMediaType(filename)
	}

	maxSize := getMaxFileSize(mediaType)
	if len(fileData) > maxSize {
		return ErrorResponse(c, 400, "File too large", "FILE_TOO_LARGE",
			fmt.Sprintf("File: %d bytes, Max: %d bytes", len(fileData), maxSize))
	}

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

	uploaded, err := session.Client.Upload(context.Background(), fileData, whatsmeowMediaType)
	if err != nil {
		return ErrorResponse(c, 500, "Failed to upload media", "UPLOAD_FAILED", err.Error())
	}

	msg := helper.CreateMediaMessage(uploaded, req.Caption, filename, mediaType)

	resp, err := session.Client.SendMessage(context.Background(), groupJID, msg)
	if err != nil {
		return ErrorResponse(c, 500, "Failed to send media", "SEND_FAILED", err.Error())
	}

	return SuccessResponse(c, 200, "Media sent to group", map[string]interface{}{
		"messageId": resp.ID,
		"timestamp": resp.Timestamp.Unix(),
		"groupJid":  req.GroupJID,
		"mediaType": mediaType,
		"fileName":  filename,
		"fileSize":  len(fileData),
	})
}
