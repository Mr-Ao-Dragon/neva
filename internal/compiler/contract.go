package compiler

import (
	"context"

	"github.com/nevalang/neva/pkg/ir"
	src "github.com/nevalang/neva/pkg/sourcecode"
)

const (
	RuntimeFuncDirective    src.Directive = "runtime_func"
	RuntimeFuncMsgDirective src.Directive = "runtime_func_msg"
)

type (
	RawBuild struct {
		EntryModRef src.ModuleRef
		Modules     map[src.ModuleRef]RawModule
	}

	Parser interface {
		ParseModules(rawMods map[src.ModuleRef]RawModule) (map[src.ModuleRef]src.Module, *Error)
	}

	RawModule struct {
		Manifest src.ModuleManifest    // Manifest must be parsed by builder before passing into compiler
		Packages map[string]RawPackage // Packages themselves on the other hand can be parsed by compiler
	}

	RawPackage map[string][]byte

	Desugarer interface {
		Desugar(build src.Build) (src.Build, *Error)
	}

	Analyzer interface {
		AnalyzeExecutableBuild(mod src.Build, mainPkgName string) (src.Build, *Error)
	}

	IRGenerator interface {
		Generate(ctx context.Context, build src.Build, mainPkgName string) (*ir.Program, *Error)
	}

	Backend interface {
		GenerateTarget(*ir.Program) ([]byte, error)
	}
)