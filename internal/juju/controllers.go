// Copyright 2025 Canonical Ltd.
// Licensed under the Apache License, Version 2.0, see LICENCE file for details.

package juju

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/version/v2"
	"gopkg.in/yaml.v2"
)

// ControllerConnectionInformation contains the connection details for a controller.
type ControllerConnectionInformation struct {
	Addresses []string
	CACert    string
	Username  string
	Password  string
}

// commandRunner manages command execution with environment variables and logging.
type commandRunner struct {
	jujuBinary  string
	logFilePath string
	envVars     map[string]string
}

// newCommandRunner creates a new command runner with a log file in the specified directory.
func newCommandRunner(jujuBinary, workDir string) (*commandRunner, error) {
	logFile, err := os.CreateTemp(workDir, "juju-bootstrap-log-*.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}
	logFilePath := logFile.Name()
	logFile.Close()

	return &commandRunner{
		jujuBinary:  jujuBinary,
		logFilePath: logFilePath,
		envVars:     make(map[string]string),
	}, nil
}

// setEnv sets an environment variable for command execution.
func (r *commandRunner) setEnv(key, value string) {
	r.envVars[key] = value
}

// run executes a juju command with the configured environment and logging.
func (r *commandRunner) run(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, r.jujuBinary, args...)

	// Open log file in append mode
	logFile, err := os.OpenFile(r.logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	// Write command being executed to log
	if _, err := logFile.WriteString(fmt.Sprintf("\n=== Executing: %s %s ===\n", r.jujuBinary, strings.Join(args, " "))); err != nil {
		return fmt.Errorf("failed to write to log file: %w", err)
	}

	// Redirect stdout and stderr to log file
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Build environment from current environment plus custom vars
	cmd.Env = os.Environ()
	for k, v := range r.envVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command failed (see log file %s): %w", r.logFilePath, err)
	}

	return nil
}

// BootstrapConfig contains configuration options that can be written to a config file.
type BootstrapConfig struct {
	// Controller configuration
	ControllerConfig map[string]string `yaml:"controller-config,omitempty"`
	// Model defaults
	ModelDefaults map[string]string `yaml:"model-defaults,omitempty"`
	// Storage pool configuration
	StoragePool map[string]string `yaml:"storage-pool,omitempty"`
}

// BootstrapFlags contains CLI flags for the bootstrap command.
type BootstrapFlags struct {
	AgentVersion              string   `flag:"agent-version"`
	BootstrapBase             string   `flag:"bootstrap-series"`
	BootstrapTimeout          string   `flag:"timeout"`
	CAPrivateKey              string   `flag:"ca-private-key"`
	SSHServerHostKey          string   `flag:"ssh-server-host-key"`
	AdminSecret               string   `flag:"admin-secret"`
	ControllerExternalName    string   `flag:"controller-external-name"`
	ControllerServiceType     string   `flag:"controller-service-type"`
	ControllerExternalIPAddrs []string `flag:"controller-external-ips"`
	BootstrapConstraints      string   `flag:"bootstrap-constraints"`
	ModelConstraints          string   `flag:"constraints"`
}

// BootstrapArguments contains all the arguments needed for bootstrap.
type BootstrapArguments struct {
	Name            string
	JujuBinary      string
	Cloud           BootstrapCloudArgument
	CloudCredential BootstrapCredentialArgument
	Config          BootstrapConfig
	Flags           BootstrapFlags
}

// BootstrapCloudArgument contains cloud configuration for bootstrap.
type BootstrapCloudArgument struct {
	Name            string
	AuthTypes       []string
	CACertificates  []string
	Config          map[string]string
	Endpoint        string
	HostCloudRegion string
	Region          *BootstrapCloudRegionArgument
	Type            string
	K8sConfig       string
}

// BootstrapCloudRegionArgument contains cloud region configuration.
type BootstrapCloudRegionArgument struct {
	Name             string
	Endpoint         string
	IdentityEndpoint string
	StorageEndpoint  string
}

// BootstrapCredentialArgument contains credential configuration for bootstrap.
type BootstrapCredentialArgument struct {
	Name       string
	AuthType   string
	Attributes map[string]string
}

// DefaultJujuCommand is the default implementation of JujuCommand.
type DefaultJujuCommand struct {
	jujuBinary string
}

// NewDefaultJujuCommand creates a new DefaultJujuCommand instance.
func NewDefaultJujuCommand(jujuBinary string) (*DefaultJujuCommand, error) {
	return &DefaultJujuCommand{jujuBinary: jujuBinary}, nil
}

