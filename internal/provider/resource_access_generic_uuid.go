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
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &accessModelResource{}
var _ resource.ResourceWithConfigure = &accessModelResource{}
var _ resource.ResourceWithImportState = &accessModelResource{}

var (
	uuidMatcher = regexp.MustCompile(`^[\w]{8}(?:-[\w]{4}){3}-[\w]{12}$`)
)

func NewJAASAccessResourceByUUID(displayName, tag string) resource.Resource {
	resourceInfo := jaasAccessResourceByUUID{
		displayName: displayName,
	}
	return NewGenericJAASAccessResource(resourceInfo)
}

type jaasAccessModelByUUID struct {
	UUID types.String `tfsdk:"uuid"`
	genericJAASAccessModel
}

type jaasAccessResourceByUUID struct {
	displayName string
	tag         string
}

func (j jaasAccessResourceByUUID) DisplayName() string {
	return j.displayName
}

func (j jaasAccessResourceByUUID) Identity(ctx context.Context, plan Getter, diag *diag.Diagnostics) string {
	p := jaasAccessModelByUUID{}
	diag.Append(plan.Get(ctx, &p)...)
	return j.tag + "-" + p.UUID.String()
}

func (j jaasAccessResourceByUUID) SchemaAttributes() map[string]schema.Attribute {
	key := "uuid"
	val := schema.StringAttribute{
		Description: "The uuid of the + " + j.displayName + " for access management",
		Required:    true,
		Validators: []validator.String{
			stringvalidator.RegexMatches(
				uuidMatcher,
				"must be a valid UUID",
			),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	return map[string]schema.Attribute{key: val}
}
