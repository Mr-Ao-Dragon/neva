package irgen

import (
	"fmt"

	"github.com/nevalang/neva/internal/compiler"
	"github.com/nevalang/neva/internal/compiler/ir"
	src "github.com/nevalang/neva/internal/compiler/sourcecode"
	"github.com/nevalang/neva/internal/compiler/sourcecode/core"
)

type Generator struct{}

type (
	nodeContext struct {
		path       []string
		node       src.Node
		portsUsage portsUsage
	}

	portsUsage struct {
		in  map[relPortAddr]struct{}
		out map[relPortAddr]struct{}
	}

	relPortAddr struct {
		Port string
		Idx  *uint8
	}
)

func (g Generator) Generate(
	build src.Build,
	mainPkgName string,
) (*ir.Program, *compiler.Error) {
	scope := src.Scope{
		Build: build,
		Location: src.Location{
			Module:   build.EntryModRef,
			Package:  mainPkgName,
			Filename: "",
		},
	}

	result := &ir.Program{
		Connections: map[ir.PortAddr]ir.PortAddr{}, // Changed to 1-1 mapping
		Funcs:       []ir.FuncCall{},
	}

	rootNodeCtx := nodeContext{
		path: []string{},
		node: src.Node{
			EntityRef: core.EntityRef{
				Pkg:  "",
				Name: "Main",
			},
		},
		portsUsage: portsUsage{
			in: map[relPortAddr]struct{}{
				{Port: "start"}: {},
			},
			out: map[relPortAddr]struct{}{
				{Port: "stop"}: {},
			},
		},
	}

	if err := g.processNode(rootNodeCtx, scope, result); err != nil {
		return nil, compiler.Error{
			Location: &scope.Location,
		}.Wrap(err)
	}

	// graph reduction is not an optimization:
	// it's a necessity for runtime not to have intermediate connections
	reducedNet := g.reduceFinalGraph(result.Connections)

	return &ir.Program{
		Connections: reducedNet,
		Funcs:       result.Funcs,
	}, nil
}

func (g Generator) processNode(
	nodeCtx nodeContext,
	scope src.Scope,
	result *ir.Program,
) *compiler.Error {
	entity, location, err := scope.Entity(nodeCtx.node.EntityRef)
	if err != nil {
		return &compiler.Error{
			Message:  err.Error(),
			Location: &scope.Location,
		}
	}

	component := entity.Component

	inportAddrs := g.insertAndReturnInports(nodeCtx)   // for inports we only use parent context because all inports are used
	outportAddrs := g.insertAndReturnOutports(nodeCtx) //  for outports we use both parent context and component's interface

	runtimeFuncRef, err := g.getFuncRef(component, nodeCtx.node.TypeArgs)
	if err != nil {
		return &compiler.Error{
			Message:  err.Error(),
			Location: &location,
			Meta:     &component.Meta,
		}
	}

	if runtimeFuncRef != "" {
		cfgMsg, err := getConfigMsg(nodeCtx.node, scope)
		if err != nil {
			return &compiler.Error{
				Message:  err.Error(),
				Location: &scope.Location,
			}
		}
		result.Funcs = append(result.Funcs, ir.FuncCall{
			Ref: runtimeFuncRef,
			IO: ir.FuncIO{
				In:  inportAddrs,
				Out: outportAddrs,
			},
			Msg: cfgMsg,
		})
		return nil
	}

	newScope := scope.Relocate(location) // only use new location if that's not builtin

	// We use network as a source of true about how subnodes ports instead subnodes interface definitions.
	// We cannot rely on them because there's no information about how many array slots are used (in case of array ports).
	// On the other hand, we believe network has everything we need because program' correctness is verified by analyzer.
	subnodesPortsUsage, err := g.processNetwork(
		component.Net,
		nodeCtx,
		result,
	)
	if err != nil {
		return &compiler.Error{
			Message:  err.Error(),
			Location: &newScope.Location,
		}
	}

	for nodeName, node := range component.Nodes {
		nodePortsUsage, ok := subnodesPortsUsage[nodeName]
		if !ok {
			return &compiler.Error{
				Message:  fmt.Sprintf("node usage not found: %v", nodeName),
				Location: &location,
				Meta:     &node.Meta,
			}
		}

		subNodeCtx := nodeContext{
			path:       append(nodeCtx.path, nodeName),
			portsUsage: nodePortsUsage,
			node:       node,
		}

		var scopeToUse src.Scope
		if injectedNode, isDINode := nodeCtx.node.Deps[nodeName]; isDINode {
			subNodeCtx.node = injectedNode
			scopeToUse = scope
		} else {
			scopeToUse = newScope
		}

		if err := g.processNode(subNodeCtx, scopeToUse, result); err != nil {
			return err
		}
	}

	return nil
}

func (Generator) insertAndReturnInports(nodeCtx nodeContext) []ir.PortAddr {
	inports := make([]ir.PortAddr, 0, len(nodeCtx.portsUsage.in))

	// in valid program all inports are used, so it's safe to depend on nodeCtx and not use component's IO
	// actually we can't use IO because we need to know how many slots are used
	for relAddr := range nodeCtx.portsUsage.in {
		absAddr := ir.PortAddr{
			Path: joinNodePath(nodeCtx.path, "in"),
			Port: relAddr.Port,
		}
		if relAddr.Idx != nil {
			absAddr.IsArray = true
			absAddr.Idx = *relAddr.Idx
		}
		inports = append(inports, absAddr)
	}

	sortPortAddrs(inports)

	return inports
}

func (Generator) insertAndReturnOutports(nodeCtx nodeContext) []ir.PortAddr {
	outports := make([]ir.PortAddr, 0, len(nodeCtx.portsUsage.out))

	// In a valid (desugared) program all outports are used so it's safe to depend on nodeCtx and not use component's IO.
	// Actually we can't use IO because we need to know how many slots are used.
	for addr := range nodeCtx.portsUsage.out {
		irAddr := ir.PortAddr{
			Path: joinNodePath(nodeCtx.path, "out"),
			Port: addr.Port,
		}
		if addr.Idx != nil {
			irAddr.IsArray = true
			irAddr.Idx = *addr.Idx
		}
		outports = append(outports, irAddr)
	}

	sortPortAddrs(outports)

	return outports
}

func New() Generator {
	return Generator{}
}