// Bootstrap creates a new controller and returns connection information.
func (d *DefaultJujuCommand) Bootstrap(ctx context.Context, args BootstrapArguments) (*ControllerConnectionInformation, error) {
	// Validate arguments
	if args.Name == "" {
		return nil, fmt.Errorf("controller name cannot be empty")
	}
	if args.Cloud.Name == "" {
		return nil, fmt.Errorf("cloud name cannot be empty")
	}
	if args.CloudCredential.Name == "" {
		return nil, fmt.Errorf("credential name cannot be empty")
	}
	if args.Flags.AgentVersion != "" {
		if _, err := version.ParseBinary(args.Flags.AgentVersion); err != nil {
			if _, err := version.Parse(args.Flags.AgentVersion); err != nil {
				return nil, fmt.Errorf("invalid agent version %q: %w", args.Flags.AgentVersion, err)
			}
		}
	}

	// Create temporary JUJU_DATA directory
	tmpDir, err := os.MkdirTemp("", "juju-bootstrap-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary JUJU_DATA directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create command runner with log file in tmpDir for better organization
	runner, err := newCommandRunner(d.jujuBinary, tmpDir)
	if err != nil {
		return nil, err
	}

	// Set JUJU_DATA environment variable for command execution only
	runner.setEnv("JUJU_DATA", tmpDir)
	osenv.SetJujuXDGDataHome(tmpDir)

	// Log the bootstrap log file path for debugging
	fmt.Printf("Bootstrap log file: %s\n", runner.logFilePath)

	// Update public clouds
	if err := runner.run(ctx, "update-public-clouds", "--client"); err != nil {
		return nil, fmt.Errorf("failed to update public clouds: %w", err)
	}

	// Setup cloud
	cloudName, regionName := splitCloudNameAndRegion(args.Cloud.Name)
	isPublicCloud, err := isValidPublicCloud(cloudName, regionName)
	if err != nil {
		return nil, fmt.Errorf("failed to validate cloud: %w", err)
	}

	if !isPublicCloud {
		// Create personal cloud
		cloud := buildJujuCloud(args.Cloud)
		if err := jujucloud.WritePersonalCloudMetadata(map[string]jujucloud.Cloud{
			cloudName: cloud,
		}); err != nil {
			return nil, fmt.Errorf("failed to write personal cloud metadata: %w", err)
		}
	}

	// Setup credentials
	store := jujuclient.NewFileClientStore()
	credentialName := args.CloudCredential.Name
	if credentialName == "" {
		credentialName = cloudName
	}
	cloudCred := jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			credentialName: buildJujuCredential(args.CloudCredential),
		},
	}
	if err := store.UpdateCredential(cloudName, cloudCred); err != nil {
		return nil, fmt.Errorf("failed to update credential: %w", err)
	}

	// Write config file
	configFilePath, err := writeBootstrapConfig(tmpDir, args.Config)
	if err != nil {
		return nil, err
	}

	// Build bootstrap command arguments
	bootstrapArgs, err := buildBootstrapArgs(args, configFilePath)
	if err != nil {
		return nil, err
	}

	// Execute bootstrap command
	if err := runner.run(ctx, bootstrapArgs...); err != nil {
		return nil, fmt.Errorf("bootstrap failed: %w", err)
	}

	// Read controller information from the client store
	controllerDetails, err := store.ControllerByName(args.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to read controller details from client store: %w", err)
	}

	accountDetails, err := store.AccountDetails(args.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to read account details from client store: %w", err)
	}

	return &ControllerConnectionInformation{
		Addresses: controllerDetails.APIEndpoints,
		CACert:    controllerDetails.CACert,
		Username:  accountDetails.User,
		Password:  accountDetails.Password,
	}, nil
}

// UpdateConfig updates controller configuration.
func (d *DefaultJujuCommand) UpdateConfig(ctx context.Context, connInfo *ControllerConnectionInformation, config map[string]string) error {
	// TODO: Implement config update logic
	return fmt.Errorf("update config not implemented")
}

// Config retrieves controller configuration settings.
func (d *DefaultJujuCommand) Config(ctx context.Context, connInfo *ControllerConnectionInformation) (map[string]string, error) {
	// TODO: Implement read logic
	return nil, fmt.Errorf("read not implemented")
}

// Destroy removes the controller.
func (d *DefaultJujuCommand) Destroy(ctx context.Context, connInfo *ControllerConnectionInformation) error {
	// TODO: Implement destroy logic
	return fmt.Errorf("not implemented")
}

// writeBootstrapConfig writes the bootstrap config to a YAML file.
func writeBootstrapConfig(workDir string, config BootstrapConfig) (string, error) {
	// Skip if config is empty
	if len(config.ControllerConfig) == 0 && len(config.ModelDefaults) == 0 && len(config.StoragePool) == 0 {
		return "", nil
	}

	configFilePath := filepath.Join(workDir, "bootstrap-config.yaml")
	data, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configFilePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write config file: %w", err)
	}

	return configFilePath, nil
}

