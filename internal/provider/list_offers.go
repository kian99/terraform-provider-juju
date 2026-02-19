// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/juju/names/v5"
	"github.com/juju/terraform-provider-juju/internal/juju"
)

type offerLister struct {
	client *juju.Client
	config juju.Config

	// context for the logging subsystem.
	subCtx context.Context
}

func NewOfferLister() list.ListResourceWithConfigure {
	return &offerLister{}
}

func (r *offerLister) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	provider, ok := req.ProviderData.(juju.ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected juju.ProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.client = provider.Client
	r.config = provider.Config
	r.subCtx = tflog.NewSubsystem(ctx, LogResourceOffer)
}

func (r *offerLister) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_offer"
}

type offerListConfigModel struct {
	ModelUUID types.String `tfsdk:"model_uuid"`
	OfferURL  types.String `tfsdk:"offer_url"`
}

func (r *offerLister) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Attributes: map[string]listschema.Attribute{
			"model_uuid": listschema.StringAttribute{
				Description: "The UUID of the model to filter offers by.",
				Optional:    true,
				Validators: []validator.String{
					ValidatorMatchString(names.IsValidModel, "must be a valid UUID"),
				},
			},
			"offer_url": listschema.StringAttribute{
				Description: "The offer URL to filter by.",
				Optional:    true,
				Validators: []validator.String{
					offerURLValidator{},
				},
			},
		},
	}
}

func (r *offerLister) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	stream.Results = func(push func(list.ListResult) bool) {
		result := req.NewListResult(ctx)

		// Read list configuration
		var config offerListConfigModel
		result.Diagnostics.Append(req.Config.Get(ctx, &config)...)
		if result.Diagnostics.HasError() {
			return
		}

		// Prepare input for ListOffers
		input := &juju.ListOffersInput{
			IncludeModelUUID: req.IncludeResource,
		}
		if !config.ModelUUID.IsNull() {
			input.ModelUUID = config.ModelUUID.ValueString()
		}
		if !config.OfferURL.IsNull() {
			input.OfferURL = config.OfferURL.ValueString()
		}

		// List offers
		offers, err := r.client.Offers.ListOffers(input)
		if err != nil {
			result.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list offers, got error: %s", err))
			return
		}

		for _, offer := range offers {
			result.DisplayName = offer.OfferURL
			identity := offerResourceIdentityModel{
				ID: types.StringValue(offer.OfferURL),
			}
			result.Diagnostics.Append(result.Identity.Set(ctx, identity)...)
			if result.Diagnostics.HasError() {
				return
			}

			if req.IncludeResource {
				schema, ok := req.ResourceSchema.(schema.Schema)
				if !ok {
					result.Diagnostics.AddError(
						"Unexpected Resource Schema Type",
						fmt.Sprintf("Expected schema.Schema, got: %T. Please report this issue to the provider developers.", req.ResourceSchema),
					)
					return
				}
				resource, err := r.getOfferResource(ctx, offer, schema)
				if err.HasError() {
					result.Diagnostics.Append(err...)
					return
				}
				result.Diagnostics.Append(result.Resource.Set(ctx, resource)...)
				if result.Diagnostics.HasError() {
					return
				}
			}
			if !push(result) {
				return
			}
		}
	}
}

type offerResourceModelForListing struct {
	OfferName       types.String `tfsdk:"name"`
	ApplicationName types.String `tfsdk:"application_name"`
	ModelUUID       types.String `tfsdk:"model_uuid"`
	Endpoints       types.Set    `tfsdk:"endpoints"`
	URL             types.String `tfsdk:"url"`
	// ID required by the testing framework
	ID types.String `tfsdk:"id"`
}

func (r *offerLister) getOfferResource(ctx context.Context, offer juju.ListOffersOutput, sc schema.Schema) (offerResourceModelV2, diag.Diagnostics) {
	resource := offerResourceModelForListing{}
	diags := diag.Diagnostics{}

	resource.OfferName = types.StringValue(offer.Name)
	resource.ApplicationName = types.StringValue(offer.ApplicationName)
	resource.ModelUUID = types.StringValue(offer.ModelUUID)
	resource.URL = types.StringValue(offer.OfferURL)
	resource.ID = types.StringValue(offer.OfferURL)

	endpointSet, errDiag := types.SetValueFrom(ctx, types.StringType, offer.Endpoints)
	diags.Append(errDiag...)
	if diags.HasError() {
		return offerResourceModelV2{}, diags
	}
	resource.Endpoints = endpointSet

	return offerResourceModelV2(resource), diags
}
