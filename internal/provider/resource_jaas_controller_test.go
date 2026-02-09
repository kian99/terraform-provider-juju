// Copyright 2026 Canonical Ltd.
// Licensed under the Apache License, Version 2.0, see LICENCE file for details.

package provider

import (
	"errors"
	"fmt"
	"testing"

	"github.com/canonical/jimm-go-sdk/v3/api/params"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	internaltesting "github.com/juju/terraform-provider-juju/internal/testing"
)

func TestAcc_ResourceJaasController(t *testing.T) {
	OnlyTestAgainstJAAS(t)

	controllers, err := TestClient.Jaas.ListControllers()
	if err != nil || len(controllers) == 0 {
		t.Fatalf("unable to list controllers from JAAS: %v", err)
	}
	ctrl := controllers[0]

	name := acctest.RandomWithPrefix("tf-jaas-controller")
	resourceName := "juju_jaas_controller.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: frameworkProviderFactories,
		CheckDestroy:             testAccCheckJaasControllerRegistered(name, false),
		Steps: []resource.TestStep{
			{
				Config: testAccResourceJaasController(name, ctrl),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "uuid", ctrl.UUID),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "status"),
					testAccCheckJaasControllerRegistered(name, true),
				),
				// JAAS may fill defaults and/or status on read.
				ExpectNonEmptyPlan: true,
			},
			{
				ImportState:       true,
				ResourceName:      resourceName,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccResourceJaasController(name string, ctrl params.ControllerInfo) string {
	c := ctrl

	// Keep this minimal: UUID+credentials are required by JIMM.
	return internaltesting.GetStringFromTemplateWithData(
		"testAccResourceJaasController",
		`
resource "juju_jaas_controller" "test" {
  name = "{{ .Name }}"
  uuid = "{{ .UUID }}"

  public_address = "{{ .PublicAddress }}"

  api_addresses = [
    {{- range $i, $a := .APIAddresses }}
    "{{ $a }}",
    {{- end }}
  ]

  ca_certificate = <<EOF
{{ .CACertificate }}
EOF

  # Use the same credentials as the provider is configured with.
  username = "${env.JUJU_CLIENT_ID}@serviceaccount"
  password = "${env.JUJU_CLIENT_SECRET}"
}
`, internaltesting.TemplateData{
			"Name":          name,
			"UUID":          c.UUID,
			"PublicAddress": c.PublicAddress,
			"APIAddresses":  c.APIAddresses,
			"CACertificate": c.CACertificate,
		})
}

func testAccCheckJaasControllerRegistered(name string, checkExists bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		controllers, err := TestClient.Jaas.ListControllers()
		if err != nil {
			return err
		}

		found := false
		for _, c := range controllers {
			if c.Name == name {
				found = true
				break
			}
		}

		if checkExists && !found {
			return fmt.Errorf("expected controller %q to be registered", name)
		}
		if !checkExists && found {
			return errors.New("controller still registered")
		}
		return nil
	}
}
