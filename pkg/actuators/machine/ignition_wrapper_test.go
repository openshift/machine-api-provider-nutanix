package machine

import (
	"encoding/base64"
	"strings"
	"testing"
)

// Sample Ignition config similar to what OpenShift generates
const sampleIgnitionConfig = `{"ignition":{"config":{"merge":[{"source":"https://10.6.197.152:22623/config/worker","verification":{}}],"replace":{"verification":{}}},"proxy":{},"security":{"tls":{"certificateAuthorities":[{"source":"data:text/plain;charset=utf-8;base64,LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t"}]}},"timeouts":{},"version":"3.2.0"},"passwd":{},"storage":{},"systemd":{}}`

// Sample cloud-config that should NOT be wrapped
const sampleCloudConfig = `#cloud-config
users:
  - name: test
    groups: wheel
`

func TestWrapIgnitionForNutanix(t *testing.T) {
	tests := []struct {
		name           string
		input          []byte
		expectWrapped  bool
		expectMIMEType string
	}{
		{
			name:           "empty input",
			input:          []byte{},
			expectWrapped:  false,
			expectMIMEType: "",
		},
		{
			name:           "ignition config should be wrapped",
			input:          []byte(sampleIgnitionConfig),
			expectWrapped:  true,
			expectMIMEType: "text/x-ignition",
		},
		{
			name:           "cloud-config should not be wrapped",
			input:          []byte(sampleCloudConfig),
			expectWrapped:  false,
			expectMIMEType: "",
		},
		{
			name:           "already MIME wrapped should not be double-wrapped",
			input:          []byte("Content-Type: multipart/mixed; boundary=\"test\"\nMIME-Version: 1.0\n\ntest"),
			expectWrapped:  false,
			expectMIMEType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapIgnitionForNutanix(tt.input)

			if len(tt.input) == 0 {
				if result != "" {
					t.Errorf("expected empty result for empty input, got: %s", result)
				}
				return
			}

			// Decode the result
			decoded, err := base64.StdEncoding.DecodeString(result)
			if err != nil {
				t.Fatalf("failed to decode base64 result: %v", err)
			}

			decodedStr := string(decoded)

			if tt.expectWrapped {
				// Should be wrapped in MIME multipart
				if !strings.Contains(decodedStr, "Content-Type: multipart/mixed") {
					t.Errorf("expected MIME multipart wrapper, got: %s", decodedStr)
				}
				if !strings.Contains(decodedStr, tt.expectMIMEType) {
					t.Errorf("expected content type %s in wrapper, got: %s", tt.expectMIMEType, decodedStr)
				}
				if !strings.Contains(decodedStr, "MIME-Version: 1.0") {
					t.Errorf("expected MIME-Version header, got: %s", decodedStr)
				}
			} else {
				// Should be the original content (possibly just base64 encoded)
				if strings.Contains(decodedStr, "Content-Type: multipart/mixed") &&
					!strings.HasPrefix(string(tt.input), "Content-Type: multipart/") {
					t.Errorf("did not expect MIME wrapper for this input type")
				}
			}
		})
	}
}

func TestIsIgnitionConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected bool
	}{
		{
			name:     "empty input",
			input:    []byte{},
			expected: false,
		},
		{
			name:     "valid ignition config",
			input:    []byte(sampleIgnitionConfig),
			expected: true,
		},
		{
			name:     "ignition with whitespace",
			input:    []byte(`  { "ignition": {"version": "3.2.0"}}`),
			expected: true,
		},
		{
			name:     "cloud-config",
			input:    []byte(sampleCloudConfig),
			expected: false,
		},
		{
			name:     "random json",
			input:    []byte(`{"foo": "bar"}`),
			expected: false,
		},
		{
			name:     "plain text",
			input:    []byte("hello world"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsIgnitionConfig(tt.input)
			if result != tt.expected {
				t.Errorf("IsIgnitionConfig(%q) = %v, expected %v", string(tt.input), result, tt.expected)
			}
		})
	}
}

func TestUnwrapIgnitionFromMIME(t *testing.T) {
	// Test round-trip: wrap then unwrap
	originalConfig := []byte(sampleIgnitionConfig)

	wrapped := WrapIgnitionForNutanix(originalConfig)

	unwrapped, err := UnwrapIgnitionFromMIME(wrapped)
	if err != nil {
		t.Fatalf("failed to unwrap: %v", err)
	}

	if string(unwrapped) != string(originalConfig) {
		t.Errorf("round-trip failed:\noriginal: %s\nunwrapped: %s", string(originalConfig), string(unwrapped))
	}
}

func TestWrapIgnitionForNutanix_ValidMIMEStructure(t *testing.T) {
	result := WrapIgnitionForNutanix([]byte(sampleIgnitionConfig))

	// Decode the outer base64
	decoded, err := base64.StdEncoding.DecodeString(result)
	if err != nil {
		t.Fatalf("failed to decode base64: %v", err)
	}

	content := string(decoded)

	// Validate MIME structure
	requiredParts := []string{
		"Content-Type: multipart/mixed; boundary=",
		"MIME-Version: 1.0",
		"--IGNITION_MIME_BOUNDARY",
		"Content-Type: text/x-ignition",
		"Content-Transfer-Encoding: base64",
		"--IGNITION_MIME_BOUNDARY--",
	}

	for _, part := range requiredParts {
		if !strings.Contains(content, part) {
			t.Errorf("MIME message missing required part: %s\nFull content:\n%s", part, content)
		}
	}
}

// TestWrapIgnitionForNutanix_NoCloudConfigHeader verifies the wrapper does NOT
// include '#cloud-config' which would trigger Nutanix validation
func TestWrapIgnitionForNutanix_NoCloudConfigHeader(t *testing.T) {
	result := WrapIgnitionForNutanix([]byte(sampleIgnitionConfig))

	decoded, err := base64.StdEncoding.DecodeString(result)
	if err != nil {
		t.Fatalf("failed to decode base64: %v", err)
	}

	if strings.Contains(string(decoded), "#cloud-config") {
		t.Error("MIME wrapper should NOT contain #cloud-config header")
	}
}

// BenchmarkWrapIgnitionForNutanix benchmarks the wrapper function
func BenchmarkWrapIgnitionForNutanix(b *testing.B) {
	config := []byte(sampleIgnitionConfig)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = WrapIgnitionForNutanix(config)
	}
}
