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

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var (
	routeGroups                     = route("groups")
	routeGroupsID                   = route("groups/%s")
	routeGroupsIDArchive            = route("groups/%s/archive")
	routeGroupsIDAvatar             = route("groups/%s/avatar")
	routeGroupsIDDescendantGroups   = route("groups/%s/descendant_groups")
	routeGroupsIDGroupsShared       = route("groups/%s/groups/shared")
	routeGroupsIDInvitedGroups      = route("groups/%s/invited_groups")
	routeGroupsIDLdapGroupLinks     = route("groups/%s/ldap_group_links")
	routeGroupsIDLdapGroupLinksID   = route("groups/%s/ldap_group_links/%s")
	routeGroupsIDLdapGroupLinksIDID = route("groups/%s/ldap_group_links/%s/%s")
	routeGroupsIDLdapSync           = route("groups/%s/ldap_sync")
	routeGroupsIDProjects           = route("groups/%s/projects")
	routeGroupsIDProjectsID         = route("groups/%s/projects/%s")
	routeGroupsIDProjectsShared     = route("groups/%s/projects/shared")
	routeGroupsIDProvisionedUsers   = route("groups/%s/provisioned_users")
	routeGroupsIDPushRule           = route("groups/%s/push_rule")
	routeGroupsIDRestore            = route("groups/%s/restore")
	routeGroupsIDSamlGroupLinks     = route("groups/%s/saml_group_links")
	routeGroupsIDSamlGroupLinksID   = route("groups/%s/saml_group_links/%s")
	routeGroupsIDSamlUsers          = route("groups/%s/saml_users")
	routeGroupsIDSubgroups          = route("groups/%s/subgroups")
	routeGroupsIDTransfer           = route("groups/%s/transfer")
	routeGroupsIDTransferLocations  = route("groups/%s/transfer_locations")
	routeGroupsIDUnarchive          = route("groups/%s/unarchive")
)

