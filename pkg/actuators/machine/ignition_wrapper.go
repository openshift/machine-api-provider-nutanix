package machine

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// mimeMultipartBoundary is the boundary used for MIME multipart messages
const mimeMultipartBoundary = "IGNITION_MIME_BOUNDARY"

// WrapIgnitionForNutanix wraps an Ignition configuration in a MIME multipart
// message format that is compatible with Nutanix v4.2+ API.
//
// The Nutanix v4.2 API validates cloud-init userdata for a '#cloud-config' header.
// This causes failures when using Ignition configurations (used by RHCOS) which
// start with '{"ignition":...}'.
//
// This function wraps the Ignition config in a MIME multipart message that:
// 1. Explicitly declares the content type as 'text/x-ignition'
// 2. Bypasses the '#cloud-config' header validation in Nutanix API
// 3. Is properly parsed by cloud-init on CONFIG_DRIVE_V2 datasource
//
// Reference: Nutanix KB 19848 - Guest customization missing Cloud-Init header
// Reference: https://cloudinit.readthedocs.io/en/latest/explanation/format.html#mime-multi-part-archive
func WrapIgnitionForNutanix(ignitionConfig []byte) string {
	if len(ignitionConfig) == 0 {
		return ""
	}

	// Check if it's already wrapped or is cloud-config
	configStr := string(ignitionConfig)
	if strings.HasPrefix(configStr, "#cloud-config") ||
		strings.HasPrefix(configStr, "Content-Type: multipart/") ||
		strings.HasPrefix(configStr, "MIME-Version:") {
		// Already in a compatible format, return base64 encoded
		return base64.StdEncoding.EncodeToString(ignitionConfig)
	}

	// Create MIME multipart wrapper for Ignition config
	// Using text/x-ignition content type which cloud-init passes through to Ignition
	mimeMessage := fmt.Sprintf(`Content-Type: multipart/mixed; boundary="%s"
MIME-Version: 1.0

--%s
Content-Type: text/x-ignition; charset="utf-8"
Content-Transfer-Encoding: base64

%s
--%s--
`, mimeMultipartBoundary, mimeMultipartBoundary,
		base64.StdEncoding.EncodeToString(ignitionConfig),
		mimeMultipartBoundary)

	return base64.StdEncoding.EncodeToString([]byte(mimeMessage))
}

// IsIgnitionConfig checks if the given data is an Ignition configuration
// by looking for the characteristic Ignition JSON structure.
func IsIgnitionConfig(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	// Ignition configs start with '{"ignition":' (possibly with whitespace)
	trimmed := strings.TrimSpace(string(data))
	return strings.HasPrefix(trimmed, `{"ignition":`) ||
		strings.HasPrefix(trimmed, `{ "ignition":`)
}

// UnwrapIgnitionFromMIME extracts the original Ignition config from a MIME-wrapped
// message. This is useful for testing and debugging.
func UnwrapIgnitionFromMIME(mimeWrappedBase64 string) ([]byte, error) {
	// Decode the outer base64
	mimeData, err := base64.StdEncoding.DecodeString(mimeWrappedBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode outer base64: %w", err)
	}

	// Find the inner base64 content between the MIME boundaries
	content := string(mimeData)

	// Look for the content after Content-Transfer-Encoding: base64
	parts := strings.Split(content, "Content-Transfer-Encoding: base64")
	if len(parts) < 2 {
		return nil, fmt.Errorf("no base64 encoded content found in MIME message")
	}

	// Extract the base64 content (between the newlines and the boundary)
	innerContent := strings.TrimSpace(parts[1])
	endBoundary := fmt.Sprintf("--%s--", mimeMultipartBoundary)
	innerContent = strings.Split(innerContent, endBoundary)[0]
	innerContent = strings.TrimSpace(innerContent)

	// Decode the inner base64
	ignitionData, err := base64.StdEncoding.DecodeString(innerContent)
	if err != nil {
		return nil, fmt.Errorf("failed to decode inner base64: %w", err)
	}

	return ignitionData, nil
}
