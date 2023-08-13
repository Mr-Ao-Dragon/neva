package llrgen

import (
	"context"
	"errors"
	"fmt"

	"github.com/nevalang/neva/internal/shared"
)

type Generator struct{}

func New() Generator {
	return Generator{}
}

var (
	ErrNoPkgs                 = errors.New("no packages")
	ErrPkgNotFound            = errors.New("pkg not found")
	ErrEntityNotFound         = errors.New("entity not found")
	ErrSubNode                = errors.New("sub node")
	ErrNodeSlotsCountNotFound = errors.New("node slots count not found")
)

func (g Generator) Generate(ctx context.Context, pkgs map[string]shared.File) (shared.LLProgram, error) {
	if len(pkgs) == 0 {
		return shared.LLProgram{}, ErrNoPkgs
	}

	rootNodeCtx := nodeContext{
		path:      "main",
		entityRef: shared.EntityRef{Pkg: "main", Name: "Main"},
		ioUsage: nodeIOUsage{
			in: map[repPortAddr]struct{}{
				{Port: "start"}: {},
			},
			out: map[string]uint8{
				"exit": 1,
			},
		},
	}

	result := shared.LLProgram{
		Ports: map[shared.LLPortAddr]uint8{},
		Net:   []shared.LLConnection{},
		Funcs: []shared.LLFunc{},
	}

	if err := g.processNode(ctx, rootNodeCtx, pkgs, &result); err != nil {
		return shared.LLProgram{}, fmt.Errorf("process root node: %w", err)
	}

	return result, nil
}

type (
	nodeContext struct {
		path      string           // including current
		entityRef shared.EntityRef // refers to component // todo what about interfaces?
		ioUsage   nodeIOUsage
	}
	nodeIOUsage struct {
		in  map[repPortAddr]struct{} // why not same as out?
		out map[string]uint8         // name -> slots used by parent
	}
	repPortAddr struct {
		Port string
		Idx  uint8
	}
)

func (g Generator) processNode(
	ctx context.Context,
	nodeCtx nodeContext,
	pkgs map[string]shared.File,
	result *shared.LLProgram,
) error {
	entity, err := g.lookupEntity(pkgs, nodeCtx.entityRef)
	if err != nil {
		return fmt.Errorf("lookup entity: %w", err)
	}

	component := entity.Component
	inportAddrs := g.insertAndReturnInports(nodeCtx, result)
	outPortAddrs := g.insertAndReturnOutports(component.Interface.IO.Out, nodeCtx, result)

	if len(component.Net) == 0 {
		result.Funcs = append(
			result.Funcs,
			shared.LLFunc{
				Ref: shared.LLFuncRef{
					Pkg:  nodeCtx.entityRef.Pkg,
					Name: nodeCtx.entityRef.Name,
				},
				IO: shared.LLFuncIO{
					In:  inportAddrs,
					Out: outPortAddrs,
				},
			},
		)
		return nil
	}

	nodesIOUsage, err := g.insertConnectionsAndReturnIOUsage(pkgs, component.Net, nodeCtx, result)
	if err != nil {
		return fmt.Errorf("handle network: %w", err)
	}

	for name := range component.Nodes {
		nodeSlots, ok := nodesIOUsage[name]
		if !ok {
			return fmt.Errorf("%w: %v", ErrNodeSlotsCountNotFound, name)
		}

		subNodeCtx := nodeContext{
			path: nodeCtx.path + "/" + name,
			ioUsage: nodeIOUsage{
				in:  nodeSlots.in,
				out: nodeSlots.out,
			},
		}

		if err := g.processNode(ctx, subNodeCtx, pkgs, result); err != nil {
			return fmt.Errorf("%w: %v", errors.Join(ErrSubNode, err), name)
		}
	}

	return nil
}

type handleNetworkResult struct {
	slotsUsage map[string]nodeIOUsage // node -> ports
}

