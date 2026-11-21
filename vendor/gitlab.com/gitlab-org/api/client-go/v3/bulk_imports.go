package gitlab

import (
	"net/http"
	"time"
)

var (
	routeBulkImports                     = route("bulk_imports")
	routeBulkImportsEntities             = route("bulk_imports/entities")
	routeBulkImportsID                   = route("bulk_imports/%d")
	routeBulkImportsIDCancel             = route("bulk_imports/%d/cancel")
	routeBulkImportsIDEntities           = route("bulk_imports/%d/entities")
	routeBulkImportsIDEntitiesID         = route("bulk_imports/%d/entities/%d")
	routeBulkImportsIDEntitiesIDFailures = route("bulk_imports/%d/entities/%d/failures")
)

type (
	BulkImportsServiceInterface interface {
		// StartMigration starts a new group or project migration.
		//
		// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#start-a-group-or-project-migration
		StartMigration(startMigrationOptions *BulkImportStartMigrationOptions, options ...RequestOptionFunc) (*BulkImportStartMigrationResponse, *Response, error)

		// ListBulkImports lists all group or project migrations.
		//
		// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#list-all-group-or-project-migrations
		ListBulkImports(opts *ListBulkImportsOptions, options ...RequestOptionFunc) ([]*BulkImport, *Response, error)

		// ListBulkImportsEntities lists all group or project migration entities.
		//
		// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#list-all-group-or-project-migration-entities
		ListBulkImportsEntities(opts *ListBulkImportsEntitiesOptions, options ...RequestOptionFunc) ([]*BulkImportEntity, *Response, error)

		// GetBulkImportEntityFailures lists failed import records for a group or project migration entity.
		//
		// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#list-failed-import-records-for-a-migration-entity
		GetBulkImportEntityFailures(id int64, entityID int64, options ...RequestOptionFunc) ([]*BulkImportEntityFailure, *Response, error)

		// GetBulkImport retrieves a group or project migration.
		//
		// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#retrieve-a-group-or-project-migration
		GetBulkImport(id int64, options ...RequestOptionFunc) (*BulkImport, *Response, error)

		// ListBulkImportsEntitiesByID lists migration entities for a specific migration.
		//
		// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#list-group-or-project-migration-entities
		ListBulkImportsEntitiesByID(id int64, opts *ListBulkImportsEntitiesOptions, options ...RequestOptionFunc) ([]*BulkImportEntity, *Response, error)

		// GetBulkImportEntity retrieves a group or project migration entity.
		//
		// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#retrieve-a-group-or-project-migration-entity
		GetBulkImportEntity(id int64, entityID int64, options ...RequestOptionFunc) (*BulkImportEntity, *Response, error)

		// CancelBulkImport cancels a direct transfer migration.
		//
		// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#cancel-a-migration
		CancelBulkImport(id int64, options ...RequestOptionFunc) (*BulkImport, *Response, error)
	}

	// BulkImportsService handles communication with GitLab's direct transfer API.
	//
	// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/
	BulkImportsService struct {
		client *Client
	}
)

var _ BulkImportsServiceInterface = (*BulkImportsService)(nil)

// BulkImportStartMigrationConfiguration represents the available configuration options to start a migration.
//
// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#start-a-new-group-or-project-migration
type BulkImportStartMigrationConfiguration struct {
	URL         *string `json:"url,omitempty"`
	AccessToken *string `json:"access_token,omitempty"`
}

// BulkImportStartMigrationEntity represents the available entity options to start a migration.
//
// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#start-a-new-group-or-project-migration
type BulkImportStartMigrationEntity struct {
	SourceType           *string `json:"source_type,omitempty"`
	SourceFullPath       *string `json:"source_full_path,omitempty"`
	DestinationSlug      *string `json:"destination_slug,omitempty"`
	DestinationNamespace *string `json:"destination_namespace,omitempty"`
	MigrateProjects      *bool   `json:"migrate_projects,omitempty"`
	MigrateMemberships   *bool   `json:"migrate_memberships,omitempty"`
}

// BulkImportStartMigrationOptions represents the available start migration options.
//
// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#start-a-new-group-or-project-migration
type BulkImportStartMigrationOptions struct {
	Configuration *BulkImportStartMigrationConfiguration `json:"configuration,omitempty"`
	Entities      []BulkImportStartMigrationEntity       `json:"entities,omitempty"`
}

// BulkImportStartMigrationResponse represents the start migration response.
//
// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#start-a-new-group-or-project-migration
type BulkImportStartMigrationResponse struct {
	ID          int64     `json:"id"`
	Status      string    `json:"status"`
	SourceType  string    `json:"source_type"`
	SourceURL   string    `json:"source_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	HasFailures bool      `json:"has_failures"`
}

func (b *BulkImportsService) StartMigration(startMigrationOptions *BulkImportStartMigrationOptions, options ...RequestOptionFunc) (*BulkImportStartMigrationResponse, *Response, error) {
	return do[*BulkImportStartMigrationResponse](b.client,
		withMethod(http.MethodPost),
		withPath(routeBulkImports),
		withAPIOpts(startMigrationOptions),
		withRequestOpts(options...),
	)
}

// ListBulkImportsOptions represents the available configuration options to list bulk imports.
//
// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#list-all-group-or-project-migrations
type ListBulkImportsOptions struct {
	ListOptions
	Status *string `url:"status,omitempty" json:"status,omitempty"`
}

type BulkImport struct {
	ID          int64     `json:"id"`
	Status      string    `json:"status"`
	SourceType  string    `json:"source_type"`
	SourceURL   string    `json:"source_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	HasFailures bool      `json:"has_failures"`
}

