package desugarer

import (
	"errors"
	"fmt"

	"github.com/nevalang/neva/internal/compiler"
	src "github.com/nevalang/neva/internal/compiler/sourcecode"
	"github.com/nevalang/neva/internal/compiler/sourcecode/core"
	"github.com/nevalang/neva/internal/compiler/sourcecode/typesystem"
	ts "github.com/nevalang/neva/internal/compiler/sourcecode/typesystem"
)

type handleNetworkResult struct {
	desugaredConnections []src.Connection
	constsToInsert       map[string]src.Const
	nodesToInsert        map[string]src.Node
	nodesPortsUsed       nodeOutportsUsed
}

func (d Desugarer) desugarNetwork(
	iface src.Interface,
	net []src.Connection,
	nodes map[string]src.Node,
	scope src.Scope,
) (handleNetworkResult, *compiler.Error) {
	nodesToInsert := map[string]src.Node{}
	constsToInsert := map[string]src.Const{}
	nodesPortsUsed := newNodePortsMap()

	desugaredConnections, err := d.desugarConnections(
		iface,
		net,
		nodesPortsUsed,
		scope,
		nodes,
		nodesToInsert,
		constsToInsert,
	)
	if err != nil {
		return handleNetworkResult{}, err
	}

	return handleNetworkResult{
		desugaredConnections: desugaredConnections,
		nodesPortsUsed:       nodesPortsUsed,
		constsToInsert:       constsToInsert,
		nodesToInsert:        nodesToInsert,
	}, nil
}

func (d Desugarer) desugarConnections(
	iface src.Interface,
	net []src.Connection,
	nodePortsUsed nodeOutportsUsed,
	scope src.Scope,
	nodes map[string]src.Node,
	nodesToInsert map[string]src.Node,
	constsToInsert map[string]src.Const,
) ([]src.Connection, *compiler.Error) {
	desugaredConnections := make([]src.Connection, 0, len(net))

	for _, conn := range net {
		result, err := d.desugarConnection(
			iface,
			conn,
			nodePortsUsed,
			scope,
			nodes,
			nodesToInsert,
			constsToInsert,
		)
		if err != nil {
			return nil, err
		}

		if result.replace != nil {
			desugaredConnections = append(desugaredConnections, *result.replace)
		}

		desugaredConnections = append(desugaredConnections, result.insert...)
	}

	return desugaredConnections, nil
}

type desugarConnectionResult struct {
	replace *src.Connection
	insert  []src.Connection
}

func (d Desugarer) desugarConnection(
	iface src.Interface,
	conn src.Connection,
	nodePortsUsed nodeOutportsUsed,
	scope src.Scope,
	nodes map[string]src.Node,
	nodesToInsert map[string]src.Node,
	constsToInsert map[string]src.Const,
) (desugarConnectionResult, *compiler.Error) {
	if conn.ArrayBypass != nil {
		nodePortsUsed.set(
			conn.ArrayBypass.SenderOutport.Node,
			conn.ArrayBypass.SenderOutport.Port,
		)
		nodePortsUsed.set(
			conn.ArrayBypass.ReceiverInport.Node,
			conn.ArrayBypass.ReceiverInport.Port,
		)
		return desugarConnectionResult{replace: &conn}, nil
	}

	return d.desugarNormalConnection(
		iface,
		*conn.Normal,
		nodePortsUsed,
		scope,
		nodes,
		nodesToInsert,
		constsToInsert,
	)
}

func (d Desugarer) desugarNormalConnection(
	iface src.Interface,
	normConn src.NormalConnection,
	nodePortsUsed nodeOutportsUsed,
	scope src.Scope,
	nodes map[string]src.Node,
	nodesToInsert map[string]src.Node,
	constsToInsert map[string]src.Const,
) (desugarConnectionResult, *compiler.Error) {
	if len(normConn.SenderSide) > 1 {
		result, err := d.desugarFanIn(
			iface,
			normConn,
			nodesToInsert,
			constsToInsert,
			nodePortsUsed,
			scope,
			nodes,
		)
		if err != nil {
			return desugarConnectionResult{}, &compiler.Error{
				Message:  err.Error(),
				Location: &scope.Location,
				Meta:     &normConn.Meta,
			}
		}
		// original connection is replaced by multiple new ones
		return desugarConnectionResult{
			insert: result,
		}, nil
	}

	desugarSenderResult, err := d.desugarSingleSender(
		iface,
		normConn,
		scope,
		nodes,
		nodePortsUsed,
		nodesToInsert,
		constsToInsert,
	)
	if err != nil {
		return desugarConnectionResult{}, compiler.Error{
			Location: &scope.Location,
			Meta:     &normConn.SenderSide[0].Meta,
		}.Wrap(err)
	}

	normConn = *desugarSenderResult.replace.Normal
	insert := desugarSenderResult.insert

	if len(normConn.ReceiverSide) > 1 {
		result, err := d.desugarFanOut(
			iface,
			normConn,
			nodesToInsert,
			constsToInsert,
			nodePortsUsed,
			scope,
			nodes,
		)
		if err != nil {
			return desugarConnectionResult{}, err
		}

		return desugarConnectionResult{
			replace: &result.replace,
			insert:  append(insert, result.insert...),
		}, nil
	}

	desugarReceiverResult, err := d.desugarSingleReceiver(
		iface,
		normConn,
		scope,
		nodes,
		nodesToInsert,
		constsToInsert,
		nodePortsUsed,
	)
	if err != nil {
		return desugarConnectionResult{}, err
	}

	return desugarConnectionResult{
		replace: &desugarReceiverResult.replace,
		insert:  append(insert, desugarReceiverResult.insert...),
	}, nil
}

