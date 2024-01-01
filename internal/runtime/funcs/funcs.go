package funcs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nevalang/neva/internal/runtime"
)

func structSelector(io runtime.FuncIO, msg runtime.Msg) (func(ctx context.Context), error) {
	field, ok := msg.(runtime.Msg)
	if !ok {
		return nil, errors.New("ctx value is not runtime message")
	}

	fieldStr := field.Str()
	if fieldStr == "" {
		return nil, errors.New("field name cannot be empty")
	}

	vin, err := io.In.Port("v")
	if err != nil {
		return nil, err
	}

	vout, err := io.Out.Port("v")
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case obj := <-vin:
				fieldValue := obj.Map()[fieldStr]
				select {
				case <-ctx.Done():
					return
				case vout <- fieldValue:
				}
			}
		}
	}, nil
}

func mapSelector(io runtime.FuncIO, _ runtime.Msg) (func(ctx context.Context), error) {
	// in
	vin, err := io.In.Port("v")
	if err != nil {
		return nil, err
	}
	kin, err := io.In.Port("k")
	if err != nil {
		return nil, err
	}

	// out
	okOut, err := io.Out.Port("ok")
	if err != nil {
		return nil, err
	}
	missOut, err := io.Out.Port("miss")
	if err != nil {
		return nil, err
	}

	// logic
	return func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case m := <-vin: // read map
				select {
				case <-ctx.Done():
					return
				case k := <-kin: // then read key
					var ( // figure out what and where to send
						msg runtime.Msg
						out chan runtime.Msg
					)
					v, ok := m.Map()[k.Str()]
					if ok {
						msg = v
						out = okOut
					} else {
						msg = k // if value not found, send missing key as a signal for miss outport
						out = missOut
					}
					select { // and send
					case <-ctx.Done():
						return
					case out <- msg:
						return
					}
				}
			}
		}
	}, nil
}

func read(io runtime.FuncIO, _ runtime.Msg) (func(ctx context.Context), error) {
	sig, err := io.In.Port("sig")
	if err != nil {
		return nil, err
	}
	vout, err := io.Out.Port("v")
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) {
		reader := bufio.NewReader(os.Stdin)
		for {
			select {
			case <-ctx.Done():
				return
			case <-sig:
				select {
				case <-ctx.Done():
					return
				default:
					bb, _, err := reader.ReadLine()
					if err != nil {
						panic(err)
					}
					select {
					case <-ctx.Done():
						return
					case vout <- runtime.NewStrMsg(string(bb)):
					}
				}
			}
		}
	}, nil
}

func print(io runtime.FuncIO, _ runtime.Msg) (func(ctx context.Context), error) { //nolint:predeclared
	vin, err := io.In.Port("v")
	if err != nil {
		return nil, err
	}
	vout, err := io.Out.Port("v")
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case v := <-vin:
				select {
				case <-ctx.Done():
					return
				default:
					fmt.Println(v.String())
					select {
					case <-ctx.Done():
						return
					case vout <- v:
					}
				}
			}
		}
	}, nil
}

func lock(io runtime.FuncIO, _ runtime.Msg) (func(ctx context.Context), error) {
	vin, err := io.In.Port("v")
	if err != nil {
		return nil, err
	}
	sig, err := io.In.Port("sig")
	if err != nil {
		return nil, err
	}
	vout, err := io.Out.Port("v")
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sig:
				select {
				case <-ctx.Done():
					return
				case v := <-vin:
					select {
					case <-ctx.Done():
						return
					case vout <- v:
					}
				}
			}
		}
	}, nil
}

func constant(io runtime.FuncIO, msg runtime.Msg) (func(ctx context.Context), error) {
	v, ok := msg.(runtime.Msg)
	if !ok {
		return nil, errors.New("ctx value is not runtime message")
	}

	vout, err := io.Out.Port("v")
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case vout <- v:
			}
		}
	}, nil
}

func void(io runtime.FuncIO, _ runtime.Msg) (func(ctx context.Context), error) {
	vin, err := io.In.Port("v")
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-vin:
			}
		}
	}, nil
}

func addInts(io runtime.FuncIO, _ runtime.Msg) (func(ctx context.Context), error) {
	ain, err := io.In.Port("a")
	if err != nil {
		return nil, err
	}
	bin, err := io.In.Port("b")
	if err != nil {
		return nil, err
	}
	vout, err := io.Out.Port("v")
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case v1 := <-ain:
				select {
				case <-ctx.Done():
					return
				case v2 := <-bin:
					select {
					case <-ctx.Done():
						return
					case vout <- runtime.NewIntMsg(v1.Int() + v2.Int()):
					}
				}
			}
		}
	}, nil
}

func addFloats(io runtime.FuncIO, _ runtime.Msg) (func(ctx context.Context), error) {
	ain, err := io.In.Port("a")
	if err != nil {
		return nil, err
	}
	bin, err := io.In.Port("b")
	if err != nil {
		return nil, err
	}
	vout, err := io.Out.Port("v")
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case v1 := <-ain:
				select {
				case <-ctx.Done():
					return
				case v2 := <-bin:
					select {
					case <-ctx.Done():
						return
					case vout <- runtime.NewFloatMsg(v1.Float() + v2.Float()):
					}
				}
			}
		}
	}, nil
}

func parseInt(io runtime.FuncIO, _ runtime.Msg) (func(ctx context.Context), error) {
	vin, err := io.In.Port("v")
	if err != nil {
		return nil, err
	}

	vout, err := io.Out.Port("v")
	if err != nil {
		return nil, err
	}

	errout, err := io.Out.Port("err")
	if err != nil {
		return nil, err
	}

	parseFunc := func(str string) (runtime.Msg, error) {
		v, err := strconv.Atoi(str)
		if err != nil {
			return nil, errors.New(strings.TrimPrefix(err.Error(), "strconv.Atoi: "))
		}
		return runtime.NewIntMsg(int64(v)), nil
	}

	return func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case str := <-vin:
				v, err := parseFunc(str.Str())
				if err != nil {
					select {
					case <-ctx.Done():
						return
					case errout <- runtime.NewStrMsg(err.Error()):
					}
					continue
				}
				select {
				case <-ctx.Done():
					return
				case vout <- v:
				}
			}
		}
	}, nil
}

func Registry() map[string]runtime.Func {
	return map[string]runtime.Func{
		"Read":           read,
		"Print":          print,
		"Lock":           lock,
		"Const":          constant,
		"AddInts":        addInts,
		"AddFloats":      addFloats,
		"ParseInt":       parseInt,
		"Void":           void,
		"StructSelector": structSelector,
		"MapSelector":    mapSelector,
	}
}
