package gitlab

import (
	"errors"
	"fmt"
	"time"
)

type (
	// AchievementsServiceInterface describes the API methods for GitLab
	// achievements and their awards.
	//
	// GitLab API docs:
	// https://docs.gitlab.com/api/graphql/reference/#achievement
	AchievementsServiceInterface interface {
		// CreateAchievement creates a new achievement in a namespace.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#mutationachievementscreate
		CreateAchievement(namespaceID int64, opt *CreateAchievementOptions, options ...RequestOptionFunc) (*Achievement, *Response, error)

		// UpdateAchievement updates an existing achievement.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#mutationachievementsupdate
		UpdateAchievement(achievementID int64, opt *UpdateAchievementOptions, options ...RequestOptionFunc) (*Achievement, *Response, error)

		// DeleteAchievement deletes an achievement.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#mutationachievementsdelete
		DeleteAchievement(achievementID int64, options ...RequestOptionFunc) (*Achievement, *Response, error)

		// AwardAchievement awards an achievement to a user.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#mutationachievementsaward
		AwardAchievement(achievementID int64, userID int64, opt *AwardAchievementOptions, options ...RequestOptionFunc) (*UserAchievement, *Response, error)

		// RevokeAchievement revokes a previously awarded achievement from a
		// user.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#mutationachievementsrevoke
		RevokeAchievement(userAchievementID int64, options ...RequestOptionFunc) (*UserAchievement, *Response, error)

		// UpdateUserAchievement updates an awarded achievement.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#mutationuserachievementsupdate
		UpdateUserAchievement(userAchievementID int64, opt *UpdateUserAchievementOptions, options ...RequestOptionFunc) (*UserAchievement, *Response, error)

		// DeleteUserAchievement deletes an awarded achievement.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#mutationuserachievementsdelete
		DeleteUserAchievement(userAchievementID int64, options ...RequestOptionFunc) (*UserAchievement, *Response, error)

		// UpdateUserAchievementPriorities reorders a user's awarded
		// achievements, from highest to lowest priority.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#mutationuserachievementprioritiesupdate
		UpdateUserAchievementPriorities(userAchievementIDs []int64, options ...RequestOptionFunc) ([]*UserAchievement, *Response, error)

		// ListUserAchievements lists the achievements awarded to a user.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#useruserachievements
		ListUserAchievements(username string, opt *ListUserAchievementsOptions, options ...RequestOptionFunc) ([]*UserAchievement, *Response, error)

		// ListAchievements lists the achievements defined in a namespace.
		//
		// fullPath is the full path of the namespace (group or project).
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#namespaceachievements
		ListAchievements(fullPath string, opt *ListAchievementsOptions, options ...RequestOptionFunc) ([]*Achievement, *Response, error)

		// ListAchievementRecipients lists the users an achievement has been
		// awarded to.
		//
		// fullPath is the full path of the namespace (group or project) the
		// achievement belongs to.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#achievement-userachievements
		ListAchievementRecipients(fullPath string, achievementID int64, opt *ListAchievementRecipientsOptions, options ...RequestOptionFunc) ([]*UserAchievement, *Response, error)

		// ListAchievementUniqueUsers lists the distinct users who have
		// received an achievement.
		//
		// fullPath is the full path of the namespace (group or project) the
		// achievement belongs to.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/graphql/reference/#achievement-uniqueusers
		ListAchievementUniqueUsers(fullPath string, achievementID int64, opt *ListAchievementUniqueUsersOptions, options ...RequestOptionFunc) ([]*BasicUser, *Response, error)
	}

	// AchievementsService handles communication with the achievement related
	// methods of the GitLab API.
	//
	// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#achievement
	AchievementsService struct {
		client *Client
	}
)

var _ AchievementsServiceInterface = (*AchievementsService)(nil)

