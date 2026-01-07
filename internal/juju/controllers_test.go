// Copyright 2025 Canonical Ltd.
// Licensed under the Apache License, Version 2.0, see LICENCE file for details.

package juju

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitCloudNameAndRegion(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedCloud  string
		expectedRegion string
	}{
		{
			name:           "cloud with region",
			input:          "aws/us-east-1",
			expectedCloud:  "aws",
			expectedRegion: "us-east-1",
		},
		{
			name:           "cloud without region",
			input:          "lxd",
			expectedCloud:  "lxd",
			expectedRegion: "",
		},
		{
			name:           "cloud with multiple slashes (only first is separator)",
			input:          "openstack/region/test",
			expectedCloud:  "openstack",
			expectedRegion: "region/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloudName, regionName := splitCloudNameAndRegion(tt.input)
			assert.Equal(t, tt.expectedCloud, cloudName)
			assert.Equal(t, tt.expectedRegion, regionName)
		})
	}
}

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
				AgentVersion: "3.6.12",
			},
			contains: []string{"bootstrap", "lxd", "test-controller", "--agent-version=3.6.12"},
		},
		{
			name: "bootstrap with admin secret",
			args: BootstrapArguments{
				Name: "test-controller",
				Cloud: BootstrapCloudArgument{
					Name: "lxd",
				},
				AdminSecret: "secret123",
			},
			contains: []string{"bootstrap", "lxd", "test-controller", "--admin-secret=secret123"},
		},
		{
			name: "bootstrap with config",
			args: BootstrapArguments{
				Name: "test-controller",
				Cloud: BootstrapCloudArgument{
					Name: "lxd",
				},
				Config: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			contains: []string{"bootstrap", "lxd", "test-controller", "--config", "key1=value1", "key2=value2"},
		},
		{
			name: "bootstrap with constraints",
			args: BootstrapArguments{
				Name: "test-controller",
				Cloud: BootstrapCloudArgument{
					Name: "lxd",
				},
				BootstrapConstraints: map[string]string{
					"arch": "amd64",
					"mem":  "4G",
				},
			},
			contains: []string{"bootstrap", "lxd", "test-controller", "--bootstrap-constraints="},
		},
		{
			name: "bootstrap with external IPs",
			args: BootstrapArguments{
				Name: "test-controller",
				Cloud: BootstrapCloudArgument{
					Name: "lxd",
				},
				ControllerExternalIPAddrs: []string{"192.168.1.1", "192.168.1.2"},
			},
			contains: []string{"bootstrap", "lxd", "test-controller", "--controller-external-ips=192.168.1.1", "--controller-external-ips=192.168.1.2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildBootstrapArgs(tt.args)
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
