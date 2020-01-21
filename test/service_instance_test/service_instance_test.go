/*
 *    Copyright 2018 The Service Manager Authors
 *
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *        http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 */

package service_test

import (
	"fmt"

	"strconv"

	"github.com/gofrs/uuid"

	"net/http"
	"testing"

	"github.com/Peripli/service-manager/pkg/types"

	"github.com/Peripli/service-manager/pkg/web"
	"github.com/Peripli/service-manager/test/common"

	"github.com/Peripli/service-manager/test"

	. "github.com/onsi/ginkgo"

	. "github.com/onsi/gomega"
)

func TestServiceInstances(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Service Instances Tests Suite")
}

const (
	TenantIdentifier = "tenant"
	TenantValue      = "tenant_value"
)

var _ = test.DescribeTestsFor(test.TestCase{
	API: web.ServiceInstancesURL,
	SupportedOps: []test.Op{
		test.Get, test.List, test.Delete, test.Patch,
	},
	MultitenancySettings: &test.MultitenancySettings{
		ClientID:           "tenancyClient",
		ClientIDTokenClaim: "cid",
		TenantTokenClaim:   "zid",
		LabelKey:           TenantIdentifier,
		TokenClaims: map[string]interface{}{
			"cid": "tenancyClient",
			"zid": TenantValue,
		},
	},
	ResourceType:                           types.ServiceInstanceType,
	SupportsAsyncOperations:                true,
	DisableTenantResources:                 true,
	ResourceBlueprint:                      blueprint,
	ResourceWithoutNullableFieldsBlueprint: blueprint,
	ResourcePropertiesToIgnore:             []string{"platform_id"},
	PatchResource:                          test.APIResourcePatch,
	AdditionalTests: func(ctx *common.TestContext) {
		Context("additional non-generic tests", func() {
			var (
				postInstanceRequest      common.Object
				expectedInstanceResponse common.Object

				instanceID string
			)

			createInstance := func(smClient *common.SMExpect) {
				smClient.POST(web.ServiceInstancesURL).WithJSON(postInstanceRequest).
					Expect().
					Status(http.StatusCreated).
					JSON().Object().
					ContainsMap(expectedInstanceResponse).ContainsKey("id").
					ValueEqual("platform_id", types.SMPlatform)
			}

			BeforeEach(func() {
				id, err := uuid.NewV4()
				if err != nil {
					panic(err)
				}

				instanceID = id.String()
				name := "test-instance"
				servicePlanID := generateServicePlan(ctx, ctx.SMWithOAuth)

				postInstanceRequest = common.Object{
					"id":               instanceID,
					"name":             name,
					"service_plan_id":  servicePlanID,
					"maintenance_info": "{}",
				}
				expectedInstanceResponse = common.Object{
					"id":               instanceID,
					"name":             name,
					"service_plan_id":  servicePlanID,
					"maintenance_info": "{}",
				}

			})

			AfterEach(func() {
				ctx.CleanupAdditionalResources()
			})

			Describe("GET", func() {
				When("service instance contains tenant identifier in OSB context", func() {
					BeforeEach(func() {
						createInstance(ctx.SMWithOAuthForTenant)
					})

					It("labels instance with tenant identifier", func() {
						ctx.SMWithOAuthForTenant.GET(web.ServiceInstancesURL + "/" + postInstanceRequest["id"].(string)).Expect().
							Status(http.StatusOK).
							JSON().
							Object().Path(fmt.Sprintf("$.labels[%s][*]", TenantIdentifier)).Array().Contains(TenantValue)
					})
				})
				When("service instance doesn't contain tenant identifier in OSB context", func() {
					BeforeEach(func() {
						createInstance(ctx.SMWithOAuth)
					})

					It("doesn't label instance with tenant identifier", func() {
						obj := ctx.SMWithOAuth.GET(web.ServiceInstancesURL + "/" + postInstanceRequest["id"].(string)).Expect().
							Status(http.StatusOK).JSON().Object()

						objMap := obj.Raw()
						objLabels, exist := objMap["labels"]
						if exist {
							labels := objLabels.(map[string]interface{})
							_, tenantLabelExists := labels[TenantIdentifier]
							Expect(tenantLabelExists).To(BeFalse())
						}
					})
				})

				When("service instance dashboard_url is not set", func() {
					BeforeEach(func() {
						postInstanceRequest["dashboard_url"] = ""
						createInstance(ctx.SMWithOAuth)
					})

					It("doesn't return dashboard_url", func() {
						ctx.SMWithOAuth.GET(web.ServiceInstancesURL + "/" + postInstanceRequest["id"].(string)).Expect().
							Status(http.StatusOK).JSON().Object().NotContainsKey("dashboard_url")
					})
				})
			})

			Describe("POST", func() {
				Context("when content type is not JSON", func() {
					It("returns 415", func() {
						ctx.SMWithOAuth.POST(web.ServiceInstancesURL).WithText("text").
							Expect().
							Status(http.StatusUnsupportedMediaType).
							JSON().Object().
							Keys().Contains("error", "description")
					})
				})

				Context("when request body is not a valid JSON", func() {
					It("returns 400", func() {
						ctx.SMWithOAuth.POST(web.ServiceInstancesURL).
							WithText("invalid json").
							WithHeader("content-type", "application/json").
							Expect().
							Status(http.StatusBadRequest).
							JSON().Object().
							Keys().Contains("error", "description")
					})
				})

				Context("when a request body field is missing", func() {
					assertPOSTReturns400WhenFieldIsMissing := func(field string) {
						BeforeEach(func() {
							delete(postInstanceRequest, field)
							delete(expectedInstanceResponse, field)
						})

						It("returns 400", func() {
							ctx.SMWithOAuth.POST(web.ServiceInstancesURL).WithJSON(postInstanceRequest).
								Expect().
								Status(http.StatusBadRequest).
								JSON().Object().
								Keys().Contains("error", "description")
						})
					}

					assertPOSTReturns201WhenFieldIsMissing := func(field string) {
						BeforeEach(func() {
							delete(postInstanceRequest, field)
							delete(expectedInstanceResponse, field)
						})

						It("returns 201", func() {
							createInstance(ctx.SMWithOAuth)
						})
					}

					Context("when id  field is missing", func() {
						assertPOSTReturns201WhenFieldIsMissing("id")
					})

					Context("when name field is missing", func() {
						assertPOSTReturns400WhenFieldIsMissing("name")
					})

					Context("when service_plan_id field is missing", func() {
						assertPOSTReturns400WhenFieldIsMissing("service_plan_id")
					})

					Context("when maintenance_info field is missing", func() {
						assertPOSTReturns201WhenFieldIsMissing("maintenance_info")
					})
				})

				Context("when request body id field is invalid", func() {
					It("should return 400", func() {
						postInstanceRequest["id"] = "instance/1"
						resp := ctx.SMWithOAuth.POST(web.ServiceInstancesURL).
							WithJSON(postInstanceRequest).
							Expect().Status(http.StatusBadRequest).JSON().Object()

						resp.Value("description").Equal("instance/1 contains invalid character(s)")
					})
				})

				Context("when request body platform_id field is provided", func() {
					Context("which is not service-manager platform", func() {
						It("should return 400", func() {
							postInstanceRequest["platform_id"] = "test-platform-id"
							resp := ctx.SMWithOAuth.POST(web.ServiceInstancesURL).
								WithJSON(postInstanceRequest).
								Expect().Status(http.StatusBadRequest).JSON().Object()

							resp.Value("description").Equal("Providing platform_id property during provisioning/updating of a service instance is forbidden")
						})
					})

					Context("which is service-manager platform", func() {
						It("should return 200", func() {
							postInstanceRequest["platform_id"] = types.SMPlatform
							createInstance(ctx.SMWithOAuth)
						})
					})
				})

				Context("With async query param", func() {
					It("succeeds", func() {
						resp := ctx.SMWithOAuth.POST(web.ServiceInstancesURL).WithJSON(postInstanceRequest).
							WithQuery("async", "true").
							Expect().
							Status(http.StatusAccepted)

						test.ExpectOperation(ctx.SMWithOAuth, resp, types.SUCCEEDED)

						ctx.SMWithOAuth.GET(web.ServiceInstancesURL + "/" + instanceID).Expect().
							Status(http.StatusOK).
							JSON().Object().
							ContainsMap(expectedInstanceResponse).ContainsKey("id")
					})
				})
			})

			Describe("PATCH", func() {
				Context("when content type is not JSON", func() {
					It("returns 415", func() {
						instanceID := fmt.Sprintf("%s", postInstanceRequest["id"])
						ctx.SMWithOAuth.PATCH(web.ServiceInstancesURL+"/"+instanceID).
							WithText("text").
							Expect().Status(http.StatusUnsupportedMediaType).
							JSON().Object().
							Keys().Contains("error", "description")
					})
				})

				Context("when instance is missing", func() {
					It("returns 404", func() {
						ctx.SMWithOAuth.PATCH(web.ServiceInstancesURL+"/no_such_id").
							WithJSON(postInstanceRequest).
							Expect().Status(http.StatusNotFound).
							JSON().Object().
							Keys().Contains("error", "description")
					})
				})

				Context("when request body is not valid JSON", func() {
					It("returns 400", func() {
						ctx.SMWithOAuth.PATCH(web.ServiceInstancesURL+"/"+instanceID).
							WithText("invalid json").
							WithHeader("content-type", "application/json").
							Expect().
							Status(http.StatusBadRequest).
							JSON().Object().
							Keys().Contains("error", "description")
					})
				})

				Context("when created_at provided in body", func() {
					It("should not change created at", func() {
						createInstance(ctx.SMWithOAuth)

						createdAt := "2015-01-01T00:00:00Z"

						ctx.SMWithOAuth.PATCH(web.ServiceInstancesURL+"/"+instanceID).
							WithJSON(common.Object{"created_at": createdAt}).
							Expect().
							Status(http.StatusOK).JSON().Object().
							ContainsKey("created_at").
							ValueNotEqual("created_at", createdAt)

						ctx.SMWithOAuth.GET(web.ServiceInstancesURL+"/"+instanceID).
							Expect().
							Status(http.StatusOK).JSON().Object().
							ContainsKey("created_at").
							ValueNotEqual("created_at", createdAt)
					})
				})

				Context("when platform_id provided in body", func() {
					Context("which is not service-manager platform", func() {
						It("should return 400", func() {
							createInstance(ctx.SMWithOAuth)

							resp := ctx.SMWithOAuth.PATCH(web.ServiceInstancesURL + "/" + instanceID).
								WithJSON(common.Object{"platform_id": "test-platform-id"}).
								Expect().Status(http.StatusBadRequest).JSON().Object()

							resp.Value("description").Equal("Providing platform_id property during provisioning/updating of a service instance is forbidden")

							ctx.SMWithOAuth.GET(web.ServiceInstancesURL+"/"+instanceID).
								Expect().
								Status(http.StatusOK).JSON().Object().
								ContainsKey("platform_id").
								ValueEqual("platform_id", types.SMPlatform)
						})
					})

					Context("which is service-manager platform", func() {
						It("should return 200", func() {
							createInstance(ctx.SMWithOAuth)

							ctx.SMWithOAuth.PATCH(web.ServiceInstancesURL + "/" + instanceID).
								WithJSON(common.Object{"platform_id": types.SMPlatform}).
								Expect().Status(http.StatusOK).JSON().Object()

							ctx.SMWithOAuth.GET(web.ServiceInstancesURL+"/"+instanceID).
								Expect().
								Status(http.StatusOK).JSON().Object().
								ContainsKey("platform_id").
								ValueEqual("platform_id", types.SMPlatform)

						})
					})
				})

				Context("when fields are updated one by one", func() {
					It("returns 200", func() {
						createInstance(ctx.SMWithOAuth)

						for _, prop := range []string{"name", "maintenance_info"} {
							updatedBrokerJSON := common.Object{}
							updatedBrokerJSON[prop] = "updated-" + prop
							ctx.SMWithOAuth.PATCH(web.ServiceInstancesURL + "/" + instanceID).
								WithJSON(updatedBrokerJSON).
								Expect().
								Status(http.StatusOK).
								JSON().Object().
								ContainsMap(updatedBrokerJSON)

							ctx.SMWithOAuth.GET(web.ServiceInstancesURL + "/" + instanceID).
								Expect().
								Status(http.StatusOK).
								JSON().Object().
								ContainsMap(updatedBrokerJSON)

						}
					})
				})

			})
		})
	},
})

