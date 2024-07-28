// Copyright 2023 Canonical Ltd.
// Licensed under the Apache License, Version 2.0, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func NewJAASAccessResourceByName(displayName, tag string) resource.Resource {
	resourceInfo := jaasAccessResourceByName{
		displayName: displayName,
		tag:         tag,
	}
	return NewGenericJAASAccessResource(resourceInfo)
}

type jaasAccessModelByName struct {
	Name types.String `tfsdk:"name"`
	genericJAASAccessModel
}

type jaasAccessResourceByName struct {
	displayName string
	tag         string
}

func (j jaasAccessResourceByName) DisplayName() string {
	return j.displayName
}

func (j jaasAccessResourceByName) Identity(ctx context.Context, plan Getter, diag *diag.Diagnostics) string {
	p := jaasAccessModelByName{}
	diag.Append(plan.Get(ctx, &p)...)
	return j.tag + "-" + p.Name.String()
}

func (j jaasAccessResourceByName) SchemaAttributes() map[string]schema.Attribute {
	key := "name"
	val := schema.StringAttribute{
		Description: "The name of the + " + j.displayName + " for access management",
		Required:    true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	return map[string]schema.Attribute{key: val}
}
