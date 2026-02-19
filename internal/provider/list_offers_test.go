// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccListOffers_query(t *testing.T) {
	if testingCloud != LXDCloudTesting {
		t.Skip(t.Name() + " only runs with LXD")
	}
	modelName := acctest.RandomWithPrefix("tf-test-offer-list")
	var offerURL string
	var modelUUID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: frameworkProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceOfferForListing(modelName),
				Check: func(s *terraform.State) error {
					rs, ok := s.RootModule().Resources["juju_offer.this"]
					if !ok {
						return fmt.Errorf("not found: juju_offer.this")
					}
					offerURL = rs.Primary.Attributes["url"]
					
					rs, ok = s.RootModule().Resources["juju_model.this"]
					if !ok {
						return fmt.Errorf("not found: juju_model.this")
					}
					modelUUID = rs.Primary.Attributes["uuid"]
					return nil
				},
			},
			{
				Config: testAccListOffers(modelUUID),
				Query:  true,
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectIdentity("juju_offer.test", map[string]knownvalue.Check{
						"id": knownvalue.StringFunc(func(actual string) error {
							return knownvalue.StringExact(offerURL).CheckValue(actual)
						}),
					}),
				},
			},
		},
	})
}

func testAccResourceOfferForListing(modelName string) string {
	return fmt.Sprintf(`
resource "juju_model" "this" {
	name = %q
}

resource "juju_application" "this" {
	model_uuid = juju_model.this.uuid
	name  = "this"

	charm {
		name = "juju-qa-dummy-source"
		base = "ubuntu@22.04"
	}
}

resource "juju_offer" "this" {
	model_uuid       = juju_model.this.uuid
	application_name = juju_application.this.name
	endpoints        = ["sink"]
}
`, modelName)
}

func testAccListOffers(modelUUID string) string {
	return fmt.Sprintf(`
list "juju_offer" "test" {
	provider         = juju
	include_resource = true
	model_uuid       = %q
}
`, modelUUID)
}
