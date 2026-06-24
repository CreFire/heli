package pb

import (
	reflect "reflect"
	sync "sync"

	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type BaseReq struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Gid     int64  `protobuf:"varint,1,opt,name=gid,proto3" json:"gid,omitempty"`
	Session string `protobuf:"bytes,2,opt,name=session,proto3" json:"session,omitempty"`
	ReqId   int32  `protobuf:"varint,3,opt,name=reqId,proto3" json:"reqId,omitempty"`
	ReqTime int32  `protobuf:"varint,4,opt,name=reqTime,proto3" json:"reqTime,omitempty"`
}

func (x *BaseReq) Reset() {
	*x = BaseReq{}
	mi := &file_demo_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *BaseReq) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BaseReq) ProtoMessage() {}

func (x *BaseReq) ProtoReflect() protoreflect.Message {
	mi := &file_demo_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BaseReq.ProtoReflect.Descriptor instead.
func (*BaseReq) Descriptor() ([]byte, []int) {
	return file_demo_proto_rawDescGZIP(), []int{0}
}

func (x *BaseReq) GetGid() int64 {
	if x != nil {
		return x.Gid
	}
	return 0
}

func (x *BaseReq) GetSession() string {
	if x != nil {
		return x.Session
	}
	return ""
}

func (x *BaseReq) GetReqId() int32 {
	if x != nil {
		return x.ReqId
	}
	return 0
}

func (x *BaseReq) GetReqTime() int32 {
	if x != nil {
		return x.ReqTime
	}
	return 0
}

type BaseRsp struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Ret int32  `protobuf:"varint,1,opt,name=ret,proto3" json:"ret,omitempty"`
	Msg string `protobuf:"bytes,2,opt,name=msg,proto3" json:"msg,omitempty"`
}

func (x *BaseRsp) Reset() {
	*x = BaseRsp{}
	mi := &file_demo_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *BaseRsp) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BaseRsp) ProtoMessage() {}

func (x *BaseRsp) ProtoReflect() protoreflect.Message {
	mi := &file_demo_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BaseRsp.ProtoReflect.Descriptor instead.
func (*BaseRsp) Descriptor() ([]byte, []int) {
	return file_demo_proto_rawDescGZIP(), []int{1}
}

func (x *BaseRsp) GetRet() int32 {
	if x != nil {
		return x.Ret
	}
	return 0
}

func (x *BaseRsp) GetMsg() string {
	if x != nil {
		return x.Msg
	}
	return ""
}

var File_demo_proto protoreflect.FileDescriptor

var file_demo_proto_rawDesc = []byte{
	0x0a, 0x0a, 0x64, 0x65, 0x6d, 0x6f, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x02, 0x70, 0x62,
	0x22, 0x65, 0x0a, 0x07, 0x62, 0x61, 0x73, 0x65, 0x52, 0x65, 0x71, 0x12, 0x10, 0x0a, 0x03, 0x67,
	0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x03, 0x52, 0x03, 0x67, 0x69, 0x64, 0x12, 0x18, 0x0a,
	0x07, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07,
	0x73, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x14, 0x0a, 0x05, 0x72, 0x65, 0x71, 0x49, 0x64,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x05, 0x52, 0x05, 0x72, 0x65, 0x71, 0x49, 0x64, 0x12, 0x18, 0x0a,
	0x07, 0x72, 0x65, 0x71, 0x54, 0x69, 0x6d, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x05, 0x52, 0x07,
	0x72, 0x65, 0x71, 0x54, 0x69, 0x6d, 0x65, 0x22, 0x2d, 0x0a, 0x07, 0x62, 0x61, 0x73, 0x65, 0x52,
	0x73, 0x70, 0x12, 0x10, 0x0a, 0x03, 0x72, 0x65, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x05, 0x52,
	0x03, 0x72, 0x65, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x6d, 0x73, 0x67, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x03, 0x6d, 0x73, 0x67, 0x42, 0x13, 0x5a, 0x11, 0x67, 0x61, 0x6d, 0x65, 0x2f, 0x73,
	0x72, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x33,
}

var (
	file_demo_proto_rawDescOnce sync.Once
	file_demo_proto_rawDescData = file_demo_proto_rawDesc
)

func file_demo_proto_rawDescGZIP() []byte {
	file_demo_proto_rawDescOnce.Do(func() {
		file_demo_proto_rawDescData = protoimpl.X.CompressGZIP(file_demo_proto_rawDescData)
	})
	return file_demo_proto_rawDescData
}

var file_demo_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_demo_proto_goTypes = []any{
	(*BaseReq)(nil), // 0: pb.baseReq
	(*BaseRsp)(nil), // 1: pb.baseRsp
}
var file_demo_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_demo_proto_init() }
func file_demo_proto_init() {
	if File_demo_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_demo_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_demo_proto_goTypes,
		DependencyIndexes: file_demo_proto_depIdxs,
		MessageInfos:      file_demo_proto_msgTypes,
	}.Build()
	File_demo_proto = out.File
	file_demo_proto_rawDesc = nil
	file_demo_proto_goTypes = nil
	file_demo_proto_depIdxs = nil
}
