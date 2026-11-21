// EXPERIMENTAL(#2257): The Saved Views API is a work in progress and may introduce breaking changes even between minor versions.

package gitlab

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type (
	// WorkItemSavedViewsServiceInterface defines the interface for saved views.
	//
	// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#savedview
	//
	// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
	WorkItemSavedViewsServiceInterface interface {
		GetWorkItemSavedView(
			namespacePath string,
			id int64,
			options ...RequestOptionFunc,
		) (*WorkItemSavedView, *Response, error)
		ListWorkItemSavedViews(
			namespacePath string,
			opt *ListWorkItemSavedViewsOptions,
			options ...RequestOptionFunc,
		) ([]*WorkItemSavedView, *Response, error)
		CreateWorkItemSavedView(
			namespacePath string,
			opt *CreateWorkItemSavedViewOptions,
			options ...RequestOptionFunc,
		) (*WorkItemSavedView, *Response, error)
		UpdateWorkItemSavedView(
			id int64,
			opt *UpdateWorkItemSavedViewOptions,
			options ...RequestOptionFunc,
		) (*WorkItemSavedView, *Response, error)
		DeleteWorkItemSavedView(id int64, options ...RequestOptionFunc) (*Response, error)
		SubscribeWorkItemSavedView(
			id int64,
			options ...RequestOptionFunc,
		) (*WorkItemSavedView, *Response, error)
		UnsubscribeWorkItemSavedView(
			id int64,
			options ...RequestOptionFunc,
		) (*WorkItemSavedView, *Response, error)
	}

	// WorkItemSavedViewsService handles communication with the work item saved views
	// related methods of the GitLab API.
	//
	// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#savedview
	//
	// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
	WorkItemSavedViewsService struct {
		client *Client
	}
)

var _ WorkItemSavedViewsServiceInterface = (*WorkItemSavedViewsService)(nil)

// GraphQL query/mutation bodies for the Work Item Saved Views service.
// Extracted to consts so tests can assert the exact request body sent to the
// server, following the pattern established in security_scan_profiles.go.
const (
	getSavedViewQuery = `
			query($fullPath: ID!, $id: WorkItemsSavedViewsSavedViewID!) {
				namespace(fullPath: $fullPath) {
					savedViews(id: $id) {
						nodes {
							id
							name
							description
							isPrivate
							subscribed
							filters
							sort
							displaySettings
						}
					}
				}
			}
		`

	listSavedViewsQuery = `
			query($fullPath: ID!, $first: Int, $after: String, $last: Int, $before: String) {
				namespace(fullPath: $fullPath) {
					savedViews(first: $first, after: $after, last: $last, before: $before) {
						nodes {
							id
							name
							description
							isPrivate
							subscribed
							sort
							displaySettings
						}
						pageInfo {
							endCursor
							hasNextPage
							startCursor
							hasPreviousPage
						}
					}
				}
			}
		`

	createSavedViewMutation = `
			mutation($input: WorkItemSavedViewCreateInput!) {
				workItemSavedViewCreate(input: $input) {
					savedView {
						id
						name
						description
						isPrivate
						subscribed
						filters
						sort
						displaySettings
					}
					errors
				}
			}
		`

	updateSavedViewMutation = `
			mutation($input: WorkItemSavedViewUpdateInput!) {
				workItemSavedViewUpdate(input: $input) {
					savedView {
						id
						name
						description
						isPrivate
						subscribed
						filters
						sort
						displaySettings
					}
					errors
				}
			}
		`

	deleteSavedViewMutation = `
			mutation($input: WorkItemSavedViewDeleteInput!) {
				workItemSavedViewDelete(input: $input) {
					errors
				}
			}
		`

	subscribeSavedViewMutation = `
			mutation($input: WorkItemSavedViewSubscribeInput!) {
				workItemSavedViewSubscribe(input: $input) {
					savedView {
						id
						name
						description
						isPrivate
						subscribed
						filters
						sort
						displaySettings
					}
					errors
				}
			}
		`

	unsubscribeSavedViewMutation = `
			mutation($input: WorkItemSavedViewUnsubscribeInput!) {
				workItemSavedViewUnsubscribe(input: $input) {
					savedView {
						id
						name
						description
						isPrivate
						subscribed
						filters
						sort
						displaySettings
					}
					errors
				}
			}
		`
)

// WorkItemSavedView represents a Work Item Saved View.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#savedview
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
// Filters and DisplaySettings are exposed as json.RawMessage because the
// GraphQL schema declares both fields as an opaque JSON scalar on the
// response type (unlike the mutation inputs, where Filters is a typed
// WorkItemSavedViewFilterInput object; see CreateWorkItemSavedViewOptions).
type WorkItemSavedView struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPrivate   bool   `json:"isPrivate"`
	Subscribed  bool   `json:"subscribed"`

	// Filters is always nil from ListWorkItemSavedViews; see that method's doc.
	Filters         json.RawMessage `json:"filters"`
	Sort            string          `json:"sort"`
	DisplaySettings json.RawMessage `json:"displaySettings"`
}

