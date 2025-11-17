# 📱 Sudev WhatsApp Multi-Device API (Go)

REST API untuk mengelola WhatsApp Web Multi-Device menggunakan Go, Echo, PostgreSQL, dan library [whatsmeow](https://github.com/tulir/whatsmeow).  
Proyek masih dalam tahap pengembangan (belum 100%), tetapi core fitur sudah bisa dipakai untuk eksperimen dan integrasi awal.

---

## ✨ Fitur Utama

### 🔐 Authentication & Session Management

- Multi-device support — kelola banyak nomor WhatsApp sekaligus
- QR Code authentication — generate QR untuk scan di WhatsApp Web / Linked Devices
- Persistent sessions — session WhatsApp tersimpan di PostgreSQL, survive restart
- Custom instance store — tabel `instances` menyimpan `instance_id`, `phone_number`, `jid`, `status`, dll.
- Auto-reconnect — setelah restart server, instance otomatis reconnect dari DB
- Graceful logout — logout via API / HP, status instance ikut ter-update di DB

### 💬 Personal Messaging (By Phone Number)

- Kirim pesan teks: `POST /send/by-number/:phoneNumber`
- Kirim media dari URL: `POST /send/by-number/:phoneNumber/media-url`
- Kirim media via upload (form-data): `POST /send/by-number/:phoneNumber/media-file`
- Normalisasi & validasi nomor tujuan, cek terdaftar di WhatsApp sebelum kirim

> Catatan: Pengiriman pesan personal sekarang berbasis **nomor pengirim**, bukan lagi `instance_id`.

### ⚙️ Instance Lifecycle (By Instance ID)

- Login & QR: generate QR per `instance_id` untuk proses pairing
- Status instance tersinkron dari event WhatsMeow:
  - `Connected` → `status = 'online'`, `is_connected = true`
  - `Disconnected` → `status = 'disconnected'`, `is_connected = false`
  - `LoggedOut` → `status = 'logged_out'`, instance tidak bisa dipakai login ulang
- Logout API: unlink device, bersihkan session in-memory, dan update status di DB

---

## 🏗️ Tech Stack

- Go 1.21+ (Echo v4)
- PostgreSQL 12+
- WhatsApp Web Multi-Device: [whatsmeow](https://github.com/tulir/whatsmeow)

> Proyek masih aktif dikembangkan. Struktur endpoint dan fitur bisa berubah seiring waktu.
