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

var ErrNoPkgs = errors.New("no packages")

func (g Generator) Generate(ctx context.Context, prog shared.HighLvlProgram) (shared.LowLvlProgram, error) {
	if len(prog) == 0 {
		return shared.LowLvlProgram{}, ErrNoPkgs
	}

	// usually we "look" at the program "inside" the root node but here we look at the root node from the outside
	ref := shared.EntityRef{Pkg: "main", Name: "main"}
	rootNodeCtx := nodeContext{
		path:      "main",
		entityRef: ref,
		io: ioContext{
			in: map[shared.RelPortAddr]inportSlotContext{
				{Name: "start"}: {},
			},
			out: map[string]uint8{
				"exit": 1, // runtime is the only one who will read (once) message (code) from this outport
			},
		},
	}

	result := shared.LowLvlProgram{
		Ports: map[shared.LLPortAddr]uint8{},
		Net:   []shared.LLConnection{},
		Funcs: []shared.LLFunc{},
	}
	if err := g.processNode(ctx, rootNodeCtx, prog, &result); err != nil {
		return shared.LowLvlProgram{}, fmt.Errorf("process root node: %w", err)
	}

	return result, nil
}

type (
	// nodeContext describes how node is used by its parent
	nodeContext struct {
		path      string           // path to current node including current node itself
		entityRef shared.EntityRef // refers to component (or interface?)
		io        ioContext
		di        map[string]shared.Node // instances must refer to components
	}
	// ioContext describes how many port slots must be created and how many incoming connections inports have
	ioContext struct {
		in  map[shared.RelPortAddr]inportSlotContext
		out map[string]uint8 // name -> slots used by parent count
	}
	inportSlotContext struct {
		// incomingConnectionsCount uint8 // how many senders (outports) writes to this receiver (inport slot)
		staticMsgRef shared.EntityRef // do we need it?
	}
)

var (
	ErrPkgNotFound            = errors.New("pkg not found")
	ErrEntityNotFound         = errors.New("entity not found")
	ErrSubNode                = errors.New("sub node")
	ErrNodeSlotsCountNotFound = errors.New("node slots count not found")
)

// processNode fills given result with generated data
func (g Generator) processNode(
	ctx context.Context,
	nodeCtx nodeContext,
	pkgs map[string]shared.Pkg,
	result *shared.HighLvlProgram,
) error {
	entity, err := g.lookupEntity(pkgs, nodeCtx.entityRef) // do we really need this func?
	if err != nil {
		return fmt.Errorf("lookup entity: %w", err)
	}

	component := entity.Component

	inportAddrs := g.handleInPortsCreation(nodeCtx, result)
	outPortAddrs := g.handleOutPortsCreation(component.IO.Out, nodeCtx, result)

	if isNative := len(component.Net) == 0; isNative {
		funcRoutine := g.getFuncRoutine(nodeCtx, inportAddrs, outPortAddrs)
		result.Funcs = append(result.Funcs, funcRoutine)
		return nil
	}

	handleNetRes, err := g.handleNetwork(pkgs, component.Net, nodeCtx, result)
	if err != nil {
		return fmt.Errorf("handle network: %w", err)
	}

	// handle giver creation
	giverSpecMsg := g.buildGiverSpecMsg(handleNetRes.giverSpecEls, result)
	giverSpecMsgPath := nodeCtx.path + "/" + "giver"
	result.Consts[giverSpecMsgPath] = giverSpecMsg
	giverFunc := shared.LLFunc{
		Ref: shared.LLFuncRef{
			Pkg:  "flow",
			Name: "Giver",
		},
		IO: shared.LLFuncIO{
			Out: make([]shared.LLPortAddr, 0, len(handleNetRes.giverSpecEls)),
		},
		MsgRef: giverSpecMsgPath,
	}
	result.Funcs = append(result.Funcs, giverFunc)
	for _, specEl := range handleNetRes.giverSpecEls {
		giverFunc.IO.Out = append(giverFunc.IO.Out, specEl.outPortAddr)
	}

	for name, node := range component.Nodes {
		nodeSlots, ok := handleNetRes.slotsUsage[name]
		if !ok {
			return fmt.Errorf("%w: %v", ErrNodeSlotsCountNotFound, name)
		}

		subNodeCtx := g.getSubNodeCtx(nodeCtx, name, node, nodeSlots)

		if err := g.processNode(ctx, subNodeCtx, pkgs, result); err != nil {
			return fmt.Errorf("%w: %v", errors.Join(ErrSubNode, err), name)
		}
	}

	return nil
}

