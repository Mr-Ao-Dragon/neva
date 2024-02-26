package desugarer

import (
	"testing"

	"github.com/nevalang/neva/pkg"
	src "github.com/nevalang/neva/pkg/sourcecode"
	"github.com/stretchr/testify/require"
)

// TODO
// 1) refactor this test, split into sub-method tests
// 2) Write test for then connection and make sure it works
// 3) Fix then connection if needed
// 4) Find problem with program number 4 with_bridge if needed

func TestDesugarer_desugarModule(t *testing.T) {
	tests := []struct {
		name    string
		mod     src.Module
		want    src.Module
		wantErr bool
	}{
		// every output module must have std module dependency
		{
			name: "every desugared module has std mod dep with right version",
			mod: src.Module{
				Manifest: src.ModuleManifest{
					Deps: map[string]src.ModuleRef{}, // <-- no std mod dep
				},
				Packages: map[string]src.Package{},
			},
			want: src.Module{
				Manifest: src.ModuleManifest{
					Deps: defaultDeps, // <-- std mod dep
				},
				Packages: map[string]src.Package{},
			},
			wantErr: false,
		},
	}

	d := Desugarer{}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			modRef := src.ModuleRef{Path: "@"}
			build := src.Build{
				Modules: map[src.ModuleRef]src.Module{
					modRef: tt.mod,
				},
			}

			got, err := d.desugarModule(build, modRef)
			require.Equal(t, err != nil, tt.wantErr)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestDesugarer_desugarFile(t *testing.T) {
	tests := []struct {
		name    string
		file    src.File
		want    src.File
		wantErr bool
	}{
		{
			name: "every desugared file has std/builtin import",
			file: src.File{
				Imports: map[string]src.Import{}, // <-- no imports of std/builtin
			},
			want: src.File{
				Imports:  defaultImports(), // <-- std/builtin import
				Entities: map[string]src.Entity{},
			},
			wantErr: false,
		},
	}
	d := Desugarer{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.desugarFile(tt.file, src.Scope{})
			require.Equal(t, err != nil, tt.wantErr)
			require.Equal(t, tt.want, got)
		})
	}
}

// === Helpers ===

func defaultImports() map[string]src.Import {
	return map[string]src.Import{
		"builtin": {
			Module:  "std",
			Package: "builtin",
		},
	}
}

var defaultDeps = map[string]src.ModuleRef{
	"std": {
		Path:    "std",
		Version: pkg.Version,
	},
}
