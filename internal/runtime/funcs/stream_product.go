package funcs

import (
	"context"

	"github.com/nevalang/neva/internal/runtime"
)

type streamProduct struct{}

func (streamProduct) Create(
	io runtime.IO,
	_ runtime.Msg,
) (func(ctx context.Context), error) {
	firstIn, err := io.In.Single("first")
	if err != nil {
		return nil, err
	}

	secondIn, err := io.In.Single("second")
	if err != nil {
		return nil, err
	}

	seqOut, err := io.Out.Single("data")
	if err != nil {
		return nil, err
	}

	// TODO: make sure it's not possible to do processing on the fly so we don't have to wait for both streams to complete
	return func(ctx context.Context) {
		for {
			firstData := []runtime.Msg{}
			for {
				seqMsg, ok := firstIn.Receive(ctx)
				if !ok {
					return
				}

				item := seqMsg.Struct()
				firstData = append(firstData, item.Get("data"))

				if item.Get("last").Bool() {
					break
				}
			}

			secondData := []runtime.Msg{}
			for {
				seqMsg, ok := secondIn.Receive(ctx)
				if !ok {
					return
				}

				item := seqMsg.Struct()
				secondData = append(secondData, item.Get("data"))

				if item.Get("last").Bool() {
					break
				}
			}

			for i, msg1 := range firstData {
				for j, msg2 := range secondData {
					seqOut.Send(
						ctx,
						streamItem(
							runtime.NewStructMsg(
								[]string{"first", "second"},
								[]runtime.Msg{msg1, msg2},
							),
							int64(i),
							i == len(firstData)-1 && j == len(secondData)-1,
						),
					)
				}
			}
		}
	}, nil
}