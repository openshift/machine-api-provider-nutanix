package machine

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// This test simulates the exact scenario from OCPBUGS-123514
// where Nutanix v4.2 API rejects Ignition configs due to missing '#cloud-config' header

// TestOCPBUGS123514_IgnitionRejection simulates the bug scenario
func TestOCPBUGS123514_IgnitionRejection(t *testing.T) {
	// This is a sample Ignition config similar to what caused the failure
	// in OCPBUGS-123514
	ignitionConfig := `{"ignition":{"config":{"merge":[{"source":"https://10.6.197.152:22623/config/worker","verification":{}}],"replace":{"verification":{}}},"proxy":{},"security":{"tls":{"certificateAuthorities":[{"source":"data:text/plain;charset=utf-8;base64,LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t"}]}},"timeouts":{},"version":"3.2.0"},"passwd":{},"storage":{},"systemd":{}}`

	// Verify this is detected as Ignition config
	if !IsIgnitionConfig([]byte(ignitionConfig)) {
		t.Fatal("Failed to detect Ignition config")
	}

	// Simulate the OLD behavior (before fix) - would fail with:
	// "Guest customization missing Cloud-Init header: 31505"
	oldEncodedUserdata := base64.StdEncoding.EncodeToString([]byte(ignitionConfig))

	// Decode and check - this is what Nutanix v4.2 API would see
	oldDecoded, _ := base64.StdEncoding.DecodeString(oldEncodedUserdata)
	if strings.HasPrefix(string(oldDecoded), "#cloud-config") {
		t.Error("Old behavior incorrectly starts with #cloud-config")
	}
	if !strings.HasPrefix(string(oldDecoded), `{"ignition":`) {
		t.Error("Old behavior should start with Ignition JSON")
	}
	t.Logf("OLD behavior (before fix): Nutanix would reject this with 'Guest customization missing Cloud-Init header'")
	t.Logf("  Raw userdata starts with: %s...", string(oldDecoded)[:50])

	// Simulate the NEW behavior (after fix) - should work
	newEncodedUserdata := WrapIgnitionForNutanix([]byte(ignitionConfig))

	// Decode the outer layer - this is what Nutanix v4.2 API sees first
	newDecoded, err := base64.StdEncoding.DecodeString(newEncodedUserdata)
	if err != nil {
		t.Fatalf("Failed to decode new userdata: %v", err)
	}

	// Verify it's a valid MIME multipart message
	if !strings.Contains(string(newDecoded), "Content-Type: multipart/mixed") {
		t.Error("New behavior should be MIME multipart")
	}

	// Verify it explicitly declares Ignition content type
	if !strings.Contains(string(newDecoded), "text/x-ignition") {
		t.Error("New behavior should declare text/x-ignition content type")
	}

	// Verify there's NO '#cloud-config' header that would confuse Nutanix
	if strings.Contains(string(newDecoded), "#cloud-config") {
		t.Error("New behavior should NOT contain #cloud-config header")
	}

	t.Logf("NEW behavior (after fix): Nutanix will accept this MIME multipart message")
	t.Logf("  MIME message preview: %s...", string(newDecoded)[:100])

	// Verify round-trip - the Ignition config can be extracted back
	extracted, err := UnwrapIgnitionFromMIME(newEncodedUserdata)
	if err != nil {
		t.Fatalf("Failed to extract Ignition config from MIME: %v", err)
	}

	if string(extracted) != ignitionConfig {
		t.Error("Round-trip failed - extracted config doesn't match original")
	}

	t.Log("PASS: Fix successfully wraps Ignition config for Nutanix v4.2+ API compatibility")
}