type (
	GroupsServiceInterface interface {
		// ListGroups gets a list of groups (as user: my groups, as admin: all groups).
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#list-groups
		ListGroups(opt *ListGroupsOptions, options ...RequestOptionFunc) ([]*Group, *Response, error)
		// ListSubGroups gets a list of subgroups for a given group.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#list-subgroups
		ListSubGroups(gid any, opt *ListSubGroupsOptions, options ...RequestOptionFunc) ([]*Group, *Response, error)
		// ListDescendantGroups gets a list of subgroups for a given project.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#list-descendant-groups
		ListDescendantGroups(gid any, opt *ListDescendantGroupsOptions, options ...RequestOptionFunc) ([]*Group, *Response, error)
		// ListGroupProjects get a list of group projects
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#list-projects
		ListGroupProjects(gid any, opt *ListGroupProjectsOptions, options ...RequestOptionFunc) ([]*Project, *Response, error)
		// GetGroup gets all details of a group.
		//
		// GitLab API docs: https://docs.gitlab.com/api/groups/#get-a-single-group
		GetGroup(gid any, opt *GetGroupOptions, options ...RequestOptionFunc) (*Group, *Response, error)
		// DownloadAvatar downloads a group avatar.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#download-a-group-avatar
		DownloadAvatar(gid any, options ...RequestOptionFunc) (*bytes.Reader, *Response, error)
		// CreateGroup creates a new project group. Available only for users who can
		// create groups.
		//
		// When `default_branch_protection_defaults` are defined with an `avatar` value,
		// only one value for `allowed_to_push` and `allowed_to_merge` will be used as
		// the GitLab API only accepts one value for those attributes even when multiples
		// are provided on the request. The API will take the highest permission level.
		// For instance, if 'developer' and 'maintainer' are provided, the API will take 'maintainer'.
		//
		// GitLab API docs: https://docs.gitlab.com/api/groups/#create-a-group
		CreateGroup(opt *CreateGroupOptions, options ...RequestOptionFunc) (*Group, *Response, error)
		// TransferGroup transfers a project to the Group namespace. Available only
		// for admin.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#transfer-a-project-to-a-group
		TransferGroup(gid any, pid any, options ...RequestOptionFunc) (*Group, *Response, error)
		// TransferSubGroup transfers a group to a new parent group or turn a subgroup
		// to a top-level group. Available to administrators and users.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#transfer-a-group
		TransferSubGroup(gid any, opt *TransferSubGroupOptions, options ...RequestOptionFunc) (*Group, *Response, error)
		// UpdateGroup updates an existing group; only available to group owners and
		// administrators.
		//
		// When `default_branch_protection_defaults` are defined with an `avatar` value,
		// only one value for `allowed_to_push` and `allowed_to_merge` will be used as
		// the GitLab API only accepts one value for those attributes even when multiples
		// are provided on the request. The API will take the highest permission level.
		// For instance, if 'developer' and 'maintainer' are provided, the API will take 'maintainer'.
		//
		// GitLab API docs: https://docs.gitlab.com/api/groups/#update-group-attributes
		UpdateGroup(gid any, opt *UpdateGroupOptions, options ...RequestOptionFunc) (*Group, *Response, error)
		// UploadAvatar uploads a group avatar.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#upload-a-group-avatar
		UploadAvatar(gid any, avatar io.Reader, filename string, options ...RequestOptionFunc) (*Group, *Response, error)
		// DeleteGroup removes group with all projects inside.
		//
		// GitLab API docs: https://docs.gitlab.com/api/groups/#delete-a-group
		DeleteGroup(gid any, opt *DeleteGroupOptions, options ...RequestOptionFunc) (*Response, error)
		// RestoreGroup restores a previously deleted group
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#restore-a-group-marked-for-deletion
		RestoreGroup(gid any, options ...RequestOptionFunc) (*Group, *Response, error)
		// SearchGroup get all groups that match your string in their name or path.
		//
		// GitLab API docs: https://docs.gitlab.com/api/groups/#search-for-a-group
		SearchGroup(query string, options ...RequestOptionFunc) ([]*Group, *Response, error)
		// ListProvisionedUsers gets a list of users provisioned by the given group.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#list-provisioned-users
		ListProvisionedUsers(gid any, opt *ListProvisionedUsersOptions, options ...RequestOptionFunc) ([]*User, *Response, error)
		// ListGroupLDAPLinks lists the group's LDAP links. Available only for users who
		// can edit groups.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/group_ldap_links/#list-ldap-group-links
		ListGroupLDAPLinks(gid any, options ...RequestOptionFunc) ([]*LDAPGroupLink, *Response, error)
		// AddGroupLDAPLink creates a new group LDAP link. Available only for users who
		// can edit groups.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/group_ldap_links/#add-an-ldap-group-link-with-cn-or-filter
		AddGroupLDAPLink(gid any, opt *AddGroupLDAPLinkOptions, options ...RequestOptionFunc) (*LDAPGroupLink, *Response, error)
		// DeleteGroupLDAPLink deletes a group LDAP link. Available only for users who
		// can edit groups.
		// Deprecated as upstream API is deprecated. Use DeleteGroupLDAPLinkWithCNOrFilter() instead.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/group_ldap_links/#delete-an-ldap-group-link-deprecated
		DeleteGroupLDAPLink(gid any, cn string, options ...RequestOptionFunc) (*Response, error)
		// DeleteGroupLDAPLinkWithCNOrFilter deletes a group LDAP link. Available only for users who
		// can edit groups.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/group_ldap_links/#delete-an-ldap-group-link-with-cn-or-filter
		DeleteGroupLDAPLinkWithCNOrFilter(gid any, opts *DeleteGroupLDAPLinkWithCNOrFilterOptions, options ...RequestOptionFunc) (*Response, error)
		// DeleteGroupLDAPLinkForProvider deletes a group LDAP link from a specific
		// provider. Available only for users who can edit groups.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/group_ldap_links/#delete-an-ldap-group-link-deprecated
		DeleteGroupLDAPLinkForProvider(gid any, provider, cn string, options ...RequestOptionFunc) (*Response, error)
		// ListGroupSAMLLinks lists the group's SAML links. Available only for users who
		// can edit groups.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/saml/#list-saml-group-links
		ListGroupSAMLLinks(gid any, options ...RequestOptionFunc) ([]*SAMLGroupLink, *Response, error)
		// ListGroupSharedProjects gets a list of projects shared to this group.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#list-shared-projects
		ListGroupSharedProjects(gid any, opt *ListGroupSharedProjectsOptions, options ...RequestOptionFunc) ([]*Project, *Response, error)
		// GetGroupSAMLLink get a specific group SAML link. Available only for users who
		// can edit groups.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/saml/#get-a-saml-group-link
		GetGroupSAMLLink(gid any, samlGroupName string, options ...RequestOptionFunc) (*SAMLGroupLink, *Response, error)
		// AddGroupSAMLLink creates a new group SAML link. Available only for users who
		// can edit groups.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/saml/#add-a-saml-group-link
		AddGroupSAMLLink(gid any, opt *AddGroupSAMLLinkOptions, options ...RequestOptionFunc) (*SAMLGroupLink, *Response, error)
		// DeleteGroupSAMLLink deletes a group SAML link. Available only for users who
		// can edit groups.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/saml/#delete-a-saml-group-link
		DeleteGroupSAMLLink(gid any, samlGroupName string, options ...RequestOptionFunc) (*Response, error)
		// ShareGroupWithGroup shares a group with another group.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#create-a-link-to-share-a-group-with-another-group
		ShareGroupWithGroup(gid any, opt *ShareGroupWithGroupOptions, options ...RequestOptionFunc) (*Group, *Response, error)
		// UnshareGroupFromGroup unshares a group from another group.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#delete-the-link-that-shares-a-group-with-another-group
		UnshareGroupFromGroup(gid any, groupID int64, options ...RequestOptionFunc) (*Response, error)
		// GetGroupPushRules gets the push rules of a group.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/group_push_rules/#get-the-push-rules-of-a-group
		GetGroupPushRules(gid any, options ...RequestOptionFunc) (*GroupPushRules, *Response, error)
		// AddGroupPushRule adds push rules to the specified group.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/group_push_rules/#add-push-rules-to-a-group
		AddGroupPushRule(gid any, opt *AddGroupPushRuleOptions, options ...RequestOptionFunc) (*GroupPushRules, *Response, error)
		// EditGroupPushRule edits a push rule for a specified group.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/group_push_rules/#edit-the-push-rules-of-a-group
		EditGroupPushRule(gid any, opt *EditGroupPushRuleOptions, options ...RequestOptionFunc) (*GroupPushRules, *Response, error)
		// DeleteGroupPushRule deletes the push rules of a group.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/group_push_rules/#delete-the-push-rules-of-a-group
		DeleteGroupPushRule(gid any, options ...RequestOptionFunc) (*Response, error)
		// ArchiveGroup archives a group.
		// You must be an administrator or have the Owner role for the group.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#archive-a-group
		ArchiveGroup(gid any, options ...RequestOptionFunc) (*Response, error)
		// UnarchiveGroup unarchives a group.
		// You must be an administrator or have the Owner role for the group.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#unarchive-a-group
		UnarchiveGroup(gid any, options ...RequestOptionFunc) (*Response, error)
		// ListSAMLUsers gets a list of SAML users provisioned for the given top-level
		// group.
		//
		// GitLab API docs: https://docs.gitlab.com/api/groups/#list-all-saml-users
		ListSAMLUsers(gid any, opt *ListSAMLUsersOptions, options ...RequestOptionFunc) ([]*User, *Response, error)
		// ListGroupsSharedWith gets a list of groups that have been shared with the
		// given group.
		//
		// GitLab API docs: https://docs.gitlab.com/api/groups/#list-shared-groups
		ListGroupsSharedWith(gid any, opt *ListGroupsSharedWithOptions, options ...RequestOptionFunc) ([]*Group, *Response, error)
		// ListInvitedGroups gets a list of groups invited to the given group.
		//
		// GitLab API docs: https://docs.gitlab.com/api/groups/#list-invited-groups
		ListInvitedGroups(gid any, opt *ListInvitedGroupsOptions, options ...RequestOptionFunc) ([]*Group, *Response, error)
		// ListTransferLocations gets the possible parent groups available for
		// transferring the given group.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#list-all-locations-available-for-group-transfer
		ListTransferLocations(gid any, opt *ListTransferLocationsOptions, options ...RequestOptionFunc) ([]*TransferLocation, *Response, error)
		// SyncGroupWithLDAP triggers a synchronization of the group's LDAP group links.
		//
		// GitLab API docs: https://docs.gitlab.com/api/groups/#sync-a-group-with-ldap
		SyncGroupWithLDAP(gid any, options ...RequestOptionFunc) (*Response, error)

		// group_hooks.go
		ListGroupHooks(gid any, opt *ListGroupHooksOptions, options ...RequestOptionFunc) ([]*GroupHook, *Response, error)
		GetGroupHook(gid any, hook int64, options ...RequestOptionFunc) (*GroupHook, *Response, error)
		ResendGroupHookEvent(gid any, hook int64, hookEventID int64, options ...RequestOptionFunc) (*Response, error)
		AddGroupHook(gid any, opt *AddGroupHookOptions, options ...RequestOptionFunc) (*GroupHook, *Response, error)
		EditGroupHook(gid any, hook int64, opt *EditGroupHookOptions, options ...RequestOptionFunc) (*GroupHook, *Response, error)
		DeleteGroupHook(gid any, hook int64, options ...RequestOptionFunc) (*Response, error)
		TriggerTestGroupHook(pid any, hook int64, trigger GroupHookTrigger, options ...RequestOptionFunc) (*Response, error)
		SetGroupCustomHeader(gid any, hook int64, key string, opt *SetHookCustomHeaderOptions, options ...RequestOptionFunc) (*Response, error)
		DeleteGroupCustomHeader(gid any, hook int64, key string, options ...RequestOptionFunc) (*Response, error)
		SetGroupHookURLVariable(gid any, hook int64, key string, opt *SetHookURLVariableOptions, options ...RequestOptionFunc) (*Response, error)
		DeleteGroupHookURLVariable(gid any, hook int64, key string, options ...RequestOptionFunc) (*Response, error)

		// group_serviceaccounts.go
		ListServiceAccounts(gid any, opt *ListServiceAccountsOptions, options ...RequestOptionFunc) ([]*GroupServiceAccount, *Response, error)
		GetServiceAccount(gid any, serviceAccount int64, options ...RequestOptionFunc) (*GroupServiceAccount, *Response, error)
		CreateServiceAccount(gid any, opt *CreateServiceAccountOptions, options ...RequestOptionFunc) (*GroupServiceAccount, *Response, error)
		UpdateServiceAccount(gid any, serviceAccount int64, opt *UpdateServiceAccountOptions, options ...RequestOptionFunc) (*GroupServiceAccount, *Response, error)
		DeleteServiceAccount(gid any, serviceAccount int64, opt *DeleteServiceAccountOptions, options ...RequestOptionFunc) (*Response, error)
		ListServiceAccountPersonalAccessTokens(gid any, serviceAccount int64, opt *ListServiceAccountPersonalAccessTokensOptions, options ...RequestOptionFunc) ([]*PersonalAccessToken, *Response, error)
		CreateServiceAccountPersonalAccessToken(gid any, serviceAccount int64, opt *CreateServiceAccountPersonalAccessTokenOptions, options ...RequestOptionFunc) (*PersonalAccessToken, *Response, error)
		RevokeServiceAccountPersonalAccessToken(gid any, serviceAccount, token int64, options ...RequestOptionFunc) (*Response, error)
		RotateServiceAccountPersonalAccessToken(gid any, serviceAccount, token int64, opt *RotateServiceAccountPersonalAccessTokenOptions, options ...RequestOptionFunc) (*PersonalAccessToken, *Response, error)

		// group_members.go
		ListGroupMembers(gid any, opt *ListGroupMembersOptions, options ...RequestOptionFunc) ([]*GroupMember, *Response, error)
		ListAllGroupMembers(gid any, opt *ListGroupMembersOptions, options ...RequestOptionFunc) ([]*GroupMember, *Response, error)
		ListBillableGroupMembers(gid any, opt *ListBillableGroupMembersOptions, options ...RequestOptionFunc) ([]*BillableGroupMember, *Response, error)
		ListMembershipsForBillableGroupMember(gid any, user int64, opt *ListMembershipsForBillableGroupMemberOptions, options ...RequestOptionFunc) ([]*BillableUserMembership, *Response, error)
		RemoveBillableGroupMember(gid any, user int64, options ...RequestOptionFunc) (*Response, error)
	}

	// GroupsService handles communication with the group related methods of
	// the GitLab API.
	//
	// GitLab API docs: https://docs.gitlab.com/api/groups/
	GroupsService struct {
		client *Client
	}
)

var _ GroupsServiceInterface = (*GroupsService)(nil)

