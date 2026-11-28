//
// Copyright 2021, Eric Stevens
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
	routeProjectsIDHooks                  = route("projects/%s/hooks")
	routeProjectsIDHooksID                = route("projects/%s/hooks/%d")
	routeProjectsIDHooksIDCustomHeadersID = route("projects/%s/hooks/%d/custom_headers/%s")
	routeProjectsIDHooksIDTestID          = route("projects/%s/hooks/%d/test/%s")
	routeProjectsIDHooksIDUrlVariablesID  = route("projects/%s/hooks/%d/url_variables/%s")
)

// HookCustomHeader represents a project or group hook custom header
// Note: "Key" is returned from the Get operation, but "Value" is not
// The List operation doesn't return any headers at all for Projects,
// but does return headers for Groups
type HookCustomHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// HookURLVariable represents a project or group hook URL variable
type HookURLVariable struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ProjectHook represents a project hook.
//
// GitLab API docs:
// https://docs.gitlab.com/api/project_webhooks/#list-webhooks-for-a-project
type ProjectHook struct {
	ID                        int64               `json:"id"`
	URL                       string              `json:"url"`
	Name                      string              `json:"name"`
	Description               string              `json:"description"`
	ProjectID                 int64               `json:"project_id"`
	PushEvents                bool                `json:"push_events"`
	PushEventsBranchFilter    string              `json:"push_events_branch_filter"`
	IssuesEvents              bool                `json:"issues_events"`
	ConfidentialIssuesEvents  bool                `json:"confidential_issues_events"`
	MergeRequestsEvents       bool                `json:"merge_requests_events"`
	TagPushEvents             bool                `json:"tag_push_events"`
	NoteEvents                bool                `json:"note_events"`
	ConfidentialNoteEvents    bool                `json:"confidential_note_events"`
	JobEvents                 bool                `json:"job_events"`
	PipelineEvents            bool                `json:"pipeline_events"`
	WikiPageEvents            bool                `json:"wiki_page_events"`
	DeploymentEvents          bool                `json:"deployment_events"`
	ReleasesEvents            bool                `json:"releases_events"`
	MilestoneEvents           bool                `json:"milestone_events"`
	FeatureFlagEvents         bool                `json:"feature_flag_events"`
	EmojiEvents               bool                `json:"emoji_events"`
	EnableSSLVerification     bool                `json:"enable_ssl_verification"`
	RepositoryUpdateEvents    bool                `json:"repository_update_events"`
	AlertStatus               string              `json:"alert_status"`
	DisabledUntil             *time.Time          `json:"disabled_until"`
	URLVariables              []HookURLVariable   `json:"url_variables"`
	CreatedAt                 *time.Time          `json:"created_at"`
	ResourceAccessTokenEvents bool                `json:"resource_access_token_events"`
	ResourceDeployTokenEvents bool                `json:"resource_deploy_token_events"`
	CustomWebhookTemplate     string              `json:"custom_webhook_template"`
	CustomHeaders             []*HookCustomHeader `json:"custom_headers"`
	VulnerabilityEvents       bool                `json:"vulnerability_events"`
	BranchFilterStrategy      string              `json:"branch_filter_strategy"`
	TokenPresent              bool                `json:"token_present"`
	SigningTokenPresent       bool                `json:"signing_token_present"`
}

// ListProjectHooksOptions represents the available ListProjectHooks() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/project_webhooks/#list-webhooks-for-a-project
type ListProjectHooksOptions struct {
	ListOptions
}