// TestOCPBUGS123514_CloudInitPassthrough verifies cloud-config is not wrapped
func TestOCPBUGS123514_CloudInitPassthrough(t *testing.T) {
	cloudConfig := `#cloud-config
users:
  - name: core
    groups: wheel
runcmd:
  - echo "Hello from cloud-init"
`

	// Should NOT be detected as Ignition
	if IsIgnitionConfig([]byte(cloudConfig)) {
		t.Error("cloud-config incorrectly detected as Ignition")
	}

	// When wrapped, should remain as-is (just base64 encoded)
	wrapped := WrapIgnitionForNutanix([]byte(cloudConfig))
	decoded, _ := base64.StdEncoding.DecodeString(wrapped)

	// Should still start with #cloud-config
	if !strings.HasPrefix(string(decoded), "#cloud-config") {
		t.Error("cloud-config should pass through unchanged")
	}

	t.Log("PASS: cloud-config passes through without MIME wrapping")
}

// TestOCPBUGS123514_IgnitionVersions tests various Ignition config versions
func TestOCPBUGS123514_IgnitionVersions(t *testing.T) {
	ignitionVersions := []struct {
		name    string
		version string
	}{
		{"Ignition 3.0.0", "3.0.0"},
		{"Ignition 3.1.0", "3.1.0"},
		{"Ignition 3.2.0", "3.2.0"},
		{"Ignition 3.3.0", "3.3.0"},
		{"Ignition 3.4.0", "3.4.0"},
	}

	for _, tc := range ignitionVersions {
		t.Run(tc.name, func(t *testing.T) {
			config := map[string]interface{}{
				"ignition": map[string]interface{}{
					"version": tc.version,
				},
				"storage": map[string]interface{}{},
				"systemd": map[string]interface{}{},
			}

			configBytes, err := json.Marshal(config)
			if err != nil {
				t.Fatalf("Failed to marshal config: %v", err)
			}

			if !IsIgnitionConfig(configBytes) {
				t.Errorf("Failed to detect %s config", tc.name)
			}

			wrapped := WrapIgnitionForNutanix(configBytes)
			decoded, _ := base64.StdEncoding.DecodeString(wrapped)

			if !strings.Contains(string(decoded), "text/x-ignition") {
				t.Errorf("%s config not properly wrapped", tc.name)
			}

			// Verify round-trip
			extracted, err := UnwrapIgnitionFromMIME(wrapped)
			if err != nil {
				t.Fatalf("Failed to unwrap %s: %v", tc.name, err)
			}

			if string(extracted) != string(configBytes) {
				t.Errorf("%s round-trip failed", tc.name)
			}
		})
	}
}

// TestOCPBUGS123514_LargeIgnitionConfig tests handling of large Ignition configs
func TestOCPBUGS123514_LargeIgnitionConfig(t *testing.T) {
	// Create a large Ignition config similar to real OpenShift deployments
	config := map[string]interface{}{
		"ignition": map[string]interface{}{
			"version": "3.2.0",
			"config": map[string]interface{}{
				"merge": []map[string]interface{}{
					{"source": "https://api.cluster.example.com:22623/config/worker"},
				},
			},
			"security": map[string]interface{}{
				"tls": map[string]interface{}{
					"certificateAuthorities": []map[string]interface{}{
						{
							// Simulate a large CA certificate
							"source": "data:text/plain;base64," + strings.Repeat("A", 4000),
						},
					},
				},
			},
		},
		"storage": map[string]interface{}{
			"files": make([]interface{}, 0),
		},
	}

	// Add some files to make it larger
	files := config["storage"].(map[string]interface{})["files"].([]interface{})
	for i := 0; i < 10; i++ {
		files = append(files, map[string]interface{}{
			"path":     "/etc/test-file-" + string(rune('a'+i)),
			"contents": map[string]interface{}{"source": "data:," + strings.Repeat("x", 1000)},
		})
	}
	config["storage"].(map[string]interface{})["files"] = files

	configBytes, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	t.Logf("Testing large config: %d bytes", len(configBytes))

	wrapped := WrapIgnitionForNutanix(configBytes)

	// Verify it can be unwrapped
	extracted, err := UnwrapIgnitionFromMIME(wrapped)
	if err != nil {
		t.Fatalf("Failed to unwrap large config: %v", err)
	}

	if string(extracted) != string(configBytes) {
		t.Error("Large config round-trip failed")
	}

	t.Logf("PASS: Large Ignition config (%d bytes) handled correctly", len(configBytes))
}
