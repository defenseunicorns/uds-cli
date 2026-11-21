// EXPERIMENTAL(#2213): The Work Items API is a work in progress and may introduce breaking changes even between minor versions.

package gitlab

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"gitlab.com/gitlab-org/api/client-go/v3/internal/graphqlfields"
)

type (
	WorkItemsServiceInterface interface {
		CreateWorkItem(fullPath string, workItemTypeID WorkItemTypeID, opt *CreateWorkItemOptions, options ...RequestOptionFunc) (*WorkItem, *Response, error)
		GetWorkItem(fullPath string, iid int64, options ...RequestOptionFunc) (*WorkItem, *Response, error)
		ListWorkItems(fullPath string, opt *ListWorkItemsOptions, options ...RequestOptionFunc) ([]*WorkItem, *Response, error)
		UpdateWorkItem(fullPath string, iid int64, opt *UpdateWorkItemOptions, options ...RequestOptionFunc) (*WorkItem, *Response, error)
		DeleteWorkItem(fullPath string, iid int64, options ...RequestOptionFunc) (*Response, error)
		ListWorkItemTypes(namespacePath string, opt *ListWorkItemTypesOptions, options ...RequestOptionFunc) ([]WorkItemType, *Response, error)
	}

	// WorkItemsService handles communication with the work item related methods
	// of the GitLab API.
	//
	// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitem
	//
	// Experimental: The Work Items API is a work in progress and may introduce breaking changes even between minor versions.
	WorkItemsService struct {
		client *Client
	}
)

var _ WorkItemsServiceInterface = (*WorkItemsService)(nil)

// WorkItem represents a GitLab work item.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitem
//
// Experimental: The Work Items API is a work in progress and may introduce breaking changes even between minor versions.
type WorkItem struct {
	ID          int64
	IID         int64
	Type        string
	State       string
	Status      *string
	Title       string
	Description string
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
	ClosedAt    *time.Time
	WebURL      string
	Author      *BasicUser
	Assignees   []*BasicUser

	Color        *string
	Confidential bool
	DueDate      *ISOTime
	HealthStatus *string
	IterationID  *int64
	Labels       []LabelDetails
	LinkedItems  []LinkedWorkItem
	MilestoneID  *int64
	Parent       *WorkItemIID
	Children     []WorkItemIID
	StartDate    *ISOTime
	Weight       *int64
}

func (wi WorkItem) GID() string {
	return gidGQL{
		Type:  "WorkItem",
		Int64: wi.ID,
	}.String()
}

// WorkItemIID identifies a work item by its namespace path and internal ID.
//
// Experimental: The Work Items API is a work in progress and may introduce breaking changes even between minor versions.
type WorkItemIID struct {
	NamespacePath string
	IID           int64
}

type workItemIIDGQL struct {
	IID       string `json:"iid"`
	Namespace struct {
		FullPath string `json:"fullPath"`
	} `json:"namespace"`
}

func (w *workItemIIDGQL) unwrap() *WorkItemIID {
	if w == nil {
		return nil
	}

	iid, err := strconv.ParseInt(w.IID, 10, 64)
	if err != nil {
		return nil
	}

	return &WorkItemIID{
		NamespacePath: w.Namespace.FullPath,
		IID:           iid,
	}
}

// LinkedWorkItem represents a linked work item with its relationship type.
//
// Experimental: The Work Items API is a work in progress and may introduce breaking changes even between minor versions.
type LinkedWorkItem struct {
	WorkItemIID

	// LinkType is the type of relationship between the work items.
	// Possible values: blocks, is_blocked_by, relates_to
	LinkType string
}

// WorkItemStateEvent represents a state change event for a work item.
type WorkItemStateEvent string

const (
	WorkItemStateEventClose  WorkItemStateEvent = "CLOSE"
	WorkItemStateEventReopen WorkItemStateEvent = "REOPEN"
)

const workItemContainer = "features"

// workItemCEFields are available on both Community and Enterprise instances.
var workItemCEFields = []graphqlfields.Field{
	{Name: "id", Fragment: "id"},
	{Name: "iid", Fragment: "iid"},
	{Name: "type", Fragment: `workItemType {
  name
}`},
	{Name: "state", Fragment: "state"},
	{Name: "title", Fragment: "title"},
	{Name: "description", Fragment: "description"},
	{Name: "confidential", Fragment: "confidential"},
	{Name: "author", Fragment: `author {
  {{ template "UserCoreBasic" }}
}`},
	{Name: "createdAt", Fragment: "createdAt"},
	{Name: "updatedAt", Fragment: "updatedAt"},
	{Name: "closedAt", Fragment: "closedAt"},
	{Name: "webUrl", Fragment: "webUrl"},

	{Name: "assignees", Container: workItemContainer, Fragment: `assignees {
	assignees {
		nodes {
			{{ template "UserCoreBasic" }}
		}
	}
}`},
	{Name: "hierarchy", Container: workItemContainer, Fragment: `hierarchy {
	hasParent
	parent {
		iid
		namespace {
			fullPath
		}
	}
	hasChildren
	children {
		nodes {
			iid
			namespace {
				fullPath
			}
		}
	}
}`},
	{Name: "labels", Container: workItemContainer, Fragment: `labels {
	labels {
		nodes {
			id
			title
			color
			description
			descriptionHtml
			textColor
		}
	}
}`},
	{Name: "linkedItems", Container: workItemContainer, Fragment: `linkedItems {
	linkedItems {
		nodes {
			workItem {
				iid
				namespace {
					fullPath
				}
			}
			linkType
		}
	}
}`},
	{Name: "milestone", Container: workItemContainer, Fragment: `milestone {
	milestone {
		id
	}
}`},
	{Name: "startAndDueDate", Container: workItemContainer, Fragment: `startAndDueDate {
	startDate
	dueDate
}`},
}

// workItemEEFields exist only in the Enterprise schema and error against CE.
var workItemEEFields = []graphqlfields.Field{
	{Name: "color", Container: workItemContainer, Fragment: `color {
	color
	textColor
}`},
	{Name: "healthStatus", Container: workItemContainer, Fragment: `healthStatus {
	healthStatus
}`},
	{Name: "iteration", Container: workItemContainer, Fragment: `iteration {
	iteration {
		id
	}
}`},
	{Name: "status", Container: workItemContainer, Fragment: `status {
	status {
		name
	}
}`},
	{Name: "weight", Container: workItemContainer, Fragment: `weight {
	weight
}`},
}

// workItemFields is the full registry accepted by ReturnedFields.
var workItemFields = append(append([]graphqlfields.Field{}, workItemCEFields...), workItemEEFields...)

// WorkItemDefaultListFields returns the CE-safe default field set used
// by ListWorkItems when ReturnedFields is unset. Append EE field names
// (only against EE instances) to opt in:
//
//	opts := &gitlab.ListWorkItemsOptions{
//	    ReturnedFields: append(gitlab.WorkItemDefaultListFields(), "weight", "healthStatus"),
//	}
//
// EXPERIMENTAL: may change between minor versions.
func WorkItemDefaultListFields() []string {
	out := make([]string, len(workItemCEFields))
	for i, f := range workItemCEFields {
		out[i] = f.Name
	}
	return out
}

func buildWorkItemFieldsFragment(fields []string) (string, error) {
	if len(fields) == 0 {
		fields = WorkItemDefaultListFields()
	}
	fragment, err := graphqlfields.BuildFragment(workItemFields, fields)
	if err != nil {
		return "", fmt.Errorf("work item field: %w", err)
	}
	return fragment, nil
}