func (g Generator) insertConnectionsAndReturnIOUsage(
	pkgs map[string]shared.File,
	conns []shared.Connection,
	nodeCtx nodeContext,
	result *shared.LLProgram,
) (map[string]nodeIOUsage, error) {
	nodesIOUsage := map[string]nodeIOUsage{}
	inPortsSlotsSet := map[shared.PortAddr]bool{}

	for _, conn := range conns {
		senderPortAddr := conn.SenderSide.PortAddr

		if _, ok := nodesIOUsage[senderPortAddr.Node]; !ok { // init
			nodesIOUsage[senderPortAddr.Node] = nodeIOUsage{
				in:  map[repPortAddr]struct{}{},
				out: map[string]uint8{},
			}
		}

		// we assume every sender is unique thus we don't increment same addr twice
		nodesIOUsage[senderPortAddr.Node].out[senderPortAddr.Port]++ // fixme why we assume that?

		senderSide := shared.LLPortAddr{
			Path: nodeCtx.path + "/" + conn.SenderSide.PortAddr.Node,
			Port: conn.SenderSide.PortAddr.Port,
			Idx:  conn.SenderSide.PortAddr.Idx,
		}

		receiverSides := make([]shared.LLReceiverConnectionSide, 0, len(conn.ReceiverSides))
		for _, receiverSide := range conn.ReceiverSides {
			irSide := g.mapReceiverConnectionSide(nodeCtx.path, receiverSide)
			receiverSides = append(receiverSides, irSide)

			// we can have same receiver for different senders and we don't want to count it twice
			if !inPortsSlotsSet[receiverSide.PortAddr] {
				nodesIOUsage[senderPortAddr.Node].in[repPortAddr{
					Port: senderPortAddr.Port,
					Idx:  senderPortAddr.Idx,
				}] = struct{}{}
			}
		}

		result.Net = append(result.Net, shared.LLConnection{
			SenderSide:    senderSide,
			ReceiverSides: receiverSides,
		})
	}

	return nodesIOUsage, nil
}

func (Generator) insertAndReturnInports(
	nodeCtx nodeContext,
	result *shared.LLProgram,
) []shared.LLPortAddr {
	inports := make([]shared.LLPortAddr, 0, len(nodeCtx.ioUsage.in))

	// in valid program all inports are used, so it's safe to depend on nodeCtx and not use component's IO
	// actually we can't use IO because we need to know how many slots are used
	for addr := range nodeCtx.ioUsage.in {
		addr := shared.LLPortAddr{
			Path: nodeCtx.path + "/in",
			Port: addr.Port,
			Idx:  addr.Idx,
		}
		result.Ports[addr] = 0
		inports = append(inports, addr)
	}

	return inports
}

func (Generator) insertAndReturnOutports(
	outports map[string]shared.Port,
	nodeCtx nodeContext,
	result *shared.LLProgram,
) []shared.LLPortAddr {
	runtimeFuncOutportAddrs := make([]shared.LLPortAddr, 0, len(nodeCtx.ioUsage.out))

	for name := range outports {
		slotsCount, ok := nodeCtx.ioUsage.out[name]
		if !ok { // outport not used by parent
			slotsCount = 1 // but component need at least 1 slot to write
		}

		for i := 0; i < int(slotsCount); i++ {
			addr := shared.LLPortAddr{
				Path: nodeCtx.path + "/out",
				Port: name,
				Idx:  uint8(i),
			}
			result.Ports[addr] = 0
			runtimeFuncOutportAddrs = append(runtimeFuncOutportAddrs, addr)
		}
	}

	return runtimeFuncOutportAddrs
}

func (Generator) lookupEntity(pkgs map[string]shared.File, ref shared.EntityRef) (shared.Entity, error) {
	pkg, ok := pkgs[ref.Pkg]
	if !ok {
		return shared.Entity{}, fmt.Errorf("%w: %v", ErrPkgNotFound, ref.Pkg)
	}

	entity, ok := pkg.Entities[ref.Name]
	if !ok {
		return shared.Entity{}, fmt.Errorf("%w: %v", ErrEntityNotFound, ref.Name)
	}

	return entity, nil
}

type handleSenderSideResult struct {
	irConnSide shared.LLPortAddr
}

// mapReceiverConnectionSide maps compiler connection side to ir connection side 1-1 just making the port addr's path absolute
func (g Generator) mapReceiverConnectionSide(nodeCtxPath string, side shared.ReceiverConnectionSide) shared.LLReceiverConnectionSide {
	return shared.LLReceiverConnectionSide{
		PortAddr: shared.LLPortAddr{
			Path: nodeCtxPath + "/" + side.PortAddr.Node,
			Port: side.PortAddr.Port,
			Idx:  side.PortAddr.Idx,
		},
		Selectors: side.Selectors,
	}
}
