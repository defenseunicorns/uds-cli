package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	"github.com/stretchr/testify/require"
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
		},
		{
			name:        "ImportDoesntMatchExport",
			description: "error when import doesn't match export",
			args: args{
				packages: []types.Package{
					{Name: "foo", Exports: []types.BundleVariableExport{{Name: "foo"}}},
					{Name: "bar", Imports: []types.BundleVariableImport{{Name: "bar", Package: "foo"}}},
				},
			},
			wantErr: true,
		},
		{
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
			err := validateBundleVars(tt.args.packages)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func Test_validateOverrides(t *testing.T) {
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
					Name: "foo", Overrides: map[string]map[string]types.BundleChartOverrides{"component": {"chart": {}}}},
				zarfPackage: zarfTypes.ZarfPackage{
					Components: []zarfTypes.ZarfComponent{
						{Name: "component", Charts: []zarfTypes.ZarfChart{{Name: "chart"}}},
					},
				}},
			wantErr: false,
		},
		{
			name:        "validOverrideMultipleComponents",
			description: "Respective components and charts exist for override when multiple charts and components are present",
			args: args{
				bundlePackage: types.Package{
					Name: "foo", Overrides: map[string]map[string]types.BundleChartOverrides{"component-a": {"chart-1": {}}}},
				zarfPackage: zarfTypes.ZarfPackage{
					Components: []zarfTypes.ZarfComponent{
						{Name: "component-a", Charts: []zarfTypes.ZarfChart{{Name: "chart-1"}, {Name: "chart-2"}}},
						{Name: "component-b", Charts: []zarfTypes.ZarfChart{{Name: "chart-b"}}},
					},
				}},
			wantErr: false,
		},
		{
			name:        "invalidComponentOverride",
			description: "Component does not exist for override",
			args: args{
				bundlePackage: types.Package{
					Name: "foo", Overrides: map[string]map[string]types.BundleChartOverrides{"hell-unleashed": {"chart": {}}}},
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
					Name: "foo", Overrides: map[string]map[string]types.BundleChartOverrides{"component": {"hell-unleashed": {}}}},
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
			err := validateOverrides(tt.args.bundlePackage, tt.args.zarfPackage)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func Test_getPkgPath(t *testing.T) {
	type args struct {
		pkg    types.Package
		arch   string
		srcDir string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "init full path",
			args: args{
				pkg:    types.Package{Name: "init", Ref: "0.0.1", Path: "../fake/path/custom-init.tar.zst"},
				arch:   "fake64",
				srcDir: "/mock/source",
			},
			want: "/mock/fake/path/custom-init.tar.zst",
		},
		{
			name: "init directory only path",
			args: args{
				pkg:    types.Package{Name: "init", Ref: "0.0.1", Path: "../fake/path"},
				arch:   "fake64",
				srcDir: "/mock/source",
			},
			want: "/mock/fake/path/zarf-init-fake64-0.0.1.tar.zst",
		},
		{
			name: "full path",
			args: args{
				pkg:    types.Package{Name: "nginx", Ref: "0.0.1", Path: "./fake/zarf-package-nginx-fake64-0.0.1.tar.zst"},
				arch:   "fake64",
				srcDir: "/mock/source",
			},
			want: "/mock/source/fake/zarf-package-nginx-fake64-0.0.1.tar.zst",
		},
		{
			name: "directory only path",
			args: args{
				pkg:    types.Package{Name: "nginx", Ref: "0.0.1", Path: "fake"},
				arch:   "fake64",
				srcDir: "/mock/source",
			},
			want: "/mock/source/fake/zarf-package-nginx-fake64-0.0.1.tar.zst",
		},
		{
			name: "absolute path",
			args: args{
				pkg:    types.Package{Name: "nginx", Ref: "0.0.1", Path: "/fake/zarf-package-nginx-fake64-0.0.1.tar.zst"},
				arch:   "fake64",
				srcDir: "/mock/source",
			},
			want: "/fake/zarf-package-nginx-fake64-0.0.1.tar.zst",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := getPkgPath(tt.args.pkg, tt.args.arch, tt.args.srcDir)
			require.Equal(t, tt.want, path)
		})
	}
}
