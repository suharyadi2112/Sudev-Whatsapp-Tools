package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"gowa-yourself/internal/model"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"
)

// OutboxRequest represents the incoming JSON structure for queue creation
type OutboxRequest struct {
	Type            *int       `json:"type"`
	FromNumber      string     `json:"from_number"`
	ClientID        *int       `json:"client_id"`
	Destination     string     `json:"destination"`
	Messages        string     `json:"messages"`
	Status          *int       `json:"status"`
	Priority        *int       `json:"priority"`
	Application     string     `json:"application"`
	SendingDateTime *time.Time `json:"sending_datetime"`
	TableID         string     `json:"table_id"`
	File            string     `json:"file"`
}

// OutboxResponse represents the clean JSON response structure
type OutboxResponse struct {
	IDOutbox        int        `json:"id_outbox"`
	Type            int        `json:"type"`
	FromNumber      *string    `json:"from_number"`
	ClientID        *int       `json:"client_id"`
	Destination     string     `json:"destination"`
	Messages        string     `json:"messages"`
	Status          int        `json:"status"`
	Priority        int        `json:"priority"`
	Application     *string    `json:"application"`
	SendingDateTime *time.Time `json:"sending_datetime"`
	InsertDateTime  time.Time  `json:"insert_datetime"`
	TableID         *string    `json:"table_id"`
	File            *string    `json:"file"`
	ErrorCount      int        `json:"error_count"`
	MsgError        *string    `json:"msg_error"`
}

func ToResponse(m model.Outbox) OutboxResponse {
	resp := OutboxResponse{
		IDOutbox:       m.IDOutbox,
		Type:           m.Type,
		Destination:    m.Destination,
		Messages:       m.Messages,
		Status:         m.Status,
		Priority:       m.Priority,
		InsertDateTime: m.InsertDateTime,
		ErrorCount:     m.ErrorCount,
	}

	if m.FromNumber.Valid {
		resp.FromNumber = &m.FromNumber.String
	}
	if m.ClientID.Valid {
		val := int(m.ClientID.Int64)
		resp.ClientID = &val
	}
	if m.Application.Valid {
		resp.Application = &m.Application.String
	}
	if m.SendingDateTime.Valid {
		resp.SendingDateTime = &m.SendingDateTime.Time
	}
	if m.TableID.Valid {
		resp.TableID = &m.TableID.String
	}
	if m.File.Valid {
		resp.File = &m.File.String
	}
	if m.MsgError.Valid {
		resp.MsgError = &m.MsgError.String
	}

	return resp
}

func (req *OutboxRequest) ToModel() model.Outbox {
	m := model.Outbox{
		Type:        1, // default
		Destination: req.Destination,
		Messages:    req.Messages,
		Status:      0, // default
		Priority:    0, // default
	}

	if req.Type != nil {
		m.Type = *req.Type
	}
	if req.FromNumber != "" {
		m.FromNumber = sql.NullString{String: req.FromNumber, Valid: true}
	}
	if req.ClientID != nil {
		m.ClientID = sql.NullInt64{Int64: int64(*req.ClientID), Valid: true}
	}
	if req.Status != nil {
		m.Status = *req.Status
	}
	if req.Priority != nil {
		m.Priority = *req.Priority
	}
	if req.Application != "" {
		m.Application = sql.NullString{String: req.Application, Valid: true}
	}
	if req.SendingDateTime != nil {
		m.SendingDateTime = sql.NullTime{Time: *req.SendingDateTime, Valid: true}
	} else {
		m.SendingDateTime = sql.NullTime{Time: time.Now(), Valid: true}
	}
	if req.TableID != "" {
		m.TableID = sql.NullString{String: req.TableID, Valid: true}
	}
	if req.File != "" {
		m.File = sql.NullString{String: req.File, Valid: true}
	}

	return m
}

