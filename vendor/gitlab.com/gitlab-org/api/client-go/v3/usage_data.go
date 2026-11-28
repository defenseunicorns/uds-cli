package gitlab

import (
	"io"
	"net/http"
	"time"
)

var (
	routeUsageDataMetricDefinitions = route("usage_data/metric_definitions")
	routeUsageDataNonSQLMetrics     = route("usage_data/non_sql_metrics")
	routeUsageDataQueries           = route("usage_data/queries")
	routeUsageDataServicePing       = route("usage_data/service_ping")
	routeUsageDataTrackEvent        = route("usage_data/track_event")
	routeUsageDataTrackEvents       = route("usage_data/track_events")
)

type (
	UsageDataServiceInterface interface {
		// GetServicePing gets the current service ping data.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/usage_data/#export-service-ping-data
		GetServicePing(options ...RequestOptionFunc) (*ServicePingData, *Response, error)
		// GetMetricDefinitionsAsYAML gets all metric definitions as a single YAML file.
		//
		// The returned io.ReadCloser must be closed by the caller to avoid
		// leaking the underlying response body.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/usage_data/#export-metric-definitions-as-a-single-yaml-file
		GetMetricDefinitionsAsYAML(options ...RequestOptionFunc) (io.ReadCloser, *Response, error)
		// GetQueries gets all raw SQL queries used to compute service ping.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/usage_data/#export-service-ping-sql-queries
		GetQueries(options ...RequestOptionFunc) (*ServicePingQueries, *Response, error)
		// GetNonSQLMetrics gets all non-SQL metrics data used in the service ping.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/usage_data/#usagedatanonsqlmetrics-api
		GetNonSQLMetrics(options ...RequestOptionFunc) (*ServicePingNonSQLMetrics, *Response, error)
		// TrackEvent tracks an internal GitLab event.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/usage_data/#events-tracking-api
		TrackEvent(opt *TrackEventOptions, options ...RequestOptionFunc) (*Response, error)
		// TrackEvents tracks multiple internal GitLab events.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/usage_data/#events-tracking-api
		TrackEvents(opt *TrackEventsOptions, options ...RequestOptionFunc) (*Response, error)
	}

	// UsageDataService handles communication with the service ping related
	// methods of the GitLab API.
	//
	// GitLab API docs: https://docs.gitlab.com/api/usage_data/
	UsageDataService struct {
		client *Client
	}
)

// ServicePingData represents a service ping data response.
type ServicePingData struct {
	RecordedAt *time.Time        `json:"recorded_at"`
	License    map[string]string `json:"license"`
	Counts     map[string]int64  `json:"counts"`
}

func (s *UsageDataService) GetServicePing(options ...RequestOptionFunc) (*ServicePingData, *Response, error) {
	return do[*ServicePingData](s.client,
		withPath(routeUsageDataServicePing),
		withRequestOpts(options...),
	)
}

// GetMetricDefinitionsAsYAML gets all metric definitions as a single YAML file.
//
// The returned io.ReadCloser must be closed by the caller to avoid
// leaking the underlying response body.
//
// GitLab API docs:
// https://docs.gitlab.com/api/usage_data/#export-metric-definitions-as-a-single-yaml-file
func (s *UsageDataService) GetMetricDefinitionsAsYAML(options ...RequestOptionFunc) (io.ReadCloser, *Response, error) {
	r, resp, err := do[bodyReader](s.client,
		withPath(routeUsageDataMetricDefinitions),
		withRequestOpts(append([]RequestOptionFunc{WithHeader("Accept", "text/yaml")}, options...)...),
	)
	if err != nil {
		return nil, resp, err
	}
	return &r, resp, nil
}

