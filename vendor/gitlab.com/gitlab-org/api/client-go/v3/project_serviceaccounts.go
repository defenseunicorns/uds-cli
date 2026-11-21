//
// Copyright 2026, Jimmy Spagnola
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package gitlab

import (
	"net/http"
	"time"
)

var (
	routeProjectsIDServiceAccounts                               = route("projects/%s/service_accounts")
	routeProjectsIDServiceAccountsID                             = route("projects/%s/service_accounts/%d")
	routeProjectsIDServiceAccountsIDPersonalAccessTokens         = route("projects/%s/service_accounts/%d/personal_access_tokens")
	routeProjectsIDServiceAccountsIDPersonalAccessTokensID       = route("projects/%s/service_accounts/%d/personal_access_tokens/%d")
	routeProjectsIDServiceAccountsIDPersonalAccessTokensIDRotate = route("projects/%s/service_accounts/%d/personal_access_tokens/%d/rotate")
)

// ProjectServiceAccount represents a GitLab project service account user.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#create-a-project-service-account
type ProjectServiceAccount struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Username         string `json:"username"`
	Email            string `json:"email"`
	UnconfirmedEmail string `json:"unconfirmed_email,omitempty"`
}

// ListProjectServiceAccountsOptions represents the available
// ListProjectServiceAccounts() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#list-all-project-service-accounts
type ListProjectServiceAccountsOptions struct {
	ListOptions
	OrderBy *string `url:"order_by,omitempty" json:"order_by,omitempty"`
	Sort    *string `url:"sort,omitempty" json:"sort,omitempty"`
}

// CreateProjectServiceAccountOptions represents the available
// CreateProjectServiceAccount() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#create-a-project-service-account
type CreateProjectServiceAccountOptions struct {
	Name     *string `url:"name,omitempty" json:"name,omitempty"`
	Username *string `url:"username,omitempty" json:"username,omitempty"`
	Email    *string `url:"email,omitempty" json:"email,omitempty"`
}

// UpdateProjectServiceAccountOptions represents the available
// UpdateProjectServiceAccount() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#update-a-project-service-account
type UpdateProjectServiceAccountOptions struct {
	Name     *string `url:"name,omitempty" json:"name,omitempty"`
	Username *string `url:"username,omitempty" json:"username,omitempty"`
	Email    *string `url:"email,omitempty" json:"email,omitempty"`
}

// DeleteProjectServiceAccountOptions represents the available
// DeleteProjectServiceAccount() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#delete-a-project-service-account
type DeleteProjectServiceAccountOptions struct {
	HardDelete *bool `url:"hard_delete,omitempty" json:"hard_delete,omitempty"`
}

// ListProjectServiceAccountPersonalAccessTokensOptions represents the available
// ListProjectServiceAccountPersonalAccessTokens() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#list-all-personal-access-tokens-for-a-project-service-account
type ListProjectServiceAccountPersonalAccessTokensOptions struct {
	ListOptions
	CreatedAfter   *time.Time `url:"created_after,omitempty" json:"created_after,omitempty"`
	CreatedBefore  *time.Time `url:"created_before,omitempty" json:"created_before,omitempty"`
	ExpiresAfter   *ISOTime   `url:"expires_after,omitempty" json:"expires_after,omitempty"`
	ExpiresBefore  *ISOTime   `url:"expires_before,omitempty" json:"expires_before,omitempty"`
	LastUsedAfter  *time.Time `url:"last_used_after,omitempty" json:"last_used_after,omitempty"`
	LastUsedBefore *time.Time `url:"last_used_before,omitempty" json:"last_used_before,omitempty"`
	Revoked        *bool      `url:"revoked,omitempty" json:"revoked,omitempty"`
	UserID         *int64     `url:"user_id,omitempty" json:"user_id,omitempty"`
	Search         *string    `url:"search,omitempty" json:"search,omitempty"`
	Sort           *string    `url:"sort,omitempty" json:"sort,omitempty"`
	State          *string    `url:"state,omitempty" json:"state,omitempty"`
}

// CreateProjectServiceAccountPersonalAccessTokenOptions represents the available
// CreateProjectServiceAccountPersonalAccessToken() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#create-a-personal-access-token-for-a-project-service-account
type CreateProjectServiceAccountPersonalAccessTokenOptions struct {
	Name        *string   `url:"name,omitempty" json:"name,omitempty"`
	Description *string   `url:"description,omitempty" json:"description,omitempty"`
	Scopes      *[]string `url:"scopes,omitempty" json:"scopes,omitempty"`
	ExpiresAt   *ISOTime  `url:"expires_at,omitempty" json:"expires_at,omitempty"`
}

// RotateProjectServiceAccountPersonalAccessTokenOptions represents the available
// RotateProjectServiceAccountPersonalAccessToken() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#rotate-a-personal-access-token-for-a-project-service-account
type RotateProjectServiceAccountPersonalAccessTokenOptions struct {
	ExpiresAt *ISOTime `url:"expires_at,omitempty" json:"expires_at,omitempty"`
}