// Group represents a GitLab group.
//
// GitLab API docs: https://docs.gitlab.com/api/groups/
type Group struct {
	ID                                   int64                         `json:"id"`
	Name                                 string                        `json:"name"`
	Path                                 string                        `json:"path"`
	Description                          string                        `json:"description"`
	MembershipLock                       bool                          `json:"membership_lock"`
	Visibility                           VisibilityValue               `json:"visibility"`
	LFSEnabled                           bool                          `json:"lfs_enabled"`
	MaxArtifactsSize                     int64                         `json:"max_artifacts_size"`
	DefaultBranch                        string                        `json:"default_branch"`
	DefaultBranchProtectionDefaults      *BranchProtectionDefaults     `json:"default_branch_protection_defaults"`
	AvatarURL                            string                        `json:"avatar_url"`
	WebURL                               string                        `json:"web_url"`
	RequestAccessEnabled                 bool                          `json:"request_access_enabled"`
	RepositoryStorage                    string                        `json:"repository_storage"`
	FullName                             string                        `json:"full_name"`
	FullPath                             string                        `json:"full_path"`
	FileTemplateProjectID                int64                         `json:"file_template_project_id"`
	ParentID                             int64                         `json:"parent_id"`
	OrganizationID                       int64                         `json:"organization_id"`
	Statistics                           *Statistics                   `json:"statistics"`
	RootStorageStatistics                *RootStorageStatistics        `json:"root_storage_statistics"`
	CustomAttributes                     []*CustomAttribute            `json:"custom_attributes"`
	ShareWithGroupLock                   bool                          `json:"share_with_group_lock"`
	PreventSharingGroupsOutsideHierarchy bool                          `json:"prevent_sharing_groups_outside_hierarchy"`
	RequireTwoFactorAuth                 bool                          `json:"require_two_factor_authentication"`
	TwoFactorGracePeriod                 int64                         `json:"two_factor_grace_period"`
	ProjectCreationLevel                 ProjectCreationLevelValue     `json:"project_creation_level"`
	AutoDevopsEnabled                    bool                          `json:"auto_devops_enabled"`
	SubGroupCreationLevel                SubGroupCreationLevelValue    `json:"subgroup_creation_level"`
	EmailsEnabled                        bool                          `json:"emails_enabled"`
	MentionsDisabled                     bool                          `json:"mentions_disabled"`
	RunnersToken                         string                        `json:"runners_token"`
	SharedRunnersSetting                 SharedRunnersSettingValue     `json:"shared_runners_setting"`
	SharedWithGroups                     []SharedWithGroup             `json:"shared_with_groups"`
	LDAPCN                               string                        `json:"ldap_cn"`
	LDAPAccess                           AccessLevelValue              `json:"ldap_access"`
	LDAPGroupLinks                       []*LDAPGroupLink              `json:"ldap_group_links"`
	SAMLGroupLinks                       []*SAMLGroupLink              `json:"saml_group_links"`
	SharedRunnersMinutesLimit            int64                         `json:"shared_runners_minutes_limit"`
	ExtraSharedRunnersMinutesLimit       int64                         `json:"extra_shared_runners_minutes_limit"`
	PreventForkingOutsideGroup           bool                          `json:"prevent_forking_outside_group"`
	Archived                             bool                          `json:"archived"`
	MarkedForDeletionOn                  *ISOTime                      `json:"marked_for_deletion_on"`
	CreatedAt                            *time.Time                    `json:"created_at"`
	IPRestrictionRanges                  string                        `json:"ip_restriction_ranges"`
	AllowedEmailDomainsList              string                        `json:"allowed_email_domains_list"`
	WikiAccessLevel                      AccessControlValue            `json:"wiki_access_level"`
	EnabledGitAccessProtocol             EnabledGitAccessProtocolValue `json:"enabled_git_access_protocol"`

	OnlyAllowMergeIfPipelineSucceeds          bool `json:"only_allow_merge_if_pipeline_succeeds"`
	AllowMergeOnSkippedPipeline               bool `json:"allow_merge_on_skipped_pipeline"`
	OnlyAllowMergeIfAllDiscussionsAreResolved bool `json:"only_allow_merge_if_all_discussions_are_resolved"`

	// Disclaimer: Experimental features may have breaking changes even between minor versions
	DuoAvailability           DuoAvailabilityValue `json:"duo_availability"`
	ExperimentFeaturesEnabled bool                 `json:"experiment_features_enabled"`
	DuoFeaturesEnabled        bool                 `json:"duo_features_enabled"`
	LockDuoFeaturesEnabled    bool                 `json:"lock_duo_features_enabled"`

	// Math rendering
	MathRenderingLimitsEnabled     bool `json:"math_rendering_limits_enabled"`
	LockMathRenderingLimitsEnabled bool `json:"lock_math_rendering_limits_enabled"`

	// Customer relations management
	CRMEnabled bool `json:"crm_enabled"`

	// Deprecated: will be removed in v5 of the API, use ListGroupProjects instead
	Projects []*Project `json:"projects"`

	// Deprecated: will be removed in v5 of the API, use ListGroupSharedProjects instead
	SharedProjects []*Project `json:"shared_projects"`

	// Deprecated: Use EmailsEnabled instead
	EmailsDisabled bool `json:"emails_disabled"`

	// Deprecated: Use DefaultBranchProtectionDefaults instead
	DefaultBranchProtection int64 `json:"default_branch_protection"`
}

// RootStorageStatistics represents the storage statistics of a namespace,
// aggregated over the whole hierarchy of the top-level group. It is only
// returned for top-level groups, and only when statistics are requested.
//
// Unlike the per-group Statistics, it also reports the container registry
// and dependency proxy sizes.
//
// GitLab API docs: https://docs.gitlab.com/api/groups/#list-all-groups
type RootStorageStatistics struct {
	BuildArtifactsSize               int64 `json:"build_artifacts_size"`
	ContainerRegistrySize            int64 `json:"container_registry_size"`
	ContainerRegistrySizeIsEstimated bool  `json:"container_registry_size_is_estimated"`
	DependencyProxySize              int64 `json:"dependency_proxy_size"`
	LFSObjectsSize                   int64 `json:"lfs_objects_size"`
	PackagesSize                     int64 `json:"packages_size"`
	PipelineArtifactsSize            int64 `json:"pipeline_artifacts_size"`
	RepositorySize                   int64 `json:"repository_size"`
	SnippetsSize                     int64 `json:"snippets_size"`
	StorageSize                      int64 `json:"storage_size"`
	UploadsSize                      int64 `json:"uploads_size"`
	WikiSize                         int64 `json:"wiki_size"`
}

// SharedWithGroup represents a GitLab group shared with a group.
//
// GitLab API docs: https://docs.gitlab.com/api/groups/
type SharedWithGroup struct {
	GroupID          int64    `json:"group_id"`
	GroupName        string   `json:"group_name"`
	GroupFullPath    string   `json:"group_full_path"`
	GroupAccessLevel int64    `json:"group_access_level"`
	ExpiresAt        *ISOTime `json:"expires_at"`
	MemberRoleID     int64    `json:"member_role_id"`
}

// BranchProtectionDefaults represents default Git protected branch permissions.
//
// GitLab API docs:
// https://docs.gitlab.com/api/groups/#options-for-default_branch_protection_defaults
type BranchProtectionDefaults struct {
	AllowedToPush             []*GroupAccessLevel `json:"allowed_to_push,omitempty"`
	AllowForcePush            bool                `json:"allow_force_push,omitempty"`
	AllowedToMerge            []*GroupAccessLevel `json:"allowed_to_merge,omitempty"`
	DeveloperCanInitialPush   bool                `json:"developer_can_initial_push,omitempty"`
	CodeOwnerApprovalRequired bool                `json:"code_owner_approval_required,omitempty"`
}

// GroupAccessLevel represents default branch protection defaults access levels.
//
// GitLab API docs:
// https://docs.gitlab.com/api/groups/#options-for-default_branch_protection_defaults
type GroupAccessLevel struct {
	AccessLevel *AccessLevelValue `url:"access_level,omitempty" json:"access_level,omitempty"`
}

// GroupAvatar represents a GitLab group avatar.
//
// GitLab API docs: https://docs.gitlab.com/api/groups/
type GroupAvatar struct {
	Filename string
	Image    io.Reader
}

// MarshalJSON implements the json.Marshaler interface.
func (a *GroupAvatar) MarshalJSON() ([]byte, error) {
	if a.Filename == "" && a.Image == nil {
		return []byte(`""`), nil
	}
	type alias GroupAvatar
	return json.Marshal((*alias)(a))
}

// LDAPGroupLink represents a GitLab LDAP group link.
//
// GitLab API docs:
// https://docs.gitlab.com/api/group_ldap_links/
type LDAPGroupLink struct {
	CN           string           `json:"cn"`
	Filter       string           `json:"filter"`
	GroupAccess  AccessLevelValue `json:"group_access"`
	Provider     string           `json:"provider"`
	MemberRoleID int64            `json:"member_role_id"`
}

// SAMLGroupLink represents a GitLab SAML group link.
//
// GitLab API docs: https://docs.gitlab.com/api/groups/#saml-group-links
type SAMLGroupLink struct {
	Name         string           `json:"name"`
	AccessLevel  AccessLevelValue `json:"access_level"`
	MemberRoleID int64            `json:"member_role_id,omitempty"`
	Provider     string           `json:"provider,omitempty"`
}

