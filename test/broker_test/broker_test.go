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
package broker_test

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Peripli/service-manager/pkg/httpclient"
	"github.com/Peripli/service-manager/pkg/web"

	"github.com/Peripli/service-manager/storage"

	"github.com/Peripli/service-manager/pkg/types"

	"github.com/Peripli/service-manager/pkg/query"

	"github.com/Peripli/service-manager/test"
	"github.com/gavv/httpexpect"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/Peripli/service-manager/test/common"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/cast"
)

func TestBrokers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ServiceBroker API Tests Suite")
}

var _ = test.DescribeTestsFor(test.TestCase{
	API: web.ServiceBrokersURL,
	SupportedOps: []test.Op{
		test.Get, test.List, test.Delete, test.DeleteList, test.Patch,
	},
	MultitenancySettings: &test.MultitenancySettings{
		ClientID:           "tenancyClient",
		ClientIDTokenClaim: "cid",
		TenantTokenClaim:   "zid",
		LabelKey:           "tenant",
		TokenClaims: map[string]interface{}{
			"cid": "tenancyClient",
			"zid": "tenantID",
		},
	},
	SupportsAsyncOperations:                true,
	ResourceBlueprint:                      blueprint(true),
	ResourceWithoutNullableFieldsBlueprint: blueprint(false),
	PatchResource:                          test.APIResourcePatch,
	AdditionalTests: func(ctx *common.TestContext, t *test.TestCase) {
		Context("additional non-generic tests", func() {
			var (
				brokerServer           *common.BrokerServer
				brokerWithLabelsServer *common.BrokerServer

				postBrokerRequestWithNoLabels common.Object
				expectedBrokerResponse        common.Object

				labels                      common.Object
				postBrokerRequestWithLabels labeledBroker

				repository storage.Repository
			)

			assertInvocationCount := func(requests []*http.Request, invocationCount int) {
				Expect(len(requests)).To(Equal(invocationCount))
			}

			AfterEach(func() {
				if brokerServer != nil {
					brokerServer.Close()
				}

				if brokerWithLabelsServer != nil {
					brokerWithLabelsServer.Close()
				}
			})

			BeforeEach(func() {
				brokerServer = common.NewBrokerServer()
				brokerWithLabelsServer = common.NewBrokerServer()
				brokerServer.Reset()
				brokerWithLabelsServer.Reset()
				brokerName := "brokerName"
				brokerWithLabelsName := "brokerWithLabelsName"
				brokerDescription := "description"
				brokerWithLabelsDescription := "descriptionWithLabels"

				postBrokerRequestWithNoLabels = common.Object{
					"name":        brokerName,
					"broker_url":  brokerServer.URL(),
					"description": brokerDescription,
					"credentials": common.Object{
						"basic": common.Object{
							"username": brokerServer.Username,
							"password": brokerServer.Password,
						},
					},
				}
				expectedBrokerResponse = common.Object{
					"name":        brokerName,
					"broker_url":  brokerServer.URL(),
					"description": brokerDescription,
				}

				labels = common.Object{
					"cluster_id": common.Array{"cluster_id_value"},
					"org_id":     common.Array{"org_id_value1", "org_id_value2", "org_id_value3"},
				}

				postBrokerRequestWithLabels = common.Object{
					"name":        brokerWithLabelsName,
					"broker_url":  brokerWithLabelsServer.URL(),
					"description": brokerWithLabelsDescription,
					"credentials": common.Object{
						"basic": common.Object{
							"username": brokerWithLabelsServer.Username,
							"password": brokerWithLabelsServer.Password,
						},
					},
					"labels": labels,
				}
				common.RemoveAllBrokers(ctx.SMRepository)

				repository = ctx.SMRepository
			})

			Describe("POST", func() {
				Context("when content type is not JSON", func() {
					It("returns 415", func() {
						ctx.SMWithOAuth.POST(web.ServiceBrokersURL).WithText("text").
							Expect().
							Status(http.StatusUnsupportedMediaType).
							JSON().Object().
							Keys().Contains("error", "description")

						assertInvocationCount(brokerServer.CatalogEndpointRequests, 0)
					})
				})

				Context("when request body is not a valid JSON", func() {
					It("returns 400", func() {
						ctx.SMWithOAuth.POST(web.ServiceBrokersURL).
							WithText("invalid json").
							WithHeader("content-type", "application/json").
							Expect().
							Status(http.StatusBadRequest).
							JSON().Object().
							Keys().Contains("error", "description")

						assertInvocationCount(brokerServer.CatalogEndpointRequests, 0)
					})
				})

				Context("when a request body field is missing", func() {
					assertPOSTReturns400WhenFieldIsMissing := func(field string) {
						BeforeEach(func() {
							delete(postBrokerRequestWithNoLabels, field)
							delete(expectedBrokerResponse, field)
						})

						It("returns 400", func() {
							ctx.SMWithOAuth.POST(web.ServiceBrokersURL).WithJSON(postBrokerRequestWithNoLabels).
								Expect().
								Status(http.StatusBadRequest).
								JSON().Object().
								Keys().Contains("error", "description")

							assertInvocationCount(brokerServer.CatalogEndpointRequests, 0)
						})
					}

					assertPOSTReturns201WhenFieldIsMissing := func(field string) {
						BeforeEach(func() {
							delete(postBrokerRequestWithNoLabels, field)
							delete(expectedBrokerResponse, field)
						})

						It("returns 201", func() {
							ctx.SMWithOAuth.POST(web.ServiceBrokersURL).WithJSON(postBrokerRequestWithNoLabels).
								Expect().
								Status(http.StatusCreated).
								JSON().Object().
								ContainsMap(expectedBrokerResponse).
								Keys().NotContains("services").Contains("credentials")

							assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
						})

						Specify("the whole catalog is returned from the repository in the brokers catalog field", func() {
							id := ctx.SMWithOAuth.POST(web.ServiceBrokersURL).WithJSON(postBrokerRequestWithNoLabels).
								Expect().
								Status(http.StatusCreated).JSON().Object().Value("id").String().Raw()

							byID := query.ByField(query.EqualsOperator, "id", id)
							brokerFromDB, err := repository.Get(context.TODO(), types.ServiceBrokerType, byID)
							Expect(err).ToNot(HaveOccurred())

							Expect(string(brokerFromDB.(*types.ServiceBroker).Catalog)).To(MatchJSON(string(brokerServer.Catalog)))
						})
					}

					Context("when name field is missing", func() {
						assertPOSTReturns400WhenFieldIsMissing("name")
					})

					Context("when broker_url field is missing", func() {
						assertPOSTReturns400WhenFieldIsMissing("broker_url")
					})

					Context("when credentials field is missing", func() {
						assertPOSTReturns400WhenFieldIsMissing("credentials")
					})

					Context("when description field is missing", func() {
						assertPOSTReturns201WhenFieldIsMissing("description")
					})
				})

				Context("when request body has invalid field", func() {
					Context("when name field is too long", func() {
						BeforeEach(func() {
							length := 500
							brokerName := make([]rune, length)
							for i := range brokerName {
								brokerName[i] = 'a'
							}
							postBrokerRequestWithLabels["name"] = string(brokerName)
						})

						It("returns 400", func() {
							ctx.SMWithOAuth.POST(web.ServiceBrokersURL).WithJSON(postBrokerRequestWithLabels).
								Expect().
								Status(http.StatusBadRequest).
								JSON().Object().
								Keys().Contains("error", "description")

							assertInvocationCount(brokerServer.CatalogEndpointRequests, 0)
						})
					})
				})

				Context("when obtaining the broker catalog fails because the broker is not reachable", func() {
					BeforeEach(func() {
						postBrokerRequestWithNoLabels["broker_url"] = "http://localhost:12345"
					})

					It("returns 502", func() {
						ctx.SMWithOAuth.POST(web.ServiceBrokersURL).WithJSON(postBrokerRequestWithNoLabels).
							Expect().
							Status(http.StatusBadGateway).JSON().Object().Keys().Contains("error", "description")
					})
				})

				Context("when the broker returns 404 for catalog", func() {
					BeforeEach(func() {
						brokerServer.CatalogHandler = func(rw http.ResponseWriter, req *http.Request) {
							common.SetResponse(rw, http.StatusNotFound, common.Object{})
						}
					})

					It("returns 400", func() {
						ctx.SMWithOAuth.POST(web.ServiceBrokersURL).WithJSON(postBrokerRequestWithNoLabels).
							Expect().
							Status(http.StatusBadRequest)
					})
				})

				Context("when the broker call for catalog times out", func() {
					const (
						timeoutDuration             = time.Millisecond * 500
						additionalDelayAfterTimeout = time.Second
					)

					BeforeEach(func() {
						settings := httpclient.DefaultSettings()
						settings.ResponseHeaderTimeout = timeoutDuration
						httpclient.Configure(settings)
						brokerServer.CatalogHandler = func(rw http.ResponseWriter, req *http.Request) {
							catalogStopDuration := timeoutDuration + additionalDelayAfterTimeout
							continueCtx, _ := context.WithTimeout(req.Context(), catalogStopDuration)

							<-continueCtx.Done()

							common.SetResponse(rw, http.StatusTeapot, common.Object{})
						}
					})

					AfterEach(func() {
						httpclient.Configure(httpclient.DefaultSettings())
					})

					It("returns 502", func() {
						ctx.SMWithOAuth.POST(web.ServiceBrokersURL).WithJSON(postBrokerRequestWithNoLabels).
							Expect().
							Status(http.StatusBadGateway).JSON().Object().Value("description").String().Contains("could not reach service broker")
					})
				})

				Context("when the broker catalog is incomplete", func() {
					verifyPOSTWhenCatalogFieldIsMissing := func(responseVerifier func(r *httpexpect.Response), fieldPath string) {
						BeforeEach(func() {
							catalog, err := sjson.Delete(string(common.NewRandomSBCatalog()), fieldPath)
							Expect(err).ToNot(HaveOccurred())

							brokerServer.Catalog = common.SBCatalog(catalog)
						})

						It("returns correct response", func() {
							responseVerifier(ctx.SMWithOAuth.POST(web.ServiceBrokersURL).WithJSON(postBrokerRequestWithNoLabels).Expect())

							assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)

						})
					}

					verifyPOSTWhenCatalogFieldHasValue := func(responseVerifier func(r *httpexpect.Response), fieldPath string, fieldValue interface{}) {
						BeforeEach(func() {
							catalog, err := sjson.Set(string(brokerServer.Catalog), fieldPath, fieldValue)
							Expect(err).ToNot(HaveOccurred())

							brokerServer.Catalog = common.SBCatalog(catalog)
						})

						It("returns correct response", func() {
							responseVerifier(ctx.SMWithOAuth.POST(web.ServiceBrokersURL).WithJSON(postBrokerRequestWithNoLabels).Expect())

							assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
						})
					}

					Context("when the broker catalog contains an incomplete service", func() {
						Context("that has an empty catalog id", func() {
							verifyPOSTWhenCatalogFieldIsMissing(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().Contains("error", "description")
							}, "services.0.id")
						})

						Context("that has an empty catalog name", func() {
							verifyPOSTWhenCatalogFieldIsMissing(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().Contains("error", "description")
							}, "services.0.name")
						})

						Context("that has an empty description", func() {
							verifyPOSTWhenCatalogFieldIsMissing(func(r *httpexpect.Response) {
								r.Status(http.StatusCreated).JSON().Object().Keys().NotContains("services").Contains("credentials")
							}, "services.0.description")
						})

						Context("that has invalid tags", func() {
							verifyPOSTWhenCatalogFieldHasValue(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().NotContains("services", "credentials")
							}, "services.0.tags", "{invalid")
						})

						Context("that has invalid requires", func() {
							verifyPOSTWhenCatalogFieldHasValue(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().NotContains("services", "credentials")
							}, "services.0.requires", "{invalid")
						})

						Context("that has invalid metadata", func() {
							verifyPOSTWhenCatalogFieldHasValue(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().NotContains("services", "credentials")
							}, "services.0.metadata", "{invalid")
						})
					})

					Context("when broker catalog contains an incomplete plan", func() {
						Context("that has an empty catalog id", func() {
							verifyPOSTWhenCatalogFieldIsMissing(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().Contains("error", "description")
							}, "services.0.plans.0.id")
						})

						Context("that has an empty catalog name", func() {
							verifyPOSTWhenCatalogFieldIsMissing(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().Contains("error", "description")
							}, "services.0.plans.0.name")
						})

						Context("that has an empty description", func() {
							verifyPOSTWhenCatalogFieldIsMissing(func(r *httpexpect.Response) {
								r.Status(http.StatusCreated).JSON().Object().Keys().NotContains("services").Contains("credentials")
							}, "services.0.plans.0.description")
						})

						Context("that has invalid metadata", func() {
							verifyPOSTWhenCatalogFieldHasValue(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().NotContains("services", "credentials")
							}, "services.0.plans.0.metadata", "{invalid")
						})

						Context("that has invalid schemas", func() {
							verifyPOSTWhenCatalogFieldHasValue(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().NotContains("services", "credentials")
							}, "services.0.plans.0.schemas", "{invalid")
						})
					})
				})

				Context("when fetching catalog fails", func() {
					BeforeEach(func() {
						brokerServer.CatalogHandler = func(w http.ResponseWriter, req *http.Request) {
							common.SetResponse(w, http.StatusInternalServerError, common.Object{})
						}
					})

					It("returns 400", func() {
						ctx.SMWithOAuth.POST(web.ServiceBrokersURL).
							WithJSON(postBrokerRequestWithNoLabels).
							Expect().Status(http.StatusBadRequest).
							JSON().Object().
							Keys().Contains("error", "description")

						assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
					})
				})

				Context("when fetching the catalog is successful", func() {
					assertPOSTReturns201 := func() {
						It("returns 201", func() {
							ctx.SMWithOAuth.POST(web.ServiceBrokersURL).WithJSON(postBrokerRequestWithNoLabels).
								Expect().
								Status(http.StatusCreated).
								JSON().Object().
								ContainsMap(expectedBrokerResponse).
								Keys().NotContains("services").Contains("credentials")

							assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
						})
					}

					Context("when broker URL does not end with trailing slash", func() {
						BeforeEach(func() {
							postBrokerRequestWithNoLabels["broker_url"] = strings.TrimRight(cast.ToString(postBrokerRequestWithNoLabels["broker_url"]), "/")
							expectedBrokerResponse["broker_url"] = strings.TrimRight(cast.ToString(expectedBrokerResponse["broker_url"]), "/")
						})

						assertPOSTReturns201()
					})

					Context("when broker URL ends with trailing slash", func() {
						BeforeEach(func() {
							postBrokerRequestWithNoLabels["broker_url"] = cast.ToString(postBrokerRequestWithNoLabels["broker_url"]) + "/"
							expectedBrokerResponse["broker_url"] = cast.ToString(expectedBrokerResponse["broker_url"]) + "/"
						})

						assertPOSTReturns201()
					})
				})

				Context("when broker with name already exists", func() {
					It("returns 409", func() {
						ctx.SMWithOAuth.POST(web.ServiceBrokersURL).WithJSON(postBrokerRequestWithNoLabels).
							Expect().
							Status(http.StatusCreated)

						ctx.SMWithOAuth.POST(web.ServiceBrokersURL).WithJSON(postBrokerRequestWithNoLabels).
							Expect().
							Status(http.StatusConflict).
							JSON().Object().
							Keys().Contains("error", "description")

						assertInvocationCount(brokerServer.CatalogEndpointRequests, 2)
					})
				})

				Context("Labelled", func() {
					Context("When labels are valid", func() {
						It("should return 201", func() {
							ctx.SMWithOAuth.POST(web.ServiceBrokersURL).
								WithJSON(postBrokerRequestWithLabels).
								Expect().Status(http.StatusCreated).JSON().Object().Keys().Contains("id", "labels")
						})
					})

					Context("When creating labeled broker with key containing forbidden character", func() {
						It("Should return 400", func() {
							labels[fmt.Sprintf("containing %s separator", query.Separator)] = common.Array{"val"}
							ctx.SMWithOAuth.POST(web.ServiceBrokersURL).
								WithJSON(postBrokerRequestWithLabels).
								Expect().Status(http.StatusBadRequest).JSON().Object().Value("description").String().Contains("cannot contain whitespaces")
						})
					})

					Context("When label key has new line", func() {
						It("Should return 400", func() {
							labels[`key with
	new line`] = common.Array{"label-value"}
							ctx.SMWithOAuth.POST(web.ServiceBrokersURL).
								WithJSON(postBrokerRequestWithLabels).
								Expect().Status(http.StatusBadRequest).JSON().Object().Value("description").String().Contains("cannot contain whitespaces")
						})
					})

					Context("When label value has new line", func() {
						It("Should return 400", func() {
							labels["cluster_id"] = common.Array{`{
	"key": "k1",
	"val": "val1"
	}`}
							ctx.SMWithOAuth.POST(web.ServiceBrokersURL).
								WithJSON(postBrokerRequestWithLabels).
								Expect().Status(http.StatusBadRequest)
						})
					})
				})
			})

			Describe("PATCH", func() {
				var brokerID string

				assertRepositoryReturnsExpectedCatalogAfterPatching := func(brokerID, expectedCatalog string) {
					ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + brokerID).
						WithJSON(common.Object{}).
						Expect()

					byID := query.ByField(query.EqualsOperator, "id", brokerID)
					brokerFromDB, err := repository.Get(context.TODO(), types.ServiceBrokerType, byID)
					Expect(err).ToNot(HaveOccurred())

					Expect(string(brokerFromDB.(*types.ServiceBroker).Catalog)).To(MatchJSON(expectedCatalog))
				}

				BeforeEach(func() {
					reply := ctx.SMWithOAuth.POST(web.ServiceBrokersURL).WithJSON(postBrokerRequestWithNoLabels).
						Expect().
						Status(http.StatusCreated).
						JSON().Object().
						ContainsMap(expectedBrokerResponse)

					brokerID = reply.Value("id").String().Raw()

					assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
					brokerServer.ResetCallHistory()
				})

				Context("when content type is not JSON", func() {
					It("returns 415", func() {
						ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).
							WithText("text").
							Expect().Status(http.StatusUnsupportedMediaType).
							JSON().Object().
							Keys().Contains("error", "description")

						assertInvocationCount(brokerServer.CatalogEndpointRequests, 0)
					})
				})

				Context("when broker is missing", func() {
					It("returns 404", func() {
						ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/no_such_id").
							WithJSON(postBrokerRequestWithNoLabels).
							Expect().Status(http.StatusNotFound).
							JSON().Object().
							Keys().Contains("error", "description")
					})
				})

				Context("when request body is not valid JSON", func() {
					It("returns 400", func() {
						ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).
							WithText("invalid json").
							WithHeader("content-type", "application/json").
							Expect().
							Status(http.StatusBadRequest).
							JSON().Object().
							Keys().Contains("error", "description")
					})
				})

				Context("when request body contains invalid credentials", func() {
					It("returns 400", func() {
						ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).
							WithJSON(common.Object{"credentials": "123"}).
							Expect().
							Status(http.StatusBadRequest).
							JSON().Object().
							Keys().Contains("error", "description")
					})
				})

				Context("when request body contains incomplete credentials", func() {
					It("returns 400", func() {
						ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).
							WithJSON(common.Object{"credentials": common.Object{"basic": common.Object{"password": ""}}}).
							Expect().
							Status(http.StatusBadRequest).
							JSON().Object().
							Keys().Contains("error", "description")
					})
				})

				Context("when broker with the name already exists", func() {
					var anotherTestBroker common.Object
					var anotherBrokerServer *common.BrokerServer

					BeforeEach(func() {
						anotherBrokerServer = common.NewBrokerServer()
						anotherBrokerServer.Username = "username"
						anotherBrokerServer.Password = "password"
						anotherTestBroker = common.Object{
							"name":        "another_name",
							"broker_url":  anotherBrokerServer.URL(),
							"description": "another_description",
							"credentials": common.Object{
								"basic": common.Object{
									"username": anotherBrokerServer.Username,
									"password": anotherBrokerServer.Password,
								},
							},
						}
					})

					AfterEach(func() {
						if anotherBrokerServer != nil {
							anotherBrokerServer.Close()
						}
					})

					It("returns 409", func() {
						ctx.SMWithOAuth.POST(web.ServiceBrokersURL).
							WithJSON(anotherTestBroker).
							Expect().
							Status(http.StatusCreated)

						assertInvocationCount(anotherBrokerServer.CatalogEndpointRequests, 1)

						ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).
							WithJSON(anotherTestBroker).
							Expect().Status(http.StatusConflict).
							JSON().Object().
							Keys().Contains("error", "description")

						assertInvocationCount(brokerServer.CatalogEndpointRequests, 0)
					})
				})

				Context("when credentials are updated", func() {
					It("returns 200", func() {
						brokerServer.Username = "updatedUsername"
						brokerServer.Password = "updatedPassword"
						updatedCredentials := common.Object{
							"credentials": common.Object{
								"basic": common.Object{
									"username": brokerServer.Username,
									"password": brokerServer.Password,
								},
							},
						}
						reply := ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + brokerID).
							WithJSON(updatedCredentials).
							Expect().
							Status(http.StatusOK).
							JSON().Object()

						assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)

						reply = ctx.SMWithOAuth.GET(web.ServiceBrokersURL + "/" + brokerID).
							Expect().
							Status(http.StatusOK).
							JSON().Object()
						reply.ContainsMap(expectedBrokerResponse)
					})
				})

				Context("when created_at provided in body", func() {
					It("should not change created_at", func() {
						createdAt := "2015-01-01T00:00:00Z"

						ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).
							WithJSON(common.Object{"created_at": createdAt}).
							Expect().
							Status(http.StatusOK).JSON().Object().
							ContainsKey("created_at").
							ValueNotEqual("created_at", createdAt)

						assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)

						ctx.SMWithOAuth.GET(web.ServiceBrokersURL+"/"+brokerID).
							Expect().
							Status(http.StatusOK).JSON().Object().
							ContainsKey("created_at").
							ValueNotEqual("created_at", createdAt)
					})
				})

				Context("when new broker server is available", func() {
					var (
						updatedBrokerServer           *common.BrokerServer
						updatedBrokerJSON             common.Object
						expectedUpdatedBrokerResponse common.Object
					)

					BeforeEach(func() {
						updatedBrokerServer = common.NewBrokerServer()
						updatedBrokerServer.Username = "updated_user"
						updatedBrokerServer.Password = "updated_password"
						updatedBrokerJSON = common.Object{
							"name":        "updated_name",
							"description": "updated_description",
							"broker_url":  updatedBrokerServer.URL(),
							"credentials": common.Object{
								"basic": common.Object{
									"username": updatedBrokerServer.Username,
									"password": updatedBrokerServer.Password,
								},
							},
						}

						expectedUpdatedBrokerResponse = common.Object{
							"name":        updatedBrokerJSON["name"],
							"description": updatedBrokerJSON["description"],
							"broker_url":  updatedBrokerJSON["broker_url"],
						}
					})

					AfterEach(func() {
						if updatedBrokerServer != nil {
							updatedBrokerServer.Close()
						}
					})

					Context("when all updatable fields are updated at once", func() {
						It("returns 200", func() {
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).
								WithJSON(updatedBrokerJSON).
								Expect().
								Status(http.StatusOK).
								JSON().Object().
								ContainsMap(expectedUpdatedBrokerResponse).
								Keys().NotContains("services", "credentials")

							assertInvocationCount(updatedBrokerServer.CatalogEndpointRequests, 1)

							ctx.SMWithOAuth.GET(web.ServiceBrokersURL+"/"+brokerID).
								Expect().
								Status(http.StatusOK).
								JSON().Object().
								ContainsMap(expectedUpdatedBrokerResponse).
								Keys().NotContains("services", "credentials")
						})
					})

					Context("when broker_url is changed and the credentials are correct", func() {
						It("returns 200", func() {
							updatedBrokerJSON := common.Object{
								"broker_url": updatedBrokerServer.URL(),
								"credentials": common.Object{
									"basic": common.Object{
										"username": brokerServer.Username,
										"password": brokerServer.Password,
									},
								},
							}
							updatedBrokerServer.Username = brokerServer.Username
							updatedBrokerServer.Password = brokerServer.Password

							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).
								WithJSON(updatedBrokerJSON).
								Expect().
								Status(http.StatusOK).
								JSON().Object().
								ContainsKey("broker_url").
								Keys().NotContains("services", "credentials")

							assertInvocationCount(brokerServer.CatalogEndpointRequests, 0)
							assertInvocationCount(updatedBrokerServer.CatalogEndpointRequests, 1)

							ctx.SMWithOAuth.GET(web.ServiceBrokersURL+"/"+brokerID).
								Expect().
								Status(http.StatusOK).
								JSON().Object().
								ContainsKey("broker_url").
								Keys().NotContains("services", "credentials")
						})
					})

					Context("when broker_url is changed but the credentials are wrong", func() {
						It("returns 400", func() {
							updatedBrokerJSON := common.Object{
								"broker_url": updatedBrokerServer.URL(),
							}
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).
								WithJSON(updatedBrokerJSON).
								Expect().
								Status(http.StatusBadRequest).JSON().Object().Keys().
								Contains("error", "description")

							assertInvocationCount(brokerServer.CatalogEndpointRequests, 0)

							ctx.SMWithOAuth.GET(web.ServiceBrokersURL+"/"+brokerID).
								Expect().
								Status(http.StatusOK).
								JSON().Object().
								ContainsMap(expectedBrokerResponse).
								Keys().NotContains("services", "credentials")
						})
					})

					Context("when broker_url is changed but the credentials are missing", func() {
						var updatedBrokerJSON common.Object

						BeforeEach(func() {
							updatedBrokerJSON = common.Object{
								"broker_url": updatedBrokerServer.URL(),
							}
						})

						Context("credentials object is missing", func() {
							It("returns 400", func() {
								ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).
									WithJSON(updatedBrokerJSON).
									Expect().
									Status(http.StatusBadRequest).JSON().Object().Keys().Contains("error", "description")
							})
						})

						Context("username is missing", func() {
							BeforeEach(func() {
								updatedBrokerJSON["credentials"] = common.Object{
									"basic": common.Object{
										"password": "b",
									},
								}
							})

							It("returns 400", func() {
								ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).
									WithJSON(updatedBrokerJSON).
									Expect().
									Status(http.StatusBadRequest).JSON().Object().Keys().Contains("error", "description")
							})
						})

						Context("password is missing", func() {
							BeforeEach(func() {
								updatedBrokerJSON["credentials"] = common.Object{
									"basic": common.Object{
										"username": "a",
									},
								}
							})

							It("returns 400", func() {
								ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).
									WithJSON(updatedBrokerJSON).
									Expect().
									Status(http.StatusBadRequest).JSON().Object().Keys().Contains("error", "description")
							})
						})
					})
				})

				Context("when fields are updated one by one", func() {
					It("returns 200", func() {
						for _, prop := range []string{"name", "description"} {
							updatedBrokerJSON := common.Object{}
							updatedBrokerJSON[prop] = "updated"
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).
								WithJSON(updatedBrokerJSON).
								Expect().
								Status(http.StatusOK).
								JSON().Object().
								ContainsMap(updatedBrokerJSON).
								Keys().NotContains("services", "credentials")

							ctx.SMWithOAuth.GET(web.ServiceBrokersURL+"/"+brokerID).
								Expect().
								Status(http.StatusOK).
								JSON().Object().
								ContainsMap(updatedBrokerJSON).
								Keys().NotContains("services", "credentials")

						}
						assertInvocationCount(brokerServer.CatalogEndpointRequests, 2)
					})
				})

				Context("when not updatable fields are provided in the request body", func() {
					Context("when broker id is provided in request body", func() {
						It("should not create the broker", func() {
							postBrokerRequestWithNoLabels = common.Object{"id": "123"}
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + brokerID).
								WithJSON(postBrokerRequestWithNoLabels).
								Expect().
								Status(http.StatusOK).
								JSON().Object().
								NotContainsMap(postBrokerRequestWithNoLabels)

							ctx.SMWithOAuth.GET(web.ServiceBrokersURL + "/123").
								Expect().
								Status(http.StatusNotFound)

							assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
						})
					})

					Context("when unmodifiable fields are provided in the request body", func() {
						BeforeEach(func() {
							postBrokerRequestWithNoLabels = common.Object{
								"created_at": "2016-06-08T16:41:26Z",
								"updated_at": "2016-06-08T16:41:26Z",
								"services":   common.Array{common.Object{"name": "serviceName"}},
							}
						})

						It("should not change them", func() {
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + brokerID).
								WithJSON(postBrokerRequestWithNoLabels).
								Expect().
								Status(http.StatusOK).
								JSON().Object().
								NotContainsMap(postBrokerRequestWithNoLabels)

							ctx.SMWithOAuth.List(web.ServiceBrokersURL).First().Object().
								ContainsMap(expectedBrokerResponse)

							assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
						})
					})
				})

				Context("when obtaining the broker catalog fails because the broker is not reachable", func() {
					BeforeEach(func() {
						postBrokerRequestWithNoLabels["broker_url"] = "http://localhost:12345"
					})

					It("returns 502", func() {
						ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).WithJSON(postBrokerRequestWithNoLabels).
							Expect().
							Status(http.StatusBadGateway).JSON().Object().Keys().Contains("error", "description")
					})
				})

				Context("when fetching the broker catalog fails", func() {
					BeforeEach(func() {
						brokerServer.CatalogHandler = func(w http.ResponseWriter, req *http.Request) {
							common.SetResponse(w, http.StatusInternalServerError, common.Object{})
						}
					})

					It("returns an error", func() {
						ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).
							WithJSON(postBrokerRequestWithNoLabels).
							Expect().Status(http.StatusBadRequest).
							JSON().Object().
							Keys().Contains("error", "description")

						assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
					})
				})

				Context("when the broker catalog is modified", func() {
					Context("when a new service offering with a plan existing for another service offering is added", func() {
						var anotherServiceID string
						var existingPlanID string

						BeforeEach(func() {
							existingServicePlan := gjson.Get(string(brokerServer.Catalog), "services.0.plans.0").String()
							existingPlanID = gjson.Get(existingServicePlan, "id").String()
							anotherServiceWithSamePlan, err := sjson.Set(common.GenerateTestServiceWithPlans(), "plans.-1", common.JSONToMap(existingServicePlan))
							Expect(err).ShouldNot(HaveOccurred())

							anotherService := common.JSONToMap(anotherServiceWithSamePlan)
							anotherServiceID = anotherService["id"].(string)
							Expect(anotherServiceID).ToNot(BeEmpty())

							catalog, err := sjson.Set(string(brokerServer.Catalog), "services.-1", anotherService)
							Expect(err).ShouldNot(HaveOccurred())

							brokerServer.Catalog = common.SBCatalog(catalog)
						})

						It("is returned from the Services API associated with the correct broker", func() {
							ctx.SMWithOAuth.List(web.ServiceOfferingsURL).
								Path("$[*].catalog_id").Array().NotContains(anotherServiceID)
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + brokerID).
								WithJSON(common.Object{}).
								Expect().
								Status(http.StatusOK)

							By("updating broker again with 2 services with identical plans, should succeed")
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + brokerID).
								WithJSON(common.Object{}).
								Expect().
								Status(http.StatusOK)

							servicesJsonResp := ctx.SMWithOAuth.List(web.ServiceOfferingsURL)
							servicesJsonResp.Path("$[*].catalog_id").Array().Contains(anotherServiceID)
							servicesJsonResp.Path("$[*].broker_id").Array().Contains(brokerID)

							var soID string
							for _, so := range servicesJsonResp.Iter() {
								sbID := so.Object().Value("broker_id").String().Raw()
								Expect(sbID).ToNot(BeEmpty())

								catalogID := so.Object().Value("catalog_id").String().Raw()
								Expect(catalogID).ToNot(BeEmpty())

								if catalogID == anotherServiceID && sbID == brokerID {
									soID = so.Object().Value("id").String().Raw()
									Expect(soID).ToNot(BeEmpty())
									break
								}
							}

							plansJsonResp := ctx.SMWithOAuth.List(web.ServicePlansURL)
							plansJsonResp.Path("$[*].catalog_id").Array().Contains(existingPlanID)
							plansJsonResp.Path("$[*].service_offering_id").Array().Contains(soID)

							assertInvocationCount(brokerServer.CatalogEndpointRequests, 2)
						})

						It("is returned from the repository as part of the brokers catalog field", func() {
							assertRepositoryReturnsExpectedCatalogAfterPatching(brokerID, string(brokerServer.Catalog))
						})
					})

					Context("when a new service offering with new plans is added", func() {
						var anotherServiceID string
						var anotherPlanID string

						BeforeEach(func() {
							anotherPlan := common.JSONToMap(common.GeneratePaidTestPlan())
							anotherPlanID = anotherPlan["id"].(string)
							anotherServiceWithAnotherPlan, err := sjson.Set(common.GenerateTestServiceWithPlans(), "plans.-1", anotherPlan)
							Expect(err).ShouldNot(HaveOccurred())

							anotherService := common.JSONToMap(anotherServiceWithAnotherPlan)
							anotherServiceID = anotherService["id"].(string)
							Expect(anotherServiceID).ToNot(BeEmpty())

							catalog, err := sjson.Set(string(brokerServer.Catalog), "services.-1", anotherService)
							Expect(err).ShouldNot(HaveOccurred())

							brokerServer.Catalog = common.SBCatalog(catalog)
						})

						It("is returned from the Services API associated with the correct broker", func() {
							ctx.SMWithOAuth.List(web.ServiceOfferingsURL).
								Path("$[*].catalog_id").Array().NotContains(anotherServiceID)
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + brokerID).
								WithJSON(common.Object{}).
								Expect().
								Status(http.StatusOK)
							servicesJsonResp := ctx.SMWithOAuth.List(web.ServiceOfferingsURL)
							servicesJsonResp.Path("$[*].catalog_id").Array().Contains(anotherServiceID)
							servicesJsonResp.Path("$[*].broker_id").Array().Contains(brokerID)

							var soID string
							for _, so := range servicesJsonResp.Iter() {
								sbID := so.Object().Value("broker_id").String().Raw()
								Expect(sbID).ToNot(BeEmpty())

								catalogID := so.Object().Value("catalog_id").String().Raw()
								Expect(catalogID).ToNot(BeEmpty())

								if catalogID == anotherServiceID && sbID == brokerID {
									soID = so.Object().Value("id").String().Raw()
									Expect(soID).ToNot(BeEmpty())
									break
								}
							}

							plansJsonResp := ctx.SMWithOAuth.List(web.ServicePlansURL)
							plansJsonResp.Path("$[*].catalog_id").Array().Contains(anotherPlanID)
							plansJsonResp.Path("$[*].service_offering_id").Array().Contains(soID)

							assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
						})

						It("is returned from the repository as part of the brokers catalog field", func() {
							assertRepositoryReturnsExpectedCatalogAfterPatching(brokerID, string(brokerServer.Catalog))
						})
					})

					verifyPATCHWhenCatalogFieldIsMissing := func(responseVerifier func(r *httpexpect.Response), shouldUpdateCatalog bool, fieldPath string) {
						var expectedCatalog string

						BeforeEach(func() {
							catalog, err := sjson.Delete(string(brokerServer.Catalog), fieldPath)
							Expect(err).ToNot(HaveOccurred())
							if !shouldUpdateCatalog {
								expectedCatalog = string(brokerServer.Catalog)
							} else {
								expectedCatalog = string(catalog)
							}
							brokerServer.Catalog = common.SBCatalog(catalog)
						})

						It("returns correct response", func() {
							responseVerifier(ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + brokerID).WithJSON(common.Object{}).Expect())

							assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
						})

						Specify("the catalog is correctly returned by the repository", func() {
							assertRepositoryReturnsExpectedCatalogAfterPatching(brokerID, expectedCatalog)

						})
					}

					verifyPATCHWhenCatalogFieldHasValue := func(responseVerifier func(r *httpexpect.Response), shouldUpdateCatalog bool, fieldPath string, fieldValue interface{}) {
						var expectedCatalog string

						BeforeEach(func() {
							catalog, err := sjson.Set(string(brokerServer.Catalog), fieldPath, fieldValue)
							Expect(err).ToNot(HaveOccurred())
							if !shouldUpdateCatalog {
								expectedCatalog = string(brokerServer.Catalog)
							} else {
								expectedCatalog = string(catalog)
							}
							brokerServer.Catalog = common.SBCatalog(catalog)
						})

						It("returns correct response", func() {
							responseVerifier(ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + brokerID).WithJSON(common.Object{}).Expect())

							assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
						})

						Specify("the catalog is correctly returned by the repository", func() {
							assertRepositoryReturnsExpectedCatalogAfterPatching(brokerID, expectedCatalog)
						})
					}

					Context("when a new service offering is added", func() {
						var anotherServiceID string

						BeforeEach(func() {
							anotherService := common.JSONToMap(common.GenerateTestServiceWithPlans())
							anotherServiceID = anotherService["id"].(string)
							Expect(anotherServiceID).ToNot(BeEmpty())

							currServices, err := sjson.Set(string(brokerServer.Catalog), "services.-1", anotherService)
							Expect(err).ShouldNot(HaveOccurred())

							brokerServer.Catalog = common.SBCatalog(currServices)
						})

						It("is returned from the Services API associated with the correct broker", func() {
							ctx.SMWithOAuth.List(web.ServiceOfferingsURL).
								Path("$[*].catalog_id").Array().NotContains(anotherServiceID)

							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + brokerID).
								WithJSON(common.Object{}).
								Expect().
								Status(http.StatusOK)

							jsonResp := ctx.SMWithOAuth.List(web.ServiceOfferingsURL)
							jsonResp.Path("$[*].catalog_id").Array().Contains(anotherServiceID)
							jsonResp.Path("$[*].broker_id").Array().Contains(brokerID)

							assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
						})

						It("is returned from the repository as part of the brokers catalog field", func() {
							assertRepositoryReturnsExpectedCatalogAfterPatching(brokerID, string(brokerServer.Catalog))

						})
					})

					Context("when an existing service offering is removed", func() {
						var serviceOfferingID string
						var planIDsForService []string

						BeforeEach(func() {
							planIDsForService = make([]string, 0)

							catalogServiceID := gjson.Get(string(brokerServer.Catalog), "services.0.id").Str
							Expect(catalogServiceID).ToNot(BeEmpty())

							serviceOffering := ctx.SMWithOAuth.ListWithQuery(web.ServiceOfferingsURL,
								fmt.Sprintf("fieldQuery=broker_id eq '%s' and catalog_id eq '%s'", brokerID, catalogServiceID))
							Expect(serviceOffering.Length().Equal(1))
							serviceOfferingID = serviceOffering.First().Object().Value("id").String().Raw()
							plans := ctx.SMWithOAuth.ListWithQuery(web.ServicePlansURL,
								fmt.Sprintf("fieldQuery=service_offering_id eq '%s'", serviceOfferingID)).Iter()

							for _, plan := range plans {
								planID := plan.Object().Value("id").String().Raw()
								planIDsForService = append(planIDsForService, planID)
							}

							s, err := sjson.Delete(string(brokerServer.Catalog), "services.0")
							Expect(err).ShouldNot(HaveOccurred())
							brokerServer.Catalog = common.SBCatalog(s)
						})

						Context("with no existing service instances", func() {
							It("is no longer returned by the Services and Plans API", func() {
								ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + brokerID).
									WithJSON(common.Object{}).
									Expect().
									Status(http.StatusOK)

								ctx.SMWithOAuth.List(web.ServiceOfferingsURL).NotContains(serviceOfferingID)
								ctx.SMWithOAuth.List(web.ServicePlansURL).NotContains(planIDsForService)

								assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
							})

							It("is not returned from the repository as part of the brokers catalog field", func() {
								assertRepositoryReturnsExpectedCatalogAfterPatching(brokerID, string(brokerServer.Catalog))
							})
						})

						Context("with existing service instances", func() {
							var serviceInstances []*types.ServiceInstance

							BeforeEach(func() {
								serviceInstances = make([]*types.ServiceInstance, 0)
								for _, planID := range planIDsForService {
									serviceInstance := common.CreateInstanceInPlatformForPlan(ctx, ctx.TestPlatform.ID, planID)
									serviceInstances = append(serviceInstances, serviceInstance)
								}
							})

							AfterEach(func() {
								for _, serviceInstance := range serviceInstances {
									err := common.DeleteInstance(ctx, serviceInstance.ID, serviceInstance.ServicePlanID)
									Expect(err).ToNot(HaveOccurred())
								}
							})

							It("should return 400 with user-friendly message", func() {
								ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + brokerID).
									WithJSON(common.Object{}).
									Expect().
									Status(http.StatusConflict).
									JSON().Object().
									Value("error").String().Contains("ExistingReferenceEntity")

								ctx.SMWithOAuth.GET(web.ServiceOfferingsURL + "/" + serviceOfferingID).
									Expect().
									Status(http.StatusOK).Body().NotEmpty()

								servicePlans := ctx.SMWithOAuth.ListWithQuery(web.ServicePlansURL, "fieldQuery="+fmt.Sprintf("id in ('%s')", strings.Join(planIDsForService, "','")))
								servicePlans.NotEmpty()
								servicePlans.Length().Equal(len(planIDsForService))
							})
						})
					})

					Context("when an existing service offering is modified", func() {
						Context("when catalog service id is modified but the catalog name is not", func() {
							var expectedCatalog string

							BeforeEach(func() {
								expectedCatalog = string(brokerServer.Catalog)
								catalog, err := sjson.Set(string(brokerServer.Catalog), "services.0.id", "new-id")
								Expect(err).ToNot(HaveOccurred())

								brokerServer.Catalog = common.SBCatalog(catalog)
							})

							It("returns 409", func() {
								ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).WithJSON(postBrokerRequestWithNoLabels).
									Expect().
									Status(http.StatusConflict).JSON().Object().Keys().Contains("error", "description")

								assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
							})

							Specify("the catalog before the modification is returned by the repository", func() {
								assertRepositoryReturnsExpectedCatalogAfterPatching(brokerID, expectedCatalog)

							})
						})

						Context("when catalog service id is removed", func() {
							verifyPATCHWhenCatalogFieldIsMissing(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().Contains("error", "description")
							}, false, "services.0.id")
						})

						Context("when catalog service name is removed", func() {
							verifyPATCHWhenCatalogFieldIsMissing(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().Contains("error", "description")
							}, false, "services.0.name")
						})

						Context("when catalog service description is removed", func() {
							verifyPATCHWhenCatalogFieldIsMissing(func(r *httpexpect.Response) {
								r.Status(http.StatusOK)
							}, true, "services.0.description")
						})

						Context("when tags are invalid json", func() {
							verifyPATCHWhenCatalogFieldHasValue(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().Contains("error", "description")
							}, false, "services.0.tags", "invalidddd")
						})

						Context("when requires is invalid json", func() {
							verifyPATCHWhenCatalogFieldHasValue(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().Contains("error", "description")
							}, false, "services.0.requires", "{invalid")
						})

						Context("when metadata is invalid json", func() {
							verifyPATCHWhenCatalogFieldHasValue(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().Contains("error", "description")
							}, false, "services.0.metadata", "{invalid")
						})
					})

					Context("when a new service plan is added", func() {
						var anotherPlanID string
						var serviceOfferingID string

						BeforeEach(func() {
							anotherPlan := common.JSONToMap(common.GeneratePaidTestPlan())
							anotherPlanID = anotherPlan["id"].(string)
							Expect(anotherPlan).ToNot(BeEmpty())
							catalogServiceID := gjson.Get(string(brokerServer.Catalog), "services.0.id").Str
							Expect(catalogServiceID).ToNot(BeEmpty())

							serviceOfferings := ctx.SMWithOAuth.List(web.ServiceOfferingsURL).Iter()

							for _, so := range serviceOfferings {
								sbID := so.Object().Value("broker_id").String().Raw()
								Expect(sbID).ToNot(BeEmpty())

								catalogID := so.Object().Value("catalog_id").String().Raw()
								Expect(catalogID).ToNot(BeEmpty())

								if catalogID == catalogServiceID && sbID == brokerID {
									serviceOfferingID = so.Object().Value("id").String().Raw()
									Expect(catalogServiceID).ToNot(BeEmpty())
									break
								}
							}
							s, err := sjson.Set(string(brokerServer.Catalog), "services.0.plans.2", anotherPlan)
							Expect(err).ShouldNot(HaveOccurred())
							brokerServer.Catalog = common.SBCatalog(s)
						})

						It("is returned from the Plans API associated with the correct service offering", func() {
							ctx.SMWithOAuth.List(web.ServicePlansURL).
								Path("$[*].catalog_id").Array().NotContains(anotherPlanID)

							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + brokerID).
								WithJSON(common.Object{}).
								Expect().
								Status(http.StatusOK)

							jsonResp := ctx.SMWithOAuth.List(web.ServicePlansURL)
							jsonResp.Path("$[*].catalog_id").Array().Contains(anotherPlanID)
							jsonResp.Path("$[*].service_offering_id").Array().Contains(serviceOfferingID)

							assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
						})

						It("is returned from the repository as part of the brokers catalog field", func() {
							assertRepositoryReturnsExpectedCatalogAfterPatching(brokerID, string(brokerServer.Catalog))

						})
					})

					Context("when an existing service plan is removed", func() {
						var removedPlanCatalogID string

						BeforeEach(func() {
							removedPlanCatalogID = gjson.Get(string(brokerServer.Catalog), "services.0.plans.0.id").Str
							Expect(removedPlanCatalogID).ToNot(BeEmpty())
							s, err := sjson.Delete(string(brokerServer.Catalog), "services.0.plans.0")
							Expect(err).ShouldNot(HaveOccurred())
							brokerServer.Catalog = common.SBCatalog(s)
						})

						Context("with no existing service instances", func() {
							It("is no longer returned by the Plans API", func() {
								ctx.SMWithOAuth.List(web.ServicePlansURL).
									Path("$[*].catalog_id").Array().Contains(removedPlanCatalogID)

								ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + brokerID).
									WithJSON(common.Object{}).
									Expect().
									Status(http.StatusOK)

								ctx.SMWithOAuth.List(web.ServicePlansURL).
									Path("$[*].catalog_id").Array().NotContains(removedPlanCatalogID)

								assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
							})

							It("is not returned from the repository as part of the brokers catalog field", func() {
								assertRepositoryReturnsExpectedCatalogAfterPatching(brokerID, string(brokerServer.Catalog))
							})
						})

						Context("with existing service instances", func() {
							var serviceInstance *types.ServiceInstance

							BeforeEach(func() {
								removedPlanID := ctx.SMWithOAuth.ListWithQuery(web.ServicePlansURL, fmt.Sprintf("fieldQuery=catalog_id eq '%s'", removedPlanCatalogID)).
									First().Object().Value("id").String().Raw()

								serviceInstance = common.CreateInstanceInPlatformForPlan(ctx, ctx.TestPlatform.ID, removedPlanID)

							})

							AfterEach(func() {
								err := common.DeleteInstance(ctx, serviceInstance.ID, serviceInstance.ServicePlanID)
								Expect(err).ToNot(HaveOccurred())
							})

							It("should return 400 with user-friendly message", func() {
								ctx.SMWithOAuth.List(web.ServicePlansURL).
									Path("$[*].catalog_id").Array().Contains(removedPlanCatalogID)

								ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + brokerID).
									WithJSON(common.Object{}).
									Expect().
									Status(http.StatusConflict).
									JSON().Object().
									Value("error").String().Contains("ExistingReferenceEntity")

								ctx.SMWithOAuth.List(web.ServicePlansURL).
									Path("$[*].catalog_id").Array().Contains(removedPlanCatalogID)
							})
						})
					})

					Context("when an existing service plan is modified", func() {
						Context("when catalog service plan id is modified but the catalog name is not", func() {
							var expectedCatalog string

							BeforeEach(func() {
								expectedCatalog = string(brokerServer.Catalog)

								catalog, err := sjson.Set(string(brokerServer.Catalog), "services.0.plans.0.id", "new-id")
								Expect(err).ToNot(HaveOccurred())

								brokerServer.Catalog = common.SBCatalog(catalog)
							})

							It("returns 409", func() {
								ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL+"/"+brokerID).WithJSON(postBrokerRequestWithNoLabels).
									Expect().
									Status(http.StatusConflict).JSON().Object().Keys().Contains("error", "description")

								assertInvocationCount(brokerServer.CatalogEndpointRequests, 1)
							})

							Specify("the catalog before the modification is returned by the repository", func() {
								assertRepositoryReturnsExpectedCatalogAfterPatching(brokerID, expectedCatalog)

							})
						})

						Context("when catalog plan id is removed", func() {
							verifyPATCHWhenCatalogFieldIsMissing(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().Contains("error", "description")
							}, false, "services.0.plans.0.id")
						})

						Context("when catalog plan name is removed", func() {
							verifyPATCHWhenCatalogFieldIsMissing(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().Contains("error", "description")
							}, false, "services.0.plans.0.name")
						})

						Context("when catalog plan description is removed", func() {
							verifyPATCHWhenCatalogFieldIsMissing(func(r *httpexpect.Response) {
								r.Status(http.StatusOK)
							}, true, "services.0.plans.0.description")
						})

						Context("when schemas is invalid json", func() {
							verifyPATCHWhenCatalogFieldHasValue(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().Contains("error", "description")
							}, false, "services.0.plans.0.schemas", "{invalid")
						})

						Context("when metadata is invalid json", func() {
							verifyPATCHWhenCatalogFieldHasValue(func(r *httpexpect.Response) {
								r.Status(http.StatusBadRequest).JSON().Object().Keys().Contains("error", "description")
							}, false, "services.0.plans.0.metadata", []byte(`{invalid`))
						})
					})
				})

				Describe("Labelled", func() {
					var id string
					var patchLabels []query.LabelChange
					var patchLabelsBody map[string]interface{}
					changedLabelKey := "label_key"
					changedLabelValues := []string{"label_value1", "label_value2"}
					operation := query.AddLabelOperation
					BeforeEach(func() {
						patchLabels = []query.LabelChange{}
					})
					JustBeforeEach(func() {
						patchLabelsBody = make(map[string]interface{})
						patchLabels = append(patchLabels, query.LabelChange{
							Operation: operation,
							Key:       changedLabelKey,
							Values:    changedLabelValues,
						})
						patchLabelsBody["labels"] = patchLabels

						id = ctx.SMWithOAuth.POST(web.ServiceBrokersURL).
							WithJSON(postBrokerRequestWithLabels).
							Expect().Status(http.StatusCreated).JSON().Object().Value("id").String().Raw()
					})

					Context("Add new label", func() {
						It("Should return 200", func() {
							label := types.Labels{changedLabelKey: changedLabelValues}
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + id).
								WithJSON(patchLabelsBody).
								Expect().
								Status(http.StatusOK).JSON().Object().Value("labels").Object().ContainsMap(label)
						})
					})

					Context("Add label with existing key and value", func() {
						It("Should return 200", func() {
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + id).
								WithJSON(patchLabelsBody).
								Expect().
								Status(http.StatusOK)

							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + id).
								WithJSON(patchLabelsBody).
								Expect().
								Status(http.StatusOK)
						})
					})

					Context("Add new label value", func() {
						BeforeEach(func() {
							operation = query.AddLabelValuesOperation
							changedLabelKey = "cluster_id"
							changedLabelValues = []string{"new-label-value"}
						})
						It("Should return 200", func() {
							var labelValuesObj []interface{}
							for _, val := range changedLabelValues {
								labelValuesObj = append(labelValuesObj, val)
							}
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + id).
								WithJSON(patchLabelsBody).
								Expect().
								Status(http.StatusOK).JSON().
								Path("$.labels").Object().Values().Path("$[*][*]").Array().Contains(labelValuesObj...)
						})
					})

					Context("Add new label value to a non-existing label", func() {
						BeforeEach(func() {
							operation = query.AddLabelValuesOperation
							changedLabelKey = "cluster_id_new"
							changedLabelValues = []string{"new-label-value"}
						})
						It("Should return 200", func() {
							var labelValuesObj []interface{}
							for _, val := range changedLabelValues {
								labelValuesObj = append(labelValuesObj, val)
							}

							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + id).
								WithJSON(patchLabelsBody).
								Expect().
								Status(http.StatusOK).JSON().
								Path("$.labels").Object().Values().Path("$[*][*]").Array().Contains(labelValuesObj...)
						})
					})

					Context("Add duplicate label value", func() {
						BeforeEach(func() {
							operation = query.AddLabelValuesOperation
							changedLabelKey = "cluster_id"
							values := labels["cluster_id"].([]interface{})
							changedLabelValues = []string{values[0].(string)}
						})
						It("Should return 200", func() {
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + id).
								WithJSON(patchLabelsBody).
								Expect().
								Status(http.StatusOK)
						})
					})

					Context("Remove a label", func() {
						BeforeEach(func() {
							operation = query.RemoveLabelOperation
							changedLabelKey = "cluster_id"
						})
						It("Should return 200", func() {
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + id).
								WithJSON(patchLabelsBody).
								Expect().
								Status(http.StatusOK).JSON().
								Path("$.labels").Object().Keys().NotContains(changedLabelKey)
						})
					})

					Context("Remove a label and providing no key", func() {
						BeforeEach(func() {
							operation = query.RemoveLabelOperation
							changedLabelKey = ""
						})
						It("Should return 400", func() {
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + id).
								WithJSON(patchLabelsBody).
								Expect().
								Status(http.StatusBadRequest)
						})
					})

					Context("Remove a label key which does not exist", func() {
						BeforeEach(func() {
							operation = query.RemoveLabelOperation
							changedLabelKey = "non-existing-ey"
						})
						It("Should return 200", func() {
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + id).
								WithJSON(patchLabelsBody).
								Expect().
								Status(http.StatusOK)
						})
					})

					Context("Remove label values and providing a single value", func() {
						var valueToRemove string
						BeforeEach(func() {
							operation = query.RemoveLabelValuesOperation
							changedLabelKey = "cluster_id"
							valueToRemove = labels[changedLabelKey].([]interface{})[0].(string)
							changedLabelValues = []string{valueToRemove}
						})
						It("Should return 200", func() {
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + id).
								WithJSON(patchLabelsBody).
								Expect().
								Status(http.StatusOK).JSON().
								Path("$.labels[*]").Array().NotContains(valueToRemove)
						})
					})

					Context("Remove label values and providing multiple values", func() {
						var valuesToRemove []string
						BeforeEach(func() {
							operation = query.RemoveLabelValuesOperation
							changedLabelKey = "org_id"
							val1 := labels[changedLabelKey].([]interface{})[0].(string)
							val2 := labels[changedLabelKey].([]interface{})[1].(string)
							valuesToRemove = []string{val1, val2}
							changedLabelValues = valuesToRemove
						})
						It("Should return 200", func() {
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + id).
								WithJSON(patchLabelsBody).
								Expect().
								Status(http.StatusOK).JSON().
								Path("$.labels[*]").Array().NotContains(valuesToRemove)
						})
					})

					Context("Remove all label values for a key", func() {
						var valuesToRemove []string
						BeforeEach(func() {
							operation = query.RemoveLabelValuesOperation
							changedLabelKey = "cluster_id"
							labelValues := labels[changedLabelKey].([]interface{})
							for _, val := range labelValues {
								valuesToRemove = append(valuesToRemove, val.(string))
							}
							changedLabelValues = valuesToRemove
						})
						It("Should return 200 with this key gone", func() {
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + id).
								WithJSON(patchLabelsBody).
								Expect().
								Status(http.StatusOK).JSON().
								Path("$.labels").Object().Keys().NotContains(changedLabelKey)
						})
					})

					Context("Remove label values and not providing value to remove", func() {
						BeforeEach(func() {
							operation = query.RemoveLabelValuesOperation
							changedLabelValues = []string{}
						})
						It("Should return 400", func() {
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + id).
								WithJSON(patchLabelsBody).
								Expect().
								Status(http.StatusBadRequest)
						})
					})

					Context("Remove label value which does not exist", func() {
						BeforeEach(func() {
							operation = query.RemoveLabelValuesOperation
							changedLabelKey = "cluster_id"
							changedLabelValues = []string{"non-existing-value"}
						})
						It("Should return 200", func() {
							ctx.SMWithOAuth.PATCH(web.ServiceBrokersURL + "/" + id).
								WithJSON(patchLabelsBody).
								Expect().
								Status(http.StatusOK)
						})
					})
				})
			})

			Describe("DELETE", func() {
				Context("with existing service instances to some broker plan", func() {
					var (
						brokerID        string
						serviceInstance *types.ServiceInstance
					)

					BeforeEach(func() {
						brokerID, serviceInstance = common.CreateInstanceInPlatform(ctx, ctx.TestPlatform.ID)
					})

					AfterEach(func() {
						err := common.DeleteInstance(ctx, serviceInstance.ID, serviceInstance.ServicePlanID)
						Expect(err).ToNot(HaveOccurred())
					})

					It("should return 400 with user-friendly message", func() {
						ctx.SMWithOAuth.DELETE(web.ServiceBrokersURL + "/" + brokerID).
							Expect().
							Status(http.StatusConflict).
							JSON().Object().
							Value("error").String().Contains("ExistingReferenceEntity")
					})
				})

				Context("when attempting async bulk delete", func() {
					It("should return 400", func() {
						ctx.SMWithOAuth.DELETE(web.ServiceBrokersURL).
							WithQuery("fieldQuery", "id in ('id1','id2','id3')").
							WithQuery("async", "true").
							Expect().
							Status(http.StatusBadRequest).
							JSON().Object().
							Value("description").String().Contains("Only one resource can be deleted asynchronously at a time")
					})
				})
			})
		})
	},
})

func blueprint(setNullFieldsValues bool) func(ctx *common.TestContext, auth *common.SMExpect, async bool) common.Object {
	return func(ctx *common.TestContext, auth *common.SMExpect, async bool) common.Object {
		brokerJSON := common.GenerateRandomBroker()

		if !setNullFieldsValues {
			delete(brokerJSON, "description")
		}

		var obj map[string]interface{}
		resp := auth.POST(web.ServiceBrokersURL).WithQuery("async", strconv.FormatBool(async)).WithJSON(brokerJSON).Expect()
		if async {
			obj = test.ExpectSuccessfulAsyncResourceCreation(resp, auth, web.ServiceBrokersURL)
		} else {
			obj = resp.Status(http.StatusCreated).JSON().Object().Raw()
			delete(obj, "credentials")
		}

		return obj
	}
}

type labeledBroker common.Object

func (b labeledBroker) AddLabel(label common.Object) {
	b["labels"] = append(b["labels"].(common.Array), label)
}