// CreateOutboxQueue handles requests to add message(s) to the outbox queue
func CreateOutboxQueue(c echo.Context) error {
	body := c.Request().Body
	if body == nil {
		return ErrorResponse(c, http.StatusBadRequest, "Request body is empty", "EMPTY_BODY", "")
	}

	// Read raw body bytes
	raw, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return ErrorResponse(c, http.StatusBadRequest, "Failed to read request body", "READ_ERROR", err.Error())
	}

	if len(raw) == 0 {
		return ErrorResponse(c, http.StatusBadRequest, "Request body is empty", "EMPTY_BODY", "")
	}

	// Check if JSON starts with '[' (Slice/Array)
	isSlice := false
	for _, b := range raw {
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			continue
		}
		if b == '[' {
			isSlice = true
		}
		break
	}

	ctx := c.Request().Context()

	if isSlice {
		var reqs []OutboxRequest
		if err := json.Unmarshal(raw, &reqs); err != nil {
			return ErrorResponse(c, http.StatusBadRequest, "Invalid JSON array format", "INVALID_JSON_ARRAY", err.Error())
		}

		if len(reqs) == 0 {
			return ErrorResponse(c, http.StatusBadRequest, "Queue records array is empty", "EMPTY_ARRAY", "")
		}

		// Validation & conversion
		var models []model.Outbox
		for _, req := range reqs {
			if req.Destination == "" || req.Messages == "" {
				return ErrorResponse(c, http.StatusBadRequest, "destination and messages are required for all records", "VALIDATION_ERROR", "")
			}
			if shouldReplace, _ := model.ShouldReplacePendingForApp(ctx, req.Application); shouldReplace {
				_, _ = model.CancelPendingOutboxForApp(ctx, req.Destination, req.Application)
			}
			models = append(models, req.ToModel())
		}

		if err := model.CreateOutboxBatch(ctx, models); err != nil {
			return ErrorResponse(c, http.StatusInternalServerError, "Failed to save outbox records", "DATABASE_ERROR", err.Error())
		}

		return SuccessResponse(c, http.StatusCreated, "Outbox queue records created successfully", map[string]interface{}{
			"count": len(models),
		})
	} else {
		var req OutboxRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return ErrorResponse(c, http.StatusBadRequest, "Invalid JSON object format", "INVALID_JSON_OBJECT", err.Error())
		}

		if req.Destination == "" || req.Messages == "" {
			return ErrorResponse(c, http.StatusBadRequest, "destination and messages are required", "VALIDATION_ERROR", "")
		}

		if shouldReplace, _ := model.ShouldReplacePendingForApp(ctx, req.Application); shouldReplace {
			_, _ = model.CancelPendingOutboxForApp(ctx, req.Destination, req.Application)
		}

		outboxModel := req.ToModel()
		if err := model.CreateOutboxSingle(ctx, &outboxModel); err != nil {
			return ErrorResponse(c, http.StatusInternalServerError, "Failed to save outbox record", "DATABASE_ERROR", err.Error())
		}

		return SuccessResponse(c, http.StatusCreated, "Outbox queue record created successfully", ToResponse(outboxModel))
	}
}

