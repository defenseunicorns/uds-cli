package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
)

func Test_validateBundleVars(t *testing.T) {
	type args struct {
		packages []types.Package
	}
	tests := []struct {
		name        string
		description string
		args        args
		wantErr     bool
	}{
		{
			name:        "ImportMatchesExport",
			description: "import matches export",
			args: args{
				packages: []types.Package{
					{Name: "foo", Exports: []types.BundleVariableExport{{Name: "foo"}}},
					{Name: "bar", Imports: []types.BundleVariableImport{{Name: "foo", Package: "foo"}}},
				},
			},
			wantErr: false,
		}, {
			name:        "ImportDoesntMatchExport",
			description: "error when import doesn't match export",
			args: args{
				packages: []types.Package{
					{Name: "foo", Exports: []types.BundleVariableExport{{Name: "foo"}}},
					{Name: "bar", Imports: []types.BundleVariableImport{{Name: "bar", Package: "foo"}}},
				},
			},
			wantErr: true,
		}, {
			name:        "FirstPkgHasImport",
			description: "error when first pkg has an import",
			args: args{
				packages: []types.Package{
					{Name: "foo", Imports: []types.BundleVariableImport{{Name: "foo", Package: "foo"}}},
				},
			},
			wantErr: true,
		},
		{
			name:        "PackageNamesMustMatch",
			description: "error when package name doesn't match",
			args: args{
				packages: []types.Package{
					{Name: "foo", Exports: []types.BundleVariableExport{{Name: "foo"}}},
					{Name: "bar", Imports: []types.BundleVariableImport{{Name: "foo", Package: "baz"}}},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateBundleVars(tt.args.packages); (err != nil) != tt.wantErr {
				t.Errorf("validateBundleVars() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_validationForBundleOverrides(t *testing.T) {
	type args struct {
		bundlePackage types.Package
		zarfPackage   zarfTypes.ZarfPackage
	}
	tests := []struct {
		name        string
		description string
		args        args
		wantErr     bool
	}{
		{
			name:        "validOverride",
			description: "Respective components and charts exist for override",
			args: args{
				bundlePackage: types.Package{
					Name: "foo", Overrides: map[string]map[string]types.BundleChartOverrides{"component": {"chart": {}}}} ,
				zarfPackage: zarfTypes.ZarfPackage{
					Components: []zarfTypes.ZarfComponent{
						{Name: "component", Charts: []zarfTypes.ZarfChart{{Name: "chart"}}},
					},
				}},
			wantErr: false,
		},
		{
			name:        "invalidComponentOverride",
			description: "Component does not exist for override",
			args: args{
				bundlePackage: types.Package{
					Name: "foo", Overrides: map[string]map[string]types.BundleChartOverrides{"hell-unleashed": {"chart": {}}}} ,
				zarfPackage: zarfTypes.ZarfPackage{
					Components: []zarfTypes.ZarfComponent{
						{Name: "hello-world", Charts: []zarfTypes.ZarfChart{{Name: "chart"}}},
					},
				}},
			wantErr: true,
		},
		{
			name:        "invalidChartOverride",
			description: "Chart does not exist for override",
			args: args{
				bundlePackage: types.Package{
					Name: "foo", Overrides: map[string]map[string]types.BundleChartOverrides{"component": {"hell-unleashed": {}}}} ,
				zarfPackage: zarfTypes.ZarfPackage{
					Components: []zarfTypes.ZarfComponent{
						{Name: "component", Charts: []zarfTypes.ZarfChart{{Name: "hello-world"}}},
					},
				}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateBundleForOverride(tt.args.bundlePackage, tt.args.zarfPackage); (err != nil) != tt.wantErr {
				t.Errorf("validateBundleForOverride() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
