package gitlab

import (
	"net/http"
	"time"
)

var (
	routeProjectsIDFeatureFlags   = route("projects/%s/feature_flags")
	routeProjectsIDFeatureFlagsID = route("projects/%s/feature_flags/%s")
)

type (
	ProjectFeatureFlagServiceInterface interface {
		// ListProjectFeatureFlags returns a list with the feature flags of a project.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/feature_flags/#list-feature-flags-for-a-project
		ListProjectFeatureFlags(pid any, opt *ListProjectFeatureFlagOptions, options ...RequestOptionFunc) ([]*ProjectFeatureFlag, *Response, error)
		// GetProjectFeatureFlag gets a single feature flag for the specified project.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/feature_flags/#get-a-single-feature-flag
		GetProjectFeatureFlag(pid any, name string, options ...RequestOptionFunc) (*ProjectFeatureFlag, *Response, error)
		// CreateProjectFeatureFlag creates a feature flag.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/feature_flags/#create-a-feature-flag
		CreateProjectFeatureFlag(pid any, opt *CreateProjectFeatureFlagOptions, options ...RequestOptionFunc) (*ProjectFeatureFlag, *Response, error)
		// UpdateProjectFeatureFlag updates a feature flag.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/feature_flags/#update-a-feature-flag
		UpdateProjectFeatureFlag(pid any, name string, opt *UpdateProjectFeatureFlagOptions, options ...RequestOptionFunc) (*ProjectFeatureFlag, *Response, error)
		// DeleteProjectFeatureFlag deletes a feature flag.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/feature_flags/#delete-a-feature-flag
		DeleteProjectFeatureFlag(pid any, name string, options ...RequestOptionFunc) (*Response, error)
	}

	// ProjectFeatureFlagService handles operations on gitlab project feature
	// flags using the following api:
	//
	// GitLab API docs: https://docs.gitlab.com/api/feature_flags/
	ProjectFeatureFlagService struct {
		client *Client
	}
)

var _ ProjectFeatureFlagServiceInterface = (*ProjectFeatureFlagService)(nil)

// ProjectFeatureFlag represents a GitLab project iteration.
//
// GitLab API docs: https://docs.gitlab.com/api/feature_flags/
type ProjectFeatureFlag struct {
	Name        string                        `json:"name"`
	Description string                        `json:"description"`
	Active      bool                          `json:"active"`
	Version     string                        `json:"version"`
	CreatedAt   *time.Time                    `json:"created_at"`
	UpdatedAt   *time.Time                    `json:"updated_at"`
	Scopes      []*ProjectFeatureFlagScope    `json:"scopes"`
	Strategies  []*ProjectFeatureFlagStrategy `json:"strategies"`
}

// ProjectFeatureFlagScope defines the scopes of a feature flag
//
// GitLab API docs: https://docs.gitlab.com/api/feature_flags/
type ProjectFeatureFlagScope struct {
	ID               int64  `json:"id,omitempty"`
	EnvironmentScope string `json:"environment_scope"`
}

// ProjectFeatureFlagStrategy defines the strategy used for a feature flag
//
// GitLab API docs: https://docs.gitlab.com/api/feature_flags/
type ProjectFeatureFlagStrategy struct {
	ID         int64                                `json:"id"`
	Name       string                               `json:"name"`
	Parameters *ProjectFeatureFlagStrategyParameter `json:"parameters"`
	Scopes     []*ProjectFeatureFlagScope           `json:"scopes"`
	UserList   *FeatureFlagUserList                 `json:"user_list,omitempty"`
}

// ProjectFeatureFlagStrategyParameter is used in updating and creating feature flags
//
// GitLab API docs: https://docs.gitlab.com/api/feature_flags/
type ProjectFeatureFlagStrategyParameter struct {
	GroupID    string `json:"groupId,omitempty"`
	UserIDs    string `json:"userIds,omitempty"`
	Percentage string `json:"percentage,omitempty"`

	// Following fields aren't documented in GitLab API docs,
	// but are present in GitLab API since 13.5.
	// Docs: https://docs.getunleash.io/reference/activation-strategies#gradual-rollout
	Rollout    string `json:"rollout,omitempty"`
	Stickiness string `json:"stickiness,omitempty"`
}

func (i ProjectFeatureFlag) String() string {
	return Stringify(i)
}

// ListProjectFeatureFlagOptions contains the options for ListProjectFeatureFlags
//
// GitLab API docs:
// https://docs.gitlab.com/api/feature_flags/#list-feature-flags-for-a-project
type ListProjectFeatureFlagOptions struct {
	ListOptions
	Scope *string `url:"scope,omitempty" json:"scope,omitempty"`
}

