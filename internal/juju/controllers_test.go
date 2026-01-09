// Copyright 2025 Canonical Ltd.
// Licensed under the Apache License, Version 2.0, see LICENCE file for details.

package juju

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/juju/juju/osenv"
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

// mockCommandRunner is a mock implementation of CommandRunner for testing.
type mockCommandRunner struct {
	commands    [][]string // Tracks all commands that were run
	envVars     map[string]string
	shouldFail  bool
	logFilePath string
}

func newMockCommandRunner() *mockCommandRunner {
	return &mockCommandRunner{
		commands:    make([][]string, 0),
		envVars:     make(map[string]string),
		logFilePath: filepath.Join(os.TempDir(), "mock-log.txt"),
	}
}

func (m *mockCommandRunner) SetEnv(key, value string) {
	m.envVars[key] = value
}

func (m *mockCommandRunner) Run(ctx context.Context, args ...string) error {
	m.commands = append(m.commands, args)
	
	if m.shouldFail {
		return fmt.Errorf("mock command failed")
	}

	// For bootstrap command, create mock controller data
	if len(args) > 0 && args[0] == "bootstrap" {
		jujuData := m.envVars["JUJU_DATA"]
		if jujuData == "" {
			return fmt.Errorf("JUJU_DATA not set")
		}

		// Extract controller name (last argument)
		controllerName := args[len(args)-1]

		// Create controllers.yaml
		controllersYAML := fmt.Sprintf(`controllers:
  %s:
    uuid: test-uuid-12345
    api-endpoints: ["127.0.0.1:17070"]
    ca-cert: |
      -----BEGIN CERTIFICATE-----
      TESTCACERT
      -----END CERTIFICATE-----
`, controllerName)
		
		if err := os.MkdirAll(jujuData, 0755); err != nil {
			return fmt.Errorf("failed to create JUJU_DATA directory: %w", err)
		}
		
		if err := os.WriteFile(filepath.Join(jujuData, "controllers.yaml"), []byte(controllersYAML), 0644); err != nil {
			return fmt.Errorf("failed to write controllers.yaml: %w", err)
		}

		// Create accounts.yaml
		accountsYAML := fmt.Sprintf(`controllers:
  %s:
    user: admin
    password: test-password-12345
`, controllerName)
		
		if err := os.WriteFile(filepath.Join(jujuData, "accounts.yaml"), []byte(accountsYAML), 0644); err != nil {
			return fmt.Errorf("failed to write accounts.yaml: %w", err)
		}
	}

	return nil
}

func (m *mockCommandRunner) LogFilePath() string {
	return m.logFilePath
}

func TestPerformBootstrap(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "juju-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Set JUJU_DATA for the test
	oldJujuData := osenv.SetJujuXDGDataHome(tmpDir)
	defer func() {
		osenv.SetJujuXDGDataHome(oldJujuData)
	}()

	// Create mock command runner
	mockRunner := newMockCommandRunner()
	mockRunner.SetEnv("JUJU_DATA", tmpDir)

	// Prepare bootstrap arguments
	bootstrapArgs := BootstrapArguments{
		Name: "test-controller",
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

	// Run performBootstrap
	ctx := context.Background()
	result, err := performBootstrap(ctx, bootstrapArgs, tmpDir, mockRunner)

	// Verify the result
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, []string{"127.0.0.1:17070"}, result.Addresses)
	assert.Contains(t, result.CACert, "TESTCACERT")
	assert.Equal(t, "admin", result.Username)
	assert.Equal(t, "test-password-12345", result.Password)

	// Verify commands were executed
	assert.GreaterOrEqual(t, len(mockRunner.commands), 2, "Expected at least 2 commands to be executed")
	
	// Check that update-public-clouds was called
	assert.Equal(t, []string{"update-public-clouds", "--client"}, mockRunner.commands[0])
	
	// Check that bootstrap was called
	assert.Equal(t, "bootstrap", mockRunner.commands[1][0])
	assert.Contains(t, mockRunner.commands[1], "test-controller")
	assert.Contains(t, mockRunner.commands[1], "--agent-version=3.6.0")

	// Verify JUJU_DATA was set
	assert.Equal(t, tmpDir, mockRunner.envVars["JUJU_DATA"])
}