// GetOutboxQueue handles fetching outbox queue records with filters, search, and pagination
func GetOutboxQueue(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit < 1 || limit > 100 {
		limit = 10
	}

	offset := (page - 1) * limit

	var status *int
	if statusStr := c.QueryParam("status"); statusStr != "" {
		if val, err := strconv.Atoi(statusStr); err == nil {
			status = &val
		}
	}

	application := c.QueryParam("application")
	search := c.QueryParam("search")

	ctx := c.Request().Context()

	// Fetch count
	total, err := model.GetOutboxQueueCount(ctx, status, application, search)
	if err != nil {
		return ErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve outbox queue count", "DATABASE_ERROR", err.Error())
	}

	records, err := model.GetOutboxQueue(ctx, status, application, search, limit, offset)
	if err != nil {
		return ErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve outbox queue", "DATABASE_ERROR", err.Error())
	}

	var responseRecords []OutboxResponse
	for _, r := range records {
		responseRecords = append(responseRecords, ToResponse(r))
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	return SuccessResponse(c, http.StatusOK, "Outbox queue retrieved successfully", map[string]interface{}{
		"data": responseRecords,
		"pagination": map[string]interface{}{
			"total_data":   total,
			"total_pages":  totalPages,
			"current_page": page,
			"limit":        limit,
		},
	})
}

// GetOutboxByID handles fetching a single outbox record by ID
func GetOutboxByID(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return ErrorResponse(c, http.StatusBadRequest, "Invalid ID parameter", "INVALID_ID", "")
	}

	ctx := c.Request().Context()
	record, err := model.GetOutboxByID(ctx, id)
	if err != nil {
		return ErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve outbox record", "DATABASE_ERROR", err.Error())
	}

	if record == nil {
		return ErrorResponse(c, http.StatusNotFound, "Outbox record not found", "NOT_FOUND", "")
	}

	return SuccessResponse(c, http.StatusOK, "Outbox record retrieved successfully", ToResponse(*record))
}

// ImportOutboxExcel handles importing outbox messages from an uploaded Excel file (.xlsx)
func ImportOutboxExcel(c echo.Context) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return ErrorResponse(c, http.StatusBadRequest, "Missing excel file", "MISSING_FILE", err.Error())
	}

	src, err := fileHeader.Open()
	if err != nil {
		return ErrorResponse(c, http.StatusBadRequest, "Failed to open uploaded file", "FILE_OPEN_ERROR", err.Error())
	}
	defer src.Close()

	f, err := excelize.OpenReader(src)
	if err != nil {
		return ErrorResponse(c, http.StatusBadRequest, "Failed to read excel file", "EXCEL_READ_ERROR", err.Error())
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	if sheet == "" {
		return ErrorResponse(c, http.StatusBadRequest, "No sheet found in excel file", "EMPTY_SHEET", "")
	}

	rows, err := f.GetRows(sheet)
	if err != nil {
		return ErrorResponse(c, http.StatusBadRequest, "Failed to parse excel rows", "EXCEL_PARSE_ERROR", err.Error())
	}

	if len(rows) == 0 {
		return ErrorResponse(c, http.StatusBadRequest, "Excel file is empty", "EMPTY_EXCEL", "")
	}

	defaultApp := c.FormValue("application")

	destIdx := 0
	msgIdx := 1
	appIdx := 2
	startRow := 0

	// Check if row 0 is header
	if len(rows[0]) > 0 {
		h0 := strings.ToLower(strings.TrimSpace(rows[0][0]))
		if strings.Contains(h0, "dest") || strings.Contains(h0, "nomor") || strings.Contains(h0, "phone") || strings.Contains(h0, "tujuan") || strings.Contains(h0, "to") {
			startRow = 1
			for cIdx, colVal := range rows[0] {
				cName := strings.ToLower(strings.TrimSpace(colVal))
				if strings.Contains(cName, "dest") || strings.Contains(cName, "nomor") || strings.Contains(cName, "phone") || strings.Contains(cName, "tujuan") || strings.Contains(cName, "to") {
					destIdx = cIdx
				} else if strings.Contains(cName, "mess") || strings.Contains(cName, "pesan") || strings.Contains(cName, "text") {
					msgIdx = cIdx
				} else if strings.Contains(cName, "app") || strings.Contains(cName, "aplikasi") {
					appIdx = cIdx
				}
			}
		}
	}

	var models []model.Outbox
	skippedCount := 0

	for i := startRow; i < len(rows); i++ {
		row := rows[i]
		if len(row) == 0 {
			continue
		}

		var dest, msg, appStr string

		if destIdx < len(row) {
			dest = strings.TrimSpace(row[destIdx])
		}
		if msgIdx < len(row) {
			msg = strings.TrimSpace(row[msgIdx])
		}
		if appIdx < len(row) {
			appStr = strings.TrimSpace(row[appIdx])
		}

		if appStr == "" && defaultApp != "" {
			appStr = defaultApp
		}

		if dest == "" || msg == "" {
			skippedCount++
			continue
		}

		m := model.Outbox{
			Type:        1,
			Destination: dest,
			Messages:    msg,
			Status:      0,
			Priority:    0,
		}
		if appStr != "" {
			m.Application = sql.NullString{String: appStr, Valid: true}
		}

		models = append(models, m)
	}

	if len(models) == 0 {
		return ErrorResponse(c, http.StatusBadRequest, "No valid message rows found in Excel file", "NO_VALID_DATA", fmt.Sprintf("Skipped rows: %d", skippedCount))
	}

	ctx := c.Request().Context()
	if err := model.CreateOutboxBatch(ctx, models); err != nil {
		return ErrorResponse(c, http.StatusInternalServerError, "Failed to save imported outbox records", "DATABASE_ERROR", err.Error())
	}

	return SuccessResponse(c, http.StatusCreated, "Excel import completed successfully", map[string]interface{}{
		"imported_count": len(models),
		"skipped_count":  skippedCount,
		"total_rows":     len(rows) - startRow,
	})
}

