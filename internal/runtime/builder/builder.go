package builder

import (
	"errors"
	"fmt"

	"github.com/emil14/neva/internal/core"
	"github.com/emil14/neva/internal/runtime"
	"github.com/emil14/neva/internal/runtime/src"
	"golang.org/x/sync/errgroup"
)

type Builder struct{}

func (b Builder) Build(prog src.Program) (runtime.Build, error) {
	var (
		g           errgroup.Group
		ports       = b.buildPorts(prog.Ports)
		connections []runtime.Connection
		effects     runtime.Effects
	)

	g.Go(func() error {
		var err error
		connections, err = b.buildConnections(ports, prog.Connections)
		if err != nil {
			return fmt.Errorf("build connections: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		effects, err = b.buildEffects(ports, prog.Effects)
		if err != nil {
			return fmt.Errorf("build effects: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return runtime.Build{}, fmt.Errorf("wait: %w", err)
	}

	return runtime.Build{
		StartPort:   prog.StartPort,
		Ports:       ports,
		Connections: connections,
		Effects:     effects,
	}, nil
}

func (b Builder) buildPorts(in src.Ports) runtime.Ports {
	out := make(
		runtime.Ports,
		len(in),
	)
	for addr, buf := range in {
		out[addr] = make(chan core.Msg, buf)
	}
	return out
}

func (b Builder) buildConnections(ports runtime.Ports, srcConns []src.Connection) ([]runtime.Connection, error) {
	cc := make([]runtime.Connection, 0, len(srcConns))

	for _, srcConn := range srcConns {
		c, err := b.buildConnection(ports, srcConn)
		if err != nil {
			return nil, fmt.Errorf("build connection: err %w, conn %v", err, srcConn)
		}

		cc = append(cc, c)
	}

	return cc, nil
}

func (b Builder) buildConnection(ports runtime.Ports, srcConn src.Connection) (runtime.Connection, error) {
	senderPort, ok := ports[srcConn.SenderPortAddr]
	if !ok {
		return runtime.Connection{}, fmt.Errorf("%w: %v", core.ErrPortNotFound, srcConn.SenderPortAddr)
	}

	rr := make([]chan core.Msg, 0, len(srcConn.ReceiversConnectionPoints))
	for _, srcReceiverPoint := range srcConn.ReceiversConnectionPoints {
		receiverPort, ok := ports[srcReceiverPoint.PortAddr]
		if !ok {
			return runtime.Connection{}, fmt.Errorf("%w: %v", core.ErrPortNotFound, srcConn.SenderPortAddr)
		}

		rr = append(rr, receiverPort)
	}

	return runtime.Connection{
		Src:       srcConn,
		Sender:    senderPort,
		Receivers: rr,
	}, nil
}

func (b Builder) buildEffects(ports runtime.Ports, effects src.Effects) (runtime.Effects, error) {
	c, err := b.buildConstEffects(ports, effects.Constants)
	if err != nil {
		return runtime.Effects{}, fmt.Errorf("build const effects: %w", err)
	}

	o, err := b.buildOperatorEffects(ports, effects.Operators)
	if err != nil {
		return runtime.Effects{}, fmt.Errorf("build operator effects: %w", err)
	}

	t, err := b.buildTriggerEffects(ports, effects.Triggers)
	if err != nil {
		return runtime.Effects{}, fmt.Errorf("build operator effects: %w", err)
	}

	return runtime.Effects{
		Constants: c,
		Operators: o,
		Triggers:  t,
	}, nil
}

var ErrPortNotFound = errors.New("port not found")

func (b Builder) buildConstEffects(
	ports runtime.Ports,
	in map[src.AbsPortAddr]src.Msg,
) ([]runtime.ConstantEffect, error) {
	result := make([]runtime.ConstantEffect, 0, len(in))

	for addr, msg := range in {
		port, ok := ports[addr]
		if !ok {
			return nil, fmt.Errorf("%w: %v", ErrPortNotFound, addr)
		}

		msg, err := b.buildCoreMsg(msg)
		if err != nil {
			return nil, fmt.Errorf("build core msg: %w", err)
		}

		result = append(result, runtime.ConstantEffect{
			OutPort: port,
			Msg:     msg,
		})
	}

	return result, nil
}

func (b Builder) buildOperatorEffects(
	ports runtime.Ports,
	ops []src.OperatorEffect,
) ([]runtime.OperatorEffect, error) {
	result := make([]runtime.OperatorEffect, 0, len(ops))

	for _, srcOpEffect := range ops {
		io := core.IO{
			In:  make(core.Ports, len(srcOpEffect.PortAddrs.In)),
			Out: make(core.Ports, len(srcOpEffect.PortAddrs.Out)),
		}

		for _, addr := range srcOpEffect.PortAddrs.In {
			port, ok := ports[addr]
			if !ok {
				return nil, fmt.Errorf("%w: %v", core.ErrPortNotFound, addr)
			}
			relativeAddr := core.RelPortAddr{
				Port: addr.Port,
				Idx:  addr.Idx,
			}
			io.In[relativeAddr] = port
		}

		for _, addr := range srcOpEffect.PortAddrs.Out {
			port, ok := ports[addr]
			if !ok {
				return nil, fmt.Errorf("%w: %v", core.ErrPortNotFound, addr)
			}
			relativeAddr := core.RelPortAddr{
				Port: addr.Port,
				Idx:  addr.Idx,
			}
			io.Out[relativeAddr] = port
		}

		result = append(result, runtime.OperatorEffect{
			Ref: srcOpEffect.Ref,
			IO:  io,
		})
	}

	return result, nil
}

var ErrUnknownMsgType = errors.New("unknown message type")

func (b Builder) buildCoreMsg(in src.Msg) (core.Msg, error) {
	var out core.Msg

	switch in.Type {
	case src.IntMsg:
		out = core.NewIntMsg(in.Int)
	case src.BoolMsg:
		out = core.NewBoolMsg(in.Bool)
	case src.StrMsg:
		out = core.NewStrMsg(in.Str)
	case src.StructMsg:
		structMsg := make(map[string]core.Msg, len(in.Struct))
		for field, value := range in.Struct {
			v, err := b.buildCoreMsg(value)
			if err != nil {
				return nil, fmt.Errorf("core msg: %w", err)
			}
			structMsg[field] = v
		}
		out = core.NewDictMsg(structMsg)
	default:
		return nil, fmt.Errorf("%w: %v", ErrUnknownMsgType, in.Type)
	}

	return out, nil
}

func (b Builder) buildTriggerEffects(
	ports runtime.Ports,
	in []src.TriggerEffect,
) ([]runtime.TriggerEffect, error) {
	result := make([]runtime.TriggerEffect, 0, len(in))

	for _, effect := range in {
		inPort, ok := ports[effect.InPortAddr]
		if !ok {
			return nil, fmt.Errorf("%w: %v", ErrPortNotFound, effect.InPortAddr)
		}

		outPort, ok := ports[effect.OutPortAddr]
		if !ok {
			return nil, fmt.Errorf("%w: %v", ErrPortNotFound, effect.InPortAddr)
		}

		msg, err := b.buildCoreMsg(effect.Msg)
		if err != nil {
			return nil, fmt.Errorf("build core msg: %w", err)
		}

		result = append(result, runtime.TriggerEffect{
			InPort:  inPort,
			OutPort: outPort,
			Msg:     msg,
		})
	}

	return result, nil
}