// ListProjectHooks gets a list of project hooks.
//
// GitLab API docs:
// https://docs.gitlab.com/api/project_webhooks/#list-webhooks-for-a-project
func (s *ProjectsService) ListProjectHooks(pid any, opt *ListProjectHooksOptions, options ...RequestOptionFunc) ([]*ProjectHook, *Response, error) {
	return do[[]*ProjectHook](
		s.client,
		withPath(routeProjectsIDHooks, ProjectID{pid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// GetProjectHook gets a specific hook for a project.
//
// GitLab API docs:
// https://docs.gitlab.com/api/project_webhooks/#get-a-project-webhook
func (s *ProjectsService) GetProjectHook(pid any, hook int64, options ...RequestOptionFunc) (*ProjectHook, *Response, error) {
	return do[*ProjectHook](
		s.client,
		withPath(routeProjectsIDHooksID, ProjectID{pid}, hook),
		withRequestOpts(options...),
	)
}

// AddProjectHookOptions represents the available AddProjectHook() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/project_webhooks/#add-a-webhook-to-a-project
type AddProjectHookOptions struct {
	Name                      *string `url:"name,omitempty" json:"name,omitempty"`
	Description               *string `url:"description,omitempty" json:"description,omitempty"`
	URL                       *string `url:"url,omitempty" json:"url,omitempty"`
	PushEvents                *bool   `url:"push_events,omitempty" json:"push_events,omitempty"`
	PushEventsBranchFilter    *string `url:"push_events_branch_filter,omitempty" json:"push_events_branch_filter,omitempty"`
	BranchFilterStrategy      *string `url:"branch_filter_strategy,omitempty" json:"branch_filter_strategy,omitempty"`
	IssuesEvents              *bool   `url:"issues_events,omitempty" json:"issues_events,omitempty"`
	ConfidentialIssuesEvents  *bool   `url:"confidential_issues_events,omitempty" json:"confidential_issues_events,omitempty"`
	MergeRequestsEvents       *bool   `url:"merge_requests_events,omitempty" json:"merge_requests_events,omitempty"`
	TagPushEvents             *bool   `url:"tag_push_events,omitempty" json:"tag_push_events,omitempty"`
	NoteEvents                *bool   `url:"note_events,omitempty" json:"note_events,omitempty"`
	ConfidentialNoteEvents    *bool   `url:"confidential_note_events,omitempty" json:"confidential_note_events,omitempty"`
	JobEvents                 *bool   `url:"job_events,omitempty" json:"job_events,omitempty"`
	PipelineEvents            *bool   `url:"pipeline_events,omitempty" json:"pipeline_events,omitempty"`
	WikiPageEvents            *bool   `url:"wiki_page_events,omitempty" json:"wiki_page_events,omitempty"`
	DeploymentEvents          *bool   `url:"deployment_events,omitempty" json:"deployment_events,omitempty"`
	FeatureFlagEvents         *bool   `url:"feature_flag_events,omitempty" json:"feature_flag_events,omitempty"`
	ReleasesEvents            *bool   `url:"releases_events,omitempty" json:"releases_events,omitempty"`
	MilestoneEvents           *bool   `url:"milestone_events,omitempty" json:"milestone_events,omitempty"`
	EmojiEvents               *bool   `url:"emoji_events,omitempty" json:"emoji_events,omitempty"`
	VulnerabilityEvents       *bool   `url:"vulnerability_events,omitempty" json:"vulnerability_events,omitempty"`
	ResourceAccessTokenEvents *bool   `url:"resource_access_token_events,omitempty" json:"resource_access_token_events,omitempty"`
	ResourceDeployTokenEvents *bool   `url:"resource_deploy_token_events,omitempty" json:"resource_deploy_token_events,omitempty"`
	EnableSSLVerification     *bool   `url:"enable_ssl_verification,omitempty" json:"enable_ssl_verification,omitempty"`
	Token                     *string `url:"token,omitempty" json:"token,omitempty"`
	// SigningToken is write-only and controlled by a feature flag currently. See https://docs.gitlab.com/api/project_webhooks/#add-a-webhook-to-a-project
	SigningToken          *string              `url:"signing_token,omitempty" json:"signing_token,omitempty"`
	CustomWebhookTemplate *string              `url:"custom_webhook_template,omitempty" json:"custom_webhook_template,omitempty"`
	CustomHeaders         *[]*HookCustomHeader `url:"custom_headers,omitempty" json:"custom_headers,omitempty"`
}

// AddProjectHook adds a hook to a specified project.
//
// GitLab API docs:
// https://docs.gitlab.com/api/project_webhooks/#add-a-webhook-to-a-project
func (s *ProjectsService) AddProjectHook(pid any, opt *AddProjectHookOptions, options ...RequestOptionFunc) (*ProjectHook, *Response, error) {
	return do[*ProjectHook](
		s.client,
		withMethod(http.MethodPost),
		withPath(routeProjectsIDHooks, ProjectID{pid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// EditProjectHookOptions represents the available EditProjectHook() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/project_webhooks/#edit-a-project-webhook
type EditProjectHookOptions struct {
	Name                      *string `url:"name,omitempty" json:"name,omitempty"`
	Description               *string `url:"description,omitempty" json:"description,omitempty"`
	URL                       *string `url:"url,omitempty" json:"url,omitempty"`
	PushEvents                *bool   `url:"push_events,omitempty" json:"push_events,omitempty"`
	PushEventsBranchFilter    *string `url:"push_events_branch_filter,omitempty" json:"push_events_branch_filter,omitempty"`
	BranchFilterStrategy      *string `url:"branch_filter_strategy,omitempty" json:"branch_filter_strategy,omitempty"`
	IssuesEvents              *bool   `url:"issues_events,omitempty" json:"issues_events,omitempty"`
	ConfidentialIssuesEvents  *bool   `url:"confidential_issues_events,omitempty" json:"confidential_issues_events,omitempty"`
	MergeRequestsEvents       *bool   `url:"merge_requests_events,omitempty" json:"merge_requests_events,omitempty"`
	TagPushEvents             *bool   `url:"tag_push_events,omitempty" json:"tag_push_events,omitempty"`
	NoteEvents                *bool   `url:"note_events,omitempty" json:"note_events,omitempty"`
	ConfidentialNoteEvents    *bool   `url:"confidential_note_events,omitempty" json:"confidential_note_events,omitempty"`
	JobEvents                 *bool   `url:"job_events,omitempty" json:"job_events,omitempty"`
	PipelineEvents            *bool   `url:"pipeline_events,omitempty" json:"pipeline_events,omitempty"`
	WikiPageEvents            *bool   `url:"wiki_page_events,omitempty" json:"wiki_page_events,omitempty"`
	DeploymentEvents          *bool   `url:"deployment_events,omitempty" json:"deployment_events,omitempty"`
	FeatureFlagEvents         *bool   `url:"feature_flag_events,omitempty" json:"feature_flag_events,omitempty"`
	ReleasesEvents            *bool   `url:"releases_events,omitempty" json:"releases_events,omitempty"`
	MilestoneEvents           *bool   `url:"milestone_events,omitempty" json:"milestone_events,omitempty"`
	EmojiEvents               *bool   `url:"emoji_events,omitempty" json:"emoji_events,omitempty"`
	VulnerabilityEvents       *bool   `url:"vulnerability_events,omitempty" json:"vulnerability_events,omitempty"`
	ResourceAccessTokenEvents *bool   `url:"resource_access_token_events,omitempty" json:"resource_access_token_events,omitempty"`
	ResourceDeployTokenEvents *bool   `url:"resource_deploy_token_events,omitempty" json:"resource_deploy_token_events,omitempty"`
	EnableSSLVerification     *bool   `url:"enable_ssl_verification,omitempty" json:"enable_ssl_verification,omitempty"`
	Token                     *string `url:"token,omitempty" json:"token,omitempty"`
	// SigningToken is write-only and controlled by a feature flag currently. See https://docs.gitlab.com/api/project_webhooks/#update-a-project-webhook
	SigningToken          *string              `url:"signing_token,omitempty" json:"signing_token,omitempty"`
	CustomWebhookTemplate *string              `url:"custom_webhook_template,omitempty" json:"custom_webhook_template,omitempty"`
	CustomHeaders         *[]*HookCustomHeader `url:"custom_headers,omitempty" json:"custom_headers,omitempty"`
}

// EditProjectHook edits a hook for a specified project.
//
// GitLab API docs:
// https://docs.gitlab.com/api/project_webhooks/#edit-a-project-webhook
func (s *ProjectsService) EditProjectHook(pid any, hook int64, opt *EditProjectHookOptions, options ...RequestOptionFunc) (*ProjectHook, *Response, error) {
	return do[*ProjectHook](
		s.client,
		withMethod(http.MethodPut),
		withPath(routeProjectsIDHooksID, ProjectID{pid}, hook),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// DeleteProjectHook removes a hook from a project. This is an idempotent
// method and can be called multiple times. Either the hook is available or not.
//
// GitLab API docs:
// https://docs.gitlab.com/api/project_webhooks/#delete-project-webhook
func (s *ProjectsService) DeleteProjectHook(pid any, hook int64, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](
		s.client,
		withMethod(http.MethodDelete),
		withPath(routeProjectsIDHooksID, ProjectID{pid}, hook),
		withRequestOpts(options...),
	)
	return resp, err
}

// TriggerTestProjectHook Trigger a test hook for a specified project.
//
// In GitLab 17.0 and later, this endpoint has a special rate limit.
// In GitLab 17.0 the rate was three requests per minute for each project hook.
// In GitLab 17.1 this was changed to five requests per minute for each project
// and authenticated user.
//
// To disable this limit on self-managed GitLab and GitLab Dedicated,
// an administrator can disable the feature flag named web_hook_test_api_endpoint_rate_limit.
//
// GitLab API docs:
// https://docs.gitlab.com/api/project_webhooks/#trigger-a-test-project-webhook
func (s *ProjectsService) TriggerTestProjectHook(pid any, hook int64, event ProjectHookEvent, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](
		s.client,
		withMethod(http.MethodPost),
		withPath(routeProjectsIDHooksIDTestID, ProjectID{pid}, hook, string(event)),
		withRequestOpts(options...),
	)
	return resp, err
}

// SetHookCustomHeaderOptions represents the available SetProjectCustomHeader()
// options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/project_webhooks/#set-a-custom-header
type SetHookCustomHeaderOptions struct {
	Value *string `json:"value,omitempty"`
}

// SetProjectCustomHeader creates or updates a project custom webhook header.
//
// GitLab API docs:
// https://docs.gitlab.com/api/project_webhooks/#set-a-custom-header
func (s *ProjectsService) SetProjectCustomHeader(pid any, hook int64, key string, opt *SetHookCustomHeaderOptions, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](
		s.client,
		withMethod(http.MethodPut),
		withPath(routeProjectsIDHooksIDCustomHeadersID, ProjectID{pid}, hook, key),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
	return resp, err
}

// DeleteProjectCustomHeader deletes a project custom webhook header.
//
// GitLab API docs:
// https://docs.gitlab.com/api/project_webhooks/#delete-a-custom-header
func (s *ProjectsService) DeleteProjectCustomHeader(pid any, hook int64, key string, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](
		s.client,
		withMethod(http.MethodDelete),
		withPath(routeProjectsIDHooksIDCustomHeadersID, ProjectID{pid}, hook, key),
		withRequestOpts(options...),
	)
	return resp, err
}

// SetProjectWebhookURLVariableOptions represents the available
// SetProjectWebhookURLVariable() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/project_webhooks/#set-a-url-variable
type SetProjectWebhookURLVariableOptions struct {
	Value *string `json:"value,omitempty"`
}

// SetProjectWebhookURLVariable creates or updates a project webhook URL variable.
//
// GitLab API docs:
// https://docs.gitlab.com/api/project_webhooks/#set-a-url-variable
func (s *ProjectsService) SetProjectWebhookURLVariable(pid any, hook int64, key string, opt *SetProjectWebhookURLVariableOptions, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](
		s.client,
		withMethod(http.MethodPut),
		withPath(routeProjectsIDHooksIDUrlVariablesID, ProjectID{pid}, hook, key),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
	return resp, err
}

// DeleteProjectWebhookURLVariable deletes a project webhook URL variable.
//
// GitLab API docs:
// https://docs.gitlab.com/api/project_webhooks/#delete-a-url-variable
func (s *ProjectsService) DeleteProjectWebhookURLVariable(pid any, hook int64, key string, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](
		s.client,
		withMethod(http.MethodDelete),
		withPath(routeProjectsIDHooksIDUrlVariablesID, ProjectID{pid}, hook, key),
		withRequestOpts(options...),
	)
	return resp, err
}