// GID returns the global ID of the saved view, for example
// gid://gitlab/WorkItems::SavedViews::SavedView/1.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#savedview
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
func (v WorkItemSavedView) GID() string {
	return gidGQL{
		Type:  "WorkItems::SavedViews::SavedView",
		Int64: v.ID,
	}.String()
}

// workItemSavedViewGQL is used to unmarshal GraphQL responses where the ID is a
// global ID string of the form gid://gitlab/WorkItems::SavedViews::SavedView/<n>.
type workItemSavedViewGQL struct {
	ID              gidGQL          `json:"id"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	IsPrivate       bool            `json:"isPrivate"`
	Subscribed      bool            `json:"subscribed"`
	Filters         json.RawMessage `json:"filters"`
	Sort            string          `json:"sort"`
	DisplaySettings json.RawMessage `json:"displaySettings"`
}

func (v *workItemSavedViewGQL) unwrap() *WorkItemSavedView {
	if v == nil {
		return nil
	}
	return &WorkItemSavedView{
		ID:              v.ID.Int64,
		Name:            v.Name,
		Description:     v.Description,
		IsPrivate:       v.IsPrivate,
		Subscribed:      v.Subscribed,
		Filters:         v.Filters,
		Sort:            v.Sort,
		DisplaySettings: v.DisplaySettings,
	}
}

// WorkItemSavedViewFilters represents the filters that can be associated
// with a Work Item Saved View. All fields are optional; the zero value
// represents no filters.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemsavedviewfilterinput
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
type WorkItemSavedViewFilters struct {
	AssigneeUsernames          []string
	AssigneeWildcardID         *string
	AuthorUsername             *string
	Confidential               *bool
	CRMContactID               *string
	CRMOrganizationID          *string
	CustomField                []WorkItemCustomFieldFilter
	ExcludeGroupWorkItems      *bool
	ExcludeProjects            *bool
	FullPath                   *string
	HealthStatusFilter         *string
	HierarchyFilters           *WorkItemHierarchyFilter
	IID                        *string
	In                         []string
	IncludeDescendantWorkItems *bool
	IncludeDescendants         *bool
	IterationCadenceID         []string
	IterationID                []string
	IterationWildcardID        *string
	LabelName                  []string
	MilestoneTitle             []string
	MilestoneWildcardID        *string
	MyReactionEmoji            *string
	Not                        *WorkItemSavedViewNegatedFilters
	Or                         *WorkItemSavedViewUnionedFilters
	ReleaseTag                 []string
	ReleaseTagWildcardID       *string
	Search                     *string
	State                      *string
	Status                     *WorkItemStatusFilter
	Subscribed                 *string
	Types                      []string
	Weight                     *string
	WeightWildcardID           *string
	WorkItemTypeIDs            []string

	// Time filters
	ClosedAfter   *time.Time
	ClosedBefore  *time.Time
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	DueAfter      *time.Time
	DueBefore     *time.Time
	UpdatedAfter  *time.Time
	UpdatedBefore *time.Time
}

// WorkItemSavedViewNegatedFilters represents the "not" sub-filter of
// WorkItemSavedViewFilters: work items matching these values are excluded.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemsavedviewnegatedfilterinput
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
type WorkItemSavedViewNegatedFilters struct {
	AssigneeUsernames   []string
	AuthorUsername      []string
	CustomField         []WorkItemCustomFieldFilter
	HealthStatusFilter  []string
	IterationID         []string
	IterationWildcardID *string
	LabelName           []string
	MilestoneTitle      []string
	MilestoneWildcardID *string
	MyReactionEmoji     *string
	ParentIDs           []string
	ReleaseTag          []string
	Types               []string
	Weight              *string
	WorkItemTypeIDs     []string
}

// WorkItemSavedViewUnionedFilters represents the "or" sub-filter of
// WorkItemSavedViewFilters: work items matching any of these values are included.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemsavedviewunionedfilterinput
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
type WorkItemSavedViewUnionedFilters struct {
	AssigneeUsernames []string
	AuthorUsernames   []string
	CustomField       []WorkItemCustomFieldFilter
	LabelNames        []string
}

// WorkItemHierarchyFilter filters work items by their position in the work
// item hierarchy.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#hierarchyfilterinput
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
type WorkItemHierarchyFilter struct {
	ParentIDs                  []string
	IncludeDescendantWorkItems *bool
	ParentWildcardID           *string
}

// WorkItemStatusFilter filters work items by their status widget value.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemwidgetstatusfilterinput
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
type WorkItemStatusFilter struct {
	ID   *string
	Name *string
}

// WorkItemCustomFieldFilter filters work items by a custom field value.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemwidgetcustomfieldfilterinputtype
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
type WorkItemCustomFieldFilter struct {
	CustomFieldID        *string
	CustomFieldName      *string
	SelectedOptionIDs    []string
	SelectedOptionValues []string
}

// workItemSavedViewFilterInputGQL is the GraphQL wire representation of
// WorkItemSavedViewFilterInput.
type workItemSavedViewFilterInputGQL struct {
	AssigneeUsernames          []string                                `json:"assigneeUsernames,omitempty"`
	AssigneeWildcardID         *string                                 `json:"assigneeWildcardId,omitempty"`
	AuthorUsername             *string                                 `json:"authorUsername,omitempty"`
	ClosedAfter                *time.Time                              `json:"closedAfter,omitempty"`
	ClosedBefore               *time.Time                              `json:"closedBefore,omitempty"`
	Confidential               *bool                                   `json:"confidential,omitempty"`
	CreatedAfter               *time.Time                              `json:"createdAfter,omitempty"`
	CreatedBefore              *time.Time                              `json:"createdBefore,omitempty"`
	CRMContactID               *string                                 `json:"crmContactId,omitempty"`
	CRMOrganizationID          *string                                 `json:"crmOrganizationId,omitempty"`
	CustomField                []*workItemCustomFieldFilterInputGQL    `json:"customField,omitempty"`
	DueAfter                   *time.Time                              `json:"dueAfter,omitempty"`
	DueBefore                  *time.Time                              `json:"dueBefore,omitempty"`
	ExcludeGroupWorkItems      *bool                                   `json:"excludeGroupWorkItems,omitempty"`
	ExcludeProjects            *bool                                   `json:"excludeProjects,omitempty"`
	FullPath                   *string                                 `json:"fullPath,omitempty"`
	HealthStatusFilter         *string                                 `json:"healthStatusFilter,omitempty"`
	HierarchyFilters           *workItemHierarchyFilterInputGQL        `json:"hierarchyFilters,omitempty"`
	IID                        *string                                 `json:"iid,omitempty"`
	In                         []string                                `json:"in,omitempty"`
	IncludeDescendantWorkItems *bool                                   `json:"includeDescendantWorkItems,omitempty"`
	IncludeDescendants         *bool                                   `json:"includeDescendants,omitempty"`
	IterationCadenceID         []string                                `json:"iterationCadenceId,omitempty"`
	IterationID                []string                                `json:"iterationId,omitempty"`
	IterationWildcardID        *string                                 `json:"iterationWildcardId,omitempty"`
	LabelName                  []string                                `json:"labelName,omitempty"`
	MilestoneTitle             []string                                `json:"milestoneTitle,omitempty"`
	MilestoneWildcardID        *string                                 `json:"milestoneWildcardId,omitempty"`
	MyReactionEmoji            *string                                 `json:"myReactionEmoji,omitempty"`
	Not                        *workItemSavedViewNegatedFilterInputGQL `json:"not,omitempty"`
	Or                         *workItemSavedViewUnionedFilterInputGQL `json:"or,omitempty"`
	ReleaseTag                 []string                                `json:"releaseTag,omitempty"`
	ReleaseTagWildcardID       *string                                 `json:"releaseTagWildcardId,omitempty"`
	Search                     *string                                 `json:"search,omitempty"`
	State                      *string                                 `json:"state,omitempty"`
	Status                     *workItemStatusFilterInputGQL           `json:"status,omitempty"`
	Subscribed                 *string                                 `json:"subscribed,omitempty"`
	Types                      []string                                `json:"types,omitempty"`
	UpdatedAfter               *time.Time                              `json:"updatedAfter,omitempty"`
	UpdatedBefore              *time.Time                              `json:"updatedBefore,omitempty"`
	Weight                     *string                                 `json:"weight,omitempty"`
	WeightWildcardID           *string                                 `json:"weightWildcardId,omitempty"`
	WorkItemTypeIDs            []string                                `json:"workItemTypeIds,omitempty"`
}

// wrap converts f into its GraphQL wire representation. A nil receiver
// converts to nil, so callers can wrap an optional *WorkItemSavedViewFilters
// field directly without a separate nil check.
func (f *WorkItemSavedViewFilters) wrap() *workItemSavedViewFilterInputGQL {
	if f == nil {
		return nil
	}
	return &workItemSavedViewFilterInputGQL{
		AssigneeUsernames:          f.AssigneeUsernames,
		AssigneeWildcardID:         f.AssigneeWildcardID,
		AuthorUsername:             f.AuthorUsername,
		ClosedAfter:                f.ClosedAfter,
		ClosedBefore:               f.ClosedBefore,
		Confidential:               f.Confidential,
		CreatedAfter:               f.CreatedAfter,
		CreatedBefore:              f.CreatedBefore,
		CRMContactID:               f.CRMContactID,
		CRMOrganizationID:          f.CRMOrganizationID,
		CustomField:                wrapWorkItemCustomFieldFilters(f.CustomField),
		DueAfter:                   f.DueAfter,
		DueBefore:                  f.DueBefore,
		ExcludeGroupWorkItems:      f.ExcludeGroupWorkItems,
		ExcludeProjects:            f.ExcludeProjects,
		FullPath:                   f.FullPath,
		HealthStatusFilter:         f.HealthStatusFilter,
		HierarchyFilters:           f.HierarchyFilters.wrap(),
		IID:                        f.IID,
		In:                         f.In,
		IncludeDescendantWorkItems: f.IncludeDescendantWorkItems,
		IncludeDescendants:         f.IncludeDescendants,
		IterationCadenceID:         f.IterationCadenceID,
		IterationID:                f.IterationID,
		IterationWildcardID:        f.IterationWildcardID,
		LabelName:                  f.LabelName,
		MilestoneTitle:             f.MilestoneTitle,
		MilestoneWildcardID:        f.MilestoneWildcardID,
		MyReactionEmoji:            f.MyReactionEmoji,
		Not:                        f.Not.wrap(),
		Or:                         f.Or.wrap(),
		ReleaseTag:                 f.ReleaseTag,
		ReleaseTagWildcardID:       f.ReleaseTagWildcardID,
		Search:                     f.Search,
		State:                      f.State,
		Status:                     f.Status.wrap(),
		Subscribed:                 f.Subscribed,
		Types:                      f.Types,
		UpdatedAfter:               f.UpdatedAfter,
		UpdatedBefore:              f.UpdatedBefore,
		Weight:                     f.Weight,
		WeightWildcardID:           f.WeightWildcardID,
		WorkItemTypeIDs:            f.WorkItemTypeIDs,
	}
}

// workItemSavedViewNegatedFilterInputGQL is the GraphQL wire representation
// of WorkItemSavedViewNegatedFilterInput.
type workItemSavedViewNegatedFilterInputGQL struct {
	AssigneeUsernames   []string                             `json:"assigneeUsernames,omitempty"`
	AuthorUsername      []string                             `json:"authorUsername,omitempty"`
	CustomField         []*workItemCustomFieldFilterInputGQL `json:"customField,omitempty"`
	HealthStatusFilter  []string                             `json:"healthStatusFilter,omitempty"`
	IterationID         []string                             `json:"iterationId,omitempty"`
	IterationWildcardID *string                              `json:"iterationWildcardId,omitempty"`
	LabelName           []string                             `json:"labelName,omitempty"`
	MilestoneTitle      []string                             `json:"milestoneTitle,omitempty"`
	MilestoneWildcardID *string                              `json:"milestoneWildcardId,omitempty"`
	MyReactionEmoji     *string                              `json:"myReactionEmoji,omitempty"`
	ParentIDs           []string                             `json:"parentIds,omitempty"`
	ReleaseTag          []string                             `json:"releaseTag,omitempty"`
	Types               []string                             `json:"types,omitempty"`
	Weight              *string                              `json:"weight,omitempty"`
	WorkItemTypeIDs     []string                             `json:"workItemTypeIds,omitempty"`
}

func (f *WorkItemSavedViewNegatedFilters) wrap() *workItemSavedViewNegatedFilterInputGQL {
	if f == nil {
		return nil
	}
	return &workItemSavedViewNegatedFilterInputGQL{
		AssigneeUsernames:   f.AssigneeUsernames,
		AuthorUsername:      f.AuthorUsername,
		CustomField:         wrapWorkItemCustomFieldFilters(f.CustomField),
		HealthStatusFilter:  f.HealthStatusFilter,
		IterationID:         f.IterationID,
		IterationWildcardID: f.IterationWildcardID,
		LabelName:           f.LabelName,
		MilestoneTitle:      f.MilestoneTitle,
		MilestoneWildcardID: f.MilestoneWildcardID,
		MyReactionEmoji:     f.MyReactionEmoji,
		ParentIDs:           f.ParentIDs,
		ReleaseTag:          f.ReleaseTag,
		Types:               f.Types,
		Weight:              f.Weight,
		WorkItemTypeIDs:     f.WorkItemTypeIDs,
	}
}

// workItemSavedViewUnionedFilterInputGQL is the GraphQL wire representation
// of WorkItemSavedViewUnionedFilterInput.
type workItemSavedViewUnionedFilterInputGQL struct {
	AssigneeUsernames []string                             `json:"assigneeUsernames,omitempty"`
	AuthorUsernames   []string                             `json:"authorUsernames,omitempty"`
	CustomField       []*workItemCustomFieldFilterInputGQL `json:"customField,omitempty"`
	LabelNames        []string                             `json:"labelNames,omitempty"`
}

func (f *WorkItemSavedViewUnionedFilters) wrap() *workItemSavedViewUnionedFilterInputGQL {
	if f == nil {
		return nil
	}
	return &workItemSavedViewUnionedFilterInputGQL{
		AssigneeUsernames: f.AssigneeUsernames,
		AuthorUsernames:   f.AuthorUsernames,
		CustomField:       wrapWorkItemCustomFieldFilters(f.CustomField),
		LabelNames:        f.LabelNames,
	}
}

// workItemHierarchyFilterInputGQL is the GraphQL wire representation of
// HierarchyFilterInput.
type workItemHierarchyFilterInputGQL struct {
	ParentIDs                  []string `json:"parentIds,omitempty"`
	IncludeDescendantWorkItems *bool    `json:"includeDescendantWorkItems,omitempty"`
	ParentWildcardID           *string  `json:"parentWildcardId,omitempty"`
}

func (f *WorkItemHierarchyFilter) wrap() *workItemHierarchyFilterInputGQL {
	if f == nil {
		return nil
	}
	return &workItemHierarchyFilterInputGQL{
		ParentIDs:                  f.ParentIDs,
		IncludeDescendantWorkItems: f.IncludeDescendantWorkItems,
		ParentWildcardID:           f.ParentWildcardID,
	}
}

// workItemStatusFilterInputGQL is the GraphQL wire representation of
// WorkItemWidgetStatusFilterInput.
type workItemStatusFilterInputGQL struct {
	ID   *string `json:"id,omitempty"`
	Name *string `json:"name,omitempty"`
}

func (f *WorkItemStatusFilter) wrap() *workItemStatusFilterInputGQL {
	if f == nil {
		return nil
	}
	return &workItemStatusFilterInputGQL{ID: f.ID, Name: f.Name}
}

// workItemCustomFieldFilterInputGQL is the GraphQL wire representation of
// WorkItemWidgetCustomFieldFilterInputType.
type workItemCustomFieldFilterInputGQL struct {
	CustomFieldID        *string  `json:"customFieldId,omitempty"`
	CustomFieldName      *string  `json:"customFieldName,omitempty"`
	SelectedOptionIDs    []string `json:"selectedOptionIds,omitempty"`
	SelectedOptionValues []string `json:"selectedOptionValues,omitempty"`
}

func wrapWorkItemCustomFieldFilters(
	in []WorkItemCustomFieldFilter,
) []*workItemCustomFieldFilterInputGQL {
	if in == nil {
		return nil
	}
	out := make([]*workItemCustomFieldFilterInputGQL, 0, len(in))
	for _, f := range in {
		out = append(out, &workItemCustomFieldFilterInputGQL{
			CustomFieldID:        f.CustomFieldID,
			CustomFieldName:      f.CustomFieldName,
			SelectedOptionIDs:    f.SelectedOptionIDs,
			SelectedOptionValues: f.SelectedOptionValues,
		})
	}
	return out
}

// CreateWorkItemSavedViewOptions represents options for creating a saved view.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationworkitemsavedviewcreate
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
type CreateWorkItemSavedViewOptions struct {
	// Name of the saved view. Required.
	Name string

	// Description of the saved view.
	Description *string

	// IsPrivate sets whether the saved view is private to the creating user. Default: true.
	IsPrivate *bool

	// Filters applied by the saved view. Required by the GraphQL server;
	// use the zero value for no filters.
	//
	// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemsavedviewfilterinput
	Filters WorkItemSavedViewFilters

	// Sort order applied by the saved view. Required. Must be one of the
	// WorkItemSort enum values, for example TITLE_ASC, TITLE_DESC, CREATED_ASC,
	// CREATED_DESC, UPDATED_ASC, or UPDATED_DESC.
	//
	// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemsort
	Sort string

	// DisplaySettings for the saved view. Required. Shape is defined by the
	// consuming UI (the GraphQL schema declares it as an opaque JSON scalar,
	// matching the input type used by the user preferences update mutation).
	// Pass json.RawMessage(`{}`) if there is nothing to store.
	DisplaySettings json.RawMessage
}

// workItemSavedViewCreateInputGQL represents the GraphQL input structure for creating a saved view.
type workItemSavedViewCreateInputGQL struct {
	// Required
	NamespacePath   string                           `json:"namespacePath"`
	Name            string                           `json:"name"`
	Filters         *workItemSavedViewFilterInputGQL `json:"filters"`
	Sort            string                           `json:"sort"`
	DisplaySettings json.RawMessage                  `json:"displaySettings"`

	// Optional
	Description *string `json:"description,omitempty"`
	IsPrivate   *bool   `json:"isPrivate,omitempty"`
}

// wrap converts CreateWorkItemSavedViewOptions into the GraphQL-facing input structure.
// opt must be non-nil; callers are expected to check this before calling wrap.
func (opt *CreateWorkItemSavedViewOptions) wrap(
	namespacePath string,
) *workItemSavedViewCreateInputGQL {
	input := &workItemSavedViewCreateInputGQL{NamespacePath: namespacePath}

	input.Name = opt.Name
	input.Description = opt.Description
	input.IsPrivate = opt.IsPrivate
	input.Filters = opt.Filters.wrap()
	input.Sort = opt.Sort
	input.DisplaySettings = opt.DisplaySettings

	return input
}

// UpdateWorkItemSavedViewOptions represents options for updating a saved view.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationworkitemsavedviewupdate
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
type UpdateWorkItemSavedViewOptions struct {
	// Name of the saved view.
	Name *string

	// Description of the saved view.
	Description *string

	// IsPrivate sets whether the saved view is private to the creating user.
	IsPrivate *bool

	// Filters applied by the saved view. Nil leaves the existing filters
	// unchanged.
	//
	// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemsavedviewfilterinput
	Filters *WorkItemSavedViewFilters

	// Sort order applied by the saved view.
	Sort *string

	// DisplaySettings for the saved view. Shape is defined by the consuming UI.
	DisplaySettings json.RawMessage
}

// workItemSavedViewUpdateInputGQL represents the GraphQL input structure for updating a saved view.
type workItemSavedViewUpdateInputGQL struct {
	ID              string                           `json:"id"`
	Name            *string                          `json:"name,omitempty"`
	Description     *string                          `json:"description,omitempty"`
	IsPrivate       *bool                            `json:"isPrivate,omitempty"`
	Filters         *workItemSavedViewFilterInputGQL `json:"filters,omitempty"`
	Sort            *string                          `json:"sort,omitempty"`
	DisplaySettings json.RawMessage                  `json:"displaySettings,omitempty"`
}

// wrap converts UpdateWorkItemSavedViewOptions into the GraphQL-facing input structure.
// opt must be non-nil; callers are expected to check this before calling wrap.
func (opt *UpdateWorkItemSavedViewOptions) wrap(id string) *workItemSavedViewUpdateInputGQL {
	input := &workItemSavedViewUpdateInputGQL{ID: id}

	input.Name = opt.Name
	input.Description = opt.Description
	input.IsPrivate = opt.IsPrivate
	input.Filters = opt.Filters.wrap()
	input.Sort = opt.Sort
	input.DisplaySettings = opt.DisplaySettings

	return input
}

// GetWorkItemSavedView returns a single saved view by ID.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#namespacesavedviews
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
func (s *WorkItemSavedViewsService) GetWorkItemSavedView(
	namespacePath string,
	id int64,
	options ...RequestOptionFunc,
) (*WorkItemSavedView, *Response, error) {
	if namespacePath == "" {
		return nil, nil, errors.New("namespacePath is required")
	}

	gid := gidGQL{Type: "WorkItems::SavedViews::SavedView", Int64: id}

	query := GraphQLQuery{
		Query: getSavedViewQuery,
		Variables: map[string]any{
			"fullPath": namespacePath,
			"id":       gid.String(),
		},
	}

	var result struct {
		Data struct {
			Namespace *struct {
				SavedViews struct {
					Nodes []*workItemSavedViewGQL `json:"nodes"`
				} `json:"savedViews"`
			} `json:"namespace"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(query, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if len(result.Errors) != 0 {
		return nil, resp, &GraphQLResponseError{
			Err:    errors.New("GraphQL query failed"),
			Errors: result.GenericGraphQLErrors,
		}
	}
	if result.Data.Namespace == nil {
		return nil, resp, ErrNotFound
	}
	if len(result.Data.Namespace.SavedViews.Nodes) == 0 ||
		result.Data.Namespace.SavedViews.Nodes[0] == nil {
		return nil, resp, ErrNotFound
	}

	return result.Data.Namespace.SavedViews.Nodes[0].unwrap(), resp, nil
}

// ListWorkItemSavedViewsOptions represents pagination options for ListWorkItemSavedViews.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#namespacesavedviews
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
type ListWorkItemSavedViewsOptions struct {
	// After returns saved views after this cursor. Pass the previous
	// response's Response.PageInfo.EndCursor to fetch the next page.
	After *string

	// Before returns saved views before this cursor. Pass the previous
	// response's Response.PageInfo.StartCursor to fetch the previous page.
	Before *string

	// First limits how many saved views are returned. Defaults to 100
	// when neither First nor Last is set.
	First *int64

	// Last returns the last N saved views before Before, for paging
	// backward. Ignored if First is also set.
	Last *int64
}

// ListWorkItemSavedViews lists saved views available under a namespace.
//
// Returns up to 100 saved views per page by default. Set
// ListWorkItemSavedViewsOptions.First and check Response.PageInfo.HasNextPage to
// page through more, passing Response.PageInfo.EndCursor as After on the
// next call.
//
// Filters is always empty on the returned views. GitLab's schema only
// allows that field to be resolved once per GraphQL request, so requesting
// it for every node in the list would error as soon as more than one saved
// view exists. Use GetWorkItemSavedView to fetch a single view's Filters.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#namespacesavedviews
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
func (s *WorkItemSavedViewsService) ListWorkItemSavedViews(
	namespacePath string,
	opt *ListWorkItemSavedViewsOptions,
	options ...RequestOptionFunc,
) ([]*WorkItemSavedView, *Response, error) {
	if namespacePath == "" {
		return nil, nil, errors.New("namespacePath is required")
	}

	vars := map[string]any{
		"fullPath": namespacePath,
	}
	if opt != nil {
		if opt.After != nil {
			vars["after"] = opt.After
		}
		if opt.Before != nil {
			vars["before"] = opt.Before
		}
		if opt.First != nil {
			vars["first"] = opt.First
		} else if opt.Last != nil {
			vars["last"] = opt.Last
		}
	}
	if opt == nil || (opt.First == nil && opt.Last == nil) {
		vars["first"] = new(int64(100))
	}

	query := GraphQLQuery{
		Query:     listSavedViewsQuery,
		Variables: vars,
	}

	var result struct {
		Data struct {
			Namespace *struct {
				SavedViews connectionGQL[*workItemSavedViewGQL] `json:"savedViews"`
			} `json:"namespace"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(query, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if len(result.Errors) != 0 {
		return nil, resp, &GraphQLResponseError{
			Err:    errors.New("GraphQL query failed"),
			Errors: result.GenericGraphQLErrors,
		}
	}
	if result.Data.Namespace == nil {
		return nil, resp, ErrNotFound
	}

	views := make([]*WorkItemSavedView, 0, len(result.Data.Namespace.SavedViews.Nodes))
	for _, v := range result.Data.Namespace.SavedViews.Nodes {
		if v == nil {
			continue
		}
		views = append(views, v.unwrap())
	}

	resp.PageInfo = &result.Data.Namespace.SavedViews.PageInfo

	return views, resp, nil
}

// CreateWorkItemSavedView creates a new Work Item Saved View.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationworkitemsavedviewcreate
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
func (s *WorkItemSavedViewsService) CreateWorkItemSavedView(
	namespacePath string,
	opt *CreateWorkItemSavedViewOptions,
	options ...RequestOptionFunc,
) (*WorkItemSavedView, *Response, error) {
	if namespacePath == "" {
		return nil, nil, errors.New("namespacePath is required")
	}
	if opt == nil {
		return nil, nil, errors.New("opt is required")
	}
	if opt.Name == "" {
		return nil, nil, errors.New("opt.Name is required")
	}
	if opt.Sort == "" {
		return nil, nil, errors.New("opt.Sort is required")
	}
	if len(opt.DisplaySettings) == 0 {
		return nil, nil, errors.New("opt.DisplaySettings is required")
	}

	query := GraphQLQuery{
		Query: createSavedViewMutation,
		Variables: map[string]any{
			"input": opt.wrap(namespacePath),
		},
	}

	var result struct {
		Data struct {
			WorkItemSavedViewCreate *struct {
				SavedView *workItemSavedViewGQL `json:"savedView"`
				Errors    []string              `json:"errors"`
			} `json:"workItemSavedViewCreate"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(query, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if len(result.Errors) != 0 {
		return nil, resp, &GraphQLResponseError{
			Err:    errors.New("mutation.workItemSavedViewCreate failed"),
			Errors: result.GenericGraphQLErrors,
		}
	}
	if result.Data.WorkItemSavedViewCreate == nil {
		return nil, resp, ErrEmptyResponse
	}
	if len(result.Data.WorkItemSavedViewCreate.Errors) > 0 {
		return nil, resp, errors.New(strings.Join(result.Data.WorkItemSavedViewCreate.Errors, "; "))
	}
	if result.Data.WorkItemSavedViewCreate.SavedView == nil {
		return nil, resp, ErrEmptyResponse
	}

	return result.Data.WorkItemSavedViewCreate.SavedView.unwrap(), resp, nil
}

// UpdateWorkItemSavedView updates an existing saved view.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationworkitemsavedviewupdate
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
func (s *WorkItemSavedViewsService) UpdateWorkItemSavedView(
	id int64,
	opt *UpdateWorkItemSavedViewOptions,
	options ...RequestOptionFunc,
) (*WorkItemSavedView, *Response, error) {
	if opt == nil {
		return nil, nil, errors.New("opt is required")
	}

	gid := gidGQL{Type: "WorkItems::SavedViews::SavedView", Int64: id}

	query := GraphQLQuery{
		Query: updateSavedViewMutation,
		Variables: map[string]any{
			"input": opt.wrap(gid.String()),
		},
	}

	var result struct {
		Data struct {
			WorkItemSavedViewUpdate *struct {
				SavedView *workItemSavedViewGQL `json:"savedView"`
				Errors    []string              `json:"errors"`
			} `json:"workItemSavedViewUpdate"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(query, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if len(result.Errors) != 0 {
		return nil, resp, &GraphQLResponseError{
			Err:    errors.New("mutation.workItemSavedViewUpdate failed"),
			Errors: result.GenericGraphQLErrors,
		}
	}
	if result.Data.WorkItemSavedViewUpdate == nil {
		return nil, resp, ErrEmptyResponse
	}
	if len(result.Data.WorkItemSavedViewUpdate.Errors) > 0 {
		return nil, resp, errors.New(strings.Join(result.Data.WorkItemSavedViewUpdate.Errors, "; "))
	}
	if result.Data.WorkItemSavedViewUpdate.SavedView == nil {
		return nil, resp, ErrEmptyResponse
	}

	return result.Data.WorkItemSavedViewUpdate.SavedView.unwrap(), resp, nil
}

// DeleteWorkItemSavedView deletes an existing saved view.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationworkitemsavedviewdelete
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
func (s *WorkItemSavedViewsService) DeleteWorkItemSavedView(
	id int64,
	options ...RequestOptionFunc,
) (*Response, error) {
	gid := gidGQL{Type: "WorkItems::SavedViews::SavedView", Int64: id}

	query := GraphQLQuery{
		Query: deleteSavedViewMutation,
		Variables: map[string]any{
			"input": map[string]any{
				"id": gid.String(),
			},
		},
	}

	var result struct {
		Data struct {
			WorkItemSavedViewDelete *struct {
				Errors []string `json:"errors"`
			} `json:"workItemSavedViewDelete"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(query, &result, options...)
	if err != nil {
		return resp, err
	}
	if len(result.Errors) != 0 {
		return resp, &GraphQLResponseError{
			Err:    errors.New("mutation.workItemSavedViewDelete failed"),
			Errors: result.GenericGraphQLErrors,
		}
	}
	if result.Data.WorkItemSavedViewDelete == nil {
		return resp, ErrEmptyResponse
	}
	if len(result.Data.WorkItemSavedViewDelete.Errors) > 0 {
		return resp, errors.New(strings.Join(result.Data.WorkItemSavedViewDelete.Errors, "; "))
	}

	return resp, nil
}

// SubscribeWorkItemSavedView subscribes the current user to a saved view.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationworkitemsavedviewsubscribe
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
func (s *WorkItemSavedViewsService) SubscribeWorkItemSavedView(
	id int64,
	options ...RequestOptionFunc,
) (*WorkItemSavedView, *Response, error) {
	gid := gidGQL{Type: "WorkItems::SavedViews::SavedView", Int64: id}

	query := GraphQLQuery{
		Query: subscribeSavedViewMutation,
		Variables: map[string]any{
			"input": map[string]any{
				"id": gid.String(),
			},
		},
	}

	var result struct {
		Data struct {
			WorkItemSavedViewSubscribe *struct {
				SavedView *workItemSavedViewGQL `json:"savedView"`
				Errors    []string              `json:"errors"`
			} `json:"workItemSavedViewSubscribe"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(query, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if len(result.Errors) != 0 {
		return nil, resp, &GraphQLResponseError{
			Err:    errors.New("mutation.workItemSavedViewSubscribe failed"),
			Errors: result.GenericGraphQLErrors,
		}
	}
	if result.Data.WorkItemSavedViewSubscribe == nil {
		return nil, resp, ErrEmptyResponse
	}
	if len(result.Data.WorkItemSavedViewSubscribe.Errors) > 0 {
		return nil, resp, errors.New(
			strings.Join(result.Data.WorkItemSavedViewSubscribe.Errors, "; "),
		)
	}
	if result.Data.WorkItemSavedViewSubscribe.SavedView == nil {
		return nil, resp, ErrEmptyResponse
	}

	return result.Data.WorkItemSavedViewSubscribe.SavedView.unwrap(), resp, nil
}

// UnsubscribeWorkItemSavedView unsubscribes the current user from a saved view.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationworkitemsavedviewunsubscribe
//
// Experimental: The Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions.
func (s *WorkItemSavedViewsService) UnsubscribeWorkItemSavedView(
	id int64,
	options ...RequestOptionFunc,
) (*WorkItemSavedView, *Response, error) {
	gid := gidGQL{Type: "WorkItems::SavedViews::SavedView", Int64: id}

	query := GraphQLQuery{
		Query: unsubscribeSavedViewMutation,
		Variables: map[string]any{
			"input": map[string]any{
				"id": gid.String(),
			},
		},
	}

	var result struct {
		Data struct {
			WorkItemSavedViewUnsubscribe *struct {
				SavedView *workItemSavedViewGQL `json:"savedView"`
				Errors    []string              `json:"errors"`
			} `json:"workItemSavedViewUnsubscribe"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(query, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if len(result.Errors) != 0 {
		return nil, resp, &GraphQLResponseError{
			Err:    errors.New("mutation.workItemSavedViewUnsubscribe failed"),
			Errors: result.GenericGraphQLErrors,
		}
	}
	if result.Data.WorkItemSavedViewUnsubscribe == nil {
		return nil, resp, ErrEmptyResponse
	}
	if len(result.Data.WorkItemSavedViewUnsubscribe.Errors) > 0 {
		return nil, resp, errors.New(
			strings.Join(result.Data.WorkItemSavedViewUnsubscribe.Errors, "; "),
		)
	}
	if result.Data.WorkItemSavedViewUnsubscribe.SavedView == nil {
		return nil, resp, ErrEmptyResponse
	}

	return result.Data.WorkItemSavedViewUnsubscribe.SavedView.unwrap(), resp, nil
}
