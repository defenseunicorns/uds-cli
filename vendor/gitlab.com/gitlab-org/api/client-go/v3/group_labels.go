//
// Copyright 2021, Sander van Harmelen
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

import "net/http"

var (
	routeGroupsIDLabels              = route("groups/%s/labels")
	routeGroupsIDLabelsID            = route("groups/%s/labels/%s")
	routeGroupsIDLabelsIDSubscribe   = route("groups/%s/labels/%s/subscribe")
	routeGroupsIDLabelsIDUnsubscribe = route("groups/%s/labels/%s/unsubscribe")
)

type (
	GroupLabelsServiceInterface interface {
		// ListGroupLabels gets all labels for a given group.
		//
		// GitLab API docs: https://docs.gitlab.com/api/group_labels/#list-group-labels
		ListGroupLabels(gid any, opt *ListGroupLabelsOptions, options ...RequestOptionFunc) ([]*GroupLabel, *Response, error)

		// GetGroupLabel gets a single label for a given group.
		//
		// GitLab API docs: https://docs.gitlab.com/api/group_labels/#get-a-single-group-label
		GetGroupLabel(gid any, lid any, options ...RequestOptionFunc) (*GroupLabel, *Response, error)

		// CreateGroupLabel creates a new label for a given group with given name and color.
		//
		// GitLab API docs: https://docs.gitlab.com/api/group_labels/#create-a-new-group-label
		CreateGroupLabel(gid any, opt *CreateGroupLabelOptions, options ...RequestOptionFunc) (*GroupLabel, *Response, error)

		// DeleteGroupLabel deletes a group label given by its name or ID.
		//
		// GitLab API docs: https://docs.gitlab.com/api/group_labels/#delete-a-group-label
		DeleteGroupLabel(gid any, lid any, opt *DeleteGroupLabelOptions, options ...RequestOptionFunc) (*Response, error)

		// UpdateGroupLabel updates an existing group label. At least one parameter is
		// required, to update the label.
		//
		// GitLab API docs: https://docs.gitlab.com/api/group_labels/#update-a-group-label
		UpdateGroupLabel(gid any, lid any, opt *UpdateGroupLabelOptions, options ...RequestOptionFunc) (*GroupLabel, *Response, error)

		// SubscribeToGroupLabel subscribes the authenticated user to a label to receive
		// notifications. If the user is already subscribed to the label, the status
		// code 304 is returned.
		//
		// GitLab API docs: https://docs.gitlab.com/api/group_labels/#subscribe-to-a-group-label
		SubscribeToGroupLabel(gid any, lid any, options ...RequestOptionFunc) (*GroupLabel, *Response, error)

		// UnsubscribeFromGroupLabel unsubscribes the authenticated user from a label to
		// not receive notifications from it. If the user is not subscribed to the label,
		// the status code 304 is returned.
		//
		// GitLab API docs: https://docs.gitlab.com/api/group_labels/#unsubscribe-from-a-group-label
		UnsubscribeFromGroupLabel(gid any, lid any, options ...RequestOptionFunc) (*Response, error)
	}

	// GroupLabelsService handles communication with the label related methods of the
	// GitLab API.
	//
	// GitLab API docs: https://docs.gitlab.com/api/group_labels/
	GroupLabelsService struct {
		client *Client
	}
)

var _ GroupLabelsServiceInterface = (*GroupLabelsService)(nil)

// GroupLabel represents a GitLab group label.
//
// GitLab API docs: https://docs.gitlab.com/api/group_labels/
type GroupLabel Label

func (l GroupLabel) String() string {
	return Stringify(l)
}

// ListGroupLabelsOptions represents the available ListGroupLabels() options.
//
// GitLab API docs: https://docs.gitlab.com/api/group_labels/#list-group-labels
type ListGroupLabelsOptions struct {
	ListOptions
	WithCounts              *bool   `url:"with_counts,omitempty" json:"with_counts,omitempty"`
	IncludeAncestorGroups   *bool   `url:"include_ancestor_groups,omitempty" json:"include_ancestor_groups,omitempty"`
	IncludeDescendantGroups *bool   `url:"include_descendant_groups,omitempty" json:"include_descendant_groups,omitempty"`
	OnlyGroupLabels         *bool   `url:"only_group_labels,omitempty" json:"only_group_labels,omitempty"`
	Search                  *string `url:"search,omitempty" json:"search,omitempty"`
	Archived                *bool   `url:"archived,omitempty" json:"archived,omitempty"`
}

