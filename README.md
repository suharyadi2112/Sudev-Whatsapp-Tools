# 📱 Sudev WhatsApp Multi-Device API (Go)

REST API untuk mengelola WhatsApp Web Multi-Device menggunakan Go, Echo Framework, PostgreSQL, dan library [whatsmeow](https://github.com/tulir/whatsmeow).

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-12+-316192?style=flat&logo=postgresql)](https://www.postgresql.org)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## ✨ Features

### 🔐 Authentication & Session Management
- ✅ **Multi-device support** - Kelola banyak nomor WhatsApp sekaligus
- ✅ **QR Code authentication** - Generate QR untuk scan
- ✅ **Persistent sessions** - Session tersimpan di PostgreSQL, survive restart
- ✅ **Auto-reconnect** - Otomatis reconnect setelah server restart

### 💬 Personal Messaging
- ✅ **Send text messages** - Kirim pesan teks ke nomor personal
- ✅ **Phone number validation** - Cek nomor terdaftar di WhatsApp
- ✅ **Media support** - Kirim gambar, video, audio, dokumen
- ✅ **Multiple upload methods** - Upload file lokal atau dari URL

### 📢 Group Messaging
- ✅ **List groups** - Tampilkan semua grup yang diikuti
- ✅ **Send to groups** - Kirim pesan teks ke grup
- ✅ **Group media** - Kirim media ke grup (upload & URL)

## 🏗️ Tech Stack

- **Language:** Go 1.21+
- **Framework:** Echo v4
- **Database:** PostgreSQL 12+
- **WhatsApp Library:** whatsmeow (official multi-device)
- **Middleware:** CORS, Logger, Recover

## 📂 Project Structure

gowa-yourself/
├── config/
│ └── config.go
├── database/
│ ├── database.go # WhatsApp protocol DB connection
│ └── app_db.go # Application custom DB connection
├── internal/
│ ├── handler/ # HTTP request handlers
│ │ ├── auth.go # Login, QR, Status, Logout
│ │ ├── message.go # Personal text messages
│ │ ├── media.go # Personal media messages
│ │ ├── group.go # Group messages & media
│ │ └── response.go # Standard API responses
│ ├── helper/ # Utility functions
│ │ ├── media.go # Media type detection
│ │ └── phone.go # Phone number formatting
│ ├── model/ # Data models & repository
│ │ └── session.go
│ └── service/ # Business logic layer
│ └── whatsapp.go # WhatsApp session management
├── main.go # Application entry point
├── go.mod
├── go.sum

### Prerequisites
- Go 1.21 or higher
- PostgreSQL 12 or higher
- Git