// ServicePingQueries represents the raw service ping SQL queries.
type ServicePingQueries struct {
	RecordedAt            *time.Time        `json:"recorded_at"`
	UUID                  string            `json:"uuid"`
	Hostname              string            `json:"hostname"`
	Version               string            `json:"version"`
	InstallationType      string            `json:"installation_type"`
	ActiveUserCount       string            `json:"active_user_count"`
	Edition               string            `json:"edition"`
	LicenseMD5            string            `json:"license_md5"`
	LicenseSHA256         string            `json:"license_sha256"`
	LicenseID             string            `json:"license_id"`
	HistoricalMaxUsers    int64             `json:"historical_max_users"`
	Licensee              map[string]string `json:"licensee"`
	LicenseUserCount      int64             `json:"license_user_count"`
	LicenseStartsAt       string            `json:"license_starts_at"`
	LicenseExpiresAt      string            `json:"license_expires_at"`
	LicensePlan           string            `json:"license_plan"`
	LicenseAddOns         map[string]int64  `json:"license_add_ons"`
	LicenseTrial          string            `json:"license_trial"`
	LicenseSubscriptionID string            `json:"license_subscription_id"`
	License               map[string]string `json:"license"`
	Settings              map[string]string `json:"settings"`
	Counts                map[string]string `json:"counts"`
}

func (s *UsageDataService) GetQueries(options ...RequestOptionFunc) (*ServicePingQueries, *Response, error) {
	return do[*ServicePingQueries](s.client,
		withPath(routeUsageDataQueries),
		withRequestOpts(options...),
	)
}

// ServicePingNonSQLMetrics represents the non-SQL metrics used in service ping.
type ServicePingNonSQLMetrics struct {
	RecordedAt            string            `json:"recorded_at"`
	UUID                  string            `json:"uuid"`
	Hostname              string            `json:"hostname"`
	Version               string            `json:"version"`
	InstallationType      string            `json:"installation_type"`
	ActiveUserCount       int64             `json:"active_user_count"`
	Edition               string            `json:"edition"`
	LicenseMD5            string            `json:"license_md5"`
	LicenseSHA256         string            `json:"license_sha256"`
	LicenseID             string            `json:"license_id"`
	HistoricalMaxUsers    int64             `json:"historical_max_users"`
	Licensee              map[string]string `json:"licensee"`
	LicenseUserCount      int64             `json:"license_user_count"`
	LicenseStartsAt       string            `json:"license_starts_at"`
	LicenseExpiresAt      string            `json:"license_expires_at"`
	LicensePlan           string            `json:"license_plan"`
	LicenseAddOns         map[string]int64  `json:"license_add_ons"`
	LicenseTrial          string            `json:"license_trial"`
	LicenseSubscriptionID string            `json:"license_subscription_id"`
	License               map[string]string `json:"license"`
	Settings              map[string]string `json:"settings"`
}

func (s *UsageDataService) GetNonSQLMetrics(options ...RequestOptionFunc) (*ServicePingNonSQLMetrics, *Response, error) {
	return do[*ServicePingNonSQLMetrics](s.client,
		withPath(routeUsageDataNonSQLMetrics),
		withRequestOpts(options...),
	)
}

// TrackEventOptions represents the available options for tracking events.
type TrackEventOptions struct {
	Event                string            `json:"event" url:"event"`
	SendToSnowplow       *bool             `json:"send_to_snowplow,omitempty" url:"send_to_snowplow,omitempty"`
	NamespaceID          *int64            `json:"namespace_id,omitempty" url:"namespace_id,omitempty"`
	ProjectID            *int64            `json:"project_id,omitempty" url:"project_id,omitempty"`
	AdditionalProperties map[string]string `json:"additional_properties,omitempty" url:"additional_properties,omitempty"`
}

func (s *UsageDataService) TrackEvent(opt *TrackEventOptions, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodPost),
		withPath(routeUsageDataTrackEvent),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
	return resp, err
}

// TrackEventsOptions represents the available options for tracking multiple events.
type TrackEventsOptions struct {
	Events []TrackEventOptions `json:"events" url:"events"`
}

func (s *UsageDataService) TrackEvents(opt *TrackEventsOptions, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodPost),
		withPath(routeUsageDataTrackEvents),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
	return resp, err
}