func (s *GroupLabelsService) ListGroupLabels(gid any, opt *ListGroupLabelsOptions, options ...RequestOptionFunc) ([]*GroupLabel, *Response, error) {
	return do[[]*GroupLabel](s.client,
		withPath(routeGroupsIDLabels, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

func (s *GroupLabelsService) GetGroupLabel(gid any, lid any, options ...RequestOptionFunc) (*GroupLabel, *Response, error) {
	return do[*GroupLabel](s.client,
		withPath(routeGroupsIDLabelsID, GroupID{gid}, LabelID{lid}),
		withRequestOpts(options...),
	)
}

// CreateGroupLabelOptions represents the available CreateGroupLabel() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/group_labels/#create-a-new-group-label
type CreateGroupLabelOptions struct {
	Name        *string         `url:"name,omitempty" json:"name,omitempty"`
	Color       *string         `url:"color,omitempty" json:"color,omitempty"`
	Description *string         `url:"description,omitempty" json:"description,omitempty"`
	Priority    Nullable[int64] `url:"priority,omitempty" json:"priority,omitempty"`
	Archived    *bool           `url:"archived,omitempty" json:"archived,omitempty"`
}

func (s *GroupLabelsService) CreateGroupLabel(gid any, opt *CreateGroupLabelOptions, options ...RequestOptionFunc) (*GroupLabel, *Response, error) {
	return do[*GroupLabel](s.client,
		withMethod(http.MethodPost),
		withPath(routeGroupsIDLabels, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// DeleteGroupLabelOptions represents the available DeleteGroupLabel() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/group_labels/#delete-a-group-label
type DeleteGroupLabelOptions struct {
	Name *string `url:"name,omitempty" json:"name,omitempty"`
}

func (s *GroupLabelsService) DeleteGroupLabel(gid any, lid any, opt *DeleteGroupLabelOptions, options ...RequestOptionFunc) (*Response, error) {
	reqOpts := make([]doOption, 0, 4)
	reqOpts = append(reqOpts,
		withMethod(http.MethodDelete),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)

	if lid != nil {
		reqOpts = append(reqOpts, withPath(routeGroupsIDLabelsID, GroupID{gid}, LabelID{lid}))
	} else {
		reqOpts = append(reqOpts, withPath(routeGroupsIDLabels, GroupID{gid}))
	}

	_, resp, err := do[none](s.client, reqOpts...)
	return resp, err
}

// UpdateGroupLabelOptions represents the available UpdateGroupLabel() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/group_labels/#update-a-group-label
type UpdateGroupLabelOptions struct {
	Name        *string         `url:"name,omitempty" json:"name,omitempty"`
	NewName     *string         `url:"new_name,omitempty" json:"new_name,omitempty"`
	Color       *string         `url:"color,omitempty" json:"color,omitempty"`
	Description *string         `url:"description,omitempty" json:"description,omitempty"`
	Priority    Nullable[int64] `url:"priority,omitempty" json:"priority,omitempty"`
	Archived    *bool           `url:"archived,omitempty" json:"archived,omitempty"`
}

func (s *GroupLabelsService) UpdateGroupLabel(gid any, lid any, opt *UpdateGroupLabelOptions, options ...RequestOptionFunc) (*GroupLabel, *Response, error) {
	reqOpts := make([]doOption, 0, 4)
	reqOpts = append(reqOpts,
		withMethod(http.MethodPut),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)

	if lid != nil {
		reqOpts = append(reqOpts, withPath(routeGroupsIDLabelsID, GroupID{gid}, LabelID{lid}))
	} else {
		reqOpts = append(reqOpts, withPath(routeGroupsIDLabels, GroupID{gid}))
	}

	return do[*GroupLabel](s.client, reqOpts...)
}

func (s *GroupLabelsService) SubscribeToGroupLabel(gid any, lid any, options ...RequestOptionFunc) (*GroupLabel, *Response, error) {
	return do[*GroupLabel](s.client,
		withMethod(http.MethodPost),
		withPath(routeGroupsIDLabelsIDSubscribe, GroupID{gid}, LabelID{lid}),
		withRequestOpts(options...),
	)
}

func (s *GroupLabelsService) UnsubscribeFromGroupLabel(gid any, lid any, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodPost),
		withPath(routeGroupsIDLabelsIDUnsubscribe, GroupID{gid}, LabelID{lid}),
		withRequestOpts(options...),
	)
	return resp, err
}