// buildGiverSpecMsg translates every spec element to ir msg and puts it into result messages.
// Then it builds and returns ir vec msg where every element points to corresponding spec element.
func (g Generator) buildGiverSpecMsg(specEls []giverSpecEl, result *shared.HighLvlProgram) shared.LLMsg {
	msg := shared.LLMsg{
		Type: shared.LLVecMsg,
		Vec:  make([]string, 0, len(specEls)),
	}

	// put string with outport name to static memory
	// put int with outport slot number to static memory
	// create spec message and put to static memory
	// remember reference to that spec message
	// collect all such references and return
	for i, el := range specEls {
		prefix := el.outPortAddr.Path + "/" + fmt.Sprint(i)

		nameMsg := shared.LLMsg{
			Type: shared.LLStrMsg,
			Str:  el.outPortAddr.Name,
		}
		namePath := prefix + "/" + "name"
		result.Consts[namePath] = nameMsg

		idxMsg := shared.LLMsg{
			Type: shared.LLIntMsg,
			Int:  int(el.outPortAddr.Idx),
		}
		idxPath := prefix + "/" + "idx"
		result.Consts[idxPath] = idxMsg

		addrMsg := shared.LLMsg{
			Type: shared.LLMapMsg,
			Map: map[string]string{
				"name": namePath,
				"idx":  idxPath,
			},
		}
		addrPath := prefix + "/" + "addr"
		result.Consts[addrPath] = addrMsg

		specElMsg := shared.LLMsg{
			Type: shared.LLMapMsg,
			Map: map[string]string{
				"msg":  el.msgToSendName,
				"addr": addrPath,
			},
		}
		result.Consts[prefix] = addrMsg

		msg.Vec = append(specElMsg.Vec, prefix)
	}

	return msg
}

func (Generator) getSubNodeCtx(
	parentNodeCtx nodeContext,
	name string,
	node shared.Node,
	slotsCount portSlotsCount,
) nodeContext {
	subNodeCtx := nodeContext{
		path: parentNodeCtx.path + "/" + name,
		io: ioContext{
			in:  slotsCount.in,
			out: slotsCount.out,
		},
	}

	for addr, msgRef := range node.StaticInports {
		subNodeCtx.io.in[addr] = inportSlotContext{
			staticMsgRef: msgRef,
		}
	}

	instance, ok := parentNodeCtx.di[name]
	if ok {
		subNodeCtx.entityRef = instance.Ref
		subNodeCtx.di = instance.ComponentDI
	} else {
		subNodeCtx.entityRef = node.Ref
		subNodeCtx.di = node.ComponentDI
	}

	return subNodeCtx
}

// getFuncRoutine simply builds and returns func routine structure
func (Generator) getFuncRoutine(
	nodeCtx nodeContext,
	runtimeFuncInportAddrs []shared.LLPortAddr,
	runtimeFuncOutPortAddrs []shared.LLPortAddr,
) shared.LLFunc {
	return shared.LLFunc{
		Ref: shared.LLFuncRef{
			Pkg:  nodeCtx.entityRef.Pkg,
			Name: nodeCtx.entityRef.Name,
		},
		IO: shared.LLFuncIO{
			In:  runtimeFuncInportAddrs,
			Out: runtimeFuncOutPortAddrs,
		},
	}
}

type portSlotsCount struct {
	in  map[shared.RelPortAddr]inportSlotContext // inportSlotContext will be empty
	out map[string]uint8
}

type handleNetworkResult struct {
	slotsUsage   map[string]portSlotsCount // node -> ports
	giverSpecEls []giverSpecEl
}