type desugarReceiverResult struct {
	replace src.Connection
	insert  []src.Connection
}

var switchCounter uint64

func (d Desugarer) desugarSingleReceiver(
	iface src.Interface,
	normConn src.NormalConnection,
	scope src.Scope,
	nodes map[string]src.Node,
	nodesToInsert map[string]src.Node,
	constsToInsert map[string]src.Const,
	nodePortsUsed nodeOutportsUsed,
) (desugarReceiverResult, *compiler.Error) {
	receiver := normConn.ReceiverSide[0]

	if receiver.PortAddr != nil {
		if receiver.PortAddr.Port != "" {
			return desugarReceiverResult{
				replace: src.Connection{
					Normal: &src.NormalConnection{
						SenderSide:   normConn.SenderSide,
						ReceiverSide: []src.ConnectionReceiver{receiver},
					},
				},
				insert: []src.Connection{},
			}, nil
		}

		firstInportName, err := getFirstInportName(scope, nodes, *receiver.PortAddr)
		if err != nil {
			return desugarReceiverResult{}, &compiler.Error{
				Message:  err.Error(),
				Location: &scope.Location,
				Meta:     &receiver.Meta,
			}
		}

		return desugarReceiverResult{
			replace: src.Connection{
				Normal: &src.NormalConnection{
					SenderSide: normConn.SenderSide,
					ReceiverSide: []src.ConnectionReceiver{
						{
							PortAddr: &src.PortAddr{
								Port: firstInportName,
								Node: receiver.PortAddr.Node,
								Idx:  receiver.PortAddr.Idx,
								Meta: receiver.PortAddr.Meta,
							},
						},
					},
				},
			},
		}, nil
	}

	if receiver.DeferredConnection != nil {
		result, err := d.desugarDeferredConnection(
			iface,
			normConn,
			scope,
			constsToInsert,
			nodesToInsert,
			nodePortsUsed,
			nodes,
		)
		if err != nil {
			return desugarReceiverResult{}, err
		}

		return desugarReceiverResult(result), nil
	}

	if receiver.Switch != nil {
		switchCounter++
		switchNodeName := fmt.Sprintf("__switch__%d", switchCounter)

		nodesToInsert[switchNodeName] = src.Node{
			EntityRef: core.EntityRef{
				Pkg:  "builtin",
				Name: "Switch",
			},
		}

		// Connect original sender to switch:data
		replace := src.Connection{
			Normal: &src.NormalConnection{
				SenderSide: normConn.SenderSide,
				ReceiverSide: []src.ConnectionReceiver{
					{
						PortAddr: &src.PortAddr{
							Node: switchNodeName,
							Port: "data",
						},
					},
				},
			},
		}

		insert := []src.Connection{}

		// For each case in the switch
		for i, caseConn := range receiver.Switch.Cases {
			// Connect case-sender to switch:case[i]
			insert = append(insert, src.Connection{
				Normal: &src.NormalConnection{
					SenderSide: caseConn.SenderSide,
					ReceiverSide: []src.ConnectionReceiver{
						{
							PortAddr: &src.PortAddr{
								Node: switchNodeName,
								Port: "case",
								Idx:  compiler.Pointer(uint8(i)),
							},
						},
					},
				},
			})

			// Connect switch:case[i] to case receiver
			insert = append(insert, src.Connection{
				Normal: &src.NormalConnection{
					SenderSide: []src.ConnectionSender{
						{
							PortAddr: &src.PortAddr{
								Node: switchNodeName,
								Port: "case",
								Idx:  compiler.Pointer(uint8(i)),
							},
						},
					},
					ReceiverSide: caseConn.ReceiverSide,
				},
			})
		}

		// Connect switch:default to its receiver
		insert = append(insert, src.Connection{
			Normal: &src.NormalConnection{
				SenderSide: []src.ConnectionSender{
					{
						PortAddr: &src.PortAddr{
							Node: switchNodeName,
							Port: "else",
						},
					},
				},
				ReceiverSide: receiver.Switch.Default,
			},
		})

		desugaredInsert, err := d.desugarConnections(
			iface,
			insert,
			nodePortsUsed,
			scope,
			nodes,
			nodesToInsert,
			constsToInsert,
		)
		if err != nil {
			return desugarReceiverResult{}, err
		}

		return desugarReceiverResult{
			replace: replace,
			insert:  desugaredInsert,
		}, nil
	}

	desugarChainResult, err := d.desugarChainedConnection(
		iface,
		receiver,
		scope,
		nodes,
		nodePortsUsed,
		nodesToInsert,
		constsToInsert,
		normConn,
	)
	if err != nil {
		return desugarReceiverResult{}, err
	}

	return desugarReceiverResult{
		replace: *desugarChainResult.replace,
		insert:  desugarChainResult.insert,
	}, nil
}