// ListProjectServiceAccounts gets a list of project service accounts.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#list-all-project-service-accounts
func (s *ProjectsService) ListProjectServiceAccounts(pid any, opt *ListProjectServiceAccountsOptions, options ...RequestOptionFunc) ([]*ProjectServiceAccount, *Response, error) {
	return do[[]*ProjectServiceAccount](
		s.client,
		withPath(routeProjectsIDServiceAccounts, ProjectID{pid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// GetProjectServiceAccount retrieves a single service account
// in a specified project.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#retrieve-a-project-service-account
func (s *ProjectsService) GetProjectServiceAccount(pid any, serviceAccount int64, options ...RequestOptionFunc) (*ProjectServiceAccount, *Response, error) {
	return do[*ProjectServiceAccount](s.client,
		withPath(routeProjectsIDServiceAccountsID, ProjectID{pid}, serviceAccount),
		withRequestOpts(options...),
	)
}

// CreateProjectServiceAccount creates a project service account user.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#create-a-project-service-account
func (s *ProjectsService) CreateProjectServiceAccount(pid any, opt *CreateProjectServiceAccountOptions, options ...RequestOptionFunc) (*ProjectServiceAccount, *Response, error) {
	return do[*ProjectServiceAccount](
		s.client,
		withMethod(http.MethodPost),
		withPath(routeProjectsIDServiceAccounts, ProjectID{pid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// UpdateProjectServiceAccount updates a project service account user.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#update-a-project-service-account
func (s *ProjectsService) UpdateProjectServiceAccount(pid any, serviceAccount int64, opt *UpdateProjectServiceAccountOptions, options ...RequestOptionFunc) (*ProjectServiceAccount, *Response, error) {
	return do[*ProjectServiceAccount](
		s.client,
		withMethod(http.MethodPatch),
		withPath(routeProjectsIDServiceAccountsID, ProjectID{pid}, serviceAccount),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// DeleteProjectServiceAccount deletes a project service account user.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#delete-a-project-service-account
func (s *ProjectsService) DeleteProjectServiceAccount(pid any, serviceAccount int64, opt *DeleteProjectServiceAccountOptions, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](
		s.client,
		withMethod(http.MethodDelete),
		withPath(routeProjectsIDServiceAccountsID, ProjectID{pid}, serviceAccount),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
	return resp, err
}

// ListProjectServiceAccountPersonalAccessTokens gets a list of personal access
// tokens for a project service account user.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#list-all-personal-access-tokens-for-a-project-service-account
func (s *ProjectsService) ListProjectServiceAccountPersonalAccessTokens(pid any, serviceAccount int64, opt *ListProjectServiceAccountPersonalAccessTokensOptions, options ...RequestOptionFunc) ([]*PersonalAccessToken, *Response, error) {
	return do[[]*PersonalAccessToken](
		s.client,
		withPath(routeProjectsIDServiceAccountsIDPersonalAccessTokens, ProjectID{pid}, serviceAccount),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// CreateProjectServiceAccountPersonalAccessToken adds a new personal access
// token for a project service account user.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#create-a-personal-access-token-for-a-project-service-account
func (s *ProjectsService) CreateProjectServiceAccountPersonalAccessToken(pid any, serviceAccount int64, opt *CreateProjectServiceAccountPersonalAccessTokenOptions, options ...RequestOptionFunc) (*PersonalAccessToken, *Response, error) {
	return do[*PersonalAccessToken](
		s.client,
		withMethod(http.MethodPost),
		withPath(routeProjectsIDServiceAccountsIDPersonalAccessTokens, ProjectID{pid}, serviceAccount),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// RevokeProjectServiceAccountPersonalAccessToken revokes a personal access
// token for an existing project service account user.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#revoke-a-personal-access-token-for-a-project-service-account
func (s *ProjectsService) RevokeProjectServiceAccountPersonalAccessToken(pid any, serviceAccount, token int64, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](
		s.client,
		withMethod(http.MethodDelete),
		withPath(routeProjectsIDServiceAccountsIDPersonalAccessTokensID, ProjectID{pid}, serviceAccount, token),
		withRequestOpts(options...),
	)
	return resp, err
}

// RotateProjectServiceAccountPersonalAccessToken rotates a personal access
// token for a project service account user.
//
// GitLab API docs:
// https://docs.gitlab.com/api/service_accounts/#rotate-a-personal-access-token-for-a-project-service-account
func (s *ProjectsService) RotateProjectServiceAccountPersonalAccessToken(pid any, serviceAccount, token int64, opt *RotateProjectServiceAccountPersonalAccessTokenOptions, options ...RequestOptionFunc) (*PersonalAccessToken, *Response, error) {
	return do[*PersonalAccessToken](
		s.client,
		withMethod(http.MethodPost),
		withPath(routeProjectsIDServiceAccountsIDPersonalAccessTokensIDRotate, ProjectID{pid}, serviceAccount, token),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}
