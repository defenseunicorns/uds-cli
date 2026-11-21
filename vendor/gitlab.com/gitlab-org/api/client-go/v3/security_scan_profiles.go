package gitlab

import (
	"errors"
	"fmt"
	"strings"
)

type (
	// SecurityScanProfilesServiceInterface defines all the methods for working
	// with security scan profiles through the GitLab GraphQL API.
	SecurityScanProfilesServiceInterface interface {
		// AttachSecurityScanProfile attaches a security scan profile to the
		// given projects and/or groups.
		//
		// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecurityscanprofileattach
		AttachSecurityScanProfile(opt *AttachSecurityScanProfileOptions, options ...RequestOptionFunc) (*Response, error)
		// DetachSecurityScanProfile detaches a security scan profile from the
		// given projects and/or groups.
		//
		// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecurityscanprofiledetach
		DetachSecurityScanProfile(opt *DetachSecurityScanProfileOptions, options ...RequestOptionFunc) (*Response, error)
		// ListProjectScanProfileStatuses returns the scan profile statuses for a
		// project. projectFullPath must be the full namespace/project path
		// string, as the GitLab GraphQL project(fullPath:) field does not accept
		// numeric IDs.
		//
		// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#project-scanprofilestatuses
		ListProjectScanProfileStatuses(projectFullPath string, options ...RequestOptionFunc) ([]ScanProfileStatus, *Response, error)
	}

	// SecurityScanProfilesService handles communication with the security scan
	// profile related methods of the GitLab GraphQL API.
	//
	// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecurityscanprofileattach
	SecurityScanProfilesService struct {
		client *Client
	}
)

var _ SecurityScanProfilesServiceInterface = (*SecurityScanProfilesService)(nil)

const (
	securityScanProfileAttachMutation = `
			mutation SecurityScanProfileAttach($input: SecurityScanProfileAttachInput!) {
				securityScanProfileAttach(input: $input) {
					errors
				}
			}
		`

	securityScanProfileDetachMutation = `
			mutation SecurityScanProfileDetach($input: SecurityScanProfileDetachInput!) {
				securityScanProfileDetach(input: $input) {
					errors
				}
			}
		`

	scanProfileStatusesQuery = `
			query($fullPath: ID!) {
				project(fullPath: $fullPath) {
					scanProfileStatuses {
						status
						scanProfile {
							id
							name
							scanType
						}
					}
				}
			}
		`
)

// ScanProfile represents a security scan profile.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#scanprofiletype
type ScanProfile struct {
	// ID is the global ID of the scan profile. Persisted profiles use a
	// numeric identifier; the built-in default profiles use their scan type
	// (for example "dependency_scanning_post_processing"), so this is kept as
	// a string rather than a numeric global ID.
	ID       string `json:"id"`
	Name     string `json:"name"`
	ScanType string `json:"scanType"`
}

// ScanProfileStatus represents the status of a scan profile for a project.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#scanprofileprojectstatus
type ScanProfileStatus struct {
	// Status is the computed display status, one of NOT_CONFIGURED, PENDING,
	// ACTIVE, WARNING, FAILED, or STALE.
	Status      string      `json:"status"`
	ScanProfile ScanProfile `json:"scanProfile"`
}

// graphQLErrors returns a non-nil error if the response carries top-level
// GraphQL errors. These come back with an HTTP 200, so the generic client does
// not flag them; callers must check explicitly.
func graphQLErrors(errs GenericGraphQLErrors) error {
	if len(errs.Errors) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(errs.Errors))
	for _, e := range errs.Errors {
		msgs = append(msgs, e.Message)
	}
	return errors.New(strings.Join(msgs, "; "))
}

// SecurityScanProfileGID builds the GraphQL global ID for a security scan
// profile. Persisted profiles use their numeric database ID; the built-in
// default profiles use their scan type as the identifier, for example
// SecurityScanProfileGID("dependency_scanning_post_processing").
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#scanprofiletype
func SecurityScanProfileGID(identifier string) string {
	return fmt.Sprintf("gid://gitlab/Security::ScanProfile/%s", identifier)
}

