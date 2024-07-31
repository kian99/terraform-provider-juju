// Copyright 2023 Canonical Ltd.
// Licensed under the Apache License, Version 2.0, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	jimmNames "github.com/canonical/jimm/pkg/names"
	"github.com/juju/names/v5"
	"github.com/juju/terraform-provider-juju/internal/juju"
	"github.com/kian99/jimm-go-api/v3/api/params"
)

// Getter is used to get details from a plan or state object.
type Getter interface {
	Get(ctx context.Context, target interface{}) diag.Diagnostics
}

type resourceInfo interface {
	Identity(ctx context.Context, plan Getter, diag *diag.Diagnostics) string
}

// genericJAASAccessResource is a generic resource that can be used for creating access rules with JAAS.
// Other types should embed this struct and implement their own metadata and schema methods. The schema
// should build on top of [PartialAccessSchema].
// The embedded struct requires a targetInfo interface to enable fetching the target object in the relation.
type genericJAASAccessResource struct {
	client     *juju.Client
	targetInfo resourceInfo

	// subCtx is the context created with the new tflog subsystem for applications.
	subCtx context.Context
}

// genericJAASAccessModel represents a partial generic object for access management.
// This struct should be embedded so that either a UUID or name field can be set.
// Note that service accounts are treated as users but kept as a separate field for improved validation.
type genericJAASAccessModel struct {
	Users           types.Set    `tfsdk:"users"`
	ServiceAccounts types.Set    `tfsdk:"service_accounts"`
	Groups          types.Set    `tfsdk:"groups"`
	Access          types.String `tfsdk:"access"`

	// ID required by the testing framework
	ID types.String `tfsdk:"id"`
}

func (r *genericJAASAccessResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		RequiresJAASValidator{Client: r.client},
		resourcevalidator.AtLeastOneOf(
			path.MatchRoot("users"),
			path.MatchRoot("groups"),
			path.MatchRoot("service_accounts"),
		),
	}
}

func PartialAccessSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"access": schema.StringAttribute{
			Description: "Type of access to the model",
			Required:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"users": schema.SetAttribute{
			Description: "List of users to grant access",
			Optional:    true,
			ElementType: types.StringType,
		},
		"groups": schema.SetAttribute{
			Description: "List of groups to grant access",
			Optional:    true,

			ElementType: types.StringType,
		},
		"service_accounts": schema.SetAttribute{
			Description: "List of service account to grant access",
			Optional:    true,
			ElementType: types.StringType,
		},
		// ID required by the testing framework
		"id": schema.StringAttribute{
			Computed: true,
		},
	}
}

