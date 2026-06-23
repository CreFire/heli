package msg

import (
	_ "game/src/proto/errorpb"
	_ "game/src/proto/pb"
	_ "game/src/proto/pbrpc"
	"slices"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

var PbParser *PBParser

func init() {
	PbParser = NewPBParser()
}

var FILTER_LIST = []string{"START", "END", "NONE"}
var PACKAGE_NAME_LIST = []string{"pb", "pbrpc"}

func filterMsgId(msgId string) bool {
	parts := strings.Split(msgId, "_")
	l := len(parts)
	if l == 0 {
		return false
	}
	return !slices.Contains(FILTER_LIST, parts[l-1])
}

// 通过消息名称创建实例
func createInstanceFromMessageName(messageName string) (protoreflect.MessageType, error) {
	fullName := protoreflect.FullName(messageName)
	return protoregistry.GlobalTypes.FindMessageByName(fullName)
}

// 将下划线命名转换为Go结构体命名
func convertToGoStructName(input string) string {
	parts := strings.Split(input, "_")

	var result strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}

		if isAbbreviation(part) {
			result.WriteString(part)
		} else {
			if len(part) > 0 {
				result.WriteString(strings.ToUpper(part[:1]))
				if len(part) > 1 {
					result.WriteString(strings.ToLower(part[1:]))
				}
			}
		}
	}

	return result.String()
}

func isAbbreviation(s string) bool {
	if len(s) < 2 || len(s) > 4 {
		return false
	}
	commonAbbreviations := map[string]bool{"REQ": true, "RSP": true, "NTF": true, "S2S": true}
	return commonAbbreviations[s]
}