// workItemTemplate is the static WorkItem fragment used by Get/Create/Update.
// ListWorkItems renders its own fragment via graphqlfields.
var workItemTemplate = template.Must(template.Must(userCoreBasicTemplate.Clone()).New("WorkItem").Parse(`
	id
	iid
	workItemType {
	  name
	}
	state
	title
	description
	confidential
	author {
	  {{ template "UserCoreBasic" }}
	}
	createdAt
	updatedAt
	closedAt
	webUrl
	features {
		assignees {
			assignees {
				nodes {
					{{ template "UserCoreBasic" }}
				}
			}
		}
		color {
			color
			textColor
		}
		healthStatus {
			healthStatus
		}
		hierarchy {
			hasParent
			parent {
				iid
				namespace {
					fullPath
				}
			}
			hasChildren
			children {
				nodes {
					iid
					namespace {
						fullPath
					}
				}
			}
		}
		iteration {
			iteration {
				id
			}
		}
		labels {
			labels {
				nodes {
					id
					title
					color
					description
					descriptionHtml
					textColor
				}
			}
		}
		linkedItems {
			linkedItems {
				nodes {
					workItem {
						iid
						namespace {
							fullPath
						}
					}
					linkType
				}
			}
		}
		milestone {
			milestone {
				id
			}
		}
		startAndDueDate {
			startDate
			dueDate
		}
		status {
			status {
				name
			}
		}
		weight {
			weight
		}
	}
`))

// getWorkItemTemplate is chained from workItemTemplate so it has access to both
// UserCoreBasic and WorkItem templates.
var getWorkItemTemplate = template.Must(template.Must(workItemTemplate.Clone()).New("GetWorkItem").Parse(`
	query GetWorkItem($fullPath: ID!, $iid: String!) {
		namespace(fullPath: $fullPath) {
			workItem(iid: $iid) {
				{{ template "WorkItem" }}
			}
		}
	}
`))

const (
	getWorkItemIDQuery = `
			query GetWorkItemID($fullPath: ID!, $iid: String!) {
				namespace(fullPath: $fullPath) {
					workItem(iid: $iid) {
						  id
					}
				}
			}
	`

	deleteWorkItemQuery = `
			mutation DeleteWorkItem($id: WorkItemID!) {
					workItemDelete(input: { id: $id }) {
							errors
					}
			}
	`
)

