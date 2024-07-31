// Copyright 2023 Canonical Ltd.
// Licensed under the Apache License, Version 2.0, see LICENCE file for details.

package provider

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/juju/names/v5"
)

var (
	uuidMatcher = regexp.MustCompile(`^[\w]{8}(?:-[\w]{4}){3}-[\w]{12}$`)
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &jaasAccessModelResource{}
var _ resource.ResourceWithConfigure = &jaasAccessModelResource{}

// var _ resource.ResourceWithImportState = &jaasAccessModelResource{}

func NewJAASAccessModelResource() resource.Resource {
	m := modelInfo{}
	return &jaasAccessModelResource{genericJAASAccessResource: genericJAASAccessResource{targetInfo: m}}
}

type jaasAccessModelResource struct {
	genericJAASAccessResource
}

type jaasAccessModelResourceModel struct {
	ModelUUID types.String `tfsdk:"model_uuid"`
	genericJAASAccessModel
}

type modelInfo struct{}

func (j modelInfo) Identity(ctx context.Context, plan Getter, diag *diag.Diagnostics) string {
	p := jaasAccessModelResourceModel{}
	diag.Append(plan.Get(ctx, &p)...)
	return names.NewModelTag(p.ModelUUID.String()).String()
}

func (a *jaasAccessModelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jaas_access_model"
}

func (a *jaasAccessModelResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := PartialAccessSchema()
	attributes["model_uuid"] = schema.StringAttribute{
		Description: "The uuid of the model for access management",
		Required:    true,
		Validators: []validator.String{
			stringvalidator.RegexMatches( // Replace with Juju validator
				uuidMatcher,
				"must be a valid UUID",
			),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	schema := schema.Schema{
		Description: "A resource that represent access to a model when using JAAS.",
		Attributes:  attributes,
	}
	resp.Schema = schema
}