// DownloadOutboxExcelTemplate provides an Excel template file for outbox queue import
func DownloadOutboxExcelTemplate(c echo.Context) error {
	f := excelize.NewFile()
	defer f.Close()

	sheetName := "OutboxImport"
	f.SetSheetName("Sheet1", sheetName)

	headers := []string{"destination", "messages", "application"}
	for i, h := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		f.SetCellValue(sheetName, cell, h)
	}

	// Add example row
	f.SetCellValue(sheetName, "A2", "6281234567890")
	f.SetCellValue(sheetName, "B2", "Halo ini pesan broadcast dari sistem!")
	f.SetCellValue(sheetName, "C2", "MARKETING")

	// Header style
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#2F5597"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	f.SetCellStyle(sheetName, "A1", "C1", headerStyle)

	// Set column widths
	f.SetColWidth(sheetName, "A", "A", 20)
	f.SetColWidth(sheetName, "B", "B", 45)
	f.SetColWidth(sheetName, "C", "C", 20)

	c.Response().Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Response().Header().Set("Content-Disposition", "attachment; filename=\"outbox_import_template.xlsx\"")

	return f.Write(c.Response().Writer)
}

type BulkStatusUpdateRequest struct {
	ToStatus *int  `json:"to_status"`
	IDs      []int `json:"ids"`
}

// BulkUpdateOutboxStatusHandler handles bulk status update requests for selected IDs
func BulkUpdateOutboxStatusHandler(c echo.Context) error {
	var req BulkStatusUpdateRequest
	if err := c.Bind(&req); err != nil {
		return ErrorResponse(c, http.StatusBadRequest, "Invalid JSON payload", "INVALID_PAYLOAD", err.Error())
	}

	if req.ToStatus == nil {
		return ErrorResponse(c, http.StatusBadRequest, "to_status is required", "VALIDATION_ERROR", "")
	}

	if len(req.IDs) == 0 {
		return ErrorResponse(c, http.StatusBadRequest, "ids array is required and must not be empty", "VALIDATION_ERROR", "")
	}

	ctx := c.Request().Context()
	updatedCount, err := model.BulkUpdateOutboxStatus(ctx, *req.ToStatus, req.IDs)
	if err != nil {
		return ErrorResponse(c, http.StatusInternalServerError, "Failed to bulk update outbox status", "DATABASE_ERROR", err.Error())
	}

	return SuccessResponse(c, http.StatusOK, "Bulk outbox status update completed successfully", map[string]interface{}{
		"updated_count": updatedCount,
		"to_status":     *req.ToStatus,
	})
}
