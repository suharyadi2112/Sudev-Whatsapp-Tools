package helper

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

// DetectMediaType detects media type from filename extension
func DetectMediaType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return "image"
	case ".mp4", ".mov", ".avi", ".mkv":
		return "video"
	case ".mp3", ".ogg", ".m4a", ".opus":
		return "audio"
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".txt", ".zip":
		return "document"
	default:
		return "document"
	}
}

// GetMimeType returns MIME type based on media type
func GetMimeType(mediaType, filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	switch mediaType {
	case "image":
		switch ext {
		case ".jpg", ".jpeg":
			return "image/jpeg"
		case ".png":
			return "image/png"
		case ".gif":
			return "image/gif"
		case ".webp":
			return "image/webp"
		default:
			return "image/jpeg"
		}
	case "video":
		return "video/mp4"
	case "audio":
		return "audio/mpeg"
	default: // document
		switch ext {
		case ".pdf":
			return "application/pdf"
		case ".doc", ".docx":
			return "application/msword"
		case ".xls", ".xlsx":
			return "application/vnd.ms-excel"
		case ".zip":
			return "application/zip"
		default:
			return "application/octet-stream"
		}
	}
}

// CreateMediaMessage creates WhatsApp media message based on type
func CreateMediaMessage(uploaded whatsmeow.UploadResponse, caption, filename, mediaType string) *waE2E.Message {
	msg := &waE2E.Message{}
	mimeType := GetMimeType(mediaType, filename)

	switch mediaType {
	case "image":
		msg.ImageMessage = &waE2E.ImageMessage{
			Caption:       &caption,
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimeType,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    &uploaded.FileLength,
		}
	case "video":
		msg.VideoMessage = &waE2E.VideoMessage{
			Caption:       &caption,
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimeType,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    &uploaded.FileLength,
		}
	case "audio":
		msg.AudioMessage = &waE2E.AudioMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimeType,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    &uploaded.FileLength,
		}
	default: // document
		msg.DocumentMessage = &waE2E.DocumentMessage{
			Caption:       &caption,
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimeType,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    &uploaded.FileLength,
			FileName:      &filename,
		}
	}

	return msg
}

// DownloadFile downloads file from URL and returns data and filename
func DownloadFile(url string) ([]byte, string, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Follow up to 10 redirects
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %v", err)
	}

	// Add comprehensive headers to avoid 403
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/pdf,image/*,video/*,audio/*,*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Referer", url)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("failed to download: status %d (%s)", resp.StatusCode, resp.Status)
	}

	// Read response body
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response: %v", err)
	}

	// Validate file size (WhatsApp limit ~100MB for documents)
	if len(data) > 100*1024*1024 {
		return nil, "", fmt.Errorf("file too large: %d bytes (max 100MB)", len(data))
	}

	if len(data) == 0 {
		return nil, "", fmt.Errorf("downloaded file is empty")
	}

	// Extract filename from URL or Content-Disposition header
	filename := ""

	// Try to get filename from Content-Disposition header
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if strings.Contains(cd, "filename=") {
			parts := strings.Split(cd, "filename=")
			if len(parts) > 1 {
				filename = strings.Trim(parts[1], "\"")
			}
		}
	}

	// Fallback to URL path
	if filename == "" {
		filename = filepath.Base(url)
		// Remove query parameters from filename
		if idx := strings.Index(filename, "?"); idx != -1 {
			filename = filename[:idx]
		}
	}

	// Default fallback
	if filename == "." || filename == "/" || filename == "" {
		// Try to detect extension from content-type
		contentType := resp.Header.Get("Content-Type")
		ext := getExtensionFromContentType(contentType)
		filename = "document" + ext
	}

	return data, filename, nil
}

// Helper to get file extension from Content-Type
func getExtensionFromContentType(contentType string) string {
	contentType = strings.ToLower(strings.Split(contentType, ";")[0])

	switch contentType {
	case "application/pdf":
		return ".pdf"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "audio/mpeg":
		return ".mp3"
	case "application/zip":
		return ".zip"
	default:
		return ".bin"
	}
}