// handleNetwork inserts ir connections into the given result
// and returns information about how many slots of each port is actually used in network.
func (g Generator) handleNetwork(
	pkgs map[string]shared.Pkg,
	net []shared.Connection, // pass only net
	nodeCtx nodeContext,
	result *shared.HighLvlProgram,
) (handleNetworkResult, error) {
	slotsUsage := map[string]portSlotsCount{}
	inPortsSlotsSet := map[shared.ConnPortAddr]bool{}
	giverSpecEls := make([]giverSpecEl, 0, len(net))

	for _, conn := range net {
		senderPortAddr := conn.SenderSide.PortAddr

		if _, ok := slotsUsage[senderPortAddr.Node]; !ok {
			slotsUsage[senderPortAddr.Node] = portSlotsCount{
				in:  map[shared.RelPortAddr]inportSlotContext{},
				out: map[string]uint8{},
			}
		}

		// we assume every sender is unique so we won't increment same port addr twice
		slotsUsage[senderPortAddr.Node].out[senderPortAddr.Name]++

		handleSenderResult, err := g.handleSenderSide(pkgs, nodeCtx.path, conn.SenderSide, result)
		if err != nil {
			return handleNetworkResult{}, fmt.Errorf("handle sender side: %w", err)
		}

		if handleSenderResult.giverParams != nil {
			giverSpecEls = append(giverSpecEls, *handleSenderResult.giverParams)
		}

		receiverSides := make([]shared.LLConnectionSide, 0, len(conn.ReceiverSides))
		for _, receiverSide := range conn.ReceiverSides {
			irSide := g.mapPortSide(nodeCtx.path, receiverSide, "in")
			receiverSides = append(receiverSides, irSide)

			// we can have same receiver for different senders and we don't want to count it twice
			if !inPortsSlotsSet[receiverSide.PortAddr] {
				slotsUsage[senderPortAddr.Node].in[receiverSide.PortAddr.RelPortAddr] = inportSlotContext{
					// staticMsgRef: shared.EntityRef{},
				}
			}
		}

		result.Net = append(result.Net, shared.LLConnection{
			SenderSide:    handleSenderResult.irConnSide,
			ReceiverSides: receiverSides,
		})
	}

	return handleNetworkResult{
		slotsUsage:   slotsUsage,
		giverSpecEls: giverSpecEls,
	}, nil
}

// handleInPortsCreation creates and inserts ir inports into the given result.
// It also returns slice of created ir port addrs.
func (Generator) handleInPortsCreation(nodeCtx nodeContext, result *shared.HighLvlProgram) []shared.LLPortAddr {
	runtimeFuncInportAddrs := make([]shared.LLPortAddr, 0, len(nodeCtx.io.in)) // only needed for nodes with runtime func

	// in valid program all inports are used, so it's safe to depend on nodeCtx and not use component's IO here at all
	// btw we can't use component's IO instead of nodeCtx because we need to know how many slots are used by parent
	for addr := range nodeCtx.io.in {
		addr := shared.LLPortAddr{
			Path: nodeCtx.path + "/" + "in",
			Name: addr.Name,
			Idx:  addr.Idx,
		}
		result.Ports[addr] = 0
		runtimeFuncInportAddrs = append(runtimeFuncInportAddrs, addr)
	}

	return runtimeFuncInportAddrs
}

// handleOutPortsCreation creates ir outports and inserts them into the given result.
// It also creates and inserts void routines and connections for outports unused by parent.
// It returns slice of ir port addrs that could be used to create a func routine.
func (Generator) handleOutPortsCreation(
	outports shared.Ports,
	nodeCtx nodeContext,
	result *shared.HighLvlProgram,
) []shared.LLPortAddr {
	runtimeFuncOutportAddrs := make([]shared.LLPortAddr, 0, len(nodeCtx.io.out)) // same as runtimeFuncInportAddrs

	for name := range outports {
		slotsCount, ok := nodeCtx.io.out[name]
		if !ok { // outport not used by parent
			// TODO insert ir void routine + connection
			slotsCount = 1 // but component need at least 1 slot to write
		}

		for i := 0; i < int(slotsCount); i++ {
			addr := shared.LLPortAddr{
				Path: nodeCtx.path + "/" + "out",
				Name: name,
				Idx:  uint8(i),
			}
			result.Ports[addr] = 0
			runtimeFuncOutportAddrs = append(runtimeFuncOutportAddrs, addr)
		}
	}

	return runtimeFuncOutportAddrs
}

