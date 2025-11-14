package handler

import "github.com/labstack/echo/v4"

// Standard response structure
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
}

type ErrorInfo struct {
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// Success response helper
func SuccessResponse(c echo.Context, statusCode int, message string, data interface{}) error {
	return c.JSON(statusCode, APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// Error response helper
func ErrorResponse(c echo.Context, statusCode int, message string, errorCode string, details string) error {
	response := APIResponse{
		Success: false,
		Message: message,
	}

	if errorCode != "" || details != "" {
		response.Error = &ErrorInfo{
			Code:    errorCode,
			Details: details,
		}
	}

	return c.JSON(statusCode, response)
}
