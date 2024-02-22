package golang

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"

	"github.com/nevalang/neva/internal/compiler"
	"github.com/nevalang/neva/pkg/ir"
)

func getMsg(msg *ir.Msg) (string, error) {
	if msg == nil {
		return "nil", nil
	}
	switch msg.Type {
	case ir.MsgTypeBool:
		return fmt.Sprintf("runtime.NewBoolMsg(%v)", msg.Bool), nil
	case ir.MsgTypeInt:
		return fmt.Sprintf("runtime.NewIntMsg(%v)", msg.Int), nil
	case ir.MsgTypeFloat:
		return fmt.Sprintf("runtime.NewFloatMsg(%v)", msg.Float), nil
	case ir.MsgTypeString:
		return fmt.Sprintf(`runtime.NewStrMsg("%v")`, msg.Str), nil
	case ir.MsgTypeList:
		s := `runtime.NewListMsg(
	`
		for _, v := range msg.List {
			el, err := getMsg(compiler.Pointer(v))
			if err != nil {
				return "", err
			}
			s += fmt.Sprintf(`	%v,
`, el)
		}
		return s + ")", nil
	case ir.MsgTypeMap:
		s := `runtime.NewMapMsg(map[string]runtime.Msg{
	`
		for k, v := range msg.Map {
			el, err := getMsg(compiler.Pointer(v))
			if err != nil {
				return "", err
			}
			s += fmt.Sprintf(`	"%v": %v,
`, k, el)
		}
		return s + `},
)`, nil
	}

	return "", fmt.Errorf("%w: %v", ErrUnknownMsgType, msg.Type)
}

func getConnComment(conn *ir.Connection) string {
	s := fmtPortAddr(conn.SenderSide) + " -> "
	for _, rcvr := range conn.ReceiverSides {
		s += fmtPortAddr(rcvr.PortAddr)
	}
	return "// " + s
}

func fmtPortAddr(addr ir.PortAddr) string {
	return fmt.Sprintf("%s:%s[%d]", addr.Path, addr.Port, addr.Idx)
}

func getPortChVarName(addr *ir.PortAddr) string {
	path := handleSpecialChars(addr.Path)
	port := addr.Port
	if path != "" {
		port = uppercaseFirstLetter(addr.Port)
	}
	return fmt.Sprintf("%s%s%dPort", path, port, addr.Idx)
}

func getPortsFunc(ports []ir.PortInfo) func(path, port string) string {
	return func(path, port string) string {
		var s string
		for _, info := range ports {
			if info.PortAddr.Path == path && info.PortAddr.Port == port {
				s = s + getPortChVarName(&info.PortAddr) + ","
			}
		}
		return s
	}
}

func handleSpecialChars(portPath string) string {
	var (
		buffer          bytes.Buffer
		shouldUppercase bool
	)

	for i := 0; i < len(portPath); i++ {
		cur := portPath[i]
		if cur == '$' {
			buffer.WriteString("_")
			continue
		}
		if cur == '.' || cur == '/' || cur == ':' {
			shouldUppercase = true
			continue
		}
		s := string(cur)
		if shouldUppercase {
			s = strings.ToUpper(s)
			shouldUppercase = false
		}
		buffer.WriteString(s)
	}

	return buffer.String()
}

func uppercaseFirstLetter(s string) string {
	if len(s) == 0 {
		return s
	}
	bb := []byte(s)
	bb[0] = byte(unicode.ToUpper(rune(bb[0])))
	return string(bb)
}