func (d Desugarer) desugarChainedConnection(
	iface src.Interface,
	receiver src.ConnectionReceiver,
	scope src.Scope,
	nodes map[string]src.Node,
	nodePortsUsed nodeOutportsUsed,
	nodesToInsert map[string]src.Node,
	constsToInsert map[string]src.Const,
	normConn src.NormalConnection,
) (desugarConnectionResult, *compiler.Error) {
	chainedConn := *receiver.ChainedConnection
	chainHead := chainedConn.Normal.SenderSide[0] // chain head is always single sender

	// it's only possible to find receiver port before desugaring of chained connection
	var chainHeadPort string
	switch {
	case chainHead.Range != nil:
		chainHeadPort = "sig"
	case len(chainHead.StructSelector) != 0:
		chainHeadPort = "data"
	case chainHead.PortAddr != nil:
		chainHeadPort = chainHead.PortAddr.Port
		if chainHeadPort == "" {
			var err error
			chainHeadPort, err = getFirstInportName(scope, nodes, *chainHead.PortAddr)
			if err != nil {
				return desugarConnectionResult{}, &compiler.Error{Message: err.Error()}
			}
		}
	default:
		panic("unexpected chain head type")
	}

	desugarChainResult, err := d.desugarConnection(
		iface,
		chainedConn,
		nodePortsUsed,
		scope,
		nodes,
		nodesToInsert,
		constsToInsert,
	)
	if err != nil {
		return desugarConnectionResult{}, err
	}

	desugaredHead := desugarChainResult.replace.Normal.SenderSide[0]

	replace := src.Connection{
		Normal: &src.NormalConnection{
			SenderSide: normConn.SenderSide,
			ReceiverSide: []src.ConnectionReceiver{
				{
					PortAddr: &src.PortAddr{
						Node: desugaredHead.PortAddr.Node,
						Port: chainHeadPort,
						Idx:  desugaredHead.PortAddr.Idx,
						Meta: chainHead.Meta,
					},
				},
			},
		},
	}

	// we need to insert both: replace and insert, example:
	// input = a -> b -> c -> d
	// sender = a
	// chain = b -> c -> d
	// ---
	// replace = b -> c
	// insert = c -> d
	// (and replace existing one with `a -> b`)
	insert := append([]src.Connection{}, desugarChainResult.insert...)
	insert = append(insert, *desugarChainResult.replace)

	return desugarConnectionResult{
		replace: &replace,
		insert:  insert,
	}, nil
}

type desugarDeferredConnectionsResult struct {
	replace src.Connection
	insert  []src.Connection
}

var virtualLocksCounter uint64

func (d Desugarer) desugarDeferredConnection(
	iface src.Interface,
	normConn src.NormalConnection,
	scope src.Scope,
	constsToInsert map[string]src.Const,
	nodesToInsert map[string]src.Node,
	nodesPortsUsed nodeOutportsUsed,
	nodes map[string]src.Node,
) (desugarDeferredConnectionsResult, *compiler.Error) {
	deferredConnection := *normConn.ReceiverSide[0].DeferredConnection

	desugarDeferredConnResult, err := d.desugarConnection(
		iface,
		deferredConnection,
		nodesPortsUsed,
		scope,
		nodes,
		nodesToInsert,
		constsToInsert,
	)
	if err != nil {
		return desugarDeferredConnectionsResult{}, err
	}

	deferredConnection = *desugarDeferredConnResult.replace
	connsToInsert := desugarDeferredConnResult.insert

	// 1) create lock node
	virtualLocksCounter++
	lockNodeName := fmt.Sprintf("__lock__%d", virtualLocksCounter)
	nodesToInsert[lockNodeName] = src.Node{
		EntityRef: core.EntityRef{
			Pkg:  "builtin",
			Name: "Lock",
		},
		TypeArgs: []typesystem.Expr{
			ts.Expr{
				Inst: &typesystem.InstExpr{
					Ref: core.EntityRef{Pkg: "builtin", Name: "any"},
				},
			},
		},
	}

	// 2) connect original sender to lock receiver
	replace := src.Connection{
		Normal: &src.NormalConnection{
			SenderSide: normConn.SenderSide,
			ReceiverSide: []src.ConnectionReceiver{
				{
					PortAddr: &src.PortAddr{
						Node: lockNodeName,
						Port: "sig",
					},
				},
			},
		},
	}

	connsToInsert = append(
		// 3) connect deferred sender to lock data
		connsToInsert,
		src.Connection{
			Normal: &src.NormalConnection{
				SenderSide: deferredConnection.Normal.SenderSide,
				ReceiverSide: []src.ConnectionReceiver{
					{
						PortAddr: &src.PortAddr{
							Node: lockNodeName,
							Port: "data",
						},
					},
				},
			},
		},
		// 4) create connection from lock:data to receiver-side of deferred connection
		src.Connection{
			Normal: &src.NormalConnection{
				SenderSide: []src.ConnectionSender{
					{
						PortAddr: &src.PortAddr{
							Node: lockNodeName,
							Port: "data",
						},
					},
				},
				ReceiverSide: deferredConnection.Normal.ReceiverSide,
			},
		},
	)

	return desugarDeferredConnectionsResult{
		replace: replace,
		insert:  connsToInsert,
	}, nil
}

