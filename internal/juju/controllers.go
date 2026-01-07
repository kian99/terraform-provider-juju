// Copyright 2025 Canonical Ltd.
// Licensed under the Apache License, Version 2.0, see LICENCE file for details.

package juju

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/version/v2"
)

// ControllerConnectionInformation contains the connection details for a controller.
type ControllerConnectionInformation struct {
	Addresses []string
	CACert    string
	Username  string
	Password  string
}

// BootstrapArguments contains all the arguments needed for bootstrap.
type BootstrapArguments struct {
	AdminSecret               string
	AgentVersion              string
	BootstrapBase             string
	BootstrapConstraints      map[string]string
	BootstrapTimeout          string
	CAPrivateKey              string
	Cloud                     BootstrapCloudArgument
	CloudCredential           BootstrapCredentialArgument
	Config                    map[string]string
	ControllerExternalIPAddrs []string
	ControllerExternalName    string
	ControllerServiceType     string
	JujuBinary                string
	ModelConstraints          map[string]string
	ModelDefault              map[string]string
	Name                      string
	SSHServerHostKey          string
	StoragePool               map[string]string
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
	if args.AgentVersion != "" {
		if _, err := version.ParseBinary(args.AgentVersion); err != nil {
			if _, err := version.Parse(args.AgentVersion); err != nil {
				return nil, fmt.Errorf("invalid agent version %q: %w", args.AgentVersion, err)
			}
		}
	}

	// Create temporary JUJU_DATA directory
	tmpDir, err := os.MkdirTemp("", "juju-bootstrap-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary JUJU_DATA directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set JUJU_DATA environment variable for this operation
	oldJujuData := os.Getenv("JUJU_DATA")
	osenv.SetJujuXDGDataHome(tmpDir)
	defer func() {
		if oldJujuData != "" {
			os.Setenv("JUJU_DATA", oldJujuData)
		} else {
			os.Unsetenv("JUJU_DATA")
		}
	}()

	// Create log file for bootstrap output
	logFile, err := os.CreateTemp("", "juju-bootstrap-log-*.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}
	logFilePath := logFile.Name()
	logFile.Close()
	// Keep log file for debugging - don't delete it

	// Update public clouds
	if err := d.runJujuCommand(ctx, logFilePath, "update-public-clouds", "--client"); err != nil {
		return nil, fmt.Errorf("failed to update public clouds (see log file %s): %w", logFilePath, err)
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
	cloudCred := jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			cloudName: buildJujuCredential(args.CloudCredential),
		},
	}
	if err := store.UpdateCredential(cloudName, cloudCred); err != nil {
		return nil, fmt.Errorf("failed to update credential: %w", err)
	}

	// Build bootstrap command arguments
	bootstrapArgs := buildBootstrapArgs(args)

	// Execute bootstrap command
	if err := d.runJujuCommand(ctx, logFilePath, bootstrapArgs...); err != nil {
		return nil, fmt.Errorf("bootstrap failed (see log file %s): %w", logFilePath, err)
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

// runJujuCommand executes a juju command with the given arguments and redirects output to a log file.
func (d *DefaultJujuCommand) runJujuCommand(ctx context.Context, logFilePath string, args ...string) error {
	cmd := exec.CommandContext(ctx, d.jujuBinary, args...)
	
	// Open log file in append mode
	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	// Write command being executed to log
	logFile.WriteString(fmt.Sprintf("\n=== Executing: %s %s ===\n", d.jujuBinary, strings.Join(args, " ")))
	
	// Redirect stdout and stderr to log file
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	
	// Set environment
	cmd.Env = os.Environ()
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	
	return nil
}

// buildBootstrapArgs constructs the bootstrap command arguments from BootstrapArguments.
func buildBootstrapArgs(args BootstrapArguments) []string {
	cmdArgs := []string{"bootstrap"}

	// Add optional flags
	if args.AgentVersion != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--agent-version=%s", args.AgentVersion))
	}

	if args.BootstrapBase != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--bootstrap-series=%s", args.BootstrapBase))
	}

	if args.BootstrapTimeout != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--timeout=%s", args.BootstrapTimeout))
	}

	if args.CAPrivateKey != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--ca-private-key=%s", args.CAPrivateKey))
	}

	if args.SSHServerHostKey != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--ssh-server-host-key=%s", args.SSHServerHostKey))
	}

	if args.AdminSecret != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--admin-secret=%s", args.AdminSecret))
	}

	if args.ControllerExternalName != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--controller-external-name=%s", args.ControllerExternalName))
	}

	if args.ControllerServiceType != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--controller-service-type=%s", args.ControllerServiceType))
	}

	// Add controller external IP addresses
	for _, addr := range args.ControllerExternalIPAddrs {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--controller-external-ips=%s", addr))
	}

	// Add bootstrap constraints
	if len(args.BootstrapConstraints) > 0 {
		constraints := buildConstraintsString(args.BootstrapConstraints)
		cmdArgs = append(cmdArgs, fmt.Sprintf("--bootstrap-constraints=%s", constraints))
	} else {
		// Add architecture constraint to ensure juju bootstraps with the correct arch
		cmdArgs = append(cmdArgs, fmt.Sprintf("--bootstrap-constraints=arch=%s", runtime.GOARCH))
	}

	// Add model constraints
	if len(args.ModelConstraints) > 0 {
		constraints := buildConstraintsString(args.ModelConstraints)
		cmdArgs = append(cmdArgs, fmt.Sprintf("--constraints=%s", constraints))
	}

	// Add config options
	for k, v := range args.Config {
		cmdArgs = append(cmdArgs, "--config", fmt.Sprintf("%s=%s", k, v))
	}

	// Add model defaults
	for k, v := range args.ModelDefault {
		cmdArgs = append(cmdArgs, "--model-default", fmt.Sprintf("%s=%s", k, v))
	}

	// Add storage pool
	for k, v := range args.StoragePool {
		cmdArgs = append(cmdArgs, "--storage-pool", fmt.Sprintf("%s=%s", k, v))
	}

	// Add cloud name/region and controller name (must be at the end)
	cmdArgs = append(cmdArgs, args.Cloud.Name, args.Name)

	return cmdArgs
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
