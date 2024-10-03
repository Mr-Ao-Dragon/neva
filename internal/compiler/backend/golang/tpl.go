package golang

type templateData struct {
	CompilerVersion string
	ChanVarNames    []string
	FuncCalls       []templateFuncCall
	Trace           bool
}

type templateFuncCall struct {
	Ref    string
	Config string
	IO     templateIO
}

type templateIO struct {
	In  map[string]string
	Out map[string]string
}

var mainGoTemplate = `// Code generated by Neva v{{.CompilerVersion}}. DO NOT EDIT.
package main

import (
    "context"

    "github.com/nevalang/neva/internal/runtime"
    "github.com/nevalang/neva/internal/runtime/funcs"
)

func main() {
    var (
        {{- range .ChanVarNames}}
        {{.}} = make(chan runtime.OrderedMsg)
        {{- end}}
    )

    {{- if .Trace }}
    interceptor := runtime.NewDebugInterceptor()

    close, err := interceptor.Open("trace.log")
    if err != nil {
        panic(err)
    }
    defer func() {
        if err := close(); err != nil {
            panic(err)
        }
    }()
    {{- else }}
    interceptor := runtime.ProdInterceptor{}
    {{- end }}

    var (
        startPort = runtime.NewSingleOutport(
            runtime.PortAddr{Path: "in", Port: "start"},
            interceptor,
            {{getPortChanNameByAddr "in" "start"}},
        )
        stopPort = runtime.NewSingleInport(
            {{getPortChanNameByAddr "out" "stop"}},
            runtime.PortAddr{Path: "out", Port: "stop"},
            interceptor,
        )
    )

    funcCalls := []runtime.FuncCall{
        {{- range .FuncCalls}}
        {
            Ref: "{{.Ref}}",
            IO: runtime.IO{
                In: runtime.NewInports(map[string]runtime.Inport{
                    {{- range $key, $value := .IO.In}}
                    "{{$key}}": {{$value}},
                    {{- end}}
                }),
                Out: runtime.NewOutports(map[string]runtime.Outport{
                    {{- range $key, $value := .IO.Out}}
                    "{{$key}}": {{$value}},
                    {{- end}}
                }),
            },
            Config: {{.Config}},
        },
        {{- end}}
    }

    rprog := runtime.Program{
        Start: startPort,
        Stop: stopPort,
        FuncCalls: funcCalls,
    }
    
    if err := runtime.Run(context.Background(), rprog, funcs.NewRegistry()); err != nil {
		panic(err)
	}
}
`
