package gitlab

import (
	"net/http"
)

var (
	routeProjectsIDRegistryProtectionTagRules   = route("projects/%s/registry/protection/tag/rules")
	routeProjectsIDRegistryProtectionTagRulesID = route("projects/%s/registry/protection/tag/rules/%d")
)

type (
	// ContainerRegistryTagProtectionRulesServiceInterface defines methods for tag protection rules.
	// The implementation uses REST calls to the GitLab API.
	//
	// GitLab API docs: https://docs.gitlab.com/api/container_registry_protection_tag_rules/
	ContainerRegistryTagProtectionRulesServiceInterface interface {
		// ListContainerRegistryTagProtectionRules gets a list of container registry protection tag
		// rules from a project.
		//
		// GitLab API docs: https://docs.gitlab.com/api/container_registry_protection_tag_rules/#list-container-registry-protection-tag-rules
		ListContainerRegistryTagProtectionRules(pid any, options ...RequestOptionFunc) ([]*ContainerRegistryTagProtectionRule, *Response, error)

		// CreateContainerRegistryTagProtectionRule creates a container registry tag protection
		// rule for a project.
		//
		// GitLab API docs: https://docs.gitlab.com/api/container_registry_protection_tag_rules/#create-a-container-registry-protection-tag-rule
		CreateContainerRegistryTagProtectionRule(pid any, opt *CreateContainerRegistryTagProtectionRuleOptions, options ...RequestOptionFunc) (*ContainerRegistryTagProtectionRule, *Response, error)

		// UpdateContainerRegistryTagProtectionRule updates a container registry tag protection
		// rule for a project.
		//
		// GitLab API docs: https://docs.gitlab.com/api/container_registry_protection_tag_rules/#update-a-container-registry-protection-tag-rule
		UpdateContainerRegistryTagProtectionRule(pid any, ruleID int64, opt *UpdateContainerRegistryTagProtectionRuleOptions, options ...RequestOptionFunc) (*ContainerRegistryTagProtectionRule, *Response, error)

		// DeleteContainerRegistryTagProtectionRule deletes a container registry tag protection
		// rule from a project.
		//
		// GitLab API docs: https://docs.gitlab.com/api/container_registry_protection_tag_rules/#delete-a-container-registry-protection-tag-rule
		DeleteContainerRegistryTagProtectionRule(pid any, ruleID int64, options ...RequestOptionFunc) (*Response, error)
	}

	// ContainerRegistryTagProtectionRulesService handles communication with
	// the container registry tag protection rules related methods of the
	// GitLab API.
	//
	// GitLab API docs: https://docs.gitlab.com/api/container_registry_protection_tag_rules/
	ContainerRegistryTagProtectionRulesService struct {
		client *Client
	}
)

var _ ContainerRegistryTagProtectionRulesServiceInterface = (*ContainerRegistryTagProtectionRulesService)(nil)

// ContainerRegistryTagProtectionRule represents a container registry tag protection
// rule from GitLab API.
//
// GitLab API docs: https://docs.gitlab.com/api/container_registry_protection_tag_rules/
type ContainerRegistryTagProtectionRule struct {
	ID                          int64                     `json:"id"`
	ProjectID                   int64                     `json:"project_id"`
	TagNamePattern              string                    `json:"tag_name_pattern"`
	MinimumAccessLevelForPush   ProtectionRuleAccessLevel `json:"minimum_access_level_for_push"`
	MinimumAccessLevelForDelete ProtectionRuleAccessLevel `json:"minimum_access_level_for_delete"`
}

// String returns a string representation of the container registry tag protection rule.
func (s ContainerRegistryTagProtectionRule) String() string {
	return Stringify(s)
}

