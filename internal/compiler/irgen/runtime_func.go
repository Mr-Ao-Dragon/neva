package irgen

import (
	"errors"
	"strings"

	"github.com/nevalang/neva/internal/compiler"
	"github.com/nevalang/neva/internal/runtime/ir"
	src "github.com/nevalang/neva/pkg/sourcecode"
	ts "github.com/nevalang/neva/pkg/typesystem"
)

func getRuntimeFuncRef(component src.Component, nodeTypeArgs []ts.Expr) (string, error) {
	args, ok := component.Directives[compiler.ExternDirective]
	if !ok {
		return "", nil
	}

	if len(args) == 1 {
		return args[0], nil
	}

	if len(nodeTypeArgs) == 0 || nodeTypeArgs[0].Inst == nil {
		// FIXME sometimes we have union here
		// we must use node argument instead of component type param
		return "", nil
	}

	firstTypeArg := nodeTypeArgs[0].Inst.Ref.String()
	for _, arg := range args {
		parts := strings.Split(arg, " ")
		if firstTypeArg == parts[0] {
			return parts[1], nil
		}
	}

	return "", errors.New("type argument mismatches runtime func directive")
}

func getRuntimeFuncMsg(node src.Node, scope src.Scope) (*ir.Msg, *compiler.Error) {
	args, ok := node.Directives[compiler.BindDirective]
	if !ok {
		return nil, nil
	}

	entity, location, err := scope.Entity(compiler.ParseEntityRef(args[0]))
	if err != nil {
		return nil, &compiler.Error{
			Err:      err,
			Location: &scope.Location,
		}
	}

	return getIRMsgBySrcRef(entity.Const, scope.WithLocation(location))
}
