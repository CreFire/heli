from export_proto_common import ExportProto

# 工作目录（相对于此脚本）
workspace = "..\\"
# protoc 可执行文件相对 workspace 路径
protoc = "tools\\bin\\protoc.exe"
# 两个插件可执行文件相对 workspace 路径
protoc_go_plugin = "tools\\bin\\protoc-gen-go.exe"
protoc_grpc_plugin = "tools\\bin\\protoc-gen-go-grpc.exe"
# 注入 tag 插件可执行文件相对 workspace 路径
protoc_go_inject = "tools\\bin\\protoc-go-inject-tag.exe"
# 生成代码输出目录
out_dir = "src\\proto"

data = ExportProto(
    file=__file__,
    workspace=workspace,
    protoc=protoc,
    proto_dir="tools\\proto",
    output_dir=out_dir,
    protoc_go_plugin=protoc_go_plugin,
    protoc_grpc_plugin=protoc_grpc_plugin,
    protoc_go_inject=protoc_go_inject,
)


def main():
    data.run()


if __name__ == "__main__":
    main()