func blueprint(ctx *common.TestContext, auth *common.SMExpect, async bool) common.Object {
	instanceID, err := uuid.NewV4()
	if err != nil {
		panic(err)
	}

	instanceReqBody := make(common.Object, 0)
	instanceReqBody["id"] = instanceID.String()
	instanceReqBody["name"] = "test-instance-" + instanceID.String()

	instanceReqBody["service_plan_id"] = generateServicePlan(ctx, auth)

	resp := auth.POST(web.ServiceInstancesURL).WithQuery("async", strconv.FormatBool(async)).WithJSON(instanceReqBody).Expect()

	var instance map[string]interface{}
	if async {
		instance = test.ExpectSuccessfulAsyncResourceCreation(resp, auth, instanceID.String(), web.ServiceInstancesURL)
	} else {
		instance = resp.Status(http.StatusCreated).JSON().Object().Raw()
	}

	return instance
}

func generateServicePlan(ctx *common.TestContext, auth *common.SMExpect) string {
	cPaidPlan := common.GeneratePaidTestPlan()
	cService := common.GenerateTestServiceWithPlans(cPaidPlan)
	catalog := common.NewEmptySBCatalog()
	catalog.AddService(cService)
	brokerID, _, server := ctx.RegisterBrokerWithCatalog(catalog)
	ctx.Servers[common.BrokerServerPrefix+brokerID] = server

	so := auth.ListWithQuery(web.ServiceOfferingsURL, fmt.Sprintf("fieldQuery=broker_id eq '%s'", brokerID)).First()

	servicePlanID := auth.ListWithQuery(web.ServicePlansURL, "fieldQuery="+fmt.Sprintf("service_offering_id eq '%s'", so.Object().Value("id").String().Raw())).
		First().Object().Value("id").String().Raw()

	return servicePlanID
}