type desugarSenderResult struct {
	replace src.Connection   // receiver side might need desugaring
	insert  []src.Connection // already desugared
}

// desugarSingleSender keeps receiver side untouched so it must be desugared by caller (except for selectors).
func (d Desugarer) desugarSingleSender(
	iface src.Interface,
	normConn src.NormalConnection,
	scope src.Scope,
	nodes map[string]src.Node,
	usedNodeOutports nodeOutportsUsed,
	nodesToInsert map[string]src.Node,
	constsToInsert map[string]src.Const,
) (desugarSenderResult, *compiler.Error) {
	sender := normConn.SenderSide[0]

	if sender.PortAddr != nil {
		portName := sender.PortAddr.Port
		if sender.PortAddr.Port == "" {
			firstOutportName, err := getFirstOutportName(scope, nodes, *sender.PortAddr)
			if err != nil {
				return desugarSenderResult{}, &compiler.Error{Message: err.Error()}
			}
			portName = firstOutportName
			normConn.SenderSide = []src.ConnectionSender{
				{
					PortAddr: &src.PortAddr{
						Port: portName,
						Node: sender.PortAddr.Node,
						Idx:  sender.PortAddr.Idx,
						Meta: sender.PortAddr.Meta,
					},
				},
			}
		}
		usedNodeOutports.set(
			sender.PortAddr.Node,
			portName,
		)
		return desugarSenderResult{
			replace: src.Connection{Normal: &normConn},
			insert:  nil,
		}, nil
	}

	if len(sender.StructSelector) != 0 {
		result, err := d.desugarStructSelectors(
			normConn,
			nodesToInsert,
			constsToInsert,
		)
		if err != nil {
			return desugarSenderResult{}, compiler.Error{
				Location: &scope.Location,
				Meta:     &sender.Meta,
			}.Wrap(err)
		}

		// connection that replaces original one might need desugaring itself
		replacedConnDesugarRes, err := d.desugarConnection(
			iface,
			result.replace,
			usedNodeOutports,
			scope,
			nodes,
			nodesToInsert,
			constsToInsert,
		)
		if err != nil {
			return desugarSenderResult{}, err
		}

		return desugarSenderResult{
			replace: src.Connection{Normal: replacedConnDesugarRes.replace.Normal},
			insert:  replacedConnDesugarRes.insert,
		}, nil
	}

	if sender.Const != nil {
		if sender.Const.Value.Ref != nil {
			result, err := d.handleConstRefSender(*sender.Const.Value.Ref, nodes, scope)
			if err != nil {
				return desugarSenderResult{}, compiler.Error{
					Location: &scope.Location,
					Meta:     &sender.Meta,
				}.Wrap(err)
			}

			normConn = src.NormalConnection{
				SenderSide: []src.ConnectionSender{
					{
						PortAddr: &result,
						Meta:     sender.Meta,
					},
				},
				ReceiverSide: normConn.ReceiverSide,
			}
		}

		if sender.Const.Value.Message != nil {
			constNodePort, err := d.handleLiteralSender(*sender.Const, nodesToInsert, constsToInsert)
			if err != nil {
				return desugarSenderResult{}, err
			}

			normConn = src.NormalConnection{
				SenderSide: []src.ConnectionSender{
					{
						PortAddr: &constNodePort,
						Meta:     sender.Meta,
					},
				},
				ReceiverSide: normConn.ReceiverSide,
			}
		}

		return desugarSenderResult{
			replace: src.Connection{Normal: &normConn},
			insert:  nil,
		}, nil
	}

	if sender.Ternary != nil {
		result, err := d.desugarTernarySender(
			iface,
			*sender.Ternary,
			normConn,
			nodesToInsert,
			constsToInsert,
			usedNodeOutports,
			scope,
			nodes,
		)
		if err != nil {
			return desugarSenderResult{}, compiler.Error{
				Location: &scope.Location,
				Meta:     &sender.Meta,
			}.Wrap(err)
		}

		return desugarSenderResult(result), nil
	}

	if sender.Binary != nil {
		result, err := d.desugarBinarySender(
			iface,
			*sender.Binary,
			normConn,
			nodesToInsert,
			constsToInsert,
			usedNodeOutports,
			scope,
			nodes,
		)
		if err != nil {
			return desugarSenderResult{}, err
		}
		return desugarSenderResult(result), nil
	}

	result, err := d.desugarRangeSender(
		*sender.Range,
		normConn,
		nodesToInsert,
		constsToInsert,
	)
	if err != nil {
		return desugarSenderResult{}, err
	}

	return desugarSenderResult{
		replace: src.Connection{Normal: &result.replace},
		insert:  result.insert,
	}, nil
}

