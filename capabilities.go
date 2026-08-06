package digitalpaper

// Capability identifies an independently testable device feature.
type Capability string

const (
	CapabilityDocuments     Capability = "documents"
	CapabilitySplitUpload   Capability = "split-upload"
	CapabilityScreenCapture Capability = "screen-capture"
	CapabilityWhiteboard    Capability = "whiteboard"
	CapabilityWiFiConfig    Capability = "wifi-config"
)
