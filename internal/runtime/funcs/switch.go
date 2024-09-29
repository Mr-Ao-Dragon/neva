package funcs

import (
	"context"
	"errors"

	"github.com/nevalang/neva/internal/runtime"
)

type switcher struct{}

func (switcher) Create(io runtime.IO, _ runtime.Msg) (func(ctx context.Context), error) {
	dataIn, err := io.In.Single("data")
	if err != nil {
		return nil, err
	}

	caseIn, err := io.In.Array("case")
	if err != nil {
		return nil, err
	}

	caseOut, err := io.Out.Array("case")
	if err != nil {
		return nil, err
	}

	elseOut, err := io.Out.Single("else")
	if err != nil {
		return nil, err
	}

	if caseIn.Len() != caseOut.Len() {
		return nil, errors.New("number of 'case' inports must match number of outports")
	}

	return func(ctx context.Context) {
		for {
			dataMsg, ok := dataIn.Receive(ctx)
			if !ok {
				return
			}

			cases := make([]runtime.Msg, caseIn.Len())
			if !caseIn.ReceiveAll(ctx, func(idx int, msg runtime.Msg) bool {
				cases[idx] = msg
				return true
			}) {
				return
			}

			matchIdx := -1
			for i, caseMsg := range cases {
				if dataMsg == caseMsg {
					matchIdx = i
					break
				}
			}

			if matchIdx != -1 {
				if !caseOut.Send(ctx, uint8(matchIdx), dataMsg) {
					return
				}
				continue
			}

			if !elseOut.Send(ctx, dataMsg) {
				return
			}
		}
	}, nil
}