func getNodeIOByPortAddr(
	scope src.Scope,
	nodes map[string]src.Node,
	portAddr *src.PortAddr,
) (src.IO, *compiler.Error) {
	node, ok := nodes[portAddr.Node]
	if !ok {
		return src.IO{}, &compiler.Error{
			Message:  fmt.Sprintf("node '%s' not found", portAddr.Node),
			Location: &scope.Location,
			Meta:     &portAddr.Meta,
		}
	}

	entity, _, err := scope.Entity(node.EntityRef)
	if err != nil {
		return src.IO{}, &compiler.Error{
			Message:  err.Error(),
			Location: &scope.Location,
			Meta:     &portAddr.Meta,
		}
	}

	var iface src.Interface
	if entity.Kind == src.InterfaceEntity {
		iface = entity.Interface
	} else {
		iface = entity.Component.Interface
	}

	return iface.IO, nil
}

func getFirstInportName(scope src.Scope, nodes map[string]src.Node, portAddr src.PortAddr) (string, error) {
	io, err := getNodeIOByPortAddr(scope, nodes, &portAddr)
	if err != nil {
		return "", err
	}
	for inport := range io.In {
		return inport, nil
	}
	return "", errors.New("first inport not found")
}

func getFirstOutportName(scope src.Scope, nodes map[string]src.Node, portAddr src.PortAddr) (string, error) {
	io, err := getNodeIOByPortAddr(scope, nodes, &portAddr)
	if err != nil {
		return "", err
	}
	for outport := range io.Out {
		return outport, nil
	}
	return "", errors.New("first outport not found")
}

type desugarFanOutResult struct {
	replace src.Connection   // original sender -> fanOut receiver
	insert  []src.Connection // fanOut sender -> original receivers
}

var fanOutCounter uint64

func (d Desugarer) desugarFanOut(
	iface src.Interface,
	normConn src.NormalConnection,
	nodesToInsert map[string]src.Node,
	constsToInsert map[string]src.Const,
	nodePortsUsed nodeOutportsUsed,
	scope src.Scope,
	nodes map[string]src.Node,
) (desugarFanOutResult, *compiler.Error) {
	fanOutCounter++
	nodeName := fmt.Sprintf("__fanOut__%d", fanOutCounter)

	nodesToInsert[nodeName] = src.Node{
		EntityRef: core.EntityRef{
			Name: "FanOut",
			Pkg:  "builtin",
		},
	}

	receiverToReplace := src.ConnectionReceiver{
		PortAddr: &src.PortAddr{
			Node: nodeName,
			Port: "data",
		},
	}

	insert := make([]src.Connection, 0, len(normConn.ReceiverSide))
	for i, receiver := range normConn.ReceiverSide {
		conn := src.Connection{
			Normal: &src.NormalConnection{
				SenderSide: []src.ConnectionSender{
					{
						PortAddr: &src.PortAddr{
							Node: nodeName,
							Port: "data",
							Idx:  compiler.Pointer(uint8(i)),
						},
					},
				},
				ReceiverSide: []src.ConnectionReceiver{receiver},
			},
		}

		desugarConnRes, err := d.desugarConnection(
			iface,
			conn,
			nodePortsUsed,
			scope,
			nodes,
			nodesToInsert,
			constsToInsert,
		)
		if err != nil {
			return desugarFanOutResult{}, err
		}

		insert = append(insert, *desugarConnRes.replace)
		insert = append(insert, desugarConnRes.insert...)
	}

	return desugarFanOutResult{
		replace: src.Connection{
			Normal: &src.NormalConnection{
				SenderSide:   normConn.SenderSide, // senders must be desugared
				ReceiverSide: []src.ConnectionReceiver{receiverToReplace},
			},
		},
		insert: insert,
	}, nil
}

// Add a new atomic counter for range nodes
var rangeCounter uint64

// Add a new function to handle range senders
type handleRangeSenderResult struct {
	replace src.NormalConnection
	insert  []src.Connection
}