// Configure enables provider-level data or clients to be set in the
// provider-defined DataSource type. It is separately executed for each
// ReadDataSource RPC.
func (a *genericJAASAccessResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (a *genericJAASAccessResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Check first if the client is configured
	if a.client == nil {
		addClientNotConfiguredError(&resp.Diagnostics, "access model", "create")
		return
	}
	var plan genericJAASAccessModel

	// Read Terraform configuration from the request into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	targetID := a.targetInfo.Identity(ctx, req.Plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	tuples := planToTuples(ctx, targetID, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
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

func (a *genericJAASAccessResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Check first if the client is configured
	if a.client == nil {
		addClientNotConfiguredError(&resp.Diagnostics, "access model", "read")
		return
	}
	var plan genericJAASAccessModel
	// Get the Terraform state from the request into the plan
	resp.Diagnostics.Append(req.State.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	targetID := a.targetInfo.Identity(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	readTuple := params.RelationshipTuple{
		TargetObject: targetID,
		Relation:     plan.Access.String(),
	}
	tuples, err := a.client.JAAS.ReadTuples(readTuple)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read access rules, got error: %s", err))
		return
	}
	newState := tuplesToPlan(ctx, tuples, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Users = newState.Users
	plan.Groups = newState.Groups
	plan.ServiceAccounts = newState.ServiceAccounts
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
func (a *genericJAASAccessResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Check first if the client is configured
	if a.client == nil {
		addClientNotConfiguredError(&resp.Diagnostics, "access model", "update")
		return
	}

	var plan, state genericJAASAccessModel

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

	toAdd, toRemove := diffPlans(plan, state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	targetID := a.targetInfo.Identity(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	tuples := planToTuples(ctx, targetID, toAdd, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	err := a.client.JAAS.AddTuples(tuples)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to add access rules, got error: %s", err))
		return
	}
	// TODO: Update the state to reflect the newly added tuples.
	// If the removal lower down fails we at least ensure that new tuples are saved to state.
	// Probably requires an intermediate state.
	// resp.Diagnostics.Append(resp.State.Set(ctx, &intermediateState)...)
	tuples = planToTuples(ctx, targetID, toRemove, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	err = a.client.JAAS.DeleteTuples(tuples)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to remove access rules, got error: %s", err))
		return
	}
	// Set the plan onto the Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func diffPlans(plan, state genericJAASAccessModel, diag *diag.Diagnostics) (toAdd, toRemove genericJAASAccessModel) {
	newUsers := diffSet(plan.Users, state.Users, diag)
	newGroups := diffSet(plan.Groups, state.Groups, diag)
	newServiceAccounts := diffSet(plan.ServiceAccounts, state.ServiceAccounts, diag)
	toAdd.Users = newUsers
	toAdd.Groups = newGroups
	toAdd.ServiceAccounts = newServiceAccounts

	removedUsers := diffSet(state.Users, plan.Users, diag)
	removedGroups := diffSet(state.Groups, plan.Groups, diag)
	removedServiceAccounts := diffSet(state.ServiceAccounts, plan.ServiceAccounts, diag)

	toRemove.Users = removedUsers
	toRemove.Groups = removedGroups
	toRemove.ServiceAccounts = removedServiceAccounts

	return
}

func diffSet(current, desired basetypes.SetValue, diag *diag.Diagnostics) basetypes.SetValue {
	var diff []attr.Value
	for _, source := range current.Elements() {
		found := false
		for _, target := range desired.Elements() {
			if source.Equal(target) {
				found = true
			}
		}
		if !found {
			diff = append(diff, source)
		}
	}
	newSet, diags := basetypes.NewSetValue(current.ElementType(context.Background()), diff)
	diag.Append(diags...)
	return newSet
}

func (a *genericJAASAccessResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Check first if the client is configured
	if a.client == nil {
		addClientNotConfiguredError(&resp.Diagnostics, "access model", "delete")
		return
	}

	var plan genericJAASAccessModel
	// Get the Terraform state from the request into the plan
	resp.Diagnostics.Append(req.State.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	targetID := a.targetInfo.Identity(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	tuples := planToTuples(ctx, targetID, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	err := a.client.JAAS.DeleteTuples(tuples)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete access rules, got error: %s", err))
		return
	}
}

// planToTuples return a list of tuples based on the plan provided.
func planToTuples(ctx context.Context, targetTag string, plan genericJAASAccessModel, diag *diag.Diagnostics) []params.RelationshipTuple {
	var users []string
	var groups []string
	var serviceAccounts []string
	diag.Append(plan.Users.ElementsAs(ctx, &users, false)...)
	diag.Append(plan.Groups.ElementsAs(ctx, &groups, false)...)
	diag.Append(plan.ServiceAccounts.ElementsAs(ctx, &serviceAccounts, false)...)
	if diag.HasError() {
		return []params.RelationshipTuple{}
	}
	baseTuple := params.RelationshipTuple{
		Object:   targetTag,
		Relation: plan.Access.String(),
	}
	// Note that service accounts are just users but kept as a separate field for improved validation.
	var tuples []params.RelationshipTuple
	userNameToTagf := func(s string) string { return names.NewUserTag(s).String() }
	groupNameToTagf := func(s string) string { return "group-" + s }
	tuples = append(tuples, makeTuples(baseTuple, users, userNameToTagf)...)
	tuples = append(tuples, makeTuples(baseTuple, groups, groupNameToTagf)...)
	tuples = append(tuples, makeTuples(baseTuple, serviceAccounts, userNameToTagf)...)
	return tuples
}

// tuplesToPlan does the reverse of planToTuples converting a slice of tuples to a plan.
func tuplesToPlan(ctx context.Context, tuples []params.RelationshipTuple, diag *diag.Diagnostics) genericJAASAccessModel {
	var users []string
	var groups []string
	var serviceAccounts []string
	for _, tuple := range tuples {
		tag, err := jimmNames.ParseTag(tuple.Object)
		if err != nil {
			diag.AddError("failed to parse relation tag", fmt.Sprintf("error parsing %s:%s", tuple.Object, err.Error()))
			continue
		}
		switch tag.Kind() {
		case names.UserTagKind:
			if jimmNames.IsValidServiceAccountId(tag.Id()) {
				serviceAccounts = append(serviceAccounts, tag.Id())
			} else {
				users = append(users, tag.Id())
			}
		case jimmNames.GroupTagKind:
			groups = append(groups, tag.Id())
		}
	}
	userSet, errDiag := basetypes.NewSetValueFrom(ctx, types.StringType, users)
	diag.Append(errDiag...)
	groupSet, errDiag := basetypes.NewSetValueFrom(ctx, types.StringType, groups)
	diag.Append(errDiag...)
	serviceAccountSet, errDiag := basetypes.NewSetValueFrom(ctx, types.StringType, serviceAccounts)
	diag.Append(errDiag...)
	var plan genericJAASAccessModel
	plan.Users = userSet
	plan.Groups = groupSet
	plan.ServiceAccounts = serviceAccountSet
	return plan
}

func makeTuples(baseTuple params.RelationshipTuple, items []string, idToTag func(string) string) []params.RelationshipTuple {
	tuples := make([]params.RelationshipTuple, 0, len(items))
	for _, item := range items {
		t := baseTuple
		t.Object = idToTag(item)
		tuples = append(tuples, t)
	}
	return tuples
}
