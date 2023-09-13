package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
)

func Test_validateBundleVars(t *testing.T) {
	type args struct {
		packages []types.BundleZarfPackage
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
				packages: []types.BundleZarfPackage{
					{Name: "foo", Exports: []types.BundleVariableExport{{Name: "foo"}}},
					{Name: "bar", Imports: []types.BundleVariableImport{{Name: "foo", Package: "foo"}}},
				},
			},
			wantErr: false,
		}, {
			name:        "ImportDoesntMatchExport",
			description: "error when import doesn't match export",
			args: args{
				packages: []types.BundleZarfPackage{
					{Name: "foo", Exports: []types.BundleVariableExport{{Name: "foo"}}},
					{Name: "bar", Imports: []types.BundleVariableImport{{Name: "bar", Package: "foo"}}},
				},
			},
			wantErr: true,
		}, {
			name:        "FirstPkgHasImport",
			description: "error when first pkg has an import",
			args: args{
				packages: []types.BundleZarfPackage{
					{Name: "foo", Imports: []types.BundleVariableImport{{Name: "foo", Package: "foo"}}},
				},
			},
			wantErr: true,
		},
		{
			name:        "PackageNamesMustMatch",
			description: "error when package name doesn't match",
			args: args{
				packages: []types.BundleZarfPackage{
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