// desugarRangeSender desugars `from..to -> XXX` part.
// It does not create connection to range:sig,
// it's done in chained connection desugaring.
func (d Desugarer) desugarRangeSender(
	rangeExpr src.Range,
	normConn src.NormalConnection,
	nodesToInsert map[string]src.Node,
	constsToInsert map[string]src.Const,
) (handleRangeSenderResult, *compiler.Error) {
	rangeCounter++

	rangeNodeName := fmt.Sprintf("__range%d__", rangeCounter)
	fromConstName := fmt.Sprintf("__range%d_from__", rangeCounter)
	toConstName := fmt.Sprintf("__range%d_to__", rangeCounter)

	constsToInsert[fromConstName] = src.Const{
		TypeExpr: ts.Expr{Inst: &ts.InstExpr{Ref: core.EntityRef{Pkg: "builtin", Name: "int"}}},
		Value:    src.ConstValue{Message: &src.MsgLiteral{Int: compiler.Pointer(int(rangeExpr.From))}},
	}
	constsToInsert[toConstName] = src.Const{
		TypeExpr: ts.Expr{Inst: &ts.InstExpr{Ref: core.EntityRef{Pkg: "builtin", Name: "int"}}},
		Value:    src.ConstValue{Message: &src.MsgLiteral{Int: compiler.Pointer(int(rangeExpr.To))}},
	}

	nodesToInsert[rangeNodeName] = src.Node{
		EntityRef: core.EntityRef{Pkg: "builtin", Name: "Range"},
	}
	nodesToInsert[fromConstName] = src.Node{
		EntityRef: core.EntityRef{Pkg: "builtin", Name: "New"},
		Directives: map[src.Directive][]string{
			"bind": {fromConstName},
		},
	}
	nodesToInsert[toConstName] = src.Node{
		EntityRef: core.EntityRef{Pkg: "builtin", Name: "New"},
		Directives: map[src.Directive][]string{
			"bind": {toConstName},
		},
	}

	replace := src.NormalConnection{
		SenderSide: []src.ConnectionSender{
			{
				PortAddr: &src.PortAddr{Node: rangeNodeName, Port: "res"},
			},
		},
		ReceiverSide: normConn.ReceiverSide,
	}

	insert := []src.Connection{
		// $from -> range:from
		{
			Normal: &src.NormalConnection{
				SenderSide: []src.ConnectionSender{
					{
						PortAddr: &src.PortAddr{Node: fromConstName, Port: "msg"},
					},
				},
				ReceiverSide: []src.ConnectionReceiver{
					{
						PortAddr: &src.PortAddr{Node: rangeNodeName, Port: "from"},
					},
				},
			},
		},
		// $to -> range:to
		{
			Normal: &src.NormalConnection{
				SenderSide: []src.ConnectionSender{
					{
						PortAddr: &src.PortAddr{Node: toConstName, Port: "msg"},
					},
				},
				ReceiverSide: []src.ConnectionReceiver{
					{
						PortAddr: &src.PortAddr{Node: rangeNodeName, Port: "to"},
					},
				},
			},
		},
	}

	return handleRangeSenderResult{
		insert:  insert,
		replace: replace,
	}, nil
}

var fanInCounter uint64

// desugarFanIn returns connections that must be used instead of given one.
// It recursevely desugars each connection before return so result is final.
func (d Desugarer) desugarFanIn(
	iface src.Interface,
	normConn src.NormalConnection,
	nodesToInsert map[string]src.Node,
	constsToInsert map[string]src.Const,
	nodePortsUsed nodeOutportsUsed,
	scope src.Scope,
	nodes map[string]src.Node,
) ([]src.Connection, error) {
	// 1. insert unique fan-in node
	fanInCounter++
	fanInNodeName := fmt.Sprintf("__fanIn__%d", fanInCounter)
	nodesToInsert[fanInNodeName] = src.Node{
		EntityRef: core.EntityRef{
			Pkg:  "builtin",
			Name: "FanIn",
		},
	}

	// 2. connection each sender with fan-in node
	netWithoutFanIn := make([]src.Connection, 0, len(normConn.SenderSide))
	for i, sender := range normConn.SenderSide {
		netWithoutFanIn = append(netWithoutFanIn, src.Connection{
			Normal: &src.NormalConnection{
				SenderSide: []src.ConnectionSender{sender},
				ReceiverSide: []src.ConnectionReceiver{
					{
						PortAddr: &src.PortAddr{
							Node: fanInNodeName,
							Port: "data",
							Idx:  compiler.Pointer(uint8(i)),
						},
					},
				},
			},
		})
	}

	// 3. insert new connection from fan-in to original receivers
	netWithoutFanIn = append(netWithoutFanIn, src.Connection{
		Normal: &src.NormalConnection{
			SenderSide: []src.ConnectionSender{
				{
					PortAddr: &src.PortAddr{
						Node: fanInNodeName,
						Port: "res",
					},
				},
			},
			ReceiverSide: normConn.ReceiverSide,
		},
	})

	// 4. desugar each connection (original senders and receivers might need it)
	desugaredConnections, err := d.desugarConnections(
		iface,
		netWithoutFanIn,
		nodePortsUsed,
		scope,
		nodes,
		nodesToInsert,
		constsToInsert,
	)
	if err != nil {
		return nil, &compiler.Error{
			Message:  err.Error(),
			Location: &scope.Location,
			Meta:     &normConn.Meta,
		}
	}

	return desugaredConnections, nil
}