// Achievement represents a GitLab achievement definition.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#achievement
type Achievement struct {
	ID          int64
	NamespaceID int64
	Name        string
	AvatarURL   *string
	Description *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// achievementGQL is used to unmarshal GraphQL responses where the ID is a
// global ID string of the form gid://gitlab/Achievements::Achievement/<n>.
type achievementGQL struct {
	ID        gidGQL `json:"id"`
	Namespace struct {
		ID gidGQL `json:"id"`
	} `json:"namespace"`
	Name        string    `json:"name"`
	AvatarURL   *string   `json:"avatarUrl"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (a *achievementGQL) unwrap() *Achievement {
	if a == nil {
		return nil
	}

	return &Achievement{
		ID:          a.ID.Int64,
		NamespaceID: a.Namespace.ID.Int64,
		Name:        a.Name,
		AvatarURL:   a.AvatarURL,
		Description: a.Description,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}
}

// UserAchievement represents an achievement awarded to a user.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#userachievement
type UserAchievement struct {
	ID              int64
	AchievementID   int64
	UserID          int64
	AwardedByUserID int64
	RevokedByUserID *int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
	RevokedAt       *time.Time
	Priority        *int64
	ShowOnProfile   bool
	AwardMessage    *string
}

// userAchievementGQL is used to unmarshal GraphQL responses where the ID is a
// global ID string of the form
// gid://gitlab/Achievements::UserAchievement/<n>.
type userAchievementGQL struct {
	ID          gidGQL `json:"id"`
	Achievement struct {
		ID gidGQL `json:"id"`
	} `json:"achievement"`
	User struct {
		ID gidGQL `json:"id"`
	} `json:"user"`
	AwardedByUser struct {
		ID gidGQL `json:"id"`
	} `json:"awardedByUser"`
	RevokedByUser *struct {
		ID gidGQL `json:"id"`
	} `json:"revokedByUser"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	RevokedAt     *time.Time `json:"revokedAt"`
	Priority      *int64     `json:"priority"`
	ShowOnProfile bool       `json:"showOnProfile"`
	AwardMessage  *string    `json:"awardMessage"`
}

func (u *userAchievementGQL) unwrap() *UserAchievement {
	if u == nil {
		return nil
	}

	ua := &UserAchievement{
		ID:              u.ID.Int64,
		AchievementID:   u.Achievement.ID.Int64,
		UserID:          u.User.ID.Int64,
		AwardedByUserID: u.AwardedByUser.ID.Int64,
		CreatedAt:       u.CreatedAt,
		UpdatedAt:       u.UpdatedAt,
		RevokedAt:       u.RevokedAt,
		Priority:        u.Priority,
		ShowOnProfile:   u.ShowOnProfile,
		AwardMessage:    u.AwardMessage,
	}
	if u.RevokedByUser != nil {
		id := u.RevokedByUser.ID.Int64
		ua.RevokedByUserID = &id
	}

	return ua
}

// achievementFields is the field selection shared by every mutation that
// returns an Achievement.
const achievementFields = `
	id
	namespace {
		id
	}
	name
	avatarUrl
	description
	createdAt
	updatedAt
`

// userAchievementFields is the field selection shared by every mutation that
// returns a UserAchievement.
const userAchievementFields = `
	id
	achievement {
		id
	}
	user {
		id
	}
	awardedByUser {
		id
	}
	revokedByUser {
		id
	}
	createdAt
	updatedAt
	revokedAt
	priority
	showOnProfile
	awardMessage
`

// CreateAchievementOptions represents the available CreateAchievement()
// options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationachievementscreate
type CreateAchievementOptions struct {
	// Name of the achievement. Required.
	Name *string

	// Description of the achievement.
	Description *string

	// Avatar image for the achievement.
	Avatar *GraphQLUpload
}

// UpdateAchievementOptions represents the available UpdateAchievement()
// options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationachievementsupdate
type UpdateAchievementOptions struct {
	// Name of the achievement.
	Name *string

	// Description of the achievement.
	Description *string

	// Avatar image for the achievement.
	Avatar *GraphQLUpload
}

// AwardAchievementOptions represents the available AwardAchievement()
// options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationachievementsaward
type AwardAchievementOptions struct {
	// AwardMessage is a message associated with the awarded achievement, up
	// to 200 characters.
	AwardMessage *string
}

// UpdateUserAchievementOptions represents the available
// UpdateUserAchievement() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationuserachievementsupdate
type UpdateUserAchievementOptions struct {
	// ShowOnProfile indicates whether the awarded achievement is shown on
	// the recipient's profile.
	ShowOnProfile *bool
}

// CreateAchievement creates a new achievement in a namespace.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationachievementscreate
func (s *AchievementsService) CreateAchievement(namespaceID int64, opt *CreateAchievementOptions, options ...RequestOptionFunc) (*Achievement, *Response, error) {
	if opt == nil {
		return nil, nil, errors.New("opt is required")
	}
	if opt.Name == nil || *opt.Name == "" {
		return nil, nil, errors.New("opt.Name is required")
	}

	namespaceGID := gidGQL{Type: "Namespace", Int64: namespaceID}

	input := map[string]any{
		"namespaceId": namespaceGID.String(),
		"name":        opt.Name,
	}
	if opt.Description != nil {
		input["description"] = opt.Description
	}
	if opt.Avatar != nil {
		input["avatar"] = opt.Avatar
	}

	mutation := GraphQLQuery{
		Query: `
			mutation CreateAchievement($input: AchievementsCreateInput!) {
				achievementsCreate(input: $input) {
					achievement {` + achievementFields + `}
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
			AchievementsCreate struct {
				Achievement *achievementGQL `json:"achievement"`
				Errors      []string        `json:"errors"`
			} `json:"achievementsCreate"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if err := graphQLErrors(result.GenericGraphQLErrors); err != nil {
		return nil, resp, err
	}
	if len(result.Data.AchievementsCreate.Errors) > 0 {
		return nil, resp, fmt.Errorf("achievementsCreate mutation errors: %v", result.Data.AchievementsCreate.Errors)
	}
	if result.Data.AchievementsCreate.Achievement == nil {
		return nil, resp, ErrNotFound
	}

	return result.Data.AchievementsCreate.Achievement.unwrap(), resp, nil
}

// UpdateAchievement updates an existing achievement.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationachievementsupdate
func (s *AchievementsService) UpdateAchievement(achievementID int64, opt *UpdateAchievementOptions, options ...RequestOptionFunc) (*Achievement, *Response, error) {
	achievementGID := gidGQL{Type: "Achievements::Achievement", Int64: achievementID}

	input := map[string]any{
		"achievementId": achievementGID.String(),
	}
	if opt != nil {
		if opt.Name != nil {
			input["name"] = opt.Name
		}
		if opt.Description != nil {
			input["description"] = opt.Description
		}
		if opt.Avatar != nil {
			input["avatar"] = opt.Avatar
		}
	}

	mutation := GraphQLQuery{
		Query: `
			mutation UpdateAchievement($input: AchievementsUpdateInput!) {
				achievementsUpdate(input: $input) {
					achievement {` + achievementFields + `}
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
			AchievementsUpdate struct {
				Achievement *achievementGQL `json:"achievement"`
				Errors      []string        `json:"errors"`
			} `json:"achievementsUpdate"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if err := graphQLErrors(result.GenericGraphQLErrors); err != nil {
		return nil, resp, err
	}
	if len(result.Data.AchievementsUpdate.Errors) > 0 {
		return nil, resp, fmt.Errorf("achievementsUpdate mutation errors: %v", result.Data.AchievementsUpdate.Errors)
	}
	if result.Data.AchievementsUpdate.Achievement == nil {
		return nil, resp, ErrNotFound
	}

	return result.Data.AchievementsUpdate.Achievement.unwrap(), resp, nil
}

// DeleteAchievement deletes an achievement.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationachievementsdelete
func (s *AchievementsService) DeleteAchievement(achievementID int64, options ...RequestOptionFunc) (*Achievement, *Response, error) {
	achievementGID := gidGQL{Type: "Achievements::Achievement", Int64: achievementID}

	mutation := GraphQLQuery{
		Query: `
			mutation DeleteAchievement($input: AchievementsDeleteInput!) {
				achievementsDelete(input: $input) {
					achievement {` + achievementFields + `}
					errors
				}
			}
		`,
		Variables: map[string]any{
			"input": map[string]any{
				"achievementId": achievementGID.String(),
			},
		},
	}

	var result struct {
		Data struct {
			AchievementsDelete struct {
				Achievement *achievementGQL `json:"achievement"`
				Errors      []string        `json:"errors"`
			} `json:"achievementsDelete"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if err := graphQLErrors(result.GenericGraphQLErrors); err != nil {
		return nil, resp, err
	}
	if len(result.Data.AchievementsDelete.Errors) > 0 {
		return nil, resp, fmt.Errorf("achievementsDelete mutation errors: %v", result.Data.AchievementsDelete.Errors)
	}
	if result.Data.AchievementsDelete.Achievement == nil {
		return nil, resp, ErrNotFound
	}

	return result.Data.AchievementsDelete.Achievement.unwrap(), resp, nil
}

// AwardAchievement awards an achievement to a user.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationachievementsaward
func (s *AchievementsService) AwardAchievement(achievementID int64, userID int64, opt *AwardAchievementOptions, options ...RequestOptionFunc) (*UserAchievement, *Response, error) {
	achievementGID := gidGQL{Type: "Achievements::Achievement", Int64: achievementID}
	userGID := gidGQL{Type: "User", Int64: userID}

	input := map[string]any{
		"achievementId": achievementGID.String(),
		"userId":        userGID.String(),
	}
	if opt != nil && opt.AwardMessage != nil {
		input["awardMessage"] = opt.AwardMessage
	}

	mutation := GraphQLQuery{
		Query: `
			mutation AwardAchievement($input: AchievementsAwardInput!) {
				achievementsAward(input: $input) {
					userAchievement {` + userAchievementFields + `}
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
			AchievementsAward struct {
				UserAchievement *userAchievementGQL `json:"userAchievement"`
				Errors          []string            `json:"errors"`
			} `json:"achievementsAward"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if err := graphQLErrors(result.GenericGraphQLErrors); err != nil {
		return nil, resp, err
	}
	if len(result.Data.AchievementsAward.Errors) > 0 {
		return nil, resp, fmt.Errorf("achievementsAward mutation errors: %v", result.Data.AchievementsAward.Errors)
	}
	if result.Data.AchievementsAward.UserAchievement == nil {
		return nil, resp, ErrNotFound
	}

	return result.Data.AchievementsAward.UserAchievement.unwrap(), resp, nil
}

// RevokeAchievement revokes a previously awarded achievement from a user.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationachievementsrevoke
func (s *AchievementsService) RevokeAchievement(userAchievementID int64, options ...RequestOptionFunc) (*UserAchievement, *Response, error) {
	userAchievementGID := gidGQL{Type: "Achievements::UserAchievement", Int64: userAchievementID}

	mutation := GraphQLQuery{
		Query: `
			mutation RevokeAchievement($input: AchievementsRevokeInput!) {
				achievementsRevoke(input: $input) {
					userAchievement {` + userAchievementFields + `}
					errors
				}
			}
		`,
		Variables: map[string]any{
			"input": map[string]any{
				"userAchievementId": userAchievementGID.String(),
			},
		},
	}

	var result struct {
		Data struct {
			AchievementsRevoke struct {
				UserAchievement *userAchievementGQL `json:"userAchievement"`
				Errors          []string            `json:"errors"`
			} `json:"achievementsRevoke"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if err := graphQLErrors(result.GenericGraphQLErrors); err != nil {
		return nil, resp, err
	}
	if len(result.Data.AchievementsRevoke.Errors) > 0 {
		return nil, resp, fmt.Errorf("achievementsRevoke mutation errors: %v", result.Data.AchievementsRevoke.Errors)
	}
	if result.Data.AchievementsRevoke.UserAchievement == nil {
		return nil, resp, ErrNotFound
	}

	return result.Data.AchievementsRevoke.UserAchievement.unwrap(), resp, nil
}

// UpdateUserAchievement updates an awarded achievement.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationuserachievementsupdate
func (s *AchievementsService) UpdateUserAchievement(userAchievementID int64, opt *UpdateUserAchievementOptions, options ...RequestOptionFunc) (*UserAchievement, *Response, error) {
	userAchievementGID := gidGQL{Type: "Achievements::UserAchievement", Int64: userAchievementID}

	input := map[string]any{
		"userAchievementId": userAchievementGID.String(),
	}
	if opt != nil {
		if opt.ShowOnProfile != nil {
			input["showOnProfile"] = opt.ShowOnProfile
		}
	}

	mutation := GraphQLQuery{
		Query: `
			mutation UpdateUserAchievement($input: UserAchievementsUpdateInput!) {
				userAchievementsUpdate(input: $input) {
					userAchievement {` + userAchievementFields + `}
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
			UserAchievementsUpdate struct {
				UserAchievement *userAchievementGQL `json:"userAchievement"`
				Errors          []string            `json:"errors"`
			} `json:"userAchievementsUpdate"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if err := graphQLErrors(result.GenericGraphQLErrors); err != nil {
		return nil, resp, err
	}
	if len(result.Data.UserAchievementsUpdate.Errors) > 0 {
		return nil, resp, fmt.Errorf("userAchievementsUpdate mutation errors: %v", result.Data.UserAchievementsUpdate.Errors)
	}
	if result.Data.UserAchievementsUpdate.UserAchievement == nil {
		return nil, resp, ErrNotFound
	}

	return result.Data.UserAchievementsUpdate.UserAchievement.unwrap(), resp, nil
}

// DeleteUserAchievement deletes an awarded achievement.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationuserachievementsdelete
func (s *AchievementsService) DeleteUserAchievement(userAchievementID int64, options ...RequestOptionFunc) (*UserAchievement, *Response, error) {
	userAchievementGID := gidGQL{Type: "Achievements::UserAchievement", Int64: userAchievementID}

	mutation := GraphQLQuery{
		Query: `
			mutation DeleteUserAchievement($input: UserAchievementsDeleteInput!) {
				userAchievementsDelete(input: $input) {
					userAchievement {` + userAchievementFields + `}
					errors
				}
			}
		`,
		Variables: map[string]any{
			"input": map[string]any{
				"userAchievementId": userAchievementGID.String(),
			},
		},
	}

	var result struct {
		Data struct {
			UserAchievementsDelete struct {
				UserAchievement *userAchievementGQL `json:"userAchievement"`
				Errors          []string            `json:"errors"`
			} `json:"userAchievementsDelete"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if err := graphQLErrors(result.GenericGraphQLErrors); err != nil {
		return nil, resp, err
	}
	if len(result.Data.UserAchievementsDelete.Errors) > 0 {
		return nil, resp, fmt.Errorf("userAchievementsDelete mutation errors: %v", result.Data.UserAchievementsDelete.Errors)
	}
	if result.Data.UserAchievementsDelete.UserAchievement == nil {
		return nil, resp, ErrNotFound
	}

	return result.Data.UserAchievementsDelete.UserAchievement.unwrap(), resp, nil
}

// UpdateUserAchievementPriorities reorders a user's awarded achievements,
// from highest to lowest priority.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#mutationuserachievementprioritiesupdate
func (s *AchievementsService) UpdateUserAchievementPriorities(userAchievementIDs []int64, options ...RequestOptionFunc) ([]*UserAchievement, *Response, error) {
	if len(userAchievementIDs) == 0 {
		return nil, nil, errors.New("userAchievementIDs is required")
	}

	ids := newGIDStrings("Achievements::UserAchievement", userAchievementIDs...)

	mutation := GraphQLQuery{
		Query: `
			mutation UpdateUserAchievementPriorities($input: UserAchievementPrioritiesUpdateInput!) {
				userAchievementPrioritiesUpdate(input: $input) {
					userAchievements {` + userAchievementFields + `}
					errors
				}
			}
		`,
		Variables: map[string]any{
			"input": map[string]any{
				"userAchievementIds": ids,
			},
		},
	}

	var result struct {
		Data struct {
			UserAchievementPrioritiesUpdate struct {
				UserAchievements []*userAchievementGQL `json:"userAchievements"`
				Errors           []string              `json:"errors"`
			} `json:"userAchievementPrioritiesUpdate"`
		} `json:"data"`
		GenericGraphQLErrors
	}

	resp, err := s.client.GraphQL.Do(mutation, &result, options...)
	if err != nil {
		return nil, resp, err
	}
	if err := graphQLErrors(result.GenericGraphQLErrors); err != nil {
		return nil, resp, err
	}
	if len(result.Data.UserAchievementPrioritiesUpdate.Errors) > 0 {
		return nil, resp, fmt.Errorf("userAchievementPrioritiesUpdate mutation errors: %v", result.Data.UserAchievementPrioritiesUpdate.Errors)
	}

	userAchievements := make([]*UserAchievement, 0, len(result.Data.UserAchievementPrioritiesUpdate.UserAchievements))
	for _, ua := range result.Data.UserAchievementPrioritiesUpdate.UserAchievements {
		userAchievements = append(userAchievements, ua.unwrap())
	}

	return userAchievements, resp, nil
}

// ListUserAchievementsOptions represents the available
// ListUserAchievements() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#useruserachievements
type ListUserAchievementsOptions struct {
	// IncludeHidden includes achievements the user has hidden from their
	// profile. Only visible to the user themself, or to
	// namespace/instance maintainers and owners.
	IncludeHidden *bool

	// Pagination
	After  *string
	Before *string
	First  *int64
	Last   *int64
}

const listUserAchievementsQuery = `
	query ListUserAchievements($username: String!, $includeHidden: Boolean, $after: String, $before: String, $first: Int, $last: Int) {
		user(username: $username) {
			userAchievements(includeHidden: $includeHidden, after: $after, before: $before, first: $first, last: $last) {
				nodes {` + userAchievementFields + `}
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

// ListUserAchievements lists the achievements awarded to a user.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#useruserachievements
func (s *AchievementsService) ListUserAchievements(username string, opt *ListUserAchievementsOptions, options ...RequestOptionFunc) ([]*UserAchievement, *Response, error) {
	if opt == nil {
		opt = &ListUserAchievementsOptions{}
	}

	query := GraphQLQuery{
		Query: listUserAchievementsQuery,
		Variables: map[string]any{
			"username":      username,
			"includeHidden": opt.IncludeHidden,
			"after":         opt.After,
			"before":        opt.Before,
			linkFirst:       opt.First,
			linkLast:        opt.Last,
		},
	}

	var result struct {
		Data struct {
			User *struct {
				UserAchievements connectionGQL[userAchievementGQL] `json:"userAchievements"`
			} `json:"user"`
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
	if result.Data.User == nil {
		return nil, resp, ErrNotFound
	}

	userAchievements := make([]*UserAchievement, 0, len(result.Data.User.UserAchievements.Nodes))
	for _, ua := range result.Data.User.UserAchievements.Nodes {
		userAchievements = append(userAchievements, ua.unwrap())
	}

	resp.PageInfo = &result.Data.User.UserAchievements.PageInfo

	return userAchievements, resp, nil
}

// ListAchievementsOptions represents the available ListAchievements()
// options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#namespaceachievements
type ListAchievementsOptions struct {
	// IDs filters the results to only these achievement IDs.
	IDs []int64

	// Pagination
	After  *string
	Before *string
	First  *int64
	Last   *int64
}

const listAchievementsQuery = `
	query ListAchievements($fullPath: ID!, $ids: [AchievementsAchievementID!], $after: String, $before: String, $first: Int, $last: Int) {
		namespace(fullPath: $fullPath) {
			achievements(ids: $ids, after: $after, before: $before, first: $first, last: $last) {
				nodes {` + achievementFields + `}
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

// ListAchievements lists the achievements defined in a namespace.
//
// fullPath is the full path of the namespace (group or project).
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#namespaceachievements
func (s *AchievementsService) ListAchievements(fullPath string, opt *ListAchievementsOptions, options ...RequestOptionFunc) ([]*Achievement, *Response, error) {
	if opt == nil {
		opt = &ListAchievementsOptions{}
	}

	var ids []string
	if len(opt.IDs) > 0 {
		ids = newGIDStrings("Achievements::Achievement", opt.IDs...)
	}

	query := GraphQLQuery{
		Query: listAchievementsQuery,
		Variables: map[string]any{
			"fullPath": fullPath,
			"ids":      ids,
			"after":    opt.After,
			"before":   opt.Before,
			linkFirst:  opt.First,
			linkLast:   opt.Last,
		},
	}

	var result struct {
		Data struct {
			Namespace *struct {
				Achievements connectionGQL[achievementGQL] `json:"achievements"`
			} `json:"namespace"`
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
	if result.Data.Namespace == nil {
		return nil, resp, ErrNotFound
	}

	achievements := make([]*Achievement, 0, len(result.Data.Namespace.Achievements.Nodes))
	for _, a := range result.Data.Namespace.Achievements.Nodes {
		achievements = append(achievements, a.unwrap())
	}

	resp.PageInfo = &result.Data.Namespace.Achievements.PageInfo

	return achievements, resp, nil
}

// ListAchievementRecipientsOptions represents the available
// ListAchievementRecipients() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#achievement-userachievements
type ListAchievementRecipientsOptions struct {
	// Pagination
	After  *string
	Before *string
	First  *int64
	Last   *int64
}

const listAchievementRecipientsQuery = `
	query ListAchievementRecipients($fullPath: ID!, $achievementId: [AchievementsAchievementID!], $after: String, $before: String, $first: Int, $last: Int) {
		namespace(fullPath: $fullPath) {
			achievements(ids: $achievementId) {
				nodes {
					userAchievements(after: $after, before: $before, first: $first, last: $last) {
						nodes {` + userAchievementFields + `}
						pageInfo {
							endCursor
							hasNextPage
							startCursor
							hasPreviousPage
						}
					}
				}
			}
		}
	}
`

// ListAchievementRecipients lists the users an achievement has been awarded
// to.
//
// fullPath is the full path of the namespace (group or project) the
// achievement belongs to.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#achievement-userachievements
func (s *AchievementsService) ListAchievementRecipients(fullPath string, achievementID int64, opt *ListAchievementRecipientsOptions, options ...RequestOptionFunc) ([]*UserAchievement, *Response, error) {
	if opt == nil {
		opt = &ListAchievementRecipientsOptions{}
	}

	achievementGID := gidGQL{Type: "Achievements::Achievement", Int64: achievementID}

	query := GraphQLQuery{
		Query: listAchievementRecipientsQuery,
		Variables: map[string]any{
			"fullPath":      fullPath,
			"achievementId": []string{achievementGID.String()},
			"after":         opt.After,
			"before":        opt.Before,
			linkFirst:       opt.First,
			linkLast:        opt.Last,
		},
	}

	var result struct {
		Data struct {
			Namespace *struct {
				Achievements connectionGQL[struct {
					UserAchievements connectionGQL[userAchievementGQL] `json:"userAchievements"`
				}] `json:"achievements"`
			} `json:"namespace"`
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
	if result.Data.Namespace == nil || len(result.Data.Namespace.Achievements.Nodes) == 0 {
		return nil, resp, ErrNotFound
	}

	achievement := result.Data.Namespace.Achievements.Nodes[0]

	userAchievements := make([]*UserAchievement, 0, len(achievement.UserAchievements.Nodes))
	for _, ua := range achievement.UserAchievements.Nodes {
		userAchievements = append(userAchievements, ua.unwrap())
	}

	resp.PageInfo = &achievement.UserAchievements.PageInfo

	return userAchievements, resp, nil
}

// ListAchievementUniqueUsersOptions represents the available
// ListAchievementUniqueUsers() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#achievement-uniqueusers
type ListAchievementUniqueUsersOptions struct {
	// Pagination
	After  *string
	Before *string
	First  *int64
	Last   *int64
}

// The user field selection comes from userCoreBasicFields (see users.go) so
// it cannot drift from the other services returning a *BasicUser.
const listAchievementUniqueUsersQuery = `
	query ListAchievementUniqueUsers($fullPath: ID!, $achievementId: [AchievementsAchievementID!], $after: String, $before: String, $first: Int, $last: Int) {
		namespace(fullPath: $fullPath) {
			achievements(ids: $achievementId) {
				nodes {
					uniqueUsers(after: $after, before: $before, first: $first, last: $last) {
						nodes {` + userCoreBasicFields + `}
						pageInfo {
							endCursor
							hasNextPage
							startCursor
							hasPreviousPage
						}
					}
				}
			}
		}
	}
`

// ListAchievementUniqueUsers lists the distinct users who have received an
// achievement.
//
// fullPath is the full path of the namespace (group or project) the
// achievement belongs to.
//
// GitLab API docs:
// https://docs.gitlab.com/api/graphql/reference/#achievement-uniqueusers
func (s *AchievementsService) ListAchievementUniqueUsers(fullPath string, achievementID int64, opt *ListAchievementUniqueUsersOptions, options ...RequestOptionFunc) ([]*BasicUser, *Response, error) {
	if opt == nil {
		opt = &ListAchievementUniqueUsersOptions{}
	}

	achievementGID := gidGQL{Type: "Achievements::Achievement", Int64: achievementID}

	query := GraphQLQuery{
		Query: listAchievementUniqueUsersQuery,
		Variables: map[string]any{
			"fullPath":      fullPath,
			"achievementId": []string{achievementGID.String()},
			"after":         opt.After,
			"before":        opt.Before,
			linkFirst:       opt.First,
			linkLast:        opt.Last,
		},
	}

	var result struct {
		Data struct {
			Namespace *struct {
				Achievements connectionGQL[struct {
					UniqueUsers connectionGQL[userCoreBasicGQL] `json:"uniqueUsers"`
				}] `json:"achievements"`
			} `json:"namespace"`
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
	if result.Data.Namespace == nil || len(result.Data.Namespace.Achievements.Nodes) == 0 {
		return nil, resp, ErrNotFound
	}

	achievement := result.Data.Namespace.Achievements.Nodes[0]

	users := make([]*BasicUser, 0, len(achievement.UniqueUsers.Nodes))
	for _, u := range achievement.UniqueUsers.Nodes {
		users = append(users, u.unwrap())
	}

	resp.PageInfo = &achievement.UniqueUsers.PageInfo

	return users, resp, nil
}