// buildBootstrapArgs constructs the bootstrap command arguments from BootstrapArguments using reflection for flags.
func buildBootstrapArgs(args BootstrapArguments, configFilePath string) ([]string, error) {
	cmdArgs := []string{"bootstrap"}

	// Add flags using reflection
	flagsValue := reflect.ValueOf(args.Flags)
	flagsType := reflect.TypeOf(args.Flags)

	for i := 0; i < flagsType.NumField(); i++ {
		field := flagsType.Field(i)
		flagTag := field.Tag.Get("flag")
		if flagTag == "" {
			continue
		}

		fieldValue := flagsValue.Field(i)

		// Handle different types
		switch fieldValue.Kind() {
		case reflect.String:
			if str := fieldValue.String(); str != "" {
				cmdArgs = append(cmdArgs, fmt.Sprintf("--%s=%s", flagTag, str))
			}
		case reflect.Slice:
			if fieldValue.Len() > 0 {
				// For slices, add multiple flags
				for j := 0; j < fieldValue.Len(); j++ {
					item := fieldValue.Index(j)
					if item.Kind() == reflect.String {
						cmdArgs = append(cmdArgs, fmt.Sprintf("--%s=%s", flagTag, item.String()))
					}
				}
			}
		default:
			// Log unhandled field types for debugging
			if !fieldValue.IsZero() {
				fmt.Printf("Warning: unhandled flag field type %s for flag %s\n", fieldValue.Kind(), flagTag)
			}
		}
	}

	// Add config file if it exists
	if configFilePath != "" {
		cmdArgs = append(cmdArgs, "--config", configFilePath)
	}

	// Add cloud name and controller name (must be at the end)
	cmdArgs = append(cmdArgs, args.Cloud.Name, args.Name)

	return cmdArgs, nil
}

// buildConstraintsString converts a constraints map to a comma-separated string.
func buildConstraintsString(constraints map[string]string) string {
	var parts []string
	for k, v := range constraints {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, ",")
}

// buildJujuCloud constructs a jujucloud.Cloud from BootstrapCloudArgument.
func buildJujuCloud(cloudArg BootstrapCloudArgument) jujucloud.Cloud {
	// Convert string map to interface map for Config
	config := make(map[string]interface{})
	for k, v := range cloudArg.Config {
		config[k] = v
	}

	cloud := jujucloud.Cloud{
		Name:            cloudArg.Name,
		Type:            cloudArg.Type,
		AuthTypes:       convertToCloudAuthTypes(cloudArg.AuthTypes),
		Endpoint:        cloudArg.Endpoint,
		HostCloudRegion: cloudArg.HostCloudRegion,
		Config:          config,
		CACertificates:  cloudArg.CACertificates,
	}

	if cloudArg.Region != nil {
		cloud.Regions = []jujucloud.Region{
			{
				Name:             cloudArg.Region.Name,
				Endpoint:         cloudArg.Region.Endpoint,
				IdentityEndpoint: cloudArg.Region.IdentityEndpoint,
				StorageEndpoint:  cloudArg.Region.StorageEndpoint,
			},
		}
	}

	return cloud
}

// buildJujuCredential constructs a jujucloud.Credential from BootstrapCredentialArgument.
func buildJujuCredential(credArg BootstrapCredentialArgument) jujucloud.Credential {
	return jujucloud.NewCredential(jujucloud.AuthType(credArg.AuthType), credArg.Attributes)
}

// convertToCloudAuthTypes converts string auth types to jujucloud.AuthType.
func convertToCloudAuthTypes(authTypes []string) []jujucloud.AuthType {
	result := make([]jujucloud.AuthType, len(authTypes))
	for i, authType := range authTypes {
		result[i] = jujucloud.AuthType(authType)
	}
	return result
}

// splitCloudNameAndRegion splits a cloud name that may contain a region (e.g., "aws/us-east-1").
func splitCloudNameAndRegion(cloudNameAndRegion string) (cloudName string, regionName string) {
	if i := strings.IndexRune(cloudNameAndRegion, '/'); i > 0 {
		cloudName, regionName = cloudNameAndRegion[:i], cloudNameAndRegion[i+1:]
	} else {
		cloudName = cloudNameAndRegion
	}
	return
}

// isValidPublicCloud checks if the cloud name (and possibly region) is a valid public cloud.
func isValidPublicCloud(cloudName, regionName string) (bool, error) {
	pubClouds, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	if err != nil {
		return false, fmt.Errorf("failed to get public cloud metadata: %w", err)
	}

	for pubCloudName, cloud := range pubClouds {
		if cloudName == pubCloudName {
			if regionName != "" {
				exists := slices.ContainsFunc(cloud.Regions, func(r jujucloud.Region) bool {
					return regionName == r.Name
				})
				if !exists {
					return false, fmt.Errorf("invalid public cloud region for cloud %s with region %s", cloudName, regionName)
				}
			}
			return true, nil
		}
	}

	return false, nil
}