// Add this variable at the package level
var ternaryCounter uint64

type handleTernarySenderResult struct {
	replace src.Connection
	insert  []src.Connection
}

// (cond ? left : right) -> XXX;
// =>
// 1) cond -> ternary:if;
// 2) left -> ternary:then;
// 3) right -> ternary:else;
// 4) ternary:res -> XXX;
func (d Desugarer) desugarTernarySender(
	iface src.Interface,
	ternary src.Ternary,
	normConn src.NormalConnection,
	nodesToInsert map[string]src.Node,
	constsToInsert map[string]src.Const,
	usedNodeOutports nodeOutportsUsed,
	scope src.Scope,
	nodes map[string]src.Node,
) (handleTernarySenderResult, *compiler.Error) {
	ternaryCounter++
	ternaryNodeName := fmt.Sprintf("__ternary__%d", ternaryCounter)

	nodesToInsert[ternaryNodeName] = src.Node{
		EntityRef: core.EntityRef{
			Pkg:  "builtin",
			Name: "Ternary",
		},
	}

	sugaredInsert := []src.Connection{
		// 1) cond -> ternary:if
		{
			Normal: &src.NormalConnection{
				SenderSide: []src.ConnectionSender{ternary.Condition},
				ReceiverSide: []src.ConnectionReceiver{
					{
						PortAddr: &src.PortAddr{
							Node: ternaryNodeName,
							Port: "if",
						},
					},
				},
			},
		},
		// 2) left -> ternary:then
		{
			Normal: &src.NormalConnection{
				SenderSide: []src.ConnectionSender{ternary.Left},
				ReceiverSide: []src.ConnectionReceiver{
					{
						PortAddr: &src.PortAddr{
							Node: ternaryNodeName,
							Port: "then",
						},
					},
				},
			},
		},
		// right -> ternary:else
		{
			Normal: &src.NormalConnection{
				SenderSide: []src.ConnectionSender{ternary.Right},
				ReceiverSide: []src.ConnectionReceiver{
					{
						PortAddr: &src.PortAddr{Node: ternaryNodeName, Port: "else"},
					},
				},
			},
		},
	}

	desugaredInsert := make([]src.Connection, 0, len(sugaredInsert))
	for _, conn := range sugaredInsert {
		desugarConnRes, err := d.desugarConnection(
			iface,
			conn,
			usedNodeOutports,
			scope,
			nodes,
			nodesToInsert,
			constsToInsert,
		)
		if err != nil {
			return handleTernarySenderResult{}, err
		}
		desugaredInsert = append(desugaredInsert, *desugarConnRes.replace)
		desugaredInsert = append(desugaredInsert, desugarConnRes.insert...)
	}

	// 4) ternary:res -> XXX;
	sugaredReplace := src.Connection{
		Normal: &src.NormalConnection{
			SenderSide: []src.ConnectionSender{
				{
					PortAddr: &src.PortAddr{
						Node: ternaryNodeName,
						Port: "res",
					},
				},
			},
			ReceiverSide: normConn.ReceiverSide,
		},
	}

	return handleTernarySenderResult{
		replace: sugaredReplace,
		insert:  desugaredInsert,
	}, nil
}

type handleBinarySenderResult struct {
	replace src.Connection
	insert  []src.Connection
}

var (
	// Arithmetic
	addCounter uint64
	subCounter uint64
	mulCounter uint64
	divCounter uint64
	modCounter uint64
	powCounter uint64
	// Comparison
	eqCounter uint64
	neCounter uint64
	gtCounter uint64
	ltCounter uint64
	geCounter uint64
	leCounter uint64
	// Logical
	andCounter uint64
	orCounter  uint64
	// Bitwise
	bitAndCounter uint64
	bitOrCounter  uint64
	bitXorCounter uint64
	bitLshCounter uint64
	bitRshCounter uint64
)