// ListBulkImports lists all group or project migrations.
//
// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#list-all-group-or-project-migrations
func (s *BulkImportsService) ListBulkImports(opts *ListBulkImportsOptions, options ...RequestOptionFunc) ([]*BulkImport, *Response, error) {
	return do[[]*BulkImport](s.client,
		withPath(routeBulkImports),
		withAPIOpts(opts),
		withRequestOpts(options...),
	)
}

// ListBulkImportsEntitiesOptions represents the available configuration options to list bulk import entities.
//
// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#list-all-group-or-project-migration-entities
type ListBulkImportsEntitiesOptions struct {
	ListOptions
	Status *string `url:"status,omitempty" json:"status,omitempty"`
}

// BulkImportEntityStats represents the import counts for each relation type in a migration entity.
//
// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#list-all-group-or-project-migration-entities
type BulkImportEntityStats struct {
	Labels     BulkImportEntityStatItem `json:"labels"`
	Milestones BulkImportEntityStatItem `json:"milestones"`
}

type BulkImportEntityStatItem struct {
	Source   int `json:"source"`
	Fetched  int `json:"fetched"`
	Imported int `json:"imported"`
}

type BulkImportEntity struct {
	ID                   int64                      `json:"id"`
	BulkImportID         int64                      `json:"bulk_import_id"`
	Status               string                     `json:"status"`
	EntityType           string                     `json:"entity_type"`
	SourceFullPath       string                     `json:"source_full_path"`
	DestinationFullPath  string                     `json:"destination_full_path"`
	DestinationName      string                     `json:"destination_name"`
	DestinationSlug      string                     `json:"destination_slug"`
	DestinationNamespace string                     `json:"destination_namespace"`
	ParentID             *int64                     `json:"parent_id"`
	NamespaceID          *int64                     `json:"namespace_id"`
	ProjectID            *int64                     `json:"project_id"`
	CreatedAt            time.Time                  `json:"created_at"`
	UpdatedAt            time.Time                  `json:"updated_at"`
	Failures             []*BulkImportEntityFailure `json:"failures"`
	MigrateProjects      bool                       `json:"migrate_projects"`
	MigrateMemberships   bool                       `json:"migrate_memberships"`
	HasFailures          bool                       `json:"has_failures"`
	Stats                BulkImportEntityStats      `json:"stats"`
}

// ListBulkImportsEntities lists all group or project migration entities.
//
// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#list-all-group-or-project-migration-entities
func (s *BulkImportsService) ListBulkImportsEntities(opts *ListBulkImportsEntitiesOptions, options ...RequestOptionFunc) ([]*BulkImportEntity, *Response, error) {
	return do[[]*BulkImportEntity](s.client,
		withPath(routeBulkImportsEntities),
		withAPIOpts(opts),
		withRequestOpts(options...),
	)
}

// BulkImportEntityFailure represents a failed import record for a migration entity.
//
// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#list-failed-import-records-for-a-migration-entity
type BulkImportEntityFailure struct {
	Relation           string    `json:"relation"`
	ExceptionMessage   string    `json:"exception_message"`
	ExceptionClass     string    `json:"exception_class"`
	CorrelationIDValue string    `json:"correlation_id_value"`
	SourceURL          string    `json:"source_url"`
	SourceTitle        string    `json:"source_title"`
	Step               string    `json:"step"`
	CreatedAt          time.Time `json:"created_at"`
	PipelineClass      string    `json:"pipeline_class"`
	PipelineStep       string    `json:"pipeline_step"`
}

// GetBulkImportEntityFailures lists failed import records for a group or project migration entity.
//
// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#list-failed-import-records-for-a-migration-entity
func (s *BulkImportsService) GetBulkImportEntityFailures(id int64, entityID int64, options ...RequestOptionFunc) ([]*BulkImportEntityFailure, *Response, error) {
	return do[[]*BulkImportEntityFailure](s.client,
		withPath(routeBulkImportsIDEntitiesIDFailures, id, entityID),
		withRequestOpts(options...),
	)
}

// GetBulkImport retrieves a group or project migration.
//
// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#retrieve-a-group-or-project-migration
func (s *BulkImportsService) GetBulkImport(id int64, options ...RequestOptionFunc) (*BulkImport, *Response, error) {
	return do[*BulkImport](s.client,
		withPath(routeBulkImportsID, id),
		withRequestOpts(options...),
	)
}

// ListBulkImportsEntitiesByID lists migration entities for a specific migration.
//
// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#list-group-or-project-migration-entities
func (s *BulkImportsService) ListBulkImportsEntitiesByID(id int64, opts *ListBulkImportsEntitiesOptions, options ...RequestOptionFunc) ([]*BulkImportEntity, *Response, error) {
	return do[[]*BulkImportEntity](s.client,
		withPath(routeBulkImportsIDEntities, id),
		withAPIOpts(opts),
		withRequestOpts(options...),
	)
}

// GetBulkImportEntity retrieves a group or project migration entity.
//
// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#retrieve-a-group-or-project-migration-entity
func (s *BulkImportsService) GetBulkImportEntity(id int64, entityID int64, options ...RequestOptionFunc) (*BulkImportEntity, *Response, error) {
	return do[*BulkImportEntity](s.client,
		withPath(routeBulkImportsIDEntitiesID, id, entityID),
		withRequestOpts(options...),
	)
}

// CancelBulkImport cancels a direct transfer migration.
//
// GitLab API docs: https://docs.gitlab.com/api/bulk_imports/#cancel-a-migration
func (s *BulkImportsService) CancelBulkImport(id int64, options ...RequestOptionFunc) (*BulkImport, *Response, error) {
	return do[*BulkImport](s.client,
		withMethod(http.MethodPost),
		withPath(routeBulkImportsIDCancel, id),
		withRequestOpts(options...),
	)
}
