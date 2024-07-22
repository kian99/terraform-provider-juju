// Copyright 2023 Canonical Ltd.
// Licensed under the Apache License, Version 2.0, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/kian99/jimm-go-api/v3/api/params"

	"github.com/juju/terraform-provider-juju/internal/juju"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &accessModelResource{}
var _ resource.ResourceWithConfigure = &accessModelResource{}
var _ resource.ResourceWithImportState = &accessModelResource{}

func NewGenericJAASAccessModelResource() resource.Resource {
	return &accessModelResource{}
}

type genericJAASAccessModelResource struct {
	client   *juju.Client
	tag      string
	resource string

	// subCtx is the context created with the new tflog subsystem for applications.
	subCtx context.Context
}

type genericJAASAccessResourceModel struct {
	UUID            types.String `tfsdk:"model"`
	Users           types.Set    `tfsdk:"users"`
	ServiceAccounts types.Set    `tfsdk:service-accounts`
	Groups          types.Set    `tfsdk:groups`
	Access          types.String `tfsdk:"access"`

	// ID required by the testing framework
	ID types.String `tfsdk:"id"`
}

func (a *genericJAASAccessModelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + a.resource + "_access_model"
}

func (r *genericJAASAccessModelResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		RequiresJAASValidator{Client: r.client},
	}
}

