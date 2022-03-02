package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/grafana/pkg/services/featuremgmt"
	"github.com/grafana/grafana/pkg/services/sqlstore/mockstore"

	"golang.org/x/oauth2"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana/pkg/components/simplejson"
	"github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/plugins"
	"github.com/grafana/grafana/pkg/services/query"
	"github.com/grafana/grafana/pkg/services/secrets/fakes"
	"github.com/stretchr/testify/assert"
)

var (
	queryDatasourceInput = `{
		"from": "",
		"to": "",
		"queries": [
			{
				"datasource": {
					"type": "datasource",
					"uid": "grafana"
				},
				"queryType": "randomWalk",
				"refId": "A"
			}
		]
	}`

	getDashboardByIdOutput = `{
		"annotations": {
			"list": [
			{
				"builtIn": 1,
				"datasource": "-- Grafana --",
				"enable": true,
				"hide": true,
				"iconColor": "rgba(0, 211, 255, 1)",
				"name": "Annotations & Alerts",
				"target": {
					"limit": 100,
					"matchAny": false,
					"tags": [],
					"type": "dashboard"
				},
				"type": "dashboard"
			}
			]
		},
		"editable": true,
		"fiscalYearStartMonth": 0,
		"graphTooltip": 0,
		"links": [],
		"liveNow": false,
		"panels": [
		{
			"fieldConfig": {
				"defaults": {
					"color": {
						"mode": "palette-classic"
					},
					"custom": {
						"axisLabel": "",
						"axisPlacement": "auto",
						"barAlignment": 0,
						"drawStyle": "line",
						"fillOpacity": 0,
						"gradientMode": "none",
						"hideFrom": {
							"legend": false,
							"tooltip": false,
							"viz": false
						},
						"lineInterpolation": "linear",
						"lineWidth": 1,
						"pointSize": 5,
						"scaleDistribution": {
							"type": "linear"
						},
						"showPoints": "auto",
						"spanNulls": false,
						"stacking": {
							"group": "A",
							"mode": "none"
						},
						"thresholdsStyle": {
							"mode": "off"
						}
					},
					"mappings": [],
					"thresholds": {
						"mode": "absolute",
						"steps": [
						{
							"color": "green",
							"value": null
						},
						{
							"color": "red",
							"value": 80
						}
						]
					}
				},
				"overrides": []
			},
			"gridPos": {
				"h": 9,
				"w": 12,
				"x": 0,
				"y": 0
			},
			"id": 2,
			"options": {
				"legend": {
					"calcs": [],
					"displayMode": "list",
					"placement": "bottom"
				},
				"tooltip": {
					"mode": "single",
					"sort": "none"
				}
			},
			"title": "Panel Title",
			"type": "timeseries"
		}
		],
		"schemaVersion": 35,
		"style": "dark",
		"tags": [],
		"templating": {
			"list": []
		},
		"time": {
			"from": "now-6h",
			"to": "now"
		},
		"timepicker": {},
		"timezone": "",
		"title": "New dashboard",
		"version": 0,
		"weekStart": ""
	}`
)

type fakePluginRequestValidator struct {
	err error
}

func (rv *fakePluginRequestValidator) Validate(dsURL string, req *http.Request) error {
	return rv.err
}

type fakeOAuthTokenService struct {
	passThruEnabled bool
	token           *oauth2.Token
}

func (ts *fakeOAuthTokenService) GetCurrentOAuthToken(context.Context, *models.SignedInUser) *oauth2.Token {
	return ts.token
}

func (ts *fakeOAuthTokenService) IsOAuthPassThruEnabled(*models.DataSource) bool {
	return ts.passThruEnabled
}

func (c *dashboardFakePluginClient) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	c.req = req
	return nil, nil
}

type dashboardFakePluginClient struct {
	plugins.Client

	req *backend.QueryDataRequest
}