// GetWorkItem gets a single work item.
//
// fullPath is the full path to either a group or project.
// iid is the internal ID of the work item.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#namespaceworkitem
//
// Experimental: The Work Items API is a work in progress and may introduce breaking changes even between minor versions.
func (s *WorkItemsService) GetWorkItem(fullPath string, iid int64, options ...RequestOptionFunc) (*WorkItem, *Response, error) {
	var queryBuilder strings.Builder
	if err := getWorkItemTemplate.Execute(&queryBuilder, nil); err != nil {
		return nil, nil, err
	}

	q := GraphQLQuery{
		Query: queryBuilder.String(),
		Variables: map[string]any{
			"fullPath": fullPath,
			"iid":      strconv.FormatInt(iid, 10),
		},
	}

	var result struct {
		Data struct {
			Namespace struct {
				WorkItem *workItemGQL `json:"workItem"`
			} `json:"namespace"`
		}
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(q, &result, options...)
	if err != nil {
		return nil, resp, err
	}

	if len(result.Errors) != 0 {
		return nil, resp, &GraphQLResponseError{
			Err:    errors.New("GraphQL query failed"),
			Errors: result.GenericGraphQLErrors,
		}
	}

	wiQL := result.Data.Namespace.WorkItem
	if wiQL == nil {
		return nil, resp, ErrNotFound
	}

	return wiQL.unwrap(), resp, nil
}

// ListWorkItemsOptions represents the available ListWorkItems() options.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#namespaceworkitems
//
// Experimental: The Work Items API is a work in progress and may introduce breaking changes even between minor versions.
type ListWorkItemsOptions struct {
	AssigneeUsernames    []string
	AssigneeWildcardID   *string
	AuthorUsername       *string
	Confidential         *bool
	CRMContactID         *string
	CRMOrganizationID    *string
	HealthStatusFilter   *string
	IDs                  []string
	IIDs                 []string
	IncludeAncestors     *bool
	IncludeDescendants   *bool
	IterationCadenceID   []string
	IterationID          []string
	IterationWildcardID  *string
	LabelName            []string
	MilestoneTitle       []string
	MilestoneWildcardID  *string
	MyReactionEmoji      *string
	ParentIDs            []string
	ReleaseTag           []string
	ReleaseTagWildcardID *string
	State                *string
	Subscribed           *string
	Types                []string
	Weight               *string
	WeightWildcardID     *string

	// Time filters
	ClosedAfter   *time.Time
	ClosedBefore  *time.Time
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	DueAfter      *time.Time
	DueBefore     *time.Time
	UpdatedAfter  *time.Time
	UpdatedBefore *time.Time

	// Sorting
	Sort *string

	// Search
	Search *string
	In     []string

	// Pagination
	After  *string
	Before *string
	First  *int64
	Last   *int64

	// ReturnedFields selects which work-item fields the GraphQL query
	// requests. When nil or empty, WorkItemDefaultListFields is used
	// (CE-safe). Unknown names return an error before the request. EE-only
	// fields ("color", "healthStatus", "iteration", "status", "weight")
	// error against Community Edition instances.
	//
	// Fields not selected are zero-valued on the returned WorkItem.
	//
	// EXPERIMENTAL: may change between minor versions.
	ReturnedFields []string
}

const listWorkItemsQueryShell = `
    query ListWorkItems($fullPath: ID!{{ if .Decls }}, {{ .Decls }}{{ end }}) {
        namespace(fullPath: $fullPath) {
            workItems({{ if .Args }}{{ .Args }}{{ end }}) {
                nodes {
                    {{ template "WorkItem" }}
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

type listWorkItemsQueryData struct {
	Decls string
	Args  string
}

// listWorkItemsTemplateCache memoizes parsed templates by canonicalized
// field-set signature, keeping paginated callers from re-parsing per page.
var listWorkItemsTemplateCache sync.Map

func buildListWorkItemsTemplate(fields []string) (*template.Template, error) {
	if len(fields) == 0 {
		fields = WorkItemDefaultListFields()
	}

	sorted := slices.Clone(fields)
	slices.Sort(sorted)
	sig := strings.Join(sorted, "\x00")

	if cached, ok := listWorkItemsTemplateCache.Load(sig); ok {
		return cached.(*template.Template), nil
	}

	fragment, err := buildWorkItemFieldsFragment(fields)
	if err != nil {
		return nil, err
	}

	t, err := userCoreBasicTemplate.Clone()
	if err != nil {
		return nil, err
	}
	if _, err := t.New("WorkItem").Parse(fragment); err != nil {
		return nil, fmt.Errorf("parsing dynamic WorkItem fragment: %w", err)
	}
	if _, err := t.New("ListWorkItems").Parse(listWorkItemsQueryShell); err != nil {
		return nil, fmt.Errorf("parsing ListWorkItems shell: %w", err)
	}
	actual, _ := listWorkItemsTemplateCache.LoadOrStore(sig, t)
	return actual.(*template.Template), nil
}

type workItemQueryVar struct {
	name    string
	gqlType string
	value   any
}

func buildListWorkItemsQuery(fullPath string, opt *ListWorkItemsOptions) (string, map[string]any, error) {
	if opt == nil {
		opt = &ListWorkItemsOptions{}
	}

	var qvars []workItemQueryVar
	varsMap := make(map[string]any)

	if len(opt.AssigneeUsernames) > 0 {
		qvars = append(qvars, workItemQueryVar{"assigneeUsernames", "[String!]", opt.AssigneeUsernames})
	}

	if opt.AssigneeWildcardID != nil {
		qvars = append(qvars, workItemQueryVar{"assigneeWildcardId", "AssigneeWildcardId", opt.AssigneeWildcardID})
	}

	if opt.AuthorUsername != nil {
		qvars = append(qvars, workItemQueryVar{"authorUsername", "String", opt.AuthorUsername})
	}

	if opt.Confidential != nil {
		qvars = append(qvars, workItemQueryVar{"confidential", "Boolean", opt.Confidential})
	}

	if opt.CRMContactID != nil {
		qvars = append(qvars, workItemQueryVar{"crmContactId", "String", opt.CRMContactID})
	}

	if opt.CRMOrganizationID != nil {
		qvars = append(qvars, workItemQueryVar{"crmOrganizationId", "String", opt.CRMOrganizationID})
	}

	if opt.HealthStatusFilter != nil {
		qvars = append(qvars, workItemQueryVar{"healthStatusFilter", "HealthStatusFilter", opt.HealthStatusFilter})
	}

	if len(opt.IDs) > 0 {
		qvars = append(qvars, workItemQueryVar{"ids", "[WorkItemID!]", opt.IDs})
	}

	if len(opt.IIDs) > 0 {
		qvars = append(qvars, workItemQueryVar{"iids", "[String!]", opt.IIDs})
	}

	if opt.IncludeAncestors != nil {
		qvars = append(qvars, workItemQueryVar{"includeAncestors", "Boolean", opt.IncludeAncestors})
	}

	if opt.IncludeDescendants != nil {
		qvars = append(qvars, workItemQueryVar{"includeDescendants", "Boolean", opt.IncludeDescendants})
	}

	if len(opt.IterationCadenceID) > 0 {
		qvars = append(qvars, workItemQueryVar{"iterationCadenceId", "[IterationsCadenceID!]", opt.IterationCadenceID})
	}

	if len(opt.IterationID) > 0 {
		qvars = append(qvars, workItemQueryVar{"iterationId", "[ID]", opt.IterationID})
	}

	if opt.IterationWildcardID != nil {
		qvars = append(qvars, workItemQueryVar{"iterationWildcardId", "IterationWildcardId", opt.IterationWildcardID})
	}

	if len(opt.LabelName) > 0 {
		qvars = append(qvars, workItemQueryVar{"labelName", "[String!]", opt.LabelName})
	}

	if len(opt.MilestoneTitle) > 0 {
		qvars = append(qvars, workItemQueryVar{"milestoneTitle", "[String!]", opt.MilestoneTitle})
	}

	if opt.MilestoneWildcardID != nil {
		qvars = append(qvars, workItemQueryVar{"milestoneWildcardId", "MilestoneWildcardId", opt.MilestoneWildcardID})
	}

	if opt.MyReactionEmoji != nil {
		qvars = append(qvars, workItemQueryVar{"myReactionEmoji", "String", opt.MyReactionEmoji})
	}

	if len(opt.ParentIDs) > 0 {
		qvars = append(qvars, workItemQueryVar{"parentIds", "[WorkItemID!]", opt.ParentIDs})
	}

	if len(opt.ReleaseTag) > 0 {
		qvars = append(qvars, workItemQueryVar{"releaseTag", "[String!]", opt.ReleaseTag})
	}

	if opt.ReleaseTagWildcardID != nil {
		qvars = append(qvars, workItemQueryVar{"releaseTagWildcardId", "ReleaseTagWildcardId", opt.ReleaseTagWildcardID})
	}

	if opt.State != nil {
		qvars = append(qvars, workItemQueryVar{"state", "IssuableState", opt.State})
	}

	if opt.Subscribed != nil {
		qvars = append(qvars, workItemQueryVar{"subscribed", "SubscriptionStatus", opt.Subscribed})
	}

	if len(opt.Types) > 0 {
		qvars = append(qvars, workItemQueryVar{"types", "[IssueType!]", opt.Types})
	}

	if opt.Weight != nil {
		qvars = append(qvars, workItemQueryVar{"weight", "String", opt.Weight})
	}

	if opt.WeightWildcardID != nil {
		qvars = append(qvars, workItemQueryVar{"weightWildcardId", "WeightWildcardId", opt.WeightWildcardID})
	}

	if opt.ClosedAfter != nil {
		qvars = append(qvars, workItemQueryVar{"closedAfter", "Time", opt.ClosedAfter})
	}

	if opt.ClosedBefore != nil {
		qvars = append(qvars, workItemQueryVar{"closedBefore", "Time", opt.ClosedBefore})
	}

	if opt.CreatedAfter != nil {
		qvars = append(qvars, workItemQueryVar{"createdAfter", "Time", opt.CreatedAfter})
	}

	if opt.CreatedBefore != nil {
		qvars = append(qvars, workItemQueryVar{"createdBefore", "Time", opt.CreatedBefore})
	}

	if opt.DueAfter != nil {
		qvars = append(qvars, workItemQueryVar{"dueAfter", "Time", opt.DueAfter})
	}

	if opt.DueBefore != nil {
		qvars = append(qvars, workItemQueryVar{"dueBefore", "Time", opt.DueBefore})
	}

	if opt.UpdatedAfter != nil {
		qvars = append(qvars, workItemQueryVar{"updatedAfter", "Time", opt.UpdatedAfter})
	}

	if opt.UpdatedBefore != nil {
		qvars = append(qvars, workItemQueryVar{"updatedBefore", "Time", opt.UpdatedBefore})
	}

	if opt.Sort != nil {
		qvars = append(qvars, workItemQueryVar{"sort", "WorkItemSort", opt.Sort})
	}

	if opt.Search != nil {
		qvars = append(qvars, workItemQueryVar{"search", "String", opt.Search})
	}

	if len(opt.In) > 0 {
		qvars = append(qvars, workItemQueryVar{"in", "[IssuableSearchableField!]", opt.In})
	}

	if opt.After != nil {
		qvars = append(qvars, workItemQueryVar{"after", "String", opt.After})
	}

	if opt.Before != nil {
		qvars = append(qvars, workItemQueryVar{"before", "String", opt.Before})
	}

	if opt.First != nil {
		qvars = append(qvars, workItemQueryVar{"first", "Int", opt.First})
	}

	if opt.Last != nil {
		qvars = append(qvars, workItemQueryVar{"last", "Int", opt.Last})
	}

	varsMap["fullPath"] = fullPath
	declParts := make([]string, 0, len(qvars))
	argParts := make([]string, 0, len(qvars))
	for _, qv := range qvars {
		declParts = append(declParts, fmt.Sprintf("$%s: %s", qv.name, qv.gqlType))
		argParts = append(argParts, fmt.Sprintf("%s: $%s", qv.name, qv.name))

		varsMap[qv.name] = qv.value
	}
	decls := strings.Join(declParts, ", ")
	args := strings.Join(argParts, ", ")

	listTemplate, err := buildListWorkItemsTemplate(opt.ReturnedFields)
	if err != nil {
		return "", nil, err
	}

	var queryBuilder strings.Builder
	if err := listTemplate.ExecuteTemplate(&queryBuilder, "ListWorkItems", listWorkItemsQueryData{Decls: decls, Args: args}); err != nil {
		return "", nil, err
	}
	return queryBuilder.String(), varsMap, nil
}

// ListWorkItems lists workitems in a given namespace (group or project).
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#namespaceworkitems
//
// Experimental: The Work Items API is a work in progress and may introduce breaking changes even between minor versions.
func (s *WorkItemsService) ListWorkItems(fullPath string, opt *ListWorkItemsOptions, options ...RequestOptionFunc) ([]*WorkItem, *Response, error) {
	queryStr, vars, err := buildListWorkItemsQuery(fullPath, opt)
	if err != nil {
		return nil, nil, err
	}
	query := GraphQLQuery{Query: queryStr, Variables: vars}

	var result struct {
		Data struct {
			Namespace struct {
				WorkItems connectionGQL[workItemGQL] `json:"workItems"`
			} `json:"namespace"`
		}
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

	var ret []*WorkItem

	for _, wi := range result.Data.Namespace.WorkItems.Nodes {
		ret = append(ret, wi.unwrap())
	}

	resp.PageInfo = &result.Data.Namespace.WorkItems.PageInfo

	return ret, resp, nil
}

var listWorkItemTypesTemplate = template.Must(template.New("listWorkItemTypes").Parse(`
query ListWorkItemTypes(
  $namespacePath: ID!,
  $name: IssueType,
  $onlyAvailable: Boolean,
  $after: String,
  $before: String,
  $first: Int,
  $last: Int
) {
  namespace(fullPath: $namespacePath) {
    workItemTypes(
      name: $name,
      onlyAvailable: $onlyAvailable,
      after: $after,
      before: $before,
      first: $first,
      last: $last
    ) {
      nodes {
        id
        name
        enabled
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
`))

// ListWorkItemTypes lists all work item types (system-defined and custom)
// for a given namespace.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemtype
//
// Experimental: The Work Items API is a work in progress and may introduce
// breaking changes even between minor versions.
func (s *WorkItemsService) ListWorkItemTypes(
	namespacePath string,
	opt *ListWorkItemTypesOptions,
	options ...RequestOptionFunc,
) ([]WorkItemType, *Response, error) {
	var queryBuilder strings.Builder

	if opt == nil {
		opt = &ListWorkItemTypesOptions{}
	}
	if err := listWorkItemTypesTemplate.Execute(&queryBuilder, nil); err != nil {
		return nil, nil, err
	}

	vars := map[string]any{
		"namespacePath": namespacePath,
		"name":          opt.Name,
		"onlyAvailable": opt.OnlyAvailable,
		"after":         opt.After,
		"before":        opt.Before,
		"first":         opt.First,
		"last":          opt.Last,
	}

	query := GraphQLQuery{
		Query:     queryBuilder.String(),
		Variables: vars,
	}

	var result struct {
		Data struct {
			Namespace struct {
				WorkItemTypes connectionGQL[WorkItemType] `json:"workItemTypes"`
			} `json:"namespace"`
		}
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

	resp.PageInfo = &result.Data.Namespace.WorkItemTypes.PageInfo

	return result.Data.Namespace.WorkItemTypes.Nodes, resp, nil
}

// CreateWorkItemOptions represents the available CreateWorkItem() options.
//
// GitLab API docs:
// https://docs.gitlab.com/ee/api/graphql/reference/#workitemcreateinput
//
// Experimental: The Work Items API is a work in progress and may introduce breaking changes even between minor versions.
type CreateWorkItemOptions struct {
	// Title of the work item. Required.
	Title string

	// Description of the work item.
	Description *string

	// Sets the work item confidentiality.
	Confidential *bool

	// Global IDs of assignees.
	AssigneeIDs []int64

	// Global ID of the milestone to assign to the work item.
	MilestoneID *int64

	// Source which triggered the creation of the work item. Used only for tracking purposes.
	CreateSource *string

	// Timestamp when the work item was created. Available only for admins and project owners.
	CreatedAt *time.Time // admins and project owners only

	// CRM contact IDs to set.
	CRMContactIDs []int64

	// Global ID of the parent work item.
	ParentID *int64

	// Global IDs of labels to be added to the work item.
	LabelIDs []int64

	// Linked work items to be added to the work item.
	LinkedItems *CreateWorkItemOptionsLinkedItems

	// Start date for the work item.
	StartDate *ISOTime

	// Due date for the work item.
	DueDate *ISOTime

	// Weight of the work item.
	Weight *int64

	// Health status to be assigned to the work item. Possible values: onTrack, needsAttention, atRisk
	HealthStatus *string

	// Global ID of the iteration to assign to the work item.
	IterationID *int64

	// Color of the work item, represented as a hex code or named color. Example: "#fefefe"
	Color *string

	// Global ID of the work item status.
	Status *WorkItemStatusID
}

// CreateWorkItemOptionsLinkedItems represents linked items to be added to a work item.
//
// Experimental: The Work Items API is a work in progress and may introduce breaking changes even between minor versions.
type CreateWorkItemOptionsLinkedItems struct {
	LinkType    *string // enum: BLOCKED_BY, BLOCKS, RELATED
	WorkItemIDs []int64
}

// workItemCreateInputGQL represents the GraphQL input structure for creating a work item.
type workItemCreateInputGQL struct {
	// Required
	NamespacePath  string         `json:"namespacePath"`
	WorkItemTypeID WorkItemTypeID `json:"workItemTypeId"`
	Title          string         `json:"title"`

	// Optional
	AssigneesWidget       *workItemWidgetAssigneesInputGQL         `json:"assigneesWidget,omitempty"`
	Confidential          *bool                                    `json:"confidential,omitempty"`
	DescriptionWidget     *workItemWidgetDescriptionInputGQL       `json:"descriptionWidget,omitempty"`
	MilestoneWidget       *workItemWidgetMilestoneInputGQL         `json:"milestoneWidget,omitempty"`
	CreateSource          *string                                  `json:"createSource,omitempty"`
	CreatedAt             *time.Time                               `json:"createdAt,omitempty"`
	CRMContactsWidget     *workItemWidgetCRMContactsCreateInputGQL `json:"crmContactsWidget,omitempty"`
	HierarchyWidget       *workItemWidgetHierarchyCreateInputGQL   `json:"hierarchyWidget,omitempty"`
	LabelsWidget          *workItemWidgetLabelsCreateInputGQL      `json:"labelsWidget,omitempty"`
	LinkedItemsWidget     *workItemWidgetLinkedItemsCreateInputGQL `json:"linkedItemsWidget,omitempty"`
	StartAndDueDateWidget *workItemWidgetStartAndDueDateInputGQL   `json:"startAndDueDateWidget,omitempty"`
	WeightWidget          *workItemWidgetWeightInputGQL            `json:"weightWidget,omitempty"`
	HealthStatusWidget    *workItemWidgetHealthStatusInputGQL      `json:"healthStatusWidget,omitempty"`
	IterationWidget       *workItemWidgetIterationInputGQL         `json:"iterationWidget,omitempty"`
	ColorWidget           *workItemWidgetColorInputGQL             `json:"colorWidget,omitempty"`
	StatusWidget          *workItemWidgetStatusInputGQL            `json:"statusWidget,omitempty"`
}

// workItemWidgetAssigneesInputGQL represents the assignees widget input.
type workItemWidgetAssigneesInputGQL struct {
	AssigneeIDs []string `json:"assigneeIds"`
}

// workItemWidgetDescriptionInputGQL represents the description widget input.
type workItemWidgetDescriptionInputGQL struct {
	Description *string `json:"description,omitempty"`
}

// workItemWidgetMilestoneInputGQL represents the milestone widget input.
type workItemWidgetMilestoneInputGQL struct {
	MilestoneID *string `json:"milestoneId,omitempty"`
}

// workItemWidgetCRMContactsCreateInputGQL represents the CRM contacts widget input.
type workItemWidgetCRMContactsCreateInputGQL struct {
	ContactIDs []string `json:"contactIds,omitempty"`
}

// workItemWidgetHierarchyCreateInputGQL represents the hierarchy widget input.
type workItemWidgetHierarchyCreateInputGQL struct {
	ParentID *string `json:"parentId,omitempty"`
}

// workItemWidgetLabelsCreateInputGQL represents the labels widget input.
type workItemWidgetLabelsCreateInputGQL struct {
	LabelIDs []string `json:"labelIds,omitempty"`
}

// workItemWidgetLinkedItemsCreateInputGQL represents the linked items widget input.
type workItemWidgetLinkedItemsCreateInputGQL struct {
	LinkType    *string  `json:"linkType,omitempty"`
	WorkItemIDs []string `json:"workItemsIds,omitempty"`
}

// workItemWidgetStartAndDueDateInputGQL represents the start and due date widget input.
type workItemWidgetStartAndDueDateInputGQL struct {
	StartDate *string `json:"startDate,omitempty"`
	DueDate   *string `json:"dueDate,omitempty"`
}

// workItemWidgetWeightInputGQL represents the weight widget input.
type workItemWidgetWeightInputGQL struct {
	Weight *int64 `json:"weight,omitempty"`
}

// workItemWidgetHealthStatusInputGQL represents the health status widget input.
type workItemWidgetHealthStatusInputGQL struct {
	HealthStatus *string `json:"healthStatus,omitempty"`
}

// workItemWidgetIterationInputGQL represents the iteration widget input.
type workItemWidgetIterationInputGQL struct {
	IterationID *string `json:"iterationId,omitempty"`
}

// workItemWidgetColorInputGQL represents the color widget input.
type workItemWidgetColorInputGQL struct {
	Color *string `json:"color,omitempty"`
}

// workItemWidgetCRMContactsUpdateInputGQL represents the CRM contacts widget input for updates.
type workItemWidgetCRMContactsUpdateInputGQL struct {
	ContactIDs    []string `json:"contactIds"`
	OperationMode *string  `json:"operationMode,omitempty"`
}

// workItemWidgetHierarchyUpdateInputGQL represents the hierarchy widget input for updates.
type workItemWidgetHierarchyUpdateInputGQL struct {
	ParentID           *string  `json:"parentId,omitempty"`
	AdjacentWorkItemID *string  `json:"adjacentWorkItemId,omitempty"`
	ChildrenIDs        []string `json:"childrenIds,omitempty"`
	RelativePosition   *string  `json:"relativePosition,omitempty"`
}

// workItemWidgetLabelsUpdateInputGQL represents the labels widget input for updates.
type workItemWidgetLabelsUpdateInputGQL struct {
	AddLabelIDs    []string `json:"addLabelIds,omitempty"`
	RemoveLabelIDs []string `json:"removeLabelIds,omitempty"`
}

// workItemWidgetStartAndDueDateUpdateInputGQL represents the start and due date widget input for updates.
type workItemWidgetStartAndDueDateUpdateInputGQL struct {
	StartDate *string `json:"startDate,omitempty"`
	DueDate   *string `json:"dueDate,omitempty"`
}

// workItemWidgetStatusInputGQL represents the status widget input.
type workItemWidgetStatusInputGQL struct {
	Status *WorkItemStatusID `json:"status,omitempty"`
}

// newWorkItemCreateInput converts the user-facing CreateWorkItemOptions to the
// backend-facing GraphQL-aligned workItemCreateInputGQL struct.
func (opt *CreateWorkItemOptions) wrap(namespacePath string, workItemTypeID WorkItemTypeID) *workItemCreateInputGQL {
	if opt == nil {
		return &workItemCreateInputGQL{
			NamespacePath:  namespacePath,
			WorkItemTypeID: workItemTypeID,
		}
	}

	input := &workItemCreateInputGQL{
		NamespacePath:  namespacePath,
		WorkItemTypeID: workItemTypeID,

		Title:        opt.Title,
		Confidential: opt.Confidential,
		CreateSource: opt.CreateSource,
		CreatedAt:    opt.CreatedAt,
	}

	if opt.Description != nil {
		input.DescriptionWidget = &workItemWidgetDescriptionInputGQL{
			Description: opt.Description,
		}
	}

	if len(opt.AssigneeIDs) > 0 {
		input.AssigneesWidget = &workItemWidgetAssigneesInputGQL{
			AssigneeIDs: newGIDStrings("User", opt.AssigneeIDs...),
		}
	}

	if opt.MilestoneID != nil {
		input.MilestoneWidget = &workItemWidgetMilestoneInputGQL{
			MilestoneID: new(gidGQL{"Milestone", *opt.MilestoneID}.String()),
		}
	}

	if len(opt.CRMContactIDs) > 0 {
		input.CRMContactsWidget = &workItemWidgetCRMContactsCreateInputGQL{
			ContactIDs: newGIDStrings("CustomerRelations::Contact", opt.CRMContactIDs...),
		}
	}

	if opt.ParentID != nil {
		input.HierarchyWidget = &workItemWidgetHierarchyCreateInputGQL{
			ParentID: new(gidGQL{"WorkItem", *opt.ParentID}.String()),
		}
	}

	if len(opt.LabelIDs) > 0 {
		input.LabelsWidget = &workItemWidgetLabelsCreateInputGQL{
			LabelIDs: newGIDStrings("Label", opt.LabelIDs...),
		}
	}

	if opt.LinkedItems != nil && len(opt.LinkedItems.WorkItemIDs) > 0 {
		input.LinkedItemsWidget = &workItemWidgetLinkedItemsCreateInputGQL{
			LinkType:    opt.LinkedItems.LinkType,
			WorkItemIDs: newGIDStrings("WorkItem", opt.LinkedItems.WorkItemIDs...),
		}
	}

	if opt.StartDate != nil || opt.DueDate != nil {
		widget := &workItemWidgetStartAndDueDateInputGQL{}
		if opt.StartDate != nil {
			widget.StartDate = new(opt.StartDate.String())
		}
		if opt.DueDate != nil {
			widget.DueDate = new(opt.DueDate.String())
		}
		input.StartAndDueDateWidget = widget
	}

	if opt.Weight != nil {
		input.WeightWidget = &workItemWidgetWeightInputGQL{
			Weight: opt.Weight,
		}
	}

	if opt.HealthStatus != nil {
		input.HealthStatusWidget = &workItemWidgetHealthStatusInputGQL{
			HealthStatus: opt.HealthStatus,
		}
	}

	if opt.IterationID != nil {
		input.IterationWidget = &workItemWidgetIterationInputGQL{
			IterationID: new(gidGQL{"Iteration", *opt.IterationID}.String()),
		}
	}

	if opt.Color != nil {
		input.ColorWidget = &workItemWidgetColorInputGQL{
			Color: opt.Color,
		}
	}

	if opt.Status != nil {
		input.StatusWidget = &workItemWidgetStatusInputGQL{
			Status: opt.Status,
		}
	}

	return input
}

// updateWorkItemTemplate is chained from workItemTemplate so it has access to both
// UserCoreBasic and WorkItem templates.
var updateWorkItemTemplate = template.Must(template.Must(workItemTemplate.Clone()).New("UpdateWorkItem").Parse(`
	mutation UpdateWorkItem($input: WorkItemUpdateInput!) {
		workItemUpdate(input: $input) {
			workItem {
				{{ template "WorkItem" }}
			}
			errors
		}
	}
`))

// createWorkItemTemplate is chained from workItemTemplate so it has access to both
// UserCoreBasic and WorkItem templates.
var createWorkItemTemplate = template.Must(template.Must(workItemTemplate.Clone()).New("CreateWorkItem").Parse(`
	mutation CreateWorkItem($input: WorkItemCreateInput!) {
	  workItemCreate(input: $input) {
	    workItem {
	      {{ template "WorkItem" }}
	    }
	    errors
	  }
	}
`))

// CreateWorkItem creates a new work item.
//
// fullPath is the full path to either a group or project.
// workItemTypeID is the GraphQL ID of the work item type.
// opt contains the options for creating the work item.
//
// GitLab API docs:
// https://docs.gitlab.com/ee/api/graphql/reference/#workitemcreateinput
//
// Experimental: The Work Items API is a work in progress and may introduce breaking changes even between minor versions.
func (s *WorkItemsService) CreateWorkItem(fullPath string, workItemTypeID WorkItemTypeID, opt *CreateWorkItemOptions, options ...RequestOptionFunc) (*WorkItem, *Response, error) {
	var queryBuilder strings.Builder
	if err := createWorkItemTemplate.Execute(&queryBuilder, nil); err != nil {
		return nil, nil, err
	}

	input := opt.wrap(fullPath, workItemTypeID)

	q := GraphQLQuery{
		Query: queryBuilder.String(),
		Variables: map[string]any{
			"input": input,
		},
	}

	var result struct {
		Data struct {
			WorkItemCreate struct {
				WorkItem *workItemGQL `json:"workItem"`
				Errors   []string     `json:"errors"`
			} `json:"workItemCreate"`
		}
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(q, &result, options...)
	if err != nil {
		return nil, resp, err
	}

	if len(result.Errors) != 0 {
		return nil, resp, &GraphQLResponseError{
			Err:    errors.New("Mutation.workItemCreate failed"),
			Errors: result.GenericGraphQLErrors,
		}
	}

	if len(result.Data.WorkItemCreate.Errors) != 0 {
		err := &ErrorResponse{
			StatusCode: resp.Response.StatusCode,
			Message:    strings.Join(result.Data.WorkItemCreate.Errors, "; "),
			Response:   resp.Response,
		}

		return nil, resp, err
	}

	wiQL := result.Data.WorkItemCreate.WorkItem
	if wiQL == nil {
		return nil, resp, ErrEmptyResponse
	}

	return wiQL.unwrap(), resp, nil
}

// UpdateWorkItemOptions represents the available UpdateWorkItem() options.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationworkitemupdate
//
// Experimental: The Work Items API is a work in progress and may introduce breaking changes even between minor versions.
type UpdateWorkItemOptions struct {
	// Title of the work item.
	Title *string

	// State event for the work item. Possible values: CLOSE, REOPEN
	StateEvent *WorkItemStateEvent

	// Description of the work item.
	Description *string

	// Global IDs of assignees. An empty (non-nil) slice can be used to remove all assignees.
	AssigneeIDs []int64

	// Global ID of the milestone to assign to the work item.
	MilestoneID *int64

	// CRM contact IDs to set. An empty (non-nil) slice can be used to remove all contacts.
	CRMContactIDs []int64

	// Global ID of the parent work item.
	ParentID *int64

	// Global IDs of labels to be added to the work item.
	AddLabelIDs []int64

	// Global IDs of labels to be removed from the work item.
	RemoveLabelIDs []int64

	// Start date for the work item.
	StartDate *ISOTime

	// Due date for the work item.
	DueDate *ISOTime

	// Weight of the work item.
	Weight *int64

	// Health status to be assigned to the work item. Possible values: onTrack, needsAttention, atRisk
	HealthStatus *string

	// Global ID of the iteration to assign to the work item.
	IterationID *int64

	// Color of the work item, represented as a hex code or named color. Example: "#fefefe"
	Color *string

	// Global ID of the work item status.
	Status *WorkItemStatusID
}

func (opt *UpdateWorkItemOptions) wrap(gid gidGQL) *workItemUpdateInputGQL {
	if opt == nil {
		return &workItemUpdateInputGQL{
			ID: gid.String(),
		}
	}

	input := &workItemUpdateInputGQL{
		ID:         gid.String(),
		Title:      opt.Title,
		StateEvent: opt.StateEvent,
	}

	if opt.Description != nil {
		input.DescriptionWidget = &workItemWidgetDescriptionInputGQL{
			Description: opt.Description,
		}
	}

	if opt.AssigneeIDs != nil {
		input.AssigneesWidget = &workItemWidgetAssigneesInputGQL{
			AssigneeIDs: newGIDStrings("User", opt.AssigneeIDs...),
		}
	}

	if opt.MilestoneID != nil {
		input.MilestoneWidget = &workItemWidgetMilestoneInputGQL{
			MilestoneID: new(gidGQL{"Milestone", *opt.MilestoneID}.String()),
		}
	}

	if opt.CRMContactIDs != nil {
		input.CRMContactsWidget = &workItemWidgetCRMContactsUpdateInputGQL{
			ContactIDs:    newGIDStrings("CustomerRelations::Contact", opt.CRMContactIDs...),
			OperationMode: new("REPLACE"),
		}
	}

	if opt.ParentID != nil {
		input.HierarchyWidget = &workItemWidgetHierarchyUpdateInputGQL{
			ParentID: new(gidGQL{"WorkItem", *opt.ParentID}.String()),
		}
	}

	if len(opt.AddLabelIDs) > 0 || len(opt.RemoveLabelIDs) > 0 {
		widget := &workItemWidgetLabelsUpdateInputGQL{}
		if len(opt.AddLabelIDs) > 0 {
			widget.AddLabelIDs = newGIDStrings("Label", opt.AddLabelIDs...)
		}
		if len(opt.RemoveLabelIDs) > 0 {
			widget.RemoveLabelIDs = newGIDStrings("Label", opt.RemoveLabelIDs...)
		}
		input.LabelsWidget = widget
	}

	if opt.StartDate != nil || opt.DueDate != nil {
		widget := &workItemWidgetStartAndDueDateUpdateInputGQL{}
		if opt.StartDate != nil {
			widget.StartDate = new(opt.StartDate.String())
		}
		if opt.DueDate != nil {
			widget.DueDate = new(opt.DueDate.String())
		}
		input.StartAndDueDateWidget = widget
	}

	if opt.Weight != nil {
		input.WeightWidget = &workItemWidgetWeightInputGQL{
			Weight: opt.Weight,
		}
	}

	if opt.HealthStatus != nil {
		input.HealthStatusWidget = &workItemWidgetHealthStatusInputGQL{
			HealthStatus: opt.HealthStatus,
		}
	}

	if opt.IterationID != nil {
		input.IterationWidget = &workItemWidgetIterationInputGQL{
			IterationID: new(gidGQL{"Iteration", *opt.IterationID}.String()),
		}
	}

	if opt.Color != nil {
		input.ColorWidget = &workItemWidgetColorInputGQL{
			Color: opt.Color,
		}
	}

	if opt.Status != nil {
		input.StatusWidget = &workItemWidgetStatusInputGQL{
			Status: opt.Status,
		}
	}

	return input
}

// UpdateWorkItem updates a work item by fullPath and iid.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationworkitemupdate
//
// Experimental: The Work Items API is a work in progress and may introduce breaking changes even between minor versions.
func (s *WorkItemsService) UpdateWorkItem(fullPath string, iid int64, opt *UpdateWorkItemOptions, options ...RequestOptionFunc) (*WorkItem, *Response, error) {
	gid, resp, err := s.workItemGID(fullPath, iid, options...)
	if err != nil {
		return nil, resp, err
	}

	var queryBuilder strings.Builder
	if err := updateWorkItemTemplate.Execute(&queryBuilder, nil); err != nil {
		return nil, nil, err
	}

	q := GraphQLQuery{
		Query: queryBuilder.String(),
		Variables: map[string]any{
			"input": opt.wrap(gid),
		},
	}

	var result struct {
		Data struct {
			WorkItemUpdate struct {
				WorkItem *workItemGQL `json:"workItem"`
				Errors   []string     `json:"errors"`
			} `json:"workItemUpdate"`
		}
		GenericGraphQLErrors
	}

	resp, err = s.client.GraphQL.Do(q, &result, options...)
	if err != nil {
		return nil, resp, err
	}

	if len(result.Errors) != 0 {
		return nil, resp, &GraphQLResponseError{
			Err:    errors.New("GraphQL query failed"),
			Errors: result.GenericGraphQLErrors,
		}
	}

	if len(result.Data.WorkItemUpdate.Errors) != 0 {
		return nil, resp, errors.New(result.Data.WorkItemUpdate.Errors[0])
	}

	wiQL := result.Data.WorkItemUpdate.WorkItem
	if wiQL == nil {
		return nil, resp, ErrNotFound
	}

	return wiQL.unwrap(), resp, nil
}

// workItemUpdateInputGQL represents the GraphQL input structure for updating a work item.
type workItemUpdateInputGQL struct {
	ID                    string                                       `json:"id"`
	Title                 *string                                      `json:"title,omitempty"`
	StateEvent            *WorkItemStateEvent                          `json:"stateEvent,omitempty"`
	DescriptionWidget     *workItemWidgetDescriptionInputGQL           `json:"descriptionWidget,omitempty"`
	AssigneesWidget       *workItemWidgetAssigneesInputGQL             `json:"assigneesWidget,omitempty"`
	MilestoneWidget       *workItemWidgetMilestoneInputGQL             `json:"milestoneWidget,omitempty"`
	CRMContactsWidget     *workItemWidgetCRMContactsUpdateInputGQL     `json:"crmContactsWidget,omitempty"`
	HierarchyWidget       *workItemWidgetHierarchyUpdateInputGQL       `json:"hierarchyWidget,omitempty"`
	LabelsWidget          *workItemWidgetLabelsUpdateInputGQL          `json:"labelsWidget,omitempty"`
	StartAndDueDateWidget *workItemWidgetStartAndDueDateUpdateInputGQL `json:"startAndDueDateWidget,omitempty"`
	WeightWidget          *workItemWidgetWeightInputGQL                `json:"weightWidget,omitempty"`
	HealthStatusWidget    *workItemWidgetHealthStatusInputGQL          `json:"healthStatusWidget,omitempty"`
	IterationWidget       *workItemWidgetIterationInputGQL             `json:"iterationWidget,omitempty"`
	ColorWidget           *workItemWidgetColorInputGQL                 `json:"colorWidget,omitempty"`
	StatusWidget          *workItemWidgetStatusInputGQL                `json:"statusWidget,omitempty"`
}

// DeleteWorkItem deletes a single work item.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#mutationworkitemdelete
func (s *WorkItemsService) DeleteWorkItem(fullPath string, iid int64, options ...RequestOptionFunc) (*Response, error) {
	// get the global ID
	gid, resp, err := s.workItemGID(fullPath, iid, options...)
	if err != nil {
		if errors.Is(err, ErrEmptyResponse) {
			return resp, ErrNotFound
		}
		return resp, err
	}

	mutation := GraphQLQuery{
		Query: deleteWorkItemQuery,
		Variables: map[string]any{
			"id": gid.String(),
		},
	}

	var result struct {
		Data struct {
			WorkItemDelete struct {
				Errors []string `json:"errors"`
			} `json:"workItemDelete"`
		}
		GenericGraphQLErrors
	}

	resp, err = s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return resp, err
	}

	if len(result.Errors) != 0 {
		return resp, &GraphQLResponseError{
			Err:    errors.New("Mutation.workItemDelete failed"),
			Errors: result.GenericGraphQLErrors,
		}
	}

	if len(result.Data.WorkItemDelete.Errors) != 0 {
		return resp, errors.New(strings.Join(result.Data.WorkItemDelete.Errors, ", "))
	}

	return resp, nil
}

// workItemGID queries for a work item's Global ID using fullPath and iid.
// Returns the Global ID string or ErrNotFound if the work item doesn't exist.
//
// API docs: https://docs.gitlab.com/api/graphql/reference/#namespaceworkitem
func (s *WorkItemsService) workItemGID(fullPath string, iid int64, options ...RequestOptionFunc) (gidGQL, *Response, error) {
	q := GraphQLQuery{
		Query: getWorkItemIDQuery,
		Variables: map[string]any{
			"fullPath": fullPath,
			"iid":      strconv.FormatInt(iid, 10),
		},
	}

	var result struct {
		Data struct {
			Namespace struct {
				WorkItem struct {
					ID gidGQL `json:"id"`
				} `json:"workItem"`
			} `json:"namespace"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(q, &result, options...)
	if err != nil {
		return gidGQL{}, resp, err
	}

	if len(result.Errors) != 0 {
		return gidGQL{}, resp, &GraphQLResponseError{
			Err:    fmt.Errorf("looking up global ID of %s#%d failed", fullPath, iid),
			Errors: result.GenericGraphQLErrors,
		}
	}

	id := result.Data.Namespace.WorkItem.ID
	if id.IsZero() {
		return gidGQL{}, resp, fmt.Errorf("looking up global ID of %s#%d failed: %w", fullPath, iid, ErrEmptyResponse)
	}

	return id, resp, nil
}

// workItemGQL represents the JSON structure returned by the GraphQL query.
// It is used to parse the response and convert it to the more user-friendly WorkItem type.
type workItemGQL struct {
	ID           gidGQL `json:"id"`
	IID          iidGQL `json:"iid"`
	WorkItemType struct {
		Name string `json:"name"`
	} `json:"workItemType"`
	State        string              `json:"state"`
	Title        string              `json:"title"`
	Description  string              `json:"description"`
	CreatedAt    *time.Time          `json:"createdAt"`
	UpdatedAt    *time.Time          `json:"updatedAt"`
	ClosedAt     *time.Time          `json:"closedAt"`
	Author       *userCoreBasicGQL   `json:"author"`
	Features     workItemFeaturesGQL `json:"features"`
	WebURL       string              `json:"webUrl"`
	Confidential bool                `json:"confidential"`
}

func (w workItemGQL) unwrap() *WorkItem {
	var assignees []*BasicUser

	wi := &WorkItem{
		ID:           w.ID.Int64,
		IID:          int64(w.IID),
		Type:         w.WorkItemType.Name,
		State:        w.State,
		Title:        w.Title,
		Description:  w.Description,
		CreatedAt:    w.CreatedAt,
		UpdatedAt:    w.UpdatedAt,
		ClosedAt:     w.ClosedAt,
		WebURL:       w.WebURL,
		Author:       w.Author.unwrap(),
		Assignees:    assignees,
		Confidential: w.Confidential,
	}

	w.Features.unwrap(wi)

	return wi
}

// workItemFeaturesGQL represents the optional features of the work item.
//
// While the "features" field in the "WorkItem" type is not nullable, each
// feature inside the struct is.
//
// API docs: https://docs.gitlab.com/api/graphql/reference/#workitemfeatures
type workItemFeaturesGQL struct {
	Assignees       *workItemWidgetAssigneesGQL       `json:"assignees"`
	Color           *workItemWidgetColorGQL           `json:"color"`
	HealthStatus    *workItemWidgetHealthStatusGQL    `json:"healthStatus"`
	Hierarchy       *workItemWidgetHierarchyGQL       `json:"hierarchy"`
	Iteration       *workItemWidgetIterationGQL       `json:"iteration"`
	Labels          *workItemWidgetLabelsGQL          `json:"labels"`
	LinkedItems     *workItemWidgetLinkedItemsGQL     `json:"linkedItems"`
	Milestone       *workItemWidgetMilestoneGQL       `json:"milestone"`
	StartAndDueDate *workItemWidgetStartAndDueDateGQL `json:"startAndDueDate"`
	Status          *workItemWidgetStatusGQL          `json:"status"`
	Weight          *workItemWidgetWeightGQL          `json:"weight"`
}

func (f workItemFeaturesGQL) unwrap(wi *WorkItem) {
	wi.Assignees = f.Assignees.unwrap()
	wi.Color = f.Color.unwrap()
	wi.HealthStatus = f.HealthStatus.unwrap()
	wi.Parent = f.Hierarchy.unwrapParent()
	wi.Children = f.Hierarchy.unwrapChildren()
	wi.IterationID = f.Iteration.unwrap()
	wi.Labels = f.Labels.unwrap()
	wi.LinkedItems = f.LinkedItems.unwrap()
	wi.MilestoneID = f.Milestone.unwrap()
	wi.StartDate, wi.DueDate = f.StartAndDueDate.unwrap()
	wi.Status = f.Status.unwrap()
	wi.Weight = f.Weight.unwrap()
}

// workItemWidgetAssigneesGQL represents the assignees widget.
//
// API docs: https://docs.gitlab.com/api/graphql/reference/#workitemwidgetassignees
type workItemWidgetAssigneesGQL struct {
	Assignees *connectionGQL[userCoreBasicGQL] `json:"assignees"`
}

func (a *workItemWidgetAssigneesGQL) unwrap() []*BasicUser {
	if a == nil || a.Assignees == nil || len(a.Assignees.Nodes) == 0 {
		return nil
	}

	ret := make([]*BasicUser, 0, len(a.Assignees.Nodes))

	for _, assignee := range a.Assignees.Nodes {
		ret = append(ret, assignee.unwrap())
	}

	return ret
}

// workItemWidgetColorGQL represents a color widget.
//
// API docs: https://docs.gitlab.com/api/graphql/reference/#workitemwidgetcolor
type workItemWidgetColorGQL struct {
	Color     *string `json:"color"`
	TextColor *string `json:"textColor"`
}

func (c *workItemWidgetColorGQL) unwrap() *string {
	if c == nil {
		return nil
	}

	return c.Color
}

// workItemWidgetHealthStatusGQL represents a health status widget.
//
// API docs: https://docs.gitlab.com/api/graphql/reference/#workitemwidgethealthstatus
type workItemWidgetHealthStatusGQL struct {
	HealthStatus *string `json:"healthStatus"`
}

func (h *workItemWidgetHealthStatusGQL) unwrap() *string {
	if h == nil {
		return nil
	}

	return h.HealthStatus
}

// workItemWidgetHierarchyGQL represents a hierarchy widget.
//
// API docs: https://docs.gitlab.com/api/graphql/reference/#workitemwidgethierarchy
type workItemWidgetHierarchyGQL struct {
	HasParent   bool                            `json:"hasParent"`
	Parent      *workItemIIDGQL                 `json:"parent"`
	HasChildren bool                            `json:"hasChildren"`
	Children    *connectionGQL[*workItemIIDGQL] `json:"children"`
}

func (h *workItemWidgetHierarchyGQL) unwrapParent() *WorkItemIID {
	if h == nil || !h.HasParent || h.Parent == nil {
		return nil
	}

	return h.Parent.unwrap()
}

func (h *workItemWidgetHierarchyGQL) unwrapChildren() []WorkItemIID {
	if h == nil || !h.HasChildren || h.Children == nil || len(h.Children.Nodes) == 0 {
		return nil
	}

	children := make([]WorkItemIID, 0, len(h.Children.Nodes))

	for _, node := range h.Children.Nodes {
		child := node.unwrap()
		if child == nil {
			continue
		}

		children = append(children, *child)
	}

	return children
}

// workItemWidgetIterationGQL represents a iteration widget.
//
// API docs: https://docs.gitlab.com/api/graphql/reference/#workitemwidgetiteration
type workItemWidgetIterationGQL struct {
	Iteration *struct {
		ID gidGQL `json:"id"`
	} `json:"iteration"`
}

func (c *workItemWidgetIterationGQL) unwrap() *int64 {
	if c == nil || c.Iteration == nil {
		return nil
	}

	return new(c.Iteration.ID.Int64)
}

// workItemWidgetLabelsGQL represents the labels widget.
//
// API docs: https://docs.gitlab.com/api/graphql/reference/#workitemwidgetlabels
type workItemWidgetLabelsGQL struct {
	Labels *connectionGQL[labelGQL] `json:"labels"`
}

func (l *workItemWidgetLabelsGQL) unwrap() []LabelDetails {
	if l == nil || l.Labels == nil || len(l.Labels.Nodes) == 0 {
		return nil
	}

	ret := make([]LabelDetails, 0, len(l.Labels.Nodes))

	for _, label := range l.Labels.Nodes {
		ret = append(ret, label.unwrap())
	}

	return ret
}

// workItemWidgetLinkedItemsGQL represents the linked items widget.
//
// API docs: https://docs.gitlab.com/api/graphql/reference/#workitemwidgetlinkeditems
type workItemWidgetLinkedItemsGQL struct {
	LinkedItems *connectionGQL[struct {
		WorkItem *workItemIIDGQL `json:"workItem"`
		LinkType string          `json:"linkType"`
	}] `json:"linkedItems"`
}

func (li *workItemWidgetLinkedItemsGQL) unwrap() []LinkedWorkItem {
	if li == nil || li.LinkedItems == nil {
		return nil
	}

	var ret []LinkedWorkItem

	for _, node := range li.LinkedItems.Nodes {
		wi := node.WorkItem.unwrap()
		if wi == nil {
			continue
		}

		ret = append(ret, LinkedWorkItem{
			WorkItemIID: *wi,
			LinkType:    node.LinkType,
		})
	}

	return ret
}

// workItemWidgetMilestoneGQL represents the milestone widget.
//
// API docs: https://docs.gitlab.com/api/graphql/reference/#workitemwidgetmilestone
type workItemWidgetMilestoneGQL struct {
	Milestone *struct {
		ID gidGQL `json:"id"`
	} `json:"milestone"`
}

func (m *workItemWidgetMilestoneGQL) unwrap() *int64 {
	if m == nil || m.Milestone == nil {
		return nil
	}

	return new(m.Milestone.ID.Int64)
}

// workItemWidgetStartAndDueDateGQL represents a start and due date widget.
//
// API docs: https://docs.gitlab.com/api/graphql/reference/#workitemwidgetstartandduedate
type workItemWidgetStartAndDueDateGQL struct {
	DueDate   *ISOTime `json:"dueDate"`
	IsFixed   bool     `json:"isFixed"`
	StartDate *ISOTime `json:"startDate"`
}

func (du *workItemWidgetStartAndDueDateGQL) unwrap() (start, due *ISOTime) {
	if du == nil {
		return nil, nil
	}

	return du.StartDate, du.DueDate
}

// workItemWidgetStatusGQL represents the status widget.
//
// API docs: https://docs.gitlab.com/api/graphql/reference/#workitemwidgetstatus
type workItemWidgetStatusGQL struct {
	Status *struct {
		Name *string `json:"name"`
	} `json:"status"`
}

func (s *workItemWidgetStatusGQL) unwrap() *string {
	if s == nil || s.Status == nil {
		return nil
	}

	return s.Status.Name
}

// workItemWidgetWeightGQL represents the weight widget.
//
// API docs: https://docs.gitlab.com/api/graphql/reference/#workitemwidgetweight
type workItemWidgetWeightGQL struct {
	Weight *int64 `json:"weight"`
}

func (w *workItemWidgetWeightGQL) unwrap() *int64 {
	if w == nil {
		return nil
	}

	return w.Weight
}

// WorkItemType represents a GitLab work item type.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemtype
//
// Experimental: The Work Items API is a work in progress and may introduce
// breaking changes even between minor versions.
type WorkItemType struct {
	ID      WorkItemTypeID `json:"id"`
	Name    string         `json:"name"`
	Enabled bool           `json:"enabled"`
}

// ListWorkItemTypesOptions specifies the optional parameters to the
// WorkItemsService.ListWorkItemTypes method.
//
// Experimental: The Work Items API is a work in progress and may introduce
// breaking changes even between minor versions.
type ListWorkItemTypesOptions struct {
	Name          *string `json:"name,omitempty"`
	OnlyAvailable *bool   `json:"onlyAvailable,omitempty"`
	After         *string `json:"after,omitempty"`
	Before        *string `json:"before,omitempty"`
	First         *int64  `json:"first,omitempty"`
	Last          *int64  `json:"last,omitempty"`
}

// WorkItemTypeID represents the global ID of a work item type.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemtype
//
// Experimental: The Work Items API is a work in progress and may introduce breaking changes even between minor versions.
type WorkItemTypeID string

// WorkItemTypeID constants for the system-defined work item types.
const (
	WorkItemTypeIssue       WorkItemTypeID = `gid://gitlab/WorkItems::Type/1`
	WorkItemTypeIncident    WorkItemTypeID = `gid://gitlab/WorkItems::Type/2`
	WorkItemTypeTestCase    WorkItemTypeID = `gid://gitlab/WorkItems::Type/3`
	WorkItemTypeRequirement WorkItemTypeID = `gid://gitlab/WorkItems::Type/4`
	WorkItemTypeTask        WorkItemTypeID = `gid://gitlab/WorkItems::Type/5`
	WorkItemTypeObjective   WorkItemTypeID = `gid://gitlab/WorkItems::Type/6`
	WorkItemTypeKeyResult   WorkItemTypeID = `gid://gitlab/WorkItems::Type/7`
	WorkItemTypeEpic        WorkItemTypeID = `gid://gitlab/WorkItems::Type/8`
	WorkItemTypeTicket      WorkItemTypeID = `gid://gitlab/WorkItems::Type/9`
)

// WorkItemStatusID represents the global ID of a work item status.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemsstatus
//
// Experimental: The Work Items API is a work in progress and may introduce breaking changes even between minor versions.
type WorkItemStatusID string

// WorkItemStatusID constants for the system-defined work item statuses.
const (
	WorkItemStatusToDo       WorkItemStatusID = `gid://gitlab/WorkItems::Statuses::SystemDefined::Status/1`
	WorkItemStatusInProgress WorkItemStatusID = `gid://gitlab/WorkItems::Statuses::SystemDefined::Status/2`
	WorkItemStatusDone       WorkItemStatusID = `gid://gitlab/WorkItems::Statuses::SystemDefined::Status/3`
	WorkItemStatusWontDo     WorkItemStatusID = `gid://gitlab/WorkItems::Statuses::SystemDefined::Status/4`
	WorkItemStatusDuplicate  WorkItemStatusID = `gid://gitlab/WorkItems::Statuses::SystemDefined::Status/5`
)
