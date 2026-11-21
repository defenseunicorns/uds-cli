package gitlab

import (
	"errors"
	"fmt"
	"time"
)

// TargetBranchRule represents a single target branch rule on a project.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#projecttargetbranchrule
type TargetBranchRule struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	TargetBranch string    `json:"targetBranch"`
	CreatedAt    time.Time `json:"createdAt"`
}

// targetBranchRuleGQL is used to unmarshal GraphQL responses where the ID is a
// global ID string of the form gid://gitlab/Projects::TargetBranchRule/<n>.
type targetBranchRuleGQL struct {
	ID           gidGQL    `json:"id"`
	Name         string    `json:"name"`
	TargetBranch string    `json:"targetBranch"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (r *targetBranchRuleGQL) unwrap() *TargetBranchRule {
	return &TargetBranchRule{
		ID:           r.ID.Int64,
		Name:         r.Name,
		TargetBranch: r.TargetBranch,
		CreatedAt:    r.CreatedAt,
	}
}

// CreateTargetBranchRuleOptions represents the available
// CreateTargetBranchRule() options.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationprojecttargetbranchrulecreate
type CreateTargetBranchRuleOptions struct {
	Name         string
	TargetBranch string
}

// ListProjectTargetBranchRules returns the target branch rules for a project.
// projectFullPath must be the full namespace/project path string, as the
// GitLab GraphQL project(fullPath:) field does not accept numeric IDs.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#projecttargetbranchruleconnection
func (s *ProjectsService) ListProjectTargetBranchRules(projectFullPath string, options ...RequestOptionFunc) ([]TargetBranchRule, *Response, error) {
	query := GraphQLQuery{
		Query: `
			query($fullPath: ID!) {
				project(fullPath: $fullPath) {
					targetBranchRules {
						nodes {
							id
							name
							targetBranch
							createdAt
						}
					}
				}
			}
		`,
		Variables: map[string]any{
			"fullPath": projectFullPath,
		},
	}

	var result struct {
		Data struct {
			Project *struct {
				TargetBranchRules struct {
					Nodes []targetBranchRuleGQL `json:"nodes"`
				} `json:"targetBranchRules"`
			} `json:"project"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(query, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if result.Data.Project == nil {
		return nil, resp, ErrNotFound
	}

	rules := make([]TargetBranchRule, 0, len(result.Data.Project.TargetBranchRules.Nodes))
	for i := range result.Data.Project.TargetBranchRules.Nodes {
		rules = append(rules, *result.Data.Project.TargetBranchRules.Nodes[i].unwrap())
	}

	return rules, resp, nil
}

// CreateTargetBranchRule creates a new target branch rule for a project.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationprojecttargetbranchrulecreate
func (s *ProjectsService) CreateTargetBranchRule(pid int64, opt *CreateTargetBranchRuleOptions, options ...RequestOptionFunc) (*TargetBranchRule, *Response, error) {
	if opt == nil {
		return nil, nil, errors.New("opt is required")
	}

	projectGID := gidGQL{Type: "Project", Int64: pid}

	mutation := GraphQLQuery{
		Query: `
			mutation CreateTargetBranchRule($input: ProjectTargetBranchRuleCreateInput!) {
				projectTargetBranchRuleCreate(input: $input) {
					targetBranchRule {
						id
						name
						targetBranch
						createdAt
					}
					errors
				}
			}
		`,
		Variables: map[string]any{
			"input": map[string]any{
				"projectId":    projectGID.String(),
				"name":         opt.Name,
				"targetBranch": opt.TargetBranch,
			},
		},
	}

	var result struct {
		Data struct {
			ProjectTargetBranchRuleCreate struct {
				TargetBranchRule *targetBranchRuleGQL `json:"targetBranchRule"`
				Errors           []string             `json:"errors"`
			} `json:"projectTargetBranchRuleCreate"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if len(result.Data.ProjectTargetBranchRuleCreate.Errors) > 0 {
		return nil, resp, fmt.Errorf("projectTargetBranchRuleCreate mutation errors: %v", result.Data.ProjectTargetBranchRuleCreate.Errors)
	}
	if result.Data.ProjectTargetBranchRuleCreate.TargetBranchRule == nil {
		return nil, resp, ErrNotFound
	}

	return result.Data.ProjectTargetBranchRuleCreate.TargetBranchRule.unwrap(), resp, nil
}

// DeleteTargetBranchRule deletes a target branch rule.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationprojecttargetbranchruledestroy
func (s *ProjectsService) DeleteTargetBranchRule(id int64, options ...RequestOptionFunc) (*Response, error) {
	gid := gidGQL{Type: "Projects::TargetBranchRule", Int64: id}

	mutation := GraphQLQuery{
		Query: `
			mutation DeleteTargetBranchRule($input: ProjectTargetBranchRuleDestroyInput!) {
				projectTargetBranchRuleDestroy(input: $input) {
					errors
				}
			}
		`,
		Variables: map[string]any{
			"input": map[string]any{
				"id": gid.String(),
			},
		},
	}

	var result struct {
		Data struct {
			ProjectTargetBranchRuleDestroy struct {
				Errors []string `json:"errors"`
			} `json:"projectTargetBranchRuleDestroy"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return resp, err
	}
	if len(result.Data.ProjectTargetBranchRuleDestroy.Errors) > 0 {
		return resp, fmt.Errorf("projectTargetBranchRuleDestroy mutation errors: %v", result.Data.ProjectTargetBranchRuleDestroy.Errors)
	}

	return resp, nil
}
