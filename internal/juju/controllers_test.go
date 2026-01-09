// Copyright 2025 Canonical Ltd.
// Licensed under the Apache License, Version 2.0, see LICENCE file for details.

package juju

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildConstraintsString(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected []string // Expected to contain these strings
	}{
		{
			name: "single constraint",
			input: map[string]string{
				"arch": "amd64",
			},
			expected: []string{"arch=amd64"},
		},
		{
			name: "multiple constraints",
			input: map[string]string{
				"arch": "amd64",
				"mem":  "4G",
			},
			expected: []string{"arch=amd64", "mem=4G"},
		},
		{
			name:     "empty constraints",
			input:    map[string]string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildConstraintsString(tt.input)

			if len(tt.expected) == 0 {
				assert.Equal(t, "", result)
			} else {
				// Check that all expected key=value pairs are present
				for _, exp := range tt.expected {
					assert.Contains(t, result, exp)
				}
			}
		})
	}
}

func TestConvertToCloudAuthTypes(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected int // expected length
	}{
		{
			name:     "single auth type",
			input:    []string{"userpass"},
			expected: 1,
		},
		{
			name:     "multiple auth types",
			input:    []string{"userpass", "oauth2", "certificate"},
			expected: 3,
		},
		{
			name:     "empty auth types",
			input:    []string{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToCloudAuthTypes(tt.input)
			assert.Equal(t, tt.expected, len(result))

			// Verify each converted auth type
			for i, authType := range tt.input {
				assert.Equal(t, authType, string(result[i]))
			}
		})
	}
}

func TestBuildBootstrapArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        BootstrapArguments
		configPath  string
		contains    []string // strings that should be in the result
		notContains []string // strings that should not be in the result
	}{
		{
			name: "minimal bootstrap args",
			args: BootstrapArguments{
				Name: "test-controller",
				Cloud: BootstrapCloudArgument{
					Name: "lxd",
				},
			},
			contains:    []string{"bootstrap", "lxd", "test-controller"},
			notContains: []string{"--agent-version", "--admin-secret"},
		},
		{
			name: "bootstrap with version",
			args: BootstrapArguments{
				Name: "test-controller",
				Cloud: BootstrapCloudArgument{
					Name: "lxd",
				},
				Flags: BootstrapFlags{
					AgentVersion: "3.6.12",
				},
			},
			contains: []string{"bootstrap", "lxd", "test-controller", "--agent-version=3.6.12"},
		},
		{
			name: "bootstrap with config file",
			args: BootstrapArguments{
				Name: "test-controller",
				Cloud: BootstrapCloudArgument{
					Name: "lxd",
				},
			},
			configPath: "/tmp/config.yaml",
			contains:   []string{"bootstrap", "lxd", "test-controller", "--config", "/tmp/config.yaml"},
		},
		{
			name: "bootstrap with constraints",
			args: BootstrapArguments{
				Name: "test-controller",
				Cloud: BootstrapCloudArgument{
					Name: "lxd",
				},
				Flags: BootstrapFlags{
					BootstrapConstraints: "arch=amd64,mem=4G",
				},
			},
			contains: []string{"bootstrap", "lxd", "test-controller", "--bootstrap-constraints=arch=amd64,mem=4G"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildBootstrapArgs(tt.args, tt.configPath)
			assert.NoError(t, err)
			resultStr := ""
			for _, arg := range result {
				resultStr += arg + " "
			}

			for _, expected := range tt.contains {
				assert.Contains(t, resultStr, expected, "Expected to find %q in bootstrap args", expected)
			}

			for _, notExpected := range tt.notContains {
				assert.NotContains(t, resultStr, notExpected, "Expected not to find %q in bootstrap args", notExpected)
			}
		})
	}
}

func TestBuildJujuCloud(t *testing.T) {
	tests := []struct {
		name  string
		input BootstrapCloudArgument
	}{
		{
			name: "basic cloud",
			input: BootstrapCloudArgument{
				Name:      "test-cloud",
				Type:      "manual",
				AuthTypes: []string{"empty"},
			},
		},
		{
			name: "cloud with region",
			input: BootstrapCloudArgument{
				Name:      "test-cloud",
				Type:      "openstack",
				AuthTypes: []string{"userpass"},
				Region: &BootstrapCloudRegionArgument{
					Name:     "region1",
					Endpoint: "https://region1.example.com",
				},
			},
		},
		{
			name: "cloud with config",
			input: BootstrapCloudArgument{
				Name:      "test-cloud",
				Type:      "kubernetes",
				AuthTypes: []string{"certificate"},
				Config: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildJujuCloud(tt.input)

			assert.Equal(t, tt.input.Name, result.Name)
			assert.Equal(t, tt.input.Type, result.Type)
			assert.Equal(t, len(tt.input.AuthTypes), len(result.AuthTypes))

			if tt.input.Region != nil {
				assert.Equal(t, 1, len(result.Regions))
				assert.Equal(t, tt.input.Region.Name, result.Regions[0].Name)
			}

			if tt.input.Config != nil {
				assert.Equal(t, len(tt.input.Config), len(result.Config))
			}
		})
	}
}

