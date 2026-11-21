package gitlab

import (
	"errors"
	"fmt"
)

type (
	// SecurityCategoriesServiceInterface describes the API methods for security
	// categories.
	//
	// GitLab API docs:
	// https://docs.gitlab.com/api/graphql/reference/#securitycategory
	SecurityCategoriesServiceInterface interface {
		// CreateSecurityCategory creates a new security category.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategorycreate
		CreateSecurityCategory(namespaceID int64, opt *CreateSecurityCategoryOptions, options ...RequestOptionFunc) (*SecurityCategory, *Response, error)

		// UpdateSecurityCategory updates an existing security category.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategoryupdate
		UpdateSecurityCategory(id int64, namespaceID int64, opt *UpdateSecurityCategoryOptions, options ...RequestOptionFunc) (*SecurityCategory, *Response, error)

		// DestroySecurityCategory deletes a security category and all its
		// associated security attributes.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategorydestroy
		DestroySecurityCategory(id int64, options ...RequestOptionFunc) (*Response, error)
	}

	// SecurityCategoriesService handles communication with the security category
	// related methods of the GitLab API.
	//
	// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#securitycategory
	SecurityCategoriesService struct {
		client *Client
	}
)

var _ SecurityCategoriesServiceInterface = (*SecurityCategoriesService)(nil)

// SecurityCategoryEditableState represents the editable state of a security
// category or attribute.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#securitycategoryeditablestate
type SecurityCategoryEditableState string

const (
	SecurityCategoryEditableStateLocked             SecurityCategoryEditableState = "LOCKED"
	SecurityCategoryEditableStateEditableAttributes SecurityCategoryEditableState = "EDITABLE_ATTRIBUTES"
	SecurityCategoryEditableStateEditable           SecurityCategoryEditableState = "EDITABLE"
)

// SecurityCategoryTemplateType represents the template type for predefined
// security categories.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#securitycategorytemplatetype
type SecurityCategoryTemplateType string

const (
	SecurityCategoryTemplateTypeBusinessImpact SecurityCategoryTemplateType = "BUSINESS_IMPACT"
	SecurityCategoryTemplateTypeBusinessUnit   SecurityCategoryTemplateType = "BUSINESS_UNIT"
	SecurityCategoryTemplateTypeApplication    SecurityCategoryTemplateType = "APPLICATION"
	SecurityCategoryTemplateTypeExposure       SecurityCategoryTemplateType = "EXPOSURE"
)

// SecurityCategory represents a GitLab security category.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#securitycategory
type SecurityCategory struct {
	ID                 int64
	Name               string
	Description        *string
	MultipleSelection  bool
	EditableState      SecurityCategoryEditableState
	TemplateType       *SecurityCategoryTemplateType
	SecurityAttributes []*SecurityAttribute
}

// securityCategoryGQL is used to unmarshal GraphQL responses where the ID is a
// global ID string of the form gid://gitlab/Security::Category/<n>.
type securityCategoryGQL struct {
	ID                 gidGQL                        `json:"id"`
	Name               string                        `json:"name"`
	Description        *string                       `json:"description"`
	MultipleSelection  bool                          `json:"multipleSelection"`
	EditableState      SecurityCategoryEditableState `json:"editableState"`
	TemplateType       *SecurityCategoryTemplateType `json:"templateType"`
	SecurityAttributes []*securityAttributeGQL       `json:"securityAttributes"`
}

func (c *securityCategoryGQL) unwrap() *SecurityCategory {
	cat := &SecurityCategory{
		ID:                c.ID.Int64,
		Name:              c.Name,
		Description:       c.Description,
		MultipleSelection: c.MultipleSelection,
		EditableState:     c.EditableState,
		TemplateType:      c.TemplateType,
	}
	for _, a := range c.SecurityAttributes {
		cat.SecurityAttributes = append(cat.SecurityAttributes, a.unwrap())
	}
	return cat
}

// CreateSecurityCategoryOptions represents the available
// CreateSecurityCategory() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategorycreate
type CreateSecurityCategoryOptions struct {
	// Name of the security category. Required.
	Name string

	// Description of the security category.
	Description *string

	// Whether multiple attributes can be selected.
	MultipleSelection *bool
}

// UpdateSecurityCategoryOptions represents the available
// UpdateSecurityCategory() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategoryupdate
type UpdateSecurityCategoryOptions struct {
	// Name of the security category.
	Name *string

	// Description of the security category.
	Description *string
}

