// EXPERIMENTAL(#2294): The Dependency Firewall API is served behind the `dependency_firewall_phase1` feature flag and may introduce breaking changes even between minor versions.

package gitlab

import (
	"net/http"
)

var routeProjectsIDDependencyFirewallEvaluate = route("projects/%s/dependency_firewall/evaluate")

type (
	SecurityDependencyFirewallServiceInterface interface {
		// EvaluatePackage evaluates a single package coordinate against the
		// project's Dependency Firewall policies and returns the outcome.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/dependency_firewall/
		//
		// Experimental: The Dependency Firewall API is served behind the `dependency_firewall_phase1` feature flag and may introduce breaking changes even between minor versions.
		EvaluatePackage(pid any, opt *EvaluatePackageOptions, options ...RequestOptionFunc) (*PackageEvaluation, *Response, error)
	}

	// SecurityDependencyFirewallService handles communication with the
	// Dependency Firewall related methods of the GitLab API.
	//
	// GitLab API docs: https://docs.gitlab.com/api/dependency_firewall/
	//
	// Experimental: The Dependency Firewall API is served behind the `dependency_firewall_phase1` feature flag and may introduce breaking changes even between minor versions.
	SecurityDependencyFirewallService struct {
		client *Client
	}
)

var _ SecurityDependencyFirewallServiceInterface = (*SecurityDependencyFirewallService)(nil)

// DependencyFirewallEcosystemValue represents a package ecosystem that the
// Dependency Firewall can evaluate.
//
// GitLab API docs: https://docs.gitlab.com/api/dependency_firewall/
//
// Experimental: The Dependency Firewall API is served behind the `dependency_firewall_phase1` feature flag and may introduce breaking changes even between minor versions.
type DependencyFirewallEcosystemValue string

// List of available Dependency Firewall package ecosystems.
//
// GitLab API docs: https://docs.gitlab.com/api/dependency_firewall/
const (
	DependencyFirewallEcosystemMaven    DependencyFirewallEcosystemValue = "maven"
	DependencyFirewallEcosystemNPM      DependencyFirewallEcosystemValue = "npm"
	DependencyFirewallEcosystemPyPI     DependencyFirewallEcosystemValue = "pypi"
	DependencyFirewallEcosystemGem      DependencyFirewallEcosystemValue = "gem"
	DependencyFirewallEcosystemComposer DependencyFirewallEcosystemValue = "composer"
	DependencyFirewallEcosystemConan    DependencyFirewallEcosystemValue = "conan"
	DependencyFirewallEcosystemGolang   DependencyFirewallEcosystemValue = "golang"
	DependencyFirewallEcosystemNuGet    DependencyFirewallEcosystemValue = "nuget"
	DependencyFirewallEcosystemCargo    DependencyFirewallEcosystemValue = "cargo"
	DependencyFirewallEcosystemSwift    DependencyFirewallEcosystemValue = "swift"
	DependencyFirewallEcosystemPub      DependencyFirewallEcosystemValue = "pub"
)

// DependencyFirewallOutcomeValue represents the outcome of a Dependency
// Firewall package evaluation.
//
// GitLab API docs: https://docs.gitlab.com/api/dependency_firewall/
//
// Experimental: The Dependency Firewall API is served behind the `dependency_firewall_phase1` feature flag and may introduce breaking changes even between minor versions.
type DependencyFirewallOutcomeValue string

// List of available Dependency Firewall evaluation outcomes.
//
// GitLab API docs: https://docs.gitlab.com/api/dependency_firewall/
const (
	// DependencyFirewallOutcomeAllowed means no policy rule matched the
	// package. It is not an assertion that GitLab holds vulnerability or
	// license data for the package: a package absent from the package
	// metadata database is also allowed.
	DependencyFirewallOutcomeAllowed DependencyFirewallOutcomeValue = "allowed"

	// DependencyFirewallOutcomeWarned means a policy rule matched the
	// package, and the matching policy is in warn mode.
	DependencyFirewallOutcomeWarned DependencyFirewallOutcomeValue = "warned"

	// DependencyFirewallOutcomeBlocked means a policy rule matched the
	// package, and the matching policy is in enforce mode.
	DependencyFirewallOutcomeBlocked DependencyFirewallOutcomeValue = "blocked"
)

// PackageEvaluation represents the outcome of a Dependency Firewall package
// evaluation.
//
// GitLab API docs: https://docs.gitlab.com/api/dependency_firewall/
//
// Experimental: The Dependency Firewall API is served behind the `dependency_firewall_phase1` feature flag and may introduce breaking changes even between minor versions.
type PackageEvaluation struct {
	Outcome DependencyFirewallOutcomeValue `json:"outcome"`

	// Reason names the policy that produced a warned or blocked outcome.
	// It is null when Outcome is allowed, so this is a pointer rather than
	// a string: a caller can tell "no reason was given" apart from "the
	// reason was the empty string".
	Reason *string `json:"reason"`
}

// EvaluatePackageOptions represents the available EvaluatePackage() options.
//
// GitLab API docs: https://docs.gitlab.com/api/dependency_firewall/
//
// Experimental: The Dependency Firewall API is served behind the `dependency_firewall_phase1` feature flag and may introduce breaking changes even between minor versions.
type EvaluatePackageOptions struct {
	// Ecosystem is the package ecosystem. Required.
	Ecosystem DependencyFirewallEcosystemValue `url:"ecosystem" json:"ecosystem"`

	// Name is the package name, maximum 255 characters. Required. For
	// maven, use the `groupId:artifactId` form, for example
	// `com.example:trivial-lib`. For pypi, names are normalized according
	// to PEP 503 before evaluation, so `Flask_Login` and `flask-login` are
	// equivalent.
	Name string `url:"name" json:"name"`

	// Version is the package version, maximum 255 characters. Required.
	Version string `url:"version" json:"version"`
}

// EvaluatePackage evaluates a single package coordinate against the project's
// Dependency Firewall policies and returns the outcome.
//
// GitLab API docs: https://docs.gitlab.com/api/dependency_firewall/
//
// Experimental: The Dependency Firewall API is served behind the `dependency_firewall_phase1` feature flag and may introduce breaking changes even between minor versions.
func (s *SecurityDependencyFirewallService) EvaluatePackage(pid any, opt *EvaluatePackageOptions, options ...RequestOptionFunc) (*PackageEvaluation, *Response, error) {
	return do[*PackageEvaluation](s.client,
		withMethod(http.MethodPost),
		withPath(routeProjectsIDDependencyFirewallEvaluate, ProjectID{pid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}