// `/dashboards/org/:orgId/uid/:dashboardUid/panels/:panelId/query` endpoints test
func TestAPIEndpoint_Metrics_QueryMetricsFromDashboard(t *testing.T) {
	sc := setupHTTPServer(t, false, false)

	ss := mockstore.NewSQLStoreMock()
	sc.db = ss

	setInitCtxSignedInViewer(sc.initCtx)
	sc.hs.queryDataService = query.ProvideService(
		nil,
		nil,
		nil,
		&fakePluginRequestValidator{},
		fakes.NewFakeSecretsService(),
		&dashboardFakePluginClient{},
		&fakeOAuthTokenService{},
	)

	sc.hs.Features = featuremgmt.WithFeatures(featuremgmt.FlagValidatedQueries, true)

	dashboardJson, err := simplejson.NewFromReader(strings.NewReader(getDashboardByIdOutput))
	if err != nil {
		t.Fatalf("Failed to unmarshal dashboard json: %v", err)
	}

	t.Run("Can query a valid dashboard", func(t *testing.T) {

		ss.ExpectedDashboard = &models.Dashboard{
			Uid:   "1",
			OrgId: testOrgID,
			Data:  dashboardJson,
		}

		//url := fmt.Sprintf("/api/dashboards/org/%d/uid/%s/panels/%s/query", testOrgID, "1", "2")
		//fmt.Println("POTATO", url)

		response := callAPI(
			sc.server,
			http.MethodPost,
			fmt.Sprintf("/api/dashboards/org/%d/uid/%s/panels/%s/query", testOrgID, "1", "2"),
			strings.NewReader(queryDatasourceInput),
			t,
		)
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("Cannot query without a valid orgid or dashboard or panel ID", func(t *testing.T) {
		response := callAPI(
			sc.server,
			http.MethodPost,
			"/api/dashboards/orgid//uid//panels//query",
			strings.NewReader(queryDatasourceInput),
			t,
		)
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.JSONEq(
			t,
			fmt.Sprintf(
				"{\"error\":\"%[1]s\",\"message\":\"%[1]s\"}",
				models.ErrDashboardOrPanelIdentifierNotSet,
			),
			response.Body.String(),
		)
	})

	t.Run("Cannot query without a valid orgid", func(t *testing.T) {
		response := callAPI(
			sc.server,
			http.MethodPost,
			"/api/dashboards/orgid//uid/%s/panels/%s/query",
			strings.NewReader(queryDatasourceInput),
			t,
		)
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.JSONEq(
			t,
			fmt.Sprintf(
				"{\"error\":\"%[1]s\",\"message\":\"%[1]s\"}",
				models.ErrDashboardOrPanelIdentifierNotSet,
			),
			response.Body.String(),
		)
	})

	t.Run("Cannot query without a valid dashboard or panel ID", func(t *testing.T) {
		response := callAPI(
			sc.server,
			http.MethodPost,
			fmt.Sprintf("/api/dashboards/org//uid/%s/panels/%s/query", "1", "2"),
			strings.NewReader(queryDatasourceInput),
			t,
		)
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.JSONEq(
			t,
			fmt.Sprintf(
				"{\"error\":\"%[1]s\",\"message\":\"%[1]s\"}",
				models.ErrDashboardOrPanelIdentifierNotSet,
			),
			response.Body.String(),
		)
	})

	t.Run("Cannot query when ValidatedQueries is disabled", func(t *testing.T) {
		sc.hs.Features = featuremgmt.WithFeatures(featuremgmt.FlagValidatedQueries, false)

		response := callAPI(
			sc.server,
			http.MethodPost,
			"/api/dashboards/uid/1/panels/1/query",
			strings.NewReader(queryDatasourceInput),
			t,
		)

		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.Equal(
			t,
			"404 page not found\n",
			response.Body.String(),
		)
	})
}

func TestAPIEndpoint_Metrics_checkDashboardAndPanel(t *testing.T) {
	dashboardJson, err := simplejson.NewFromReader(strings.NewReader(getDashboardByIdOutput))
	if err != nil {
		t.Fatalf("Failed to unmarshal dashboard json: %v", err)
	}

	type dashboardQueryResult struct {
		result *models.Dashboard
		err    error
	}
	tests := []struct {
		name                 string
		orgId                int64
		dashboardUid         string
		panelId              int64
		dashboardQueryResult *dashboardQueryResult
		expectedError        error
	}{
		{
			name:         "Work when correct dashboardId and panelId given",
			orgId:        testOrgID,
			dashboardUid: "1",
			panelId:      2,
			dashboardQueryResult: &dashboardQueryResult{
				result: &models.Dashboard{
					Uid:   "1",
					OrgId: testOrgID,
					Data:  dashboardJson,
				},
			},
			expectedError: nil,
		},
		{
			name:                 "Cannot query without a valid panel ID",
			orgId:                testOrgID,
			dashboardUid:         "1",
			panelId:              0,
			dashboardQueryResult: nil,
			expectedError:        models.ErrDashboardOrPanelIdentifierNotSet,
		},
		{
			name:                 "Cannot query without a valid dashboard ID",
			orgId:                testOrgID,
			dashboardUid:         "",
			panelId:              2,
			dashboardQueryResult: nil,
			expectedError:        models.ErrDashboardOrPanelIdentifierNotSet,
		},
		{
			name:         "Fails when the dashboard does not exist",
			orgId:        testOrgID,
			dashboardUid: "1",
			panelId:      2,
			dashboardQueryResult: &dashboardQueryResult{
				result: nil,
				err:    models.ErrDashboardNotFound,
			},
			expectedError: models.ErrDashboardNotFound,
		},
		{
			name:         "Fails when the dashboard does not exist",
			orgId:        testOrgID,
			dashboardUid: "1",
			panelId:      3,
			dashboardQueryResult: &dashboardQueryResult{
				result: &models.Dashboard{
					Id:    1,
					OrgId: testOrgID,
					Data:  dashboardJson,
				},
			},
			expectedError: models.ErrDashboardPanelNotFound,
		},
		{
			name:         "Fails when the dashboard contents are nil",
			orgId:        testOrgID,
			dashboardUid: "1",
			panelId:      3,
			dashboardQueryResult: &dashboardQueryResult{
				result: &models.Dashboard{
					Uid:   "1",
					OrgId: testOrgID,
					Data:  nil,
				},
			},
			expectedError: models.ErrDashboardCorrupt,
		},
	}

	//sqlStore := sqlstore.InitTestDB(t)
	ss := mockstore.NewSQLStoreMock()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			if test.dashboardQueryResult != nil {
				ss.ExpectedDashboard = test.dashboardQueryResult.result
				ss.ExpectedError = test.dashboardQueryResult.err
			}

			query := models.GetDashboardQuery{
				OrgId: test.orgId,
				Uid:   test.dashboardUid,
			}

			assert.Equal(t, test.expectedError, checkDashboardAndPanel(context.Background(), ss, query, test.panelId))
		})
	}
}