func (Generator) lookupEntity(pkgs map[string]shared.Pkg, ref shared.EntityRef) (shared.Entity, error) {
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
	irConnSide  shared.LLConnectionSide
	giverParams *giverSpecEl // nil means sender is normal outport and no giver is needed
}

type giverSpecEl struct {
	msgToSendName string // this message is already inserted into the result ir
	outPortAddr   shared.LLPortAddr
}

// handleSenderSide checks if sender side refers to a message instead of port.
// If not, then it acts just like a mapPortSide without any side-effects.
// Otherwise it first builds the message, then inserts it into result, then returns params for giver creation.
func (g Generator) handleSenderSide(
	pkgs map[string]shared.Pkg,
	nodeCtxPath string,
	side shared.SenderConnectionSide,
	result *shared.HighLvlProgram,
) (handleSenderSideResult, error) {
	if side.MsgRef == nil {
		irConnSide := g.mapPortSide(nodeCtxPath, side.PortConnectionSide, "out")
		return handleSenderSideResult{irConnSide: irConnSide}, nil
	}

	irMsg, err := g.buildIRMsg(pkgs, *side.MsgRef)
	if err != nil {
		return handleSenderSideResult{}, err
	}

	msgName := nodeCtxPath + "/" + side.MsgRef.Pkg + "." + side.MsgRef.Name
	result.Consts[msgName] = irMsg

	giverOutport := shared.LLPortAddr{
		Path: nodeCtxPath,
		Name: side.MsgRef.Pkg + "." + side.MsgRef.Name,
	}
	result.Ports[giverOutport] = 0

	selectors := make([]shared.LLSelector, 0, len(side.Selectors))
	for _, selector := range side.Selectors {
		selectors = append(selectors, shared.LLSelector(selector))
	}

	return handleSenderSideResult{
		irConnSide: shared.LLConnectionSide{
			PortAddr:  giverOutport,
			Selectors: selectors,
		},
		giverParams: &giverSpecEl{
			msgToSendName: msgName,
			outPortAddr:   giverOutport,
		},
	}, nil
}

// buildIRMsg recursively builds the message by following references and analyzing the type expressions.
// it assumes all type expressions are resolved and it thus possible to 1-1 map them to IR types.
func (g Generator) buildIRMsg(pkgs map[string]shared.Pkg, ref shared.EntityRef) (shared.LLMsg, error) {
	entity, err := g.lookupEntity(pkgs, ref)
	if err != nil {
		return shared.LLMsg{}, fmt.Errorf("loopup entity: %w", err)
	}

	msg := entity.Msg

	if msg.Ref != nil {
		result, err := g.buildIRMsg(pkgs, *msg.Ref)
		if err != nil {
			return shared.LLMsg{}, fmt.Errorf("get ir msg: %w", err)
		}

		return result, nil
	}

	// TODO

	// typ := msg.Value.TypeExpr

	// instRef := typ.Inst.Ref

	// switch {
	// case msg.Value.TypeExpr:

	// }

	// msg.Value

	return shared.LLMsg{}, nil
}

// mapPortSide maps compiler connection side to ir connection side 1-1 just making the port addr's path absolute
func (Generator) mapPortSide(nodeCtxPath string, side shared.PortConnectionSide, pathPostfix string) shared.LLConnectionSide {
	addr := shared.LLPortAddr{
		Path: nodeCtxPath + "/" + side.PortAddr.Node,
		Name: side.PortAddr.Name,
		Idx:  side.PortAddr.Idx,
	}

	if side.PortAddr.Node != "in" && side.PortAddr.Node != "out" {
		addr.Path += "/" + pathPostfix
	}

	selectors := make([]shared.LLSelector, 0, len(side.Selectors))
	for _, selector := range side.Selectors {
		selectors = append(selectors, shared.LLSelector(selector))
	}

	return shared.LLConnectionSide{
		PortAddr:  addr,
		Selectors: selectors,
	}
}