func TestBuildJujuCredential(t *testing.T) {
	tests := []struct {
		name  string
		input BootstrapCredentialArgument
	}{
		{
			name: "basic credential",
			input: BootstrapCredentialArgument{
				Name:     "test-cred",
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "admin",
					"password": "secret",
				},
			},
		},
		{
			name: "certificate credential",
			input: BootstrapCredentialArgument{
				Name:     "cert-cred",
				AuthType: "certificate",
				Attributes: map[string]string{
					"client-cert": "cert-data",
					"client-key":  "key-data",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildJujuCredential(tt.input)

			assert.Equal(t, tt.input.AuthType, string(result.AuthType()))
			assert.Equal(t, tt.input.Attributes, result.Attributes())
		})
	}
}

func TestBootstrapIntegration(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "juju-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a mock juju binary that logs all commands it receives
	mockJujuPath := filepath.Join(tmpDir, "mock-juju")
	logFilePath := filepath.Join(tmpDir, "juju-commands.log")

	// Create the mock script that echoes all commands to a log file
	mockScript := fmt.Sprintf(`#!/bin/bash
# Mock juju binary that logs commands
echo "$@" >> %s

# Handle different commands
case "$1" in
  "update-public-clouds")
    exit 0
    ;;
  "bootstrap")
    # Create mock controller data in JUJU_DATA
    if [ -z "$JUJU_DATA" ]; then
      echo "Error: JUJU_DATA not set" >&2
      exit 1
    fi
    
    # Extract controller name (last argument)
    CONTROLLER_NAME="${@: -1}"
    
    # Create controllers.yaml
    mkdir -p "$JUJU_DATA"
    cat > "$JUJU_DATA/controllers.yaml" <<EOF
controllers:
  $CONTROLLER_NAME:
    uuid: test-uuid-12345
    api-endpoints: ["127.0.0.1:17070"]
    ca-cert: |
      -----BEGIN CERTIFICATE-----
      TESTCACERT
      -----END CERTIFICATE-----
EOF
    
    # Create accounts.yaml
    cat > "$JUJU_DATA/accounts.yaml" <<EOF
controllers:
  $CONTROLLER_NAME:
    user: admin
    password: test-password-12345
EOF
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`, logFilePath)

	err = os.WriteFile(mockJujuPath, []byte(mockScript), 0755)
	assert.NoError(t, err)

	// Create a DefaultJujuCommand with the mock binary
	cmd, err := NewDefaultJujuCommand(mockJujuPath)
	assert.NoError(t, err)

	// Prepare bootstrap arguments
	bootstrapArgs := BootstrapArguments{
		Name:       "test-controller",
		JujuBinary: mockJujuPath,
		Cloud: BootstrapCloudArgument{
			Name:      "test-cloud",
			Type:      "manual",
			AuthTypes: []string{"empty"},
			Endpoint:  "https://test.example.com",
		},
		CloudCredential: BootstrapCredentialArgument{
			Name:     "test-cred",
			AuthType: "empty",
			Attributes: map[string]string{
				"endpoint": "https://test.example.com",
			},
		},
		Config: BootstrapConfig{
			ControllerConfig: map[string]string{
				"test-key": "test-value",
			},
		},
		Flags: BootstrapFlags{
			AgentVersion: "3.6.0",
		},
	}

	// Run bootstrap
	ctx := context.Background()
	result, err := cmd.Bootstrap(ctx, bootstrapArgs)

	// Verify the result
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, []string{"127.0.0.1:17070"}, result.Addresses)
	assert.Contains(t, result.CACert, "TESTCACERT")
	assert.Equal(t, "admin", result.Username)
	assert.Equal(t, "test-password-12345", result.Password)

	// Verify commands were logged
	logContent, err := os.ReadFile(logFilePath)
	assert.NoError(t, err)

	logStr := string(logContent)
	// Check that update-public-clouds was called
	assert.Contains(t, logStr, "update-public-clouds --client")
	// Check that bootstrap was called with the controller name
	assert.Contains(t, logStr, "bootstrap")
	assert.Contains(t, logStr, "test-controller")
	// Check that flags were passed
	assert.Contains(t, logStr, "--agent-version=3.6.0")
}

