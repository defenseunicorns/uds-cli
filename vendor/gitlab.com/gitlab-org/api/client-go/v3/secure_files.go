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
	"io"
	"net/http"
	"time"
)

var (
	routeProjectsIDSecureFiles           = route("projects/%s/secure_files")
	routeProjectsIDSecureFilesID         = route("projects/%s/secure_files/%d")
	routeProjectsIDSecureFilesIDDownload = route("projects/%s/secure_files/%d/download")
)

type (
	SecureFilesServiceInterface interface {
		ListProjectSecureFiles(pid any, opt *ListProjectSecureFilesOptions, options ...RequestOptionFunc) ([]*SecureFile, *Response, error)
		ShowSecureFileDetails(pid any, id int64, options ...RequestOptionFunc) (*SecureFile, *Response, error)
		CreateSecureFile(pid any, content io.Reader, opt *CreateSecureFileOptions, options ...RequestOptionFunc) (*SecureFile, *Response, error)
		// DownloadSecureFile downloads the contents of a project's secure file.
		//
		// The returned io.ReadCloser must be closed by the caller to avoid
		// leaking the underlying response body.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/secure_files/#download-secure-file
		DownloadSecureFile(pid any, id int64, options ...RequestOptionFunc) (io.ReadCloser, *Response, error)
		RemoveSecureFile(pid any, id int64, options ...RequestOptionFunc) (*Response, error)
	}

	// SecureFilesService handles communication with the secure files related
	// methods of the GitLab API.
	//
	// GitLab API docs:
	// https://docs.gitlab.com/api/secure_files/
	SecureFilesService struct {
		client *Client
	}
)

var _ SecureFilesServiceInterface = (*SecureFilesService)(nil)

// SecureFile represents a single secure file.
//
// GitLab API docs:
// https://docs.gitlab.com/api/secure_files/
type SecureFile struct {
	ID                int64               `json:"id"`
	Name              string              `json:"name"`
	Checksum          string              `json:"checksum"`
	ChecksumAlgorithm string              `json:"checksum_algorithm"`
	CreatedAt         *time.Time          `json:"created_at"`
	ExpiresAt         *time.Time          `json:"expires_at"`
	Metadata          *SecureFileMetadata `json:"metadata"`
	FileExtension     string              `json:"file_extension"`
}

// SecureFileMetadata represents the metadata for a secure file.
//
// GitLab API docs:
// https://docs.gitlab.com/api/secure_files/
type SecureFileMetadata struct {
	ID        string            `json:"id"`
	Issuer    SecureFileIssuer  `json:"issuer"`
	Subject   SecureFileSubject `json:"subject"`
	ExpiresAt *time.Time        `json:"expires_at"`
}

// SecureFileIssuer represents the issuer of a secure file.
//
// GitLab API docs:
// https://docs.gitlab.com/api/secure_files/
type SecureFileIssuer struct {
	C  string `json:"C"`
	O  string `json:"O"`
	CN string `json:"CN"`
	OU string `json:"OU"`
}

// SecureFileSubject represents the subject of a secure file.
//
// GitLab API docs:
// https://docs.gitlab.com/api/secure_files/
type SecureFileSubject struct {
	C   string `json:"C"`
	O   string `json:"O"`
	CN  string `json:"CN"`
	OU  string `json:"OU"`
	UID string `json:"UID"`
}

// String gets a string representation of a SecureFile.
//
// GitLab API docs:
// https://docs.gitlab.com/api/secure_files/
func (f SecureFile) String() string {
	return Stringify(f)
}

// ListProjectSecureFilesOptions represents the available
// ListProjectSecureFiles() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/secure_files/#list-project-secure-files
type ListProjectSecureFilesOptions struct {
	ListOptions
}

// ListProjectSecureFiles gets a list of secure files in a project.
//
// GitLab API docs:
// https://docs.gitlab.com/api/secure_files/#list-project-secure-files
func (s SecureFilesService) ListProjectSecureFiles(pid any, opt *ListProjectSecureFilesOptions, options ...RequestOptionFunc) ([]*SecureFile, *Response, error) {
	return do[[]*SecureFile](s.client,
		withPath(routeProjectsIDSecureFiles, ProjectID{pid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// ShowSecureFileDetails gets the details of a specific secure file in a project.
//
// GitLab API docs:
// https://docs.gitlab.com/api/secure_files/#show-secure-file-details
func (s SecureFilesService) ShowSecureFileDetails(pid any, id int64, options ...RequestOptionFunc) (*SecureFile, *Response, error) {
	return do[*SecureFile](s.client,
		withPath(routeProjectsIDSecureFilesID, ProjectID{pid}, id),
		withRequestOpts(options...),
	)
}

// CreateSecureFileOptions represents the available
// CreateSecureFile() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/secure_files/#create-secure-file
type CreateSecureFileOptions struct {
	Name *string `url:"name,omitempty" json:"name,omitempty"`
}

// CreateSecureFile creates a new secure file.
//
// GitLab API docs:
// https://docs.gitlab.com/api/secure_files/#create-secure-file
func (s SecureFilesService) CreateSecureFile(pid any, content io.Reader, opt *CreateSecureFileOptions, options ...RequestOptionFunc) (*SecureFile, *Response, error) {
	return do[*SecureFile](s.client,
		withMethod(http.MethodPost),
		withPath(routeProjectsIDSecureFiles, ProjectID{pid}),
		withUpload(content, *opt.Name, UploadFile),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// DownloadSecureFile downloads the contents of a project's secure file.
//
// The returned io.ReadCloser must be closed by the caller to avoid
// leaking the underlying response body.
//
// GitLab API docs:
// https://docs.gitlab.com/api/secure_files/#download-secure-file
func (s SecureFilesService) DownloadSecureFile(pid any, id int64, options ...RequestOptionFunc) (io.ReadCloser, *Response, error) {
	r, resp, err := do[bodyReader](s.client,
		withPath(routeProjectsIDSecureFilesIDDownload, ProjectID{pid}, id),
		withRequestOpts(options...),
	)
	if err != nil {
		return nil, resp, err
	}
	return &r, resp, nil
}

// RemoveSecureFile removes a project's secure file.
//
// GitLab API docs:
// https://docs.gitlab.com/api/secure_files/#remove-secure-file
func (s SecureFilesService) RemoveSecureFile(pid any, id int64, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodDelete),
		withPath(routeProjectsIDSecureFilesID, ProjectID{pid}, id),
		withRequestOpts(options...),
	)
	return resp, err
}