// ListGroupsOptions represents the available ListGroups() options.
//
// GitLab API docs: https://docs.gitlab.com/api/groups/#list-groups
type ListGroupsOptions struct {
	ListOptions
	SkipGroups           *[]int64          `url:"skip_groups,omitempty" del:"," json:"skip_groups,omitempty"`
	AllAvailable         *bool             `url:"all_available,omitempty" json:"all_available,omitempty"`
	Search               *string           `url:"search,omitempty" json:"search,omitempty"`
	OrderBy              *string           `url:"order_by,omitempty" json:"order_by,omitempty"`
	Sort                 *string           `url:"sort,omitempty" json:"sort,omitempty"`
	Statistics           *bool             `url:"statistics,omitempty" json:"statistics,omitempty"`
	Visibility           *VisibilityValue  `url:"visibility,omitempty" json:"visibility,omitempty"`
	WithCustomAttributes *bool             `url:"with_custom_attributes,omitempty" json:"with_custom_attributes,omitempty"`
	Owned                *bool             `url:"owned,omitempty" json:"owned,omitempty"`
	MinAccessLevel       *AccessLevelValue `url:"min_access_level,omitempty" json:"min_access_level,omitempty"`
	TopLevelOnly         *bool             `url:"top_level_only,omitempty" json:"top_level_only,omitempty"`
	RepositoryStorage    *string           `url:"repository_storage,omitempty" json:"repository_storage,omitempty"`
	MarkedForDeletionOn  *ISOTime          `url:"marked_for_deletion_on,omitempty" json:"marked_for_deletion_on,omitempty"`
	Active               *bool             `url:"active,omitempty" json:"active,omitempty"`
	Archived             *bool             `url:"archived,omitempty" json:"archived,omitempty"`

	// CustomAttributes filters groups by custom attributes, producing query
	// parameters of the form custom_attributes[key]=value. This is distinct
	// from WithCustomAttributes, which only controls whether custom attributes
	// are included in the response.
	CustomAttributes CustomAttributesFilter `url:"custom_attributes,omitempty" json:"custom_attributes,omitempty"`
}