// CreateSecurityCategory creates a new security category.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategorycreate
func (s *SecurityCategoriesService) CreateSecurityCategory(namespaceID int64, opt *CreateSecurityCategoryOptions, options ...RequestOptionFunc) (*SecurityCategory, *Response, error) {
	if opt == nil {
		return nil, nil, errors.New("opt is required")
	}

	namespaceGID := gidGQL{Type: "Namespace", Int64: namespaceID}

	input := map[string]any{
		"namespaceId": namespaceGID.String(),
		"name":        opt.Name,
	}
	if opt.Description != nil {
		input["description"] = opt.Description
	}
	if opt.MultipleSelection != nil {
		input["multipleSelection"] = opt.MultipleSelection
	}

	mutation := GraphQLQuery{
		Query: `
			mutation CreateSecurityCategory($input: SecurityCategoryCreateInput!) {
				securityCategoryCreate(input: $input) {
					securityCategory {
						id
						name
						description
						multipleSelection
						editableState
						templateType
						securityAttributes {
							id
							name
							color
							description
							editableState
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
			SecurityCategoryCreate struct {
				SecurityCategory *securityCategoryGQL `json:"securityCategory"`
				Errors           []string             `json:"errors"`
			} `json:"securityCategoryCreate"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if len(result.Data.SecurityCategoryCreate.Errors) > 0 {
		return nil, resp, fmt.Errorf("securityCategoryCreate mutation errors: %v", result.Data.SecurityCategoryCreate.Errors)
	}
	if result.Data.SecurityCategoryCreate.SecurityCategory == nil {
		return nil, resp, ErrNotFound
	}

	return result.Data.SecurityCategoryCreate.SecurityCategory.unwrap(), resp, nil
}

// UpdateSecurityCategory updates an existing security category.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategoryupdate
func (s *SecurityCategoriesService) UpdateSecurityCategory(id int64, namespaceID int64, opt *UpdateSecurityCategoryOptions, options ...RequestOptionFunc) (*SecurityCategory, *Response, error) {
	categoryGID := gidGQL{Type: "Security::Category", Int64: id}
	namespaceGID := gidGQL{Type: "Namespace", Int64: namespaceID}

	input := map[string]any{
		"id":          categoryGID.String(),
		"namespaceId": namespaceGID.String(),
	}
	if opt != nil {
		if opt.Name != nil {
			input["name"] = opt.Name
		}
		if opt.Description != nil {
			input["description"] = opt.Description
		}
	}

	mutation := GraphQLQuery{
		Query: `
			mutation UpdateSecurityCategory($input: SecurityCategoryUpdateInput!) {
				securityCategoryUpdate(input: $input) {
					securityCategory {
						id
						name
						description
						multipleSelection
						editableState
						templateType
						securityAttributes {
							id
							name
							color
							description
							editableState
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
			SecurityCategoryUpdate struct {
				SecurityCategory *securityCategoryGQL `json:"securityCategory"`
				Errors           []string             `json:"errors"`
			} `json:"securityCategoryUpdate"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if len(result.Data.SecurityCategoryUpdate.Errors) > 0 {
		return nil, resp, fmt.Errorf("securityCategoryUpdate mutation errors: %v", result.Data.SecurityCategoryUpdate.Errors)
	}
	if result.Data.SecurityCategoryUpdate.SecurityCategory == nil {
		return nil, resp, ErrNotFound
	}

	return result.Data.SecurityCategoryUpdate.SecurityCategory.unwrap(), resp, nil
}

// DestroySecurityCategory deletes a security category and all its associated
// security attributes.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategorydestroy
func (s *SecurityCategoriesService) DestroySecurityCategory(id int64, options ...RequestOptionFunc) (*Response, error) {
	categoryGID := gidGQL{Type: "Security::Category", Int64: id}

	mutation := GraphQLQuery{
		Query: `
			mutation DestroySecurityCategory($input: SecurityCategoryDestroyInput!) {
				securityCategoryDestroy(input: $input) {
					errors
				}
			}
		`,
		Variables: map[string]any{
			"input": map[string]any{
				"id": categoryGID.String(),
			},
		},
	}

	var result struct {
		Data struct {
			SecurityCategoryDestroy struct {
				Errors []string `json:"errors"`
			} `json:"securityCategoryDestroy"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return resp, err
	}
	if len(result.Data.SecurityCategoryDestroy.Errors) > 0 {
		return resp, fmt.Errorf("securityCategoryDestroy mutation errors: %v", result.Data.SecurityCategoryDestroy.Errors)
	}

	return resp, nil
}