func (a *genericJAASAccessModelResource) Schema(_ context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A resource that represent a JAAS Access " + a.resource + ".",
		Attributes: map[string]schema.Attribute{
			"uuid": schema.StringAttribute{
				Description: "The uuid of the + " + a.resource + " for access management",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"access": schema.StringAttribute{
				Description: "Type of access to the model",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"users": schema.SetAttribute{
				Description: "List of users to grant access",
				Required:    false,
				ElementType: types.StringType,
			},
			"groups": schema.SetAttribute{
				Description: "List of groups to grant access",
				Required:    false,
				ElementType: types.StringType,
			},
			"service-accounts": schema.SetAttribute{
				Description: "List of service account to grant access",
				Required:    false,
				ElementType: types.StringType,
			},
			// ID required by the testing framework
			"id": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

// Configure enables provider-level data or clients to be set in the
// provider-defined DataSource type. It is separately executed for each
// ReadDataSource RPC.
func (a *genericJAASAccessModelResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*juju.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *juju.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	a.client = client
	// Create the local logging subsystem here, using the TF context when creating it.
	a.subCtx = tflog.NewSubsystem(ctx, LogResourceAccessModel)
}

func (a *genericJAASAccessModelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Check first if the client is configured
	if a.client == nil {
		addClientNotConfiguredError(&resp.Diagnostics, "access model", "create")
		return
	}
	var plan genericJAASAccessResourceModel

	// Read Terraform configuration from the request into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tuples, ok := a.getTuples(ctx, plan, resp)
	if !ok {
		return
	}
	err := a.client.JAAS.AddTuples(tuples)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read add access relationships, got error: %s", err))
		return
	}
	// Set the plan onto the Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (a *genericJAASAccessModelResource) getTuples(ctx context.Context, plan genericJAASAccessResourceModel, resp *resource.CreateResponse) ([]params.RelationshipTuple, bool) {
	var users []string
	resp.Diagnostics.Append(plan.Users.ElementsAs(ctx, &users, false)...)
	if resp.Diagnostics.HasError() {
		return []params.RelationshipTuple{}, false
	}
	baseTuple := params.RelationshipTuple{
		Object:   a.tag + "-" + plan.UUID.String(),
		Relation: plan.Access.String(),
	}
	var tuples []params.RelationshipTuple
	for _, user := range users {
		t := baseTuple
		t.Object = "user-" + user
		tuples = append(tuples, t)
	}
	var groups []string
	resp.Diagnostics.Append(plan.Groups.ElementsAs(ctx, &groups, false)...)
	if resp.Diagnostics.HasError() {
		return []params.RelationshipTuple{}, false
	}
	for _, group := range groups {
		t := baseTuple
		t.Object = "group-" + group
		tuples = append(tuples, t)
	}
	var serviceAccounts []string
	resp.Diagnostics.Append(plan.ServiceAccounts.ElementsAs(ctx, &serviceAccounts, false)...)
	if resp.Diagnostics.HasError() {
		return []params.RelationshipTuple{}, false
	}
	for _, serviceAccount := range serviceAccounts {
		t := baseTuple
		t.Object = "group-" + serviceAccount
		tuples = append(tuples, t)
	}
	return tuples, true
}

func (a *genericJAASAccessModelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Check first if the client is configured
	if a.client == nil {
		addClientNotConfiguredError(&resp.Diagnostics, "access model", "read")
		return
	}
	var plan genericJAASAccessResourceModel

	// Get the Terraform state from the request into the plan
	resp.Diagnostics.Append(req.State.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tuple := params.RelationshipTuple{
		TargetObject: a.tag + "-" + plan.UUID.String(),
		Relation:     plan.Access.String(),
	}
	tuples, err := a.client.JAAS.ReadTuples(tuple)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read access rules, got error: %s", err))
		return
	}
	modelName, access, stateUsers := retrieveAccessModelDataFromID(ctx, plan.ID, plan.Users, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	response, err := a.client.Users.ModelUserInfo(modelName)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read access model resource, got error: %s", err))
		return
	}

	plan.Model = types.StringValue(modelName)
	plan.Access = types.StringValue(access)

	var users []string

	for _, user := range stateUsers {
		for _, modelUser := range response.ModelUserInfo {
			if user == modelUser.UserName && string(modelUser.Access) == access {
				users = append(users, modelUser.UserName)
			}
		}
	}

	uss, errDiag := basetypes.NewListValueFrom(ctx, types.StringType, users)
	plan.Users = uss
	resp.Diagnostics.Append(errDiag...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set the plan onto the Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Update on the access model supports three cases
// access and users both changed:
// for missing users - revoke access
// for changed users - apply new access
// users changed:
// for missing users - revoke access
// for new users - apply access
// access changed - apply new access
func (a *genericJAASAccessModelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Check first if the client is configured
	if a.client == nil {
		addClientNotConfiguredError(&resp.Diagnostics, "access model", "update")
		return
	}

	var plan, state genericJAASAccessResourceModel

	// Get the Terraform state from the request into the plan
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read Terraform configuration from the request into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	anyChange := false

	// items that could be changed
	access := state.Access.ValueString()
	var missingUserList []string
	var addedUserList []string

	// Get the users that are in the planned state
	var planUsers []string
	resp.Diagnostics.Append(plan.Users.ElementsAs(ctx, &planUsers, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check if the users has changed
	if !plan.Users.Equal(state.Users) {
		anyChange = true

		// Get the users that are in the current state
		var stateUsers []string
		resp.Diagnostics.Append(plan.Users.ElementsAs(ctx, &stateUsers, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		missingUserList = getMissingUsers(stateUsers, planUsers)
		addedUserList = getAddedUsers(stateUsers, planUsers)
	}

	// Check if access has changed
	if !plan.Access.Equal(state.Access) {
		anyChange = true
		access = plan.Access.ValueString()
	}

	if !anyChange {
		a.trace("Update is returning without any changes.")
		return
	}

	modelName, oldAccess, _ := retrieveAccessModelDataFromID(ctx, state.ID, state.Users, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	err := a.client.Models.UpdateAccessModel(juju.UpdateAccessModelInput{
		ModelName: modelName,
		OldAccess: oldAccess,
		Grant:     addedUserList,
		Revoke:    missingUserList,
		Access:    access,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update access model resource, got error: %s", err))
	}
	a.trace(fmt.Sprintf("updated access model resource for model %q", modelName))

	plan.ID = types.StringValue(newAccessModelIDFrom(modelName, access, planUsers))

	// Set the plan onto the Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (a *genericJAASAccessModelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Check first if the client is configured
	if a.client == nil {
		addClientNotConfiguredError(&resp.Diagnostics, "access model", "delete")
		return
	}

	var plan genericJAASAccessResourceModel

	// Get the Terraform state from the request into the plan
	resp.Diagnostics.Append(req.State.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get the users
	var stateUsers []string
	resp.Diagnostics.Append(plan.Users.ElementsAs(ctx, &stateUsers, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := a.client.Models.DestroyAccessModel(juju.DestroyAccessModelInput{
		ModelName: plan.Model.ValueString(),
		Revoke:    stateUsers,
		Access:    plan.Access.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete access model resource, got error: %s", err))
	}
}

func (a *genericJAASAccessModelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	IDstr := req.ID
	if len(strings.Split(IDstr, ":")) != 3 {
		resp.Diagnostics.AddError(
			"ImportState Failure",
			fmt.Sprintf("Malformed AccessModel ID %q, "+
				"please use format '<modelname>:<access>:<user1,user1>'", IDstr),
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (a *genericJAASAccessModelResource) trace(msg string, additionalFields ...map[string]interface{}) {
	if a.subCtx == nil {
		return
	}

	//SubsystemTrace(subCtx, "my-subsystem", "hello, world", map[string]interface{}{"foo": 123})
	// Output:
	// {"@level":"trace","@message":"hello, world","@module":"provider.my-subsystem","foo":123}
	tflog.SubsystemTrace(a.subCtx, LogResourceAccessModel, msg, additionalFields...)
}
