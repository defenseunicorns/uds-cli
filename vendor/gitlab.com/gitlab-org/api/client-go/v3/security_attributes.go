package gitlab

import (
	"errors"
	"fmt"
)

type (
	// SecurityAttributesServiceInterface describes the API methods for security
	// attributes.
	//
	// GitLab API docs:
	// https://docs.gitlab.com/api/graphql/reference/#securityattribute
	SecurityAttributesServiceInterface interface {
		// CreateSecurityAttributes creates one or more security attributes under
		// a security category.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributecreate
		CreateSecurityAttributes(namespaceID int64, categoryID int64, opt *CreateSecurityAttributesOptions, options ...RequestOptionFunc) ([]*SecurityAttribute, *Response, error)

		// UpdateSecurityAttribute updates an existing security attribute.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributeupdate
		UpdateSecurityAttribute(id int64, opt *UpdateSecurityAttributeOptions, options ...RequestOptionFunc) (*SecurityAttribute, *Response, error)

		// DestroySecurityAttribute deletes a security attribute.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributedestroy
		DestroySecurityAttribute(id int64, options ...RequestOptionFunc) (*Response, error)

		// ProjectUpdateSecurityAttribute adds or removes security attributes on
		// a project.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributeprojectupdate
		ProjectUpdateSecurityAttribute(projectID int64, opt *ProjectUpdateSecurityAttributeOptions, options ...RequestOptionFunc) (*SecurityAttributeProjectUpdateResult, *Response, error)

		// BulkUpdateSecurityAttributes adds, removes, or replaces security
		// attributes on multiple groups and projects in a single operation.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#mutationbulkupdatesecurityattributes
		BulkUpdateSecurityAttributes(opt *BulkUpdateSecurityAttributesOptions, options ...RequestOptionFunc) (*Response, error)
	}

	// SecurityAttributesService handles communication with the security attribute
	// related methods of the GitLab API.
	//
	// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#securityattribute
	SecurityAttributesService struct {
		client *Client
	}
)

var _ SecurityAttributesServiceInterface = (*SecurityAttributesService)(nil)

// SecurityAttributeBulkUpdateMode represents the mode used when bulk updating
// security attributes on groups and projects.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#securityattributebulkupdatemode
type SecurityAttributeBulkUpdateMode string

const (
	SecurityAttributeBulkUpdateModeAdd     SecurityAttributeBulkUpdateMode = "ADD"
	SecurityAttributeBulkUpdateModeRemove  SecurityAttributeBulkUpdateMode = "REMOVE"
	SecurityAttributeBulkUpdateModeReplace SecurityAttributeBulkUpdateMode = "REPLACE"
)

// SecurityAttribute represents a GitLab security attribute.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#securityattribute
type SecurityAttribute struct {
	ID               int64
	Name             string
	Color            string
	Description      string
	EditableState    SecurityCategoryEditableState
	SecurityCategory *SecurityCategory
}

// securityAttributeGQL is used to unmarshal GraphQL responses where the ID is a
// global ID string of the form gid://gitlab/Security::Attribute/<n>.
type securityAttributeGQL struct {
	ID               gidGQL                        `json:"id"`
	Name             string                        `json:"name"`
	Color            string                        `json:"color"`
	Description      string                        `json:"description"`
	EditableState    SecurityCategoryEditableState `json:"editableState"`
	SecurityCategory *securityCategoryGQL          `json:"securityCategory"`
}

func (a *securityAttributeGQL) unwrap() *SecurityAttribute {
	attr := &SecurityAttribute{
		ID:            a.ID.Int64,
		Name:          a.Name,
		Color:         a.Color,
		Description:   a.Description,
		EditableState: a.EditableState,
	}
	if a.SecurityCategory != nil {
		attr.SecurityCategory = a.SecurityCategory.unwrap()
	}
	return attr
}

// SecurityAttributeInput represents a single security attribute to create.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#securityattributeinput
type SecurityAttributeInput struct {
	// Name of the security attribute. Required.
	Name *string

	// Description of the security attribute. Required.
	Description *string

	// Color of the security attribute, represented as a hex code. Example:
	// "#ff0000". Required.
	Color *string
}

// SecurityAttributeProjectUpdateResult represents the result of a
// ProjectUpdateSecurityAttribute mutation.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributeprojectupdate
type SecurityAttributeProjectUpdateResult struct {
	// AddedCount is the number of attributes added to the project.
	AddedCount int64

	// RemovedCount is the number of attributes removed from the project.
	RemovedCount int64
}

// CreateSecurityAttributesOptions represents the available
// CreateSecurityAttributes() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributecreate
type CreateSecurityAttributesOptions struct {
	// Attributes is the list of security attributes to create. Required.
	Attributes *[]SecurityAttributeInput
}

// UpdateSecurityAttributeOptions represents the available
// UpdateSecurityAttribute() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributeupdate
type UpdateSecurityAttributeOptions struct {
	// Name of the security attribute.
	Name *string

	// Description of the security attribute.
	Description *string

	// Color of the security attribute, represented as a hex code. Example:
	// "#ff0000".
	Color *string
}