func (s *ProjectFeatureFlagService) ListProjectFeatureFlags(pid any, opt *ListProjectFeatureFlagOptions, options ...RequestOptionFunc) ([]*ProjectFeatureFlag, *Response, error) {
	return do[[]*ProjectFeatureFlag](s.client,
		withPath(routeProjectsIDFeatureFlags, ProjectID{pid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

func (s *ProjectFeatureFlagService) GetProjectFeatureFlag(pid any, name string, options ...RequestOptionFunc) (*ProjectFeatureFlag, *Response, error) {
	return do[*ProjectFeatureFlag](s.client,
		withPath(routeProjectsIDFeatureFlagsID, ProjectID{pid}, name),
		withRequestOpts(options...),
	)
}

// CreateProjectFeatureFlagOptions represents the available
// CreateProjectFeatureFlag() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/feature_flags/#create-a-feature-flag
type CreateProjectFeatureFlagOptions struct {
	Name        *string                              `url:"name,omitempty" json:"name,omitempty"`
	Description *string                              `url:"description,omitempty" json:"description,omitempty"`
	Version     *string                              `url:"version,omitempty" json:"version,omitempty"`
	Active      *bool                                `url:"active,omitempty" json:"active,omitempty"`
	Strategies  *[]*CreateFeatureFlagStrategyOptions `url:"strategies,omitempty" json:"strategies,omitempty"`
}

// CreateFeatureFlagStrategyOptions represents the available feature flag
// strategy options for CreateProjectFeatureFlag().
//
// GitLab API docs:
// https://docs.gitlab.com/api/feature_flags/#create-a-feature-flag
type CreateFeatureFlagStrategyOptions struct {
	Name       *string                                  `url:"name,omitempty" json:"name,omitempty"`
	Parameters *ProjectFeatureFlagStrategyParameter     `url:"parameters,omitempty" json:"parameters,omitempty"`
	Scopes     *[]*CreateProjectFeatureFlagScopeOptions `url:"scopes,omitempty" json:"scopes,omitempty"`

	// UserListID sets the feature flag user list bound to the strategy. Only
	// used when Name is "gitlabUserList".
	UserListID *int64 `url:"user_list_id,omitempty" json:"user_list_id,omitempty"`
}

// CreateProjectFeatureFlagScopeOptions defines the scopes of a feature flag
//
// GitLab API docs: https://docs.gitlab.com/api/feature_flags/
type CreateProjectFeatureFlagScopeOptions struct {
	EnvironmentScope *string `url:"environment_scope,omitempty" json:"environment_scope,omitempty"`
}

func (s *ProjectFeatureFlagService) CreateProjectFeatureFlag(pid any, opt *CreateProjectFeatureFlagOptions, options ...RequestOptionFunc) (*ProjectFeatureFlag, *Response, error) {
	return do[*ProjectFeatureFlag](s.client,
		withMethod(http.MethodPost),
		withPath(routeProjectsIDFeatureFlags, ProjectID{pid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// UpdateProjectFeatureFlagOptions represents the available
// UpdateProjectFeatureFlag() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/feature_flags/#update-a-feature-flag
type UpdateProjectFeatureFlagOptions struct {
	Name        *string                              `url:"name,omitempty" json:"name,omitempty"`
	Description *string                              `url:"description,omitempty" json:"description,omitempty"`
	Active      *bool                                `url:"active,omitempty" json:"active,omitempty"`
	Strategies  *[]*UpdateFeatureFlagStrategyOptions `url:"strategies,omitempty" json:"strategies,omitempty"`
}

// UpdateFeatureFlagStrategyOptions represents the available feature flag
// strategy options for UpdateProjectFeatureFlag().
//
// GitLab API docs:
// https://docs.gitlab.com/api/feature_flags/#update-a-feature-flag
type UpdateFeatureFlagStrategyOptions struct {
	ID         *int64                                   `url:"id,omitempty" json:"id,omitempty"`
	Name       *string                                  `url:"name,omitempty" json:"name,omitempty"`
	Parameters *ProjectFeatureFlagStrategyParameter     `url:"parameters,omitempty" json:"parameters,omitempty"`
	Scopes     *[]*UpdateProjectFeatureFlagScopeOptions `url:"scopes,omitempty" json:"scopes,omitempty"`

	// UserListID sets the feature flag user list bound to the strategy. Only
	// used when Name is "gitlabUserList".
	UserListID *int64 `url:"user_list_id,omitempty" json:"user_list_id,omitempty"`

	// Destroy removes the strategy identified by ID when set to true. Used
	// to delete a strategy that is no longer wanted.
	Destroy *bool `url:"_destroy,omitempty" json:"_destroy,omitempty"`
}

// UpdateProjectFeatureFlagScopeOptions defines the scopes of a feature flag
//
// GitLab API docs: https://docs.gitlab.com/api/feature_flags/
type UpdateProjectFeatureFlagScopeOptions struct {
	ID               *int64  `url:"id,omitempty" json:"id,omitempty"`
	EnvironmentScope *string `url:"environment_scope,omitempty" json:"environment_scope,omitempty"`

	// Destroy removes the scope identified by ID when set to true. Used
	// during UpdateProjectFeatureFlag to delete a scope that is no longer
	// wanted.
	Destroy *bool `url:"_destroy,omitempty" json:"_destroy,omitempty"`
}

func (s *ProjectFeatureFlagService) UpdateProjectFeatureFlag(pid any, name string, opt *UpdateProjectFeatureFlagOptions, options ...RequestOptionFunc) (*ProjectFeatureFlag, *Response, error) {
	return do[*ProjectFeatureFlag](s.client,
		withMethod(http.MethodPut),
		withPath(routeProjectsIDFeatureFlagsID, ProjectID{pid}, name),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

func (s *ProjectFeatureFlagService) DeleteProjectFeatureFlag(pid any, name string, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodDelete),
		withPath(routeProjectsIDFeatureFlagsID, ProjectID{pid}, name),
		withRequestOpts(options...),
	)
	return resp, err
}