// AttachSecurityScanProfileOptions represents the available
// AttachSecurityScanProfile() options.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecurityscanprofileattach
type AttachSecurityScanProfileOptions struct {
	// SecurityScanProfileID is the global ID of the scan profile to attach,
	// e.g. gid://gitlab/Security::ScanProfile/dependency_scanning_post_processing.
	// Use SecurityScanProfileGID to build it.
	SecurityScanProfileID string
	// ProjectIDs are the numeric IDs of the projects to attach the profile to.
	ProjectIDs []int64
	// GroupIDs are the numeric IDs of the groups to attach the profile to.
	GroupIDs []int64
}

// DetachSecurityScanProfileOptions represents the available
// DetachSecurityScanProfile() options.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecurityscanprofiledetach
type DetachSecurityScanProfileOptions struct {
	SecurityScanProfileID string
	ProjectIDs            []int64
	GroupIDs              []int64
}

// AttachSecurityScanProfile attaches a security scan profile to the given
// projects and/or groups. The caller must be a Maintainer or Owner of the
// targets, and the feature must be enabled on the instance, otherwise the
// mutation returns an error.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecurityscanprofileattach
func (s *SecurityScanProfilesService) AttachSecurityScanProfile(opt *AttachSecurityScanProfileOptions, options ...RequestOptionFunc) (*Response, error) {
	if opt == nil {
		return nil, errors.New("opt is required")
	}

	mutation := GraphQLQuery{
		Query: securityScanProfileAttachMutation,
		Variables: map[string]any{
			"input": map[string]any{
				"securityScanProfileId": opt.SecurityScanProfileID,
				"projectIds":            newGIDStrings("Project", opt.ProjectIDs...),
				"groupIds":              newGIDStrings("Group", opt.GroupIDs...),
			},
		},
	}

	var result struct {
		Data struct {
			SecurityScanProfileAttach struct {
				Errors []string `json:"errors"`
			} `json:"securityScanProfileAttach"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return resp, err
	}
	if err := graphQLErrors(result.GenericGraphQLErrors); err != nil {
		return resp, err
	}
	if len(result.Data.SecurityScanProfileAttach.Errors) > 0 {
		return resp, fmt.Errorf("securityScanProfileAttach mutation errors: %v", result.Data.SecurityScanProfileAttach.Errors)
	}

	return resp, nil
}

// DetachSecurityScanProfile detaches a security scan profile from the given
// projects and/or groups.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecurityscanprofiledetach
func (s *SecurityScanProfilesService) DetachSecurityScanProfile(opt *DetachSecurityScanProfileOptions, options ...RequestOptionFunc) (*Response, error) {
	if opt == nil {
		return nil, errors.New("opt is required")
	}

	mutation := GraphQLQuery{
		Query: securityScanProfileDetachMutation,
		Variables: map[string]any{
			"input": map[string]any{
				"securityScanProfileId": opt.SecurityScanProfileID,
				"projectIds":            newGIDStrings("Project", opt.ProjectIDs...),
				"groupIds":              newGIDStrings("Group", opt.GroupIDs...),
			},
		},
	}

	var result struct {
		Data struct {
			SecurityScanProfileDetach struct {
				Errors []string `json:"errors"`
			} `json:"securityScanProfileDetach"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return resp, err
	}
	if err := graphQLErrors(result.GenericGraphQLErrors); err != nil {
		return resp, err
	}
	if len(result.Data.SecurityScanProfileDetach.Errors) > 0 {
		return resp, fmt.Errorf("securityScanProfileDetach mutation errors: %v", result.Data.SecurityScanProfileDetach.Errors)
	}

	return resp, nil
}

// ListProjectScanProfileStatuses returns the scan profile statuses for a
// project. projectFullPath must be the full namespace/project path string, as
// the GitLab GraphQL project(fullPath:) field does not accept numeric IDs.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#project-scanprofilestatuses
func (s *SecurityScanProfilesService) ListProjectScanProfileStatuses(projectFullPath string, options ...RequestOptionFunc) ([]ScanProfileStatus, *Response, error) {
	query := GraphQLQuery{
		Query: scanProfileStatusesQuery,
		Variables: map[string]any{
			"fullPath": projectFullPath,
		},
	}

	var result struct {
		Data struct {
			Project *struct {
				ScanProfileStatuses []ScanProfileStatus `json:"scanProfileStatuses"`
			} `json:"project"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(query, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if err := graphQLErrors(result.GenericGraphQLErrors); err != nil {
		return nil, resp, err
	}
	if result.Data.Project == nil {
		return nil, resp, ErrNotFound
	}

	return result.Data.Project.ScanProfileStatuses, resp, nil
}
