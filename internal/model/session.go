package model

import "go.mau.fi/whatsmeow"

type Session struct {
	ID          string
	JID         string
	Client      *whatsmeow.Client
	IsConnected bool
}