func (d Desugarer) desugarBinarySender(
	iface src.Interface,
	binary src.Binary,
	normConn src.NormalConnection,
	nodesToInsert map[string]src.Node,
	constsToInsert map[string]src.Const,
	usedNodeOutports nodeOutportsUsed,
	scope src.Scope,
	nodes map[string]src.Node,
) (handleBinarySenderResult, *compiler.Error) {
	var (
		opNode      string
		opComponent string
	)

	switch binary.Operator {
	// Arithmetic
	case src.AddOp:
		addCounter++
		opNode = fmt.Sprintf("__add__%d", addCounter)
		opComponent = "Add"
	case src.SubOp:
		subCounter++
		opNode = fmt.Sprintf("__sub__%d", subCounter)
		opComponent = "Sub"
	case src.MulOp:
		mulCounter++
		opNode = fmt.Sprintf("__mul__%d", mulCounter)
		opComponent = "Mul"
	case src.DivOp:
		divCounter++
		opNode = fmt.Sprintf("__div__%d", divCounter)
		opComponent = "Div"
	case src.ModOp:
		modCounter++
		opNode = fmt.Sprintf("__mod__%d", modCounter)
		opComponent = "Mod"
	case src.PowOp:
		powCounter++
		opNode = fmt.Sprintf("__pow__%d", powCounter)
		opComponent = "Pow"
	// Comparison
	case src.EqOp:
		eqCounter++
		opNode = fmt.Sprintf("__eq__%d", eqCounter)
		opComponent = "Eq"
	case src.NeOp:
		neCounter++
		opNode = fmt.Sprintf("__ne__%d", neCounter)
		opComponent = "Ne"
	case src.GtOp:
		gtCounter++
		opNode = fmt.Sprintf("__gt__%d", gtCounter)
		opComponent = "Gt"
	case src.LtOp:
		ltCounter++
		opNode = fmt.Sprintf("__lt__%d", ltCounter)
		opComponent = "Lt"
	case src.GeOp:
		geCounter++
		opNode = fmt.Sprintf("__ge__%d", geCounter)
		opComponent = "Ge"
	case src.LeOp:
		leCounter++
		opNode = fmt.Sprintf("__le__%d", leCounter)
		opComponent = "Le"
	// Logical
	case src.AndOp:
		andCounter++
		opNode = fmt.Sprintf("__and__%d", andCounter)
		opComponent = "And"
	case src.OrOp:
		orCounter++
		opNode = fmt.Sprintf("__or__%d", orCounter)
		opComponent = "Or"
	// Bitwise
	case src.BitAndOp:
		bitAndCounter++
		opNode = fmt.Sprintf("__bitAnd__%d", bitAndCounter)
		opComponent = "BitAnd"
	case src.BitOrOp:
		bitOrCounter++
		opNode = fmt.Sprintf("__bitOr__%d", bitOrCounter)
		opComponent = "BitOr"
	case src.BitXorOp:
		bitXorCounter++
		opNode = fmt.Sprintf("__bitXor__%d", bitXorCounter)
		opComponent = "BitXor"
	case src.BitLshOp:
		bitLshCounter++
		opNode = fmt.Sprintf("__bitLsh__%d", bitLshCounter)
		opComponent = "BitLsh"
	case src.BitRshOp:
		bitRshCounter++
		opNode = fmt.Sprintf("__bitRsh__%d", bitRshCounter)
		opComponent = "BitRsh"
	default:
		return handleBinarySenderResult{}, &compiler.Error{
			Message:  fmt.Sprintf("unsupported binary operator: %s", binary.Operator),
			Location: &scope.Location,
			Meta:     &binary.Meta,
		}
	}

	nodesToInsert[opNode] = src.Node{
		EntityRef: core.EntityRef{
			Pkg:  "builtin",
			Name: opComponent,
		},
		TypeArgs: []ts.Expr{binary.AnalyzedType},
	}

	// left -> op:left
	// right -> op:right
	sugaredInsert := []src.Connection{
		{
			Normal: &src.NormalConnection{
				SenderSide: []src.ConnectionSender{binary.Left},
				ReceiverSide: []src.ConnectionReceiver{
					{
						PortAddr: &src.PortAddr{Node: opNode, Port: "left"},
					},
				},
			},
		},
		{
			Normal: &src.NormalConnection{
				SenderSide: []src.ConnectionSender{binary.Right},
				ReceiverSide: []src.ConnectionReceiver{
					{
						PortAddr: &src.PortAddr{Node: opNode, Port: "right"},
					},
				},
			},
		},
	}

	// operand-senders might be sugared, so we need to desugar them
	desugaredInsert := make([]src.Connection, 0, len(sugaredInsert))
	for _, conn := range sugaredInsert {
		desugarConnRes, err := d.desugarConnection(
			iface,
			conn,
			usedNodeOutports,
			scope,
			nodes,
			nodesToInsert,
			constsToInsert,
		)
		if err != nil {
			return handleBinarySenderResult{}, err
		}
		desugaredInsert = append(desugaredInsert, *desugarConnRes.replace)
		desugaredInsert = append(desugaredInsert, desugarConnRes.insert...)
	}

	// op:res -> XXX
	replace := src.Connection{
		Normal: &src.NormalConnection{
			SenderSide: []src.ConnectionSender{
				{
					PortAddr: &src.PortAddr{
						Node: opNode,
						Port: "res",
					},
				},
			},
			ReceiverSide: normConn.ReceiverSide, // desugaring of original receivers is job of caller
		},
	}

	return handleBinarySenderResult{
		replace: replace,
		insert:  desugaredInsert,
	}, nil
}
