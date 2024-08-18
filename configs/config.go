package config

type CaptureTransportMode int

const (
	CaptureTransportModeNone   CaptureTransportMode = iota
	CaptureTransportModeRecord CaptureTransportMode = iota
	CaptureTransportModeFake   CaptureTransportMode = iota
)
