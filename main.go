package main

import (
	"log"
	"os"

	"gowa-yourself/database"
	"gowa-yourself/internal/handler"
	"gowa-yourself/internal/service"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// Load env
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:12345678@localhost:5432/whatsapp-2121?sslmode=disable"
	}

	database.InitWhatsmeow(dbURL)

	// App DB (custom instances)
	appDbURL := os.Getenv("APP_DATABASE_URL")
	if appDbURL == "" {
		appDbURL = "postgres://postgres:12345678@localhost:5432/custom-sudevwa?sslmode=disable"
	}
	database.InitAppDB(appDbURL)

	// Load all existing devices from database
	log.Println("Loading existing devices...")
	err := service.LoadAllDevices()
	if err != nil {
		log.Printf("Warning: Failed to load devices: %v", err)
	}

	// Setup Echo
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Health check
	e.GET("/", func(c echo.Context) error {
		return c.JSON(200, map[string]interface{}{
			"success": true,
			"message": "WhatsApp API is running",
			"version": "1.0.0",
		})
	})

	// Routes
	e.POST("/login", handler.Login)
	e.GET("/qr/:instanceId", handler.GetQR)
	e.GET("/status/:instanceId", handler.GetStatus)
	e.POST("/logout/:instanceId", handler.Logout)

	// ambil semua instance
	e.GET("/instances", handler.GetAllInstances)

	// Message routes
	e.POST("/send/:instanceId", handler.SendMessage)
	e.POST("/check/:instanceId", handler.CheckNumber)

	// Media routes
	e.POST("/send/:instanceId/media", handler.SendMediaFile)
	e.POST("/send/:instanceId/media-url", handler.SendMediaURL)

	// Group routes
	e.GET("/groups/:instanceId", handler.GetGroups)
	e.POST("/send-group/:instanceId", handler.SendGroupMessage)
	e.POST("/send-group/:instanceId/media", handler.SendGroupMedia)
	e.POST("/send-group/:instanceId/media-url", handler.SendGroupMediaURL)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "2121"
	}

	log.Printf("Server starting on port %s", port)
	log.Fatal(e.Start("127.0.0.1:" + port))
}