// CreateContainerRegistryTagProtectionRuleOptions represents the available
// CreateContainerRegistryTagProtectionRule() options.
//
// GitLab API docs: https://docs.gitlab.com/api/container_registry_protection_tag_rules/#create-a-container-registry-protection-tag-rule
type CreateContainerRegistryTagProtectionRuleOptions struct {
	TagNamePattern              *string                    `url:"tag_name_pattern,omitempty" json:"tag_name_pattern,omitempty"`
	MinimumAccessLevelForPush   *ProtectionRuleAccessLevel `url:"minimum_access_level_for_push,omitempty" json:"minimum_access_level_for_push,omitempty"`
	MinimumAccessLevelForDelete *ProtectionRuleAccessLevel `url:"minimum_access_level_for_delete,omitempty" json:"minimum_access_level_for_delete,omitempty"`
}

// UpdateContainerRegistryTagProtectionRuleOptions represents the available
// UpdateContainerRegistryTagProtectionRule() options.
//
// GitLab API docs: https://docs.gitlab.com/api/container_registry_protection_tag_rules/#update-a-container-registry-protection-tag-rule
type UpdateContainerRegistryTagProtectionRuleOptions struct {
	TagNamePattern              *string                    `url:"tag_name_pattern,omitempty" json:"tag_name_pattern,omitempty"`
	MinimumAccessLevelForPush   *ProtectionRuleAccessLevel `url:"minimum_access_level_for_push,omitempty" json:"minimum_access_level_for_push,omitempty"`
	MinimumAccessLevelForDelete *ProtectionRuleAccessLevel `url:"minimum_access_level_for_delete,omitempty" json:"minimum_access_level_for_delete,omitempty"`
}

// ListContainerRegistryTagProtectionRules gets a list of container registry protection tag
// rules from a project's container registry.
//
// GitLab API docs: https://docs.gitlab.com/api/container_registry_protection_tag_rules/#list-container-registry-protection-tag-rules
func (s *ContainerRegistryTagProtectionRulesService) ListContainerRegistryTagProtectionRules(pid any, options ...RequestOptionFunc) ([]*ContainerRegistryTagProtectionRule, *Response, error) {
	return do[[]*ContainerRegistryTagProtectionRule](s.client,
		withMethod(http.MethodGet),
		withPath(routeProjectsIDRegistryProtectionTagRules, ProjectID{pid}),
		withRequestOpts(options...),
	)
}

// CreateContainerRegistryTagProtectionRule creates a container registry tag protection
// rule for a project's container registry.
//
// GitLab API docs: https://docs.gitlab.com/api/container_registry_protection_tag_rules/#create-a-container-registry-protection-tag-rule
func (s *ContainerRegistryTagProtectionRulesService) CreateContainerRegistryTagProtectionRule(pid any, opt *CreateContainerRegistryTagProtectionRuleOptions, options ...RequestOptionFunc) (*ContainerRegistryTagProtectionRule, *Response, error) {
	return do[*ContainerRegistryTagProtectionRule](s.client,
		withMethod(http.MethodPost),
		withPath(routeProjectsIDRegistryProtectionTagRules, ProjectID{pid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// UpdateContainerRegistryTagProtectionRule updates a container registry tag protection
// rule for a project's container registry.
//
// GitLab API docs: https://docs.gitlab.com/api/container_registry_protection_tag_rules/#update-a-container-registry-protection-tag-rule
func (s *ContainerRegistryTagProtectionRulesService) UpdateContainerRegistryTagProtectionRule(pid any, ruleID int64, opt *UpdateContainerRegistryTagProtectionRuleOptions, options ...RequestOptionFunc) (*ContainerRegistryTagProtectionRule, *Response, error) {
	return do[*ContainerRegistryTagProtectionRule](s.client,
		withMethod(http.MethodPatch),
		withPath(routeProjectsIDRegistryProtectionTagRulesID, ProjectID{pid}, ruleID),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// DeleteContainerRegistryTagProtectionRule deletes a container registry tag protection
// rule from a project's container registry.
//
// GitLab API docs: https://docs.gitlab.com/api/container_registry_protection_tag_rules/#delete-a-container-registry-protection-tag-rule
func (s *ContainerRegistryTagProtectionRulesService) DeleteContainerRegistryTagProtectionRule(pid any, ruleID int64, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodDelete),
		withPath(routeProjectsIDRegistryProtectionTagRulesID, ProjectID{pid}, ruleID),
		withRequestOpts(options...),
	)
	return resp, err
}