func (s *GroupsService) ListGroups(opt *ListGroupsOptions, options ...RequestOptionFunc) ([]*Group, *Response, error) {
	return do[[]*Group](s.client,
		withPath(routeGroups),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// ListSubGroupsOptions represents the available ListSubGroups() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/groups/#list-subgroups
type ListSubGroupsOptions ListGroupsOptions

func (s *GroupsService) ListSubGroups(gid any, opt *ListSubGroupsOptions, options ...RequestOptionFunc) ([]*Group, *Response, error) {
	return do[[]*Group](s.client,
		withPath(routeGroupsIDSubgroups, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// ListDescendantGroupsOptions represents the available ListDescendantGroups()
// options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/groups/#list-descendant-groups
type ListDescendantGroupsOptions ListGroupsOptions

func (s *GroupsService) ListDescendantGroups(gid any, opt *ListDescendantGroupsOptions, options ...RequestOptionFunc) ([]*Group, *Response, error) {
	return do[[]*Group](s.client,
		withPath(routeGroupsIDDescendantGroups, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// ListGroupProjectsOptions represents the available ListGroupProjects() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/groups/#list-projects
type ListGroupProjectsOptions struct {
	ListOptions
	Active                   *bool             `url:"active,omitempty" json:"active,omitempty"`
	Archived                 *bool             `url:"archived,omitempty" json:"archived,omitempty"`
	IncludeSubGroups         *bool             `url:"include_subgroups,omitempty" json:"include_subgroups,omitempty"`
	MinAccessLevel           *AccessLevelValue `url:"min_access_level,omitempty" json:"min_access_level,omitempty"`
	OrderBy                  *string           `url:"order_by,omitempty" json:"order_by,omitempty"`
	Owned                    *bool             `url:"owned,omitempty" json:"owned,omitempty"`
	Search                   *string           `url:"search,omitempty" json:"search,omitempty"`
	Simple                   *bool             `url:"simple,omitempty" json:"simple,omitempty"`
	Sort                     *string           `url:"sort,omitempty" json:"sort,omitempty"`
	Starred                  *bool             `url:"starred,omitempty" json:"starred,omitempty"`
	Topic                    *string           `url:"topic,omitempty" json:"topic,omitempty"`
	Visibility               *VisibilityValue  `url:"visibility,omitempty" json:"visibility,omitempty"`
	WithCustomAttributes     *bool             `url:"with_custom_attributes,omitempty" json:"with_custom_attributes,omitempty"`
	WithIssuesEnabled        *bool             `url:"with_issues_enabled,omitempty" json:"with_issues_enabled,omitempty"`
	WithMergeRequestsEnabled *bool             `url:"with_merge_requests_enabled,omitempty" json:"with_merge_requests_enabled,omitempty"`
	WithSecurityReports      *bool             `url:"with_security_reports,omitempty" json:"with_security_reports,omitempty"`
	WithShared               *bool             `url:"with_shared,omitempty" json:"with_shared,omitempty"`
}

func (s *GroupsService) ListGroupProjects(gid any, opt *ListGroupProjectsOptions, options ...RequestOptionFunc) ([]*Project, *Response, error) {
	return do[[]*Project](s.client,
		withPath(routeGroupsIDProjects, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// GetGroupOptions represents the available GetGroup() options.
//
// GitLab API docs: https://docs.gitlab.com/api/groups/#get-a-single-group
type GetGroupOptions struct {
	ListOptions
	WithCustomAttributes *bool `url:"with_custom_attributes,omitempty" json:"with_custom_attributes,omitempty"`

	// Deprecated: will be removed in v5 of the API, use ListGroupProjects instead
	WithProjects *bool `url:"with_projects,omitempty" json:"with_projects,omitempty"`
}

func (s *GroupsService) GetGroup(gid any, opt *GetGroupOptions, options ...RequestOptionFunc) (*Group, *Response, error) {
	return do[*Group](s.client,
		withPath(routeGroupsID, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

func (s *GroupsService) DownloadAvatar(gid any, options ...RequestOptionFunc) (*bytes.Reader, *Response, error) {
	buf, resp, err := do[bytes.Buffer](s.client,
		withPath(routeGroupsIDAvatar, GroupID{gid}),
		withRequestOpts(options...),
	)
	if err != nil {
		return nil, resp, err
	}
	return bytes.NewReader(buf.Bytes()), resp, nil
}

// CreateGroupOptions represents the available CreateGroup() options.
//
// GitLab API docs: https://docs.gitlab.com/api/groups/#create-a-group
type CreateGroupOptions struct {
	Name                            *string                                 `url:"name,omitempty" json:"name,omitempty"`
	Path                            *string                                 `url:"path,omitempty" json:"path,omitempty"`
	Avatar                          *GroupAvatar                            `url:"-" json:"-"`
	DefaultBranch                   *string                                 `url:"default_branch,omitempty" json:"default_branch,omitempty"`
	Description                     *string                                 `url:"description,omitempty" json:"description,omitempty"`
	MembershipLock                  *bool                                   `url:"membership_lock,omitempty" json:"membership_lock,omitempty"`
	Visibility                      *VisibilityValue                        `url:"visibility,omitempty" json:"visibility,omitempty"`
	ShareWithGroupLock              *bool                                   `url:"share_with_group_lock,omitempty" json:"share_with_group_lock,omitempty"`
	RequireTwoFactorAuth            *bool                                   `url:"require_two_factor_authentication,omitempty" json:"require_two_factor_authentication,omitempty"`
	TwoFactorGracePeriod            *int64                                  `url:"two_factor_grace_period,omitempty" json:"two_factor_grace_period,omitempty"`
	ProjectCreationLevel            *ProjectCreationLevelValue              `url:"project_creation_level,omitempty" json:"project_creation_level,omitempty"`
	AutoDevopsEnabled               *bool                                   `url:"auto_devops_enabled,omitempty" json:"auto_devops_enabled,omitempty"`
	SubGroupCreationLevel           *SubGroupCreationLevelValue             `url:"subgroup_creation_level,omitempty" json:"subgroup_creation_level,omitempty"`
	EmailsEnabled                   *bool                                   `url:"emails_enabled,omitempty" json:"emails_enabled,omitempty"`
	MentionsDisabled                *bool                                   `url:"mentions_disabled,omitempty" json:"mentions_disabled,omitempty"`
	LFSEnabled                      *bool                                   `url:"lfs_enabled,omitempty" json:"lfs_enabled,omitempty"`
	DefaultBranchProtectionDefaults *DefaultBranchProtectionDefaultsOptions `url:"default_branch_protection_defaults,omitempty" json:"default_branch_protection_defaults,omitempty"`
	RequestAccessEnabled            *bool                                   `url:"request_access_enabled,omitempty" json:"request_access_enabled,omitempty"`
	ParentID                        *int64                                  `url:"parent_id,omitempty" json:"parent_id,omitempty"`
	SharedRunnersMinutesLimit       *int64                                  `url:"shared_runners_minutes_limit,omitempty" json:"shared_runners_minutes_limit,omitempty"`
	ExtraSharedRunnersMinutesLimit  *int64                                  `url:"extra_shared_runners_minutes_limit,omitempty" json:"extra_shared_runners_minutes_limit,omitempty"`
	WikiAccessLevel                 *AccessControlValue                     `url:"wiki_access_level,omitempty" json:"wiki_access_level,omitempty"`

	// Deprecated: Use EmailsEnabled instead
	EmailsDisabled *bool `url:"emails_disabled,omitempty" json:"emails_disabled,omitempty"`

	// Deprecated: User DefaultBranchProtectionDefaults instead
	DefaultBranchProtection *int64 `url:"default_branch_protection,omitempty" json:"default_branch_protection,omitempty"`

	EnabledGitAccessProtocol  *EnabledGitAccessProtocolValue `url:"enabled_git_access_protocol,omitempty" json:"enabled_git_access_protocol,omitempty"`
	OrganizationID            *int64                         `url:"organization_id,omitempty" json:"organization_id,omitempty"`
	DuoAvailability           *DuoAvailabilityValue          `url:"duo_availability,omitempty" json:"duo_availability,omitempty"`
	ExperimentFeaturesEnabled *bool                          `url:"experiment_features_enabled,omitempty" json:"experiment_features_enabled,omitempty"`

	UniqueProjectDownloadLimit                  *int64    `url:"unique_project_download_limit,omitempty" json:"unique_project_download_limit,omitempty"`
	UniqueProjectDownloadLimitIntervalInSeconds *int64    `url:"unique_project_download_limit_interval_in_seconds,omitempty" json:"unique_project_download_limit_interval_in_seconds,omitempty"`
	UniqueProjectDownloadLimitAllowlist         *[]string `url:"unique_project_download_limit_allowlist,omitempty" json:"unique_project_download_limit_allowlist,omitempty"`
	UniqueProjectDownloadLimitAlertlist         *[]int64  `url:"unique_project_download_limit_alertlist,omitempty" json:"unique_project_download_limit_alertlist,omitempty"`
	AutoBanUserOnExcessiveProjectsDownload      *bool     `url:"auto_ban_user_on_excessive_projects_download,omitempty" json:"auto_ban_user_on_excessive_projects_download,omitempty"`

	MathRenderingLimitsEnabled *bool `url:"math_rendering_limits_enabled,omitempty" json:"math_rendering_limits_enabled,omitempty"`

	CRMEnabled *bool `url:"crm_enabled,omitempty" json:"crm_enabled,omitempty"`

	WebBasedCommitSigningEnabled *bool `url:"web_based_commit_signing_enabled,omitempty" json:"web_based_commit_signing_enabled,omitempty"`
	AllowPersonalSnippets        *bool `url:"allow_personal_snippets,omitempty" json:"allow_personal_snippets,omitempty"`
}

// DefaultBranchProtectionDefaultsOptions represents the available options for
// using default_branch_protection_defaults in CreateGroup() or UpdateGroup()
//
// GitLab API docs:
// https://docs.gitlab.com/api/groups/#options-for-default_branch_protection_defaults
type DefaultBranchProtectionDefaultsOptions struct {
	AllowedToPush             []*GroupAccessLevel `url:"allowed_to_push,omitempty" json:"allowed_to_push,omitzero"`
	AllowForcePush            *bool               `url:"allow_force_push,omitempty" json:"allow_force_push,omitempty"`
	AllowedToMerge            []*GroupAccessLevel `url:"allowed_to_merge,omitempty" json:"allowed_to_merge,omitzero"`
	DeveloperCanInitialPush   *bool               `url:"developer_can_initial_push,omitempty" json:"developer_can_initial_push,omitempty"`
	CodeOwnerApprovalRequired *bool               `url:"code_owner_approval_required,omitempty" json:"code_owner_approval_required,omitempty"`
}

// EncodeValues implements the query.Encoder interface
func (d *DefaultBranchProtectionDefaultsOptions) EncodeValues(key string, v *url.Values) error {
	if d.AllowForcePush != nil {
		v.Add(key+"[allow_force_push]", strconv.FormatBool(*d.AllowForcePush))
	}
	if d.DeveloperCanInitialPush != nil {
		v.Add(key+"[developer_can_initial_push]", strconv.FormatBool(*d.DeveloperCanInitialPush))
	}
	if d.CodeOwnerApprovalRequired != nil {
		v.Add(key+"[code_owner_approval_required]", strconv.FormatBool(*d.CodeOwnerApprovalRequired))
	}
	// The GitLab API only accepts one value for `allowed_to_merge` even when multiples are
	// provided on the request.  The API will take the highest permission level.  For instance,
	// if 'developer' and 'maintainer' are provided, the API will take 'maintainer'.
	for _, atm := range d.AllowedToMerge {
		if atm != nil {
			v.Add(key+"[allowed_to_merge][][access_level]", strconv.Itoa(int(*atm.AccessLevel)))
		}
	}
	// The GitLab API only accepts one value for `allowed_to_push` even when multiples are
	// provided on the request.  The API will take the highest permission level.  For instance,
	// if 'developer' and 'maintainer' are provided, the API will take 'maintainer'.
	for _, atp := range d.AllowedToPush {
		if atp != nil {
			v.Add(key+"[allowed_to_push][][access_level]", strconv.Itoa(int(*atp.AccessLevel)))
		}
	}

	return nil
}

func (s *GroupsService) CreateGroup(opt *CreateGroupOptions, options ...RequestOptionFunc) (*Group, *Response, error) {
	reqOpts := []doOption{
		withMethod(http.MethodPost),
		withPath(routeGroups),
		withAPIOpts(opt),
		withRequestOpts(options...),
	}
	if opt.Avatar != nil {
		if opt.DefaultBranchProtectionDefaults != nil && (len(opt.DefaultBranchProtectionDefaults.AllowedToMerge) > 1 || len(opt.DefaultBranchProtectionDefaults.AllowedToPush) > 1) {
			return nil, nil, errors.New("multiple access levels for allowed_to_merge or allowed_to_push are not permitted when an Avatar is also specified as it will result in unexpected behavior")
		}
		reqOpts = append(reqOpts, withUpload(opt.Avatar.Image, opt.Avatar.Filename, UploadAvatar))
	}
	return do[*Group](s.client, reqOpts...)
}

func (s *GroupsService) TransferGroup(gid any, pid any, options ...RequestOptionFunc) (*Group, *Response, error) {
	return do[*Group](s.client,
		withMethod(http.MethodPost),
		withPath(routeGroupsIDProjectsID, GroupID{gid}, ProjectID{pid}),
		withRequestOpts(options...),
	)
}

// TransferSubGroupOptions represents the available TransferSubGroup() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/groups/#transfer-a-group
type TransferSubGroupOptions struct {
	GroupID *int64 `url:"group_id,omitempty" json:"group_id,omitempty"`
}

func (s *GroupsService) TransferSubGroup(gid any, opt *TransferSubGroupOptions, options ...RequestOptionFunc) (*Group, *Response, error) {
	return do[*Group](s.client,
		withMethod(http.MethodPost),
		withPath(routeGroupsIDTransfer, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// UpdateGroupOptions represents the available UpdateGroup() options.
//
// GitLab API docs: https://docs.gitlab.com/api/groups/#update-group-attributes
type UpdateGroupOptions struct {
	Name                                 *string                                 `url:"name,omitempty" json:"name,omitempty"`
	Path                                 *string                                 `url:"path,omitempty" json:"path,omitempty"`
	Avatar                               *GroupAvatar                            `url:"-" json:"avatar,omitempty"`
	DefaultBranch                        *string                                 `url:"default_branch,omitempty" json:"default_branch,omitempty"`
	Description                          *string                                 `url:"description,omitempty" json:"description,omitempty"`
	MembershipLock                       *bool                                   `url:"membership_lock,omitempty" json:"membership_lock,omitempty"`
	Visibility                           *VisibilityValue                        `url:"visibility,omitempty" json:"visibility,omitempty"`
	ShareWithGroupLock                   *bool                                   `url:"share_with_group_lock,omitempty" json:"share_with_group_lock,omitempty"`
	RequireTwoFactorAuth                 *bool                                   `url:"require_two_factor_authentication,omitempty" json:"require_two_factor_authentication,omitempty"`
	TwoFactorGracePeriod                 *int64                                  `url:"two_factor_grace_period,omitempty" json:"two_factor_grace_period,omitempty"`
	ProjectCreationLevel                 *ProjectCreationLevelValue              `url:"project_creation_level,omitempty" json:"project_creation_level,omitempty"`
	AutoDevopsEnabled                    *bool                                   `url:"auto_devops_enabled,omitempty" json:"auto_devops_enabled,omitempty"`
	SubGroupCreationLevel                *SubGroupCreationLevelValue             `url:"subgroup_creation_level,omitempty" json:"subgroup_creation_level,omitempty"`
	EmailsEnabled                        *bool                                   `url:"emails_enabled,omitempty" json:"emails_enabled,omitempty"`
	MentionsDisabled                     *bool                                   `url:"mentions_disabled,omitempty" json:"mentions_disabled,omitempty"`
	LFSEnabled                           *bool                                   `url:"lfs_enabled,omitempty" json:"lfs_enabled,omitempty"`
	MaxArtifactsSize                     *int64                                  `url:"max_artifacts_size,omitempty" json:"max_artifacts_size,omitempty"`
	RequestAccessEnabled                 *bool                                   `url:"request_access_enabled,omitempty" json:"request_access_enabled,omitempty"`
	DefaultBranchProtectionDefaults      *DefaultBranchProtectionDefaultsOptions `url:"default_branch_protection_defaults,omitempty" json:"default_branch_protection_defaults,omitempty"`
	FileTemplateProjectID                *int64                                  `url:"file_template_project_id,omitempty" json:"file_template_project_id,omitempty"`
	SharedRunnersMinutesLimit            *int64                                  `url:"shared_runners_minutes_limit,omitempty" json:"shared_runners_minutes_limit,omitempty"`
	ExtraSharedRunnersMinutesLimit       *int64                                  `url:"extra_shared_runners_minutes_limit,omitempty" json:"extra_shared_runners_minutes_limit,omitempty"`
	PreventForkingOutsideGroup           *bool                                   `url:"prevent_forking_outside_group,omitempty" json:"prevent_forking_outside_group,omitempty"`
	SharedRunnersSetting                 *SharedRunnersSettingValue              `url:"shared_runners_setting,omitempty" json:"shared_runners_setting,omitempty"`
	PreventSharingGroupsOutsideHierarchy *bool                                   `url:"prevent_sharing_groups_outside_hierarchy,omitempty" json:"prevent_sharing_groups_outside_hierarchy,omitempty"`
	IPRestrictionRanges                  *string                                 `url:"ip_restriction_ranges,omitempty" json:"ip_restriction_ranges,omitempty"`
	AllowedEmailDomainsList              *string                                 `url:"allowed_email_domains_list,omitempty" json:"allowed_email_domains_list,omitempty"`
	WikiAccessLevel                      *AccessControlValue                     `url:"wiki_access_level,omitempty" json:"wiki_access_level,omitempty"`

	OnlyAllowMergeIfPipelineSucceeds          *bool `url:"only_allow_merge_if_pipeline_succeeds,omitempty" json:"only_allow_merge_if_pipeline_succeeds,omitempty"`
	AllowMergeOnSkippedPipeline               *bool `url:"allow_merge_on_skipped_pipeline,omitempty" json:"allow_merge_on_skipped_pipeline,omitempty"`
	OnlyAllowMergeIfAllDiscussionsAreResolved *bool `url:"only_allow_merge_if_all_discussions_are_resolved,omitempty" json:"only_allow_merge_if_all_discussions_are_resolved,omitempty"`

	// Deprecated: Use EmailsEnabled instead
	EmailsDisabled *bool `url:"emails_disabled,omitempty" json:"emails_disabled,omitempty"`

	// Deprecated: Use DefaultBranchProtectionDefaults instead
	DefaultBranchProtection         *int64                         `url:"default_branch_protection,omitempty" json:"default_branch_protection,omitempty"`
	EnabledGitAccessProtocol        *EnabledGitAccessProtocolValue `url:"enabled_git_access_protocol,omitempty" json:"enabled_git_access_protocol,omitempty"`
	StepUpAuthRequiredOAuthProvider *string                        `url:"step_up_auth_required_oauth_provider,omitempty" json:"step_up_auth_required_oauth_provider,omitempty"`
	// The following fields are Premium and Ultimate only.
	UniqueProjectDownloadLimit                  *int64    `url:"unique_project_download_limit,omitempty" json:"unique_project_download_limit,omitempty"`
	UniqueProjectDownloadLimitIntervalInSeconds *int64    `url:"unique_project_download_limit_interval_in_seconds,omitempty" json:"unique_project_download_limit_interval_in_seconds,omitempty"`
	UniqueProjectDownloadLimitAllowlist         *[]string `url:"unique_project_download_limit_allowlist,omitempty" json:"unique_project_download_limit_allowlist,omitempty"`
	UniqueProjectDownloadLimitAlertlist         *[]int64  `url:"unique_project_download_limit_alertlist,omitempty" json:"unique_project_download_limit_alertlist,omitempty"`
	AutoBanUserOnExcessiveProjectsDownload      *bool     `url:"auto_ban_user_on_excessive_projects_download,omitempty" json:"auto_ban_user_on_excessive_projects_download,omitempty"`

	DuoAvailability                *DuoAvailabilityValue `url:"duo_availability,omitempty" json:"duo_availability,omitempty"`
	ExperimentFeaturesEnabled      *bool                 `url:"experiment_features_enabled,omitempty" json:"experiment_features_enabled,omitempty"`
	MathRenderingLimitsEnabled     *bool                 `url:"math_rendering_limits_enabled,omitempty" json:"math_rendering_limits_enabled,omitempty"`
	LockMathRenderingLimitsEnabled *bool                 `url:"lock_math_rendering_limits_enabled,omitempty" json:"lock_math_rendering_limits_enabled,omitempty"`
	DuoFeaturesEnabled             *bool                 `url:"duo_features_enabled,omitempty" json:"duo_features_enabled,omitempty"`
	LockDuoFeaturesEnabled         *bool                 `url:"lock_duo_features_enabled,omitempty" json:"lock_duo_features_enabled,omitempty"`

	CRMEnabled *bool `url:"crm_enabled,omitempty" json:"crm_enabled,omitempty"`

	WebBasedCommitSigningEnabled *bool `url:"web_based_commit_signing_enabled,omitempty" json:"web_based_commit_signing_enabled,omitempty"`
	AllowPersonalSnippets        *bool `url:"allow_personal_snippets,omitempty" json:"allow_personal_snippets,omitempty"`
}

func (s *GroupsService) UpdateGroup(gid any, opt *UpdateGroupOptions, options ...RequestOptionFunc) (*Group, *Response, error) {
	reqOpts := []doOption{
		withMethod(http.MethodPut),
		withPath(routeGroupsID, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	}
	if opt.Avatar != nil && (opt.Avatar.Filename != "" || opt.Avatar.Image != nil) {
		if opt.DefaultBranchProtectionDefaults != nil && (len(opt.DefaultBranchProtectionDefaults.AllowedToMerge) > 1 || len(opt.DefaultBranchProtectionDefaults.AllowedToPush) > 1) {
			return nil, nil, errors.New("multiple access levels for allowed_to_merge or allowed_to_push are not permitted when an Avatar is also specified as it will result in unexpected behavior")
		}
		reqOpts = append(reqOpts, withUpload(opt.Avatar.Image, opt.Avatar.Filename, UploadAvatar))
	}
	return do[*Group](s.client, reqOpts...)
}

func (s *GroupsService) UploadAvatar(gid any, avatar io.Reader, filename string, options ...RequestOptionFunc) (*Group, *Response, error) {
	return do[*Group](s.client,
		withMethod(http.MethodPut),
		withPath(routeGroupsID, GroupID{gid}),
		withUpload(avatar, filename, UploadAvatar),
		withRequestOpts(options...),
	)
}

// DeleteGroupOptions represents the available DeleteGroup() options.
//
// GitLab API docs: https://docs.gitlab.com/api/groups/#delete-a-group
type DeleteGroupOptions struct {
	PermanentlyRemove *bool   `url:"permanently_remove,omitempty" json:"permanently_remove,omitempty"`
	FullPath          *string `url:"full_path,omitempty" json:"full_path,omitempty"`
}

func (s *GroupsService) DeleteGroup(gid any, opt *DeleteGroupOptions, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodDelete),
		withPath(routeGroupsID, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
	return resp, err
}

func (s *GroupsService) RestoreGroup(gid any, options ...RequestOptionFunc) (*Group, *Response, error) {
	return do[*Group](s.client,
		withMethod(http.MethodPost),
		withPath(routeGroupsIDRestore, GroupID{gid}),
		withRequestOpts(options...),
	)
}

func (s *GroupsService) SearchGroup(query string, options ...RequestOptionFunc) ([]*Group, *Response, error) {
	var q struct {
		Search string `url:"search,omitempty" json:"search,omitempty"`
	}
	q.Search = query

	return do[[]*Group](s.client,
		withPath(routeGroups),
		withAPIOpts(&q),
		withRequestOpts(options...),
	)
}

// ListProvisionedUsersOptions represents the available ListProvisionedUsers()
// options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/groups/#list-provisioned-users
type ListProvisionedUsersOptions struct {
	ListOptions
	Username      *string    `url:"username,omitempty" json:"username,omitempty"`
	Search        *string    `url:"search,omitempty" json:"search,omitempty"`
	Active        *bool      `url:"active,omitempty" json:"active,omitempty"`
	Blocked       *bool      `url:"blocked,omitempty" json:"blocked,omitempty"`
	CreatedAfter  *time.Time `url:"created_after,omitempty" json:"created_after,omitempty"`
	CreatedBefore *time.Time `url:"created_before,omitempty" json:"created_before,omitempty"`
}

func (s *GroupsService) ListProvisionedUsers(gid any, opt *ListProvisionedUsersOptions, options ...RequestOptionFunc) ([]*User, *Response, error) {
	return do[[]*User](s.client,
		withPath(routeGroupsIDProvisionedUsers, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

func (s *GroupsService) ListGroupLDAPLinks(gid any, options ...RequestOptionFunc) ([]*LDAPGroupLink, *Response, error) {
	return do[[]*LDAPGroupLink](s.client,
		withPath(routeGroupsIDLdapGroupLinks, GroupID{gid}),
		withRequestOpts(options...),
	)
}

// AddGroupLDAPLinkOptions represents the available AddGroupLDAPLink() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/group_ldap_links/#add-an-ldap-group-link-with-cn-or-filter
type AddGroupLDAPLinkOptions struct {
	CN           *string           `url:"cn,omitempty" json:"cn,omitempty"`
	Filter       *string           `url:"filter,omitempty" json:"filter,omitempty"`
	GroupAccess  *AccessLevelValue `url:"group_access,omitempty" json:"group_access,omitempty"`
	Provider     *string           `url:"provider,omitempty" json:"provider,omitempty"`
	MemberRoleID *int64            `url:"member_role_id,omitempty" json:"member_role_id,omitempty"`
}

func (s *GroupsService) AddGroupLDAPLink(gid any, opt *AddGroupLDAPLinkOptions, options ...RequestOptionFunc) (*LDAPGroupLink, *Response, error) {
	return do[*LDAPGroupLink](s.client,
		withMethod(http.MethodPost),
		withPath(routeGroupsIDLdapGroupLinks, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

func (s *GroupsService) DeleteGroupLDAPLink(gid any, cn string, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodDelete),
		withPath(routeGroupsIDLdapGroupLinksID, GroupID{gid}, cn),
		withRequestOpts(options...),
	)
	return resp, err
}

// DeleteGroupLDAPLinkWithCNOrFilterOptions represents the available DeleteGroupLDAPLinkWithCNOrFilter() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/group_ldap_links/#delete-an-ldap-group-link-with-cn-or-filter
type DeleteGroupLDAPLinkWithCNOrFilterOptions struct {
	CN       *string `url:"cn,omitempty" json:"cn,omitempty"`
	Filter   *string `url:"filter,omitempty" json:"filter,omitempty"`
	Provider *string `url:"provider,omitempty" json:"provider,omitempty"`
}

func (s *GroupsService) DeleteGroupLDAPLinkWithCNOrFilter(gid any, opts *DeleteGroupLDAPLinkWithCNOrFilterOptions, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodDelete),
		withPath(routeGroupsIDLdapGroupLinks, GroupID{gid}),
		withAPIOpts(opts),
		withRequestOpts(options...),
	)
	return resp, err
}

func (s *GroupsService) DeleteGroupLDAPLinkForProvider(gid any, provider, cn string, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodDelete),
		withPath(routeGroupsIDLdapGroupLinksIDID, GroupID{gid}, provider, cn),
		withRequestOpts(options...),
	)
	return resp, err
}

func (s *GroupsService) ListGroupSAMLLinks(gid any, options ...RequestOptionFunc) ([]*SAMLGroupLink, *Response, error) {
	return do[[]*SAMLGroupLink](s.client,
		withPath(routeGroupsIDSamlGroupLinks, GroupID{gid}),
		withRequestOpts(options...),
	)
}

// ListGroupSharedProjectsOptions represents the available ListGroupSharedProjects() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/groups/#list-shared-projects
type ListGroupSharedProjectsOptions struct {
	ListOptions
	Archived                 *bool             `url:"archived,omitempty" json:"archived,omitempty"`
	MinAccessLevel           *AccessLevelValue `url:"min_access_level,omitempty" json:"min_access_level,omitempty"`
	OrderBy                  *string           `url:"order_by,omitempty" json:"order_by,omitempty"`
	Search                   *string           `url:"search,omitempty" json:"search,omitempty"`
	Simple                   *bool             `url:"simple,omitempty" json:"simple,omitempty"`
	Sort                     *string           `url:"sort,omitempty" json:"sort,omitempty"`
	Starred                  *bool             `url:"starred,omitempty" json:"starred,omitempty"`
	Visibility               *VisibilityValue  `url:"visibility,omitempty" json:"visibility,omitempty"`
	WithCustomAttributes     *bool             `url:"with_custom_attributes,omitempty" json:"with_custom_attributes,omitempty"`
	WithIssuesEnabled        *bool             `url:"with_issues_enabled,omitempty" json:"with_issues_enabled,omitempty"`
	WithMergeRequestsEnabled *bool             `url:"with_merge_requests_enabled,omitempty" json:"with_merge_requests_enabled,omitempty"`
}

func (s *GroupsService) ListGroupSharedProjects(gid any, opt *ListGroupSharedProjectsOptions, options ...RequestOptionFunc) ([]*Project, *Response, error) {
	return do[[]*Project](s.client,
		withPath(routeGroupsIDProjectsShared, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

func (s *GroupsService) GetGroupSAMLLink(gid any, samlGroupName string, options ...RequestOptionFunc) (*SAMLGroupLink, *Response, error) {
	return do[*SAMLGroupLink](s.client,
		withPath(routeGroupsIDSamlGroupLinksID, GroupID{gid}, samlGroupName),
		withRequestOpts(options...),
	)
}

// AddGroupSAMLLinkOptions represents the available AddGroupSAMLLink() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/saml/#add-a-saml-group-link
type AddGroupSAMLLinkOptions struct {
	SAMLGroupName *string           `url:"saml_group_name,omitempty" json:"saml_group_name,omitempty"`
	AccessLevel   *AccessLevelValue `url:"access_level,omitempty" json:"access_level,omitempty"`
	MemberRoleID  *int64            `url:"member_role_id,omitempty" json:"member_role_id,omitempty"`
	Provider      *string           `url:"provider,omitempty" json:"provider,omitempty"`
}

func (s *GroupsService) AddGroupSAMLLink(gid any, opt *AddGroupSAMLLinkOptions, options ...RequestOptionFunc) (*SAMLGroupLink, *Response, error) {
	return do[*SAMLGroupLink](s.client,
		withMethod(http.MethodPost),
		withPath(routeGroupsIDSamlGroupLinks, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

func (s *GroupsService) DeleteGroupSAMLLink(gid any, samlGroupName string, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodDelete),
		withPath(routeGroupsIDSamlGroupLinksID, GroupID{gid}, samlGroupName),
		withRequestOpts(options...),
	)
	return resp, err
}

// ShareGroupWithGroupOptions represents the available ShareGroupWithGroup() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/groups/#share-groups-with-groups
type ShareGroupWithGroupOptions struct {
	GroupID      *int64            `url:"group_id,omitempty" json:"group_id,omitempty"`
	GroupAccess  *AccessLevelValue `url:"group_access,omitempty" json:"group_access,omitempty"`
	ExpiresAt    *ISOTime          `url:"expires_at,omitempty" json:"expires_at,omitempty"`
	MemberRoleID *int64            `url:"member_role_id,omitempty" json:"member_role_id,omitempty"`
}

func (s *GroupsService) ShareGroupWithGroup(gid any, opt *ShareGroupWithGroupOptions, options ...RequestOptionFunc) (*Group, *Response, error) {
	return do[*Group](s.client,
		withMethod(http.MethodPost),
		withPath(routeGroupsIDShare, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

func (s *GroupsService) UnshareGroupFromGroup(gid any, groupID int64, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodDelete),
		withPath(routeGroupsIDShareID, GroupID{gid}, groupID),
		withRequestOpts(options...),
	)
	return resp, err
}

// GroupPushRules represents a group push rule.
//
// GitLab API docs:
// https://docs.gitlab.com/api/group_push_rules/#get-the-push-rules-of-a-group
type GroupPushRules struct {
	ID                         int64      `json:"id"`
	CreatedAt                  *time.Time `json:"created_at"`
	CommitMessageRegex         string     `json:"commit_message_regex"`
	CommitMessageNegativeRegex string     `json:"commit_message_negative_regex"`
	BranchNameRegex            string     `json:"branch_name_regex"`
	DenyDeleteTag              bool       `json:"deny_delete_tag"`
	MemberCheck                bool       `json:"member_check"`
	PreventSecrets             bool       `json:"prevent_secrets"`
	AuthorEmailRegex           string     `json:"author_email_regex"`
	FileNameRegex              string     `json:"file_name_regex"`
	MaxFileSize                int64      `json:"max_file_size"`
	CommitCommitterCheck       bool       `json:"commit_committer_check"`
	CommitCommitterNameCheck   bool       `json:"commit_committer_name_check"`
	RejectUnsignedCommits      bool       `json:"reject_unsigned_commits"`
	RejectNonDCOCommits        bool       `json:"reject_non_dco_commits"`
}

func (s *GroupsService) GetGroupPushRules(gid any, options ...RequestOptionFunc) (*GroupPushRules, *Response, error) {
	return do[*GroupPushRules](s.client,
		withPath(routeGroupsIDPushRule, GroupID{gid}),
		withRequestOpts(options...),
	)
}

// AddGroupPushRuleOptions represents the available AddGroupPushRule()
// options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/group_push_rules/#add-push-rules-to-a-group
type AddGroupPushRuleOptions struct {
	AuthorEmailRegex           *string `url:"author_email_regex,omitempty" json:"author_email_regex,omitempty"`
	BranchNameRegex            *string `url:"branch_name_regex,omitempty" json:"branch_name_regex,omitempty"`
	CommitCommitterCheck       *bool   `url:"commit_committer_check,omitempty" json:"commit_committer_check,omitempty"`
	CommitCommitterNameCheck   *bool   `url:"commit_committer_name_check,omitempty" json:"commit_committer_name_check,omitempty"`
	CommitMessageNegativeRegex *string `url:"commit_message_negative_regex,omitempty" json:"commit_message_negative_regex,omitempty"`
	CommitMessageRegex         *string `url:"commit_message_regex,omitempty" json:"commit_message_regex,omitempty"`
	DenyDeleteTag              *bool   `url:"deny_delete_tag,omitempty" json:"deny_delete_tag,omitempty"`
	FileNameRegex              *string `url:"file_name_regex,omitempty" json:"file_name_regex,omitempty"`
	MaxFileSize                *int64  `url:"max_file_size,omitempty" json:"max_file_size,omitempty"`
	MemberCheck                *bool   `url:"member_check,omitempty" json:"member_check,omitempty"`
	PreventSecrets             *bool   `url:"prevent_secrets,omitempty" json:"prevent_secrets,omitempty"`
	RejectUnsignedCommits      *bool   `url:"reject_unsigned_commits,omitempty" json:"reject_unsigned_commits,omitempty"`
	RejectNonDCOCommits        *bool   `url:"reject_non_dco_commits,omitempty" json:"reject_non_dco_commits,omitempty"`
}

func (s *GroupsService) AddGroupPushRule(gid any, opt *AddGroupPushRuleOptions, options ...RequestOptionFunc) (*GroupPushRules, *Response, error) {
	return do[*GroupPushRules](s.client,
		withMethod(http.MethodPost),
		withPath(routeGroupsIDPushRule, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// EditGroupPushRuleOptions represents the available EditGroupPushRule()
// options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/group_push_rules/#edit-the-push-rules-of-a-group
type EditGroupPushRuleOptions struct {
	AuthorEmailRegex           *string `url:"author_email_regex,omitempty" json:"author_email_regex,omitempty"`
	BranchNameRegex            *string `url:"branch_name_regex,omitempty" json:"branch_name_regex,omitempty"`
	CommitCommitterCheck       *bool   `url:"commit_committer_check,omitempty" json:"commit_committer_check,omitempty"`
	CommitCommitterNameCheck   *bool   `url:"commit_committer_name_check,omitempty" json:"commit_committer_name_check,omitempty"`
	CommitMessageNegativeRegex *string `url:"commit_message_negative_regex,omitempty" json:"commit_message_negative_regex,omitempty"`
	CommitMessageRegex         *string `url:"commit_message_regex,omitempty" json:"commit_message_regex,omitempty"`
	DenyDeleteTag              *bool   `url:"deny_delete_tag,omitempty" json:"deny_delete_tag,omitempty"`
	FileNameRegex              *string `url:"file_name_regex,omitempty" json:"file_name_regex,omitempty"`
	MaxFileSize                *int64  `url:"max_file_size,omitempty" json:"max_file_size,omitempty"`
	MemberCheck                *bool   `url:"member_check,omitempty" json:"member_check,omitempty"`
	PreventSecrets             *bool   `url:"prevent_secrets,omitempty" json:"prevent_secrets,omitempty"`
	RejectUnsignedCommits      *bool   `url:"reject_unsigned_commits,omitempty" json:"reject_unsigned_commits,omitempty"`
	RejectNonDCOCommits        *bool   `url:"reject_non_dco_commits,omitempty" json:"reject_non_dco_commits,omitempty"`
}

func (s *GroupsService) EditGroupPushRule(gid any, opt *EditGroupPushRuleOptions, options ...RequestOptionFunc) (*GroupPushRules, *Response, error) {
	return do[*GroupPushRules](s.client,
		withMethod(http.MethodPut),
		withPath(routeGroupsIDPushRule, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

func (s *GroupsService) DeleteGroupPushRule(gid any, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodDelete),
		withPath(routeGroupsIDPushRule, GroupID{gid}),
		withRequestOpts(options...),
	)
	return resp, err
}

func (s *GroupsService) ArchiveGroup(gid any, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodPost),
		withPath(routeGroupsIDArchive, GroupID{gid}),
		withRequestOpts(options...),
	)
	return resp, err
}

func (s *GroupsService) UnarchiveGroup(gid any, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodPost),
		withPath(routeGroupsIDUnarchive, GroupID{gid}),
		withRequestOpts(options...),
	)
	return resp, err
}

// ListSAMLUsersOptions represents the available ListSAMLUsers() options.
//
// GitLab API docs: https://docs.gitlab.com/api/groups/#list-all-saml-users
type ListSAMLUsersOptions struct {
	ListOptions
	Username      *string    `url:"username,omitempty" json:"username,omitempty"`
	Search        *string    `url:"search,omitempty" json:"search,omitempty"`
	Active        *bool      `url:"active,omitempty" json:"active,omitempty"`
	Blocked       *bool      `url:"blocked,omitempty" json:"blocked,omitempty"`
	CreatedAfter  *time.Time `url:"created_after,omitempty" json:"created_after,omitempty"`
	CreatedBefore *time.Time `url:"created_before,omitempty" json:"created_before,omitempty"`
}

func (s *GroupsService) ListSAMLUsers(gid any, opt *ListSAMLUsersOptions, options ...RequestOptionFunc) ([]*User, *Response, error) {
	return do[[]*User](s.client,
		withPath(routeGroupsIDSamlUsers, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// ListGroupsSharedWithOptions represents the available ListGroupsSharedWith()
// options.
//
// GitLab API docs: https://docs.gitlab.com/api/groups/#list-shared-groups
type ListGroupsSharedWithOptions struct {
	ListOptions
	SkipGroups           *[]int64          `url:"skip_groups,omitempty" del:"," json:"skip_groups,omitempty"`
	Search               *string           `url:"search,omitempty" json:"search,omitempty"`
	OrderBy              *string           `url:"order_by,omitempty" json:"order_by,omitempty"`
	Sort                 *string           `url:"sort,omitempty" json:"sort,omitempty"`
	Visibility           *VisibilityValue  `url:"visibility,omitempty" json:"visibility,omitempty"`
	MinAccessLevel       *AccessLevelValue `url:"min_access_level,omitempty" json:"min_access_level,omitempty"`
	WithCustomAttributes *bool             `url:"with_custom_attributes,omitempty" json:"with_custom_attributes,omitempty"`
}

func (s *GroupsService) ListGroupsSharedWith(gid any, opt *ListGroupsSharedWithOptions, options ...RequestOptionFunc) ([]*Group, *Response, error) {
	return do[[]*Group](s.client,
		withPath(routeGroupsIDGroupsShared, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// ListInvitedGroupsOptions represents the available ListInvitedGroups()
// options.
//
// GitLab API docs: https://docs.gitlab.com/api/groups/#list-invited-groups
type ListInvitedGroupsOptions struct {
	ListOptions
	Search               *string           `url:"search,omitempty" json:"search,omitempty"`
	MinAccessLevel       *AccessLevelValue `url:"min_access_level,omitempty" json:"min_access_level,omitempty"`
	Relation             *[]string         `url:"relation,omitempty" del:"," json:"relation,omitempty"`
	WithCustomAttributes *bool             `url:"with_custom_attributes,omitempty" json:"with_custom_attributes,omitempty"`
}

func (s *GroupsService) ListInvitedGroups(gid any, opt *ListInvitedGroupsOptions, options ...RequestOptionFunc) ([]*Group, *Response, error) {
	return do[[]*Group](s.client,
		withPath(routeGroupsIDInvitedGroups, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// TransferLocation represents a possible parent namespace returned by
// ListTransferLocations.
//
// GitLab API docs:
// https://docs.gitlab.com/api/groups/#list-all-locations-available-for-group-transfer
type TransferLocation struct {
	ID        int64  `json:"id"`
	WebURL    string `json:"web_url"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	FullName  string `json:"full_name"`
	FullPath  string `json:"full_path"`
}

// ListTransferLocationsOptions represents the available ListTransferLocations()
// options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/groups/#list-all-locations-available-for-group-transfer
type ListTransferLocationsOptions struct {
	ListOptions
	Search *string `url:"search,omitempty" json:"search,omitempty"`
}

func (s *GroupsService) ListTransferLocations(gid any, opt *ListTransferLocationsOptions, options ...RequestOptionFunc) ([]*TransferLocation, *Response, error) {
	return do[[]*TransferLocation](s.client,
		withPath(routeGroupsIDTransferLocations, GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

func (s *GroupsService) SyncGroupWithLDAP(gid any, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodPost),
		withPath(routeGroupsIDLdapSync, GroupID{gid}),
		withRequestOpts(options...),
	)
	return resp, err
}