// ProjectUpdateSecurityAttributeOptions represents the available
// ProjectUpdateSecurityAttribute() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributeprojectupdate
type ProjectUpdateSecurityAttributeOptions struct {
	// AddAttributeIDs are the IDs of the security attributes to add to the
	// project.
	AddAttributeIDs *[]int64

	// RemoveAttributeIDs are the IDs of the security attributes to remove from
	// the project.
	RemoveAttributeIDs *[]int64
}

// BulkUpdateSecurityAttributesOptions represents the available
// BulkUpdateSecurityAttributes() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationbulkupdatesecurityattributes
type BulkUpdateSecurityAttributesOptions struct {
	// GroupIDs are the IDs of the groups to update.
	GroupIDs *[]int64

	// ProjectIDs are the IDs of the projects to update.
	ProjectIDs *[]int64

	// AttributeIDs are the IDs of the security attributes to apply. Required.
	AttributeIDs *[]int64

	// Mode is the update mode. Required.
	Mode *SecurityAttributeBulkUpdateMode
}

// CreateSecurityAttributes creates one or more security attributes under a
// security category.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributecreate
func (s *SecurityAttributesService) CreateSecurityAttributes(namespaceID int64, categoryID int64, opt *CreateSecurityAttributesOptions, options ...RequestOptionFunc) ([]*SecurityAttribute, *Response, error) {
	if opt == nil {
		return nil, nil, errors.New("opt is required")
	}

	namespaceGID := gidGQL{Type: "Namespace", Int64: namespaceID}
	categoryGID := gidGQL{Type: "Security::Category", Int64: categoryID}

	attrs := make([]map[string]any, 0, len(*opt.Attributes))
	for _, a := range *opt.Attributes {
		attrs = append(attrs, map[string]any{
			"name":        a.Name,
			"description": a.Description,
			"color":       a.Color,
		})
	}

	mutation := GraphQLQuery{
		Query: `
			mutation CreateSecurityAttributes($input: SecurityAttributeCreateInput!) {
				securityAttributeCreate(input: $input) {
					securityAttributes {
						id
						name
						color
						description
						editableState
						securityCategory {
							id
							name
							description
							multipleSelection
							editableState
							templateType
						}
					}
					errors
				}
			}
		`,
		Variables: map[string]any{
			"input": map[string]any{
				"namespaceId": namespaceGID.String(),
				"categoryId":  categoryGID.String(),
				"attributes":  attrs,
			},
		},
	}

	var result struct {
		Data struct {
			SecurityAttributeCreate struct {
				SecurityAttributes []*securityAttributeGQL `json:"securityAttributes"`
				Errors             []string                `json:"errors"`
			} `json:"securityAttributeCreate"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if len(result.Data.SecurityAttributeCreate.Errors) > 0 {
		return nil, resp, fmt.Errorf("securityAttributeCreate mutation errors: %v", result.Data.SecurityAttributeCreate.Errors)
	}

	var ret []*SecurityAttribute
	for _, a := range result.Data.SecurityAttributeCreate.SecurityAttributes {
		ret = append(ret, a.unwrap())
	}

	return ret, resp, nil
}

// UpdateSecurityAttribute updates an existing security attribute.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributeupdate
func (s *SecurityAttributesService) UpdateSecurityAttribute(id int64, opt *UpdateSecurityAttributeOptions, options ...RequestOptionFunc) (*SecurityAttribute, *Response, error) {
	attributeGID := gidGQL{Type: "Security::Attribute", Int64: id}

	input := map[string]any{
		"id": attributeGID.String(),
	}
	if opt != nil {
		if opt.Name != nil {
			input["name"] = opt.Name
		}
		if opt.Description != nil {
			input["description"] = opt.Description
		}
		if opt.Color != nil {
			input["color"] = opt.Color
		}
	}

	mutation := GraphQLQuery{
		Query: `
			mutation UpdateSecurityAttribute($input: SecurityAttributeUpdateInput!) {
				securityAttributeUpdate(input: $input) {
					securityAttribute {
						id
						name
						color
						description
						editableState
						securityCategory {
							id
							name
							description
							multipleSelection
							editableState
							templateType
						}
					}
					errors
				}
			}
		`,
		Variables: map[string]any{
			"input": input,
		},
	}

	var result struct {
		Data struct {
			SecurityAttributeUpdate struct {
				SecurityAttribute *securityAttributeGQL `json:"securityAttribute"`
				Errors            []string              `json:"errors"`
			} `json:"securityAttributeUpdate"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if len(result.Data.SecurityAttributeUpdate.Errors) > 0 {
		return nil, resp, fmt.Errorf("securityAttributeUpdate mutation errors: %v", result.Data.SecurityAttributeUpdate.Errors)
	}
	if result.Data.SecurityAttributeUpdate.SecurityAttribute == nil {
		return nil, resp, ErrNotFound
	}

	return result.Data.SecurityAttributeUpdate.SecurityAttribute.unwrap(), resp, nil
}

// DestroySecurityAttribute deletes a security attribute.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributedestroy
func (s *SecurityAttributesService) DestroySecurityAttribute(id int64, options ...RequestOptionFunc) (*Response, error) {
	attributeGID := gidGQL{Type: "Security::Attribute", Int64: id}

	mutation := GraphQLQuery{
		Query: `
			mutation DestroySecurityAttribute($input: SecurityAttributeDestroyInput!) {
				securityAttributeDestroy(input: $input) {
					errors
				}
			}
		`,
		Variables: map[string]any{
			"input": map[string]any{
				"id": attributeGID.String(),
			},
		},
	}

	var result struct {
		Data struct {
			SecurityAttributeDestroy struct {
				Errors []string `json:"errors"`
			} `json:"securityAttributeDestroy"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return resp, err
	}
	if len(result.Data.SecurityAttributeDestroy.Errors) > 0 {
		return resp, fmt.Errorf("securityAttributeDestroy mutation errors: %v", result.Data.SecurityAttributeDestroy.Errors)
	}

	return resp, nil
}

// ProjectUpdateSecurityAttribute adds or removes security attributes on a
// project.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributeprojectupdate
func (s *SecurityAttributesService) ProjectUpdateSecurityAttribute(projectID int64, opt *ProjectUpdateSecurityAttributeOptions, options ...RequestOptionFunc) (*SecurityAttributeProjectUpdateResult, *Response, error) {
	if opt == nil {
		return nil, nil, errors.New("opt is required")
	}

	projectGID := gidGQL{Type: "Project", Int64: projectID}

	input := map[string]any{
		"projectId": projectGID.String(),
	}
	if opt.AddAttributeIDs != nil && len(*opt.AddAttributeIDs) > 0 {
		input["addAttributeIds"] = newGIDStrings("Security::Attribute", *opt.AddAttributeIDs...)
	}
	if opt.RemoveAttributeIDs != nil && len(*opt.RemoveAttributeIDs) > 0 {
		input["removeAttributeIds"] = newGIDStrings("Security::Attribute", *opt.RemoveAttributeIDs...)
	}

	mutation := GraphQLQuery{
		Query: `
			mutation ProjectUpdateSecurityAttribute($input: SecurityAttributeProjectUpdateInput!) {
				securityAttributeProjectUpdate(input: $input) {
					addedCount
					removedCount
					errors
				}
			}
		`,
		Variables: map[string]any{
			"input": input,
		},
	}

	var result struct {
		Data struct {
			SecurityAttributeProjectUpdate struct {
				AddedCount   int64    `json:"addedCount"`
				RemovedCount int64    `json:"removedCount"`
				Errors       []string `json:"errors"`
			} `json:"securityAttributeProjectUpdate"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if len(result.Data.SecurityAttributeProjectUpdate.Errors) > 0 {
		return nil, resp, fmt.Errorf("securityAttributeProjectUpdate mutation errors: %v", result.Data.SecurityAttributeProjectUpdate.Errors)
	}

	return &SecurityAttributeProjectUpdateResult{
		AddedCount:   result.Data.SecurityAttributeProjectUpdate.AddedCount,
		RemovedCount: result.Data.SecurityAttributeProjectUpdate.RemovedCount,
	}, resp, nil
}

// BulkUpdateSecurityAttributes adds, removes, or replaces security attributes
// on multiple groups and projects in a single operation.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationbulkupdatesecurityattributes
func (s *SecurityAttributesService) BulkUpdateSecurityAttributes(opt *BulkUpdateSecurityAttributesOptions, options ...RequestOptionFunc) (*Response, error) {
	if opt == nil {
		return nil, errors.New("opt is required")
	}

	var items []string
	if opt.GroupIDs != nil {
		items = append(items, newGIDStrings("Group", *opt.GroupIDs...)...)
	}
	if opt.ProjectIDs != nil {
		items = append(items, newGIDStrings("Project", *opt.ProjectIDs...)...)
	}

	mutation := GraphQLQuery{
		Query: `
			mutation BulkUpdateSecurityAttributes($input: BulkUpdateSecurityAttributesInput!) {
				bulkUpdateSecurityAttributes(input: $input) {
					errors
				}
			}
		`,
		Variables: map[string]any{
			"input": map[string]any{
				"items":      items,
				"attributes": newGIDStrings("Security::Attribute", *opt.AttributeIDs...),
				"mode":       *opt.Mode,
			},
		},
	}

	var result struct {
		Data struct {
			BulkUpdateSecurityAttributes struct {
				Errors []string `json:"errors"`
			} `json:"bulkUpdateSecurityAttributes"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return resp, err
	}
	if len(result.Data.BulkUpdateSecurityAttributes.Errors) > 0 {
		return resp, fmt.Errorf("bulkUpdateSecurityAttributes mutation errors: %v", result.Data.BulkUpdateSecurityAttributes.Errors)
	}

	return resp, nil
}
