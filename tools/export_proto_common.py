import logging
import os
import shutil
from pathlib import Path

import utils


class ExportProto:
    """
    Protobuf 导出类：扫描 workspace 下的 proto_dir，
    调用 protoc 生成 .pb.go/.grpc.pb.go，并给 .pb.go 注入 tag。
    """

    def __init__(
        self,
        file: str,
        workspace: str,
        protoc: str,
        proto_dir: str,
        output_dir: str,
        protoc_go_plugin: str,
        protoc_grpc_plugin: str,
        protoc_go_inject: str,
    ):
        """
        :param file:           脚本文件，用于日志标识
        :param workspace:      项目根目录，相对于脚本所在目录
        :param protoc:         protoc 可执行文件相对 workspace 路径
        :param proto_dir:      .proto 文件目录，相对 workspace
        :param output_dir:     生成代码输出目录，相对 workspace
        :param protoc_go_plugin:   protoc-gen-go 可执行文件相对 workspace 路径
        :param protoc_grpc_plugin: protoc-gen-go-grpc 可执行文件相对 workspace 路径
        :param protoc_go_inject:   注入 tag 插件可执行文件相对 workspace 路径
        """
        script_dir = os.path.dirname(os.path.abspath(file))
        self.file = file
        self.workspace = os.path.abspath(os.path.join(script_dir, workspace))
        self.protoc = os.path.join(self.workspace, protoc)
        self.proto_dir = os.path.join(self.workspace, proto_dir)
        self.output_dir = os.path.join(self.workspace, output_dir)
        self.tmp_dir = os.path.join(self.workspace, "tools", "tmp_proto")
        self.protoc_go_plugin = os.path.join(self.workspace, protoc_go_plugin)
        self.protoc_grpc_plugin = os.path.join(self.workspace, protoc_grpc_plugin)
        self.protoc_go_inject = os.path.join(self.workspace, protoc_go_inject)
        utils.set_logger(file)

    def run(self):
        logging.info("===== Protobuf 代码生成 开始 =====")
        utils.print_current_info(self.file)
        logging.info(f"workspace={self.workspace}")
        logging.info(f"protoc={self.protoc}")
        logging.info(f"proto_dir={self.proto_dir}")
        logging.info(f"output_dir={self.output_dir}")
        logging.info(f"tmp_dir={self.tmp_dir}")

        self._prepare_output()
        for root, _, files in os.walk(self.proto_dir):
            for fn in files:
                if fn.endswith(".proto"):
                    self._gen_one(os.path.join(root, fn))
        self._copy_generated_files()

        logging.info("===== Protobuf 代码生成 完成 =====")

    def _prepare_output(self):
        if os.path.exists(self.tmp_dir):
            shutil.rmtree(self.tmp_dir)
        os.makedirs(self.tmp_dir)
        os.makedirs(self.output_dir, exist_ok=True)

    def _gen_one(self, proto_file: str):
        # 1) 调用 protoc 一次生成 pb + grpc_pb
        protoc_cmd = (
            f"{self.protoc} "
            f"{self._proto_path_flags()} "
            f"--plugin=protoc-gen-go={self.protoc_go_plugin} "
            f"--plugin=protoc-gen-go-grpc={self.protoc_grpc_plugin} "
            f"--go_out={self.tmp_dir} --go_opt=module=game "
            f"--go-grpc_out={self.tmp_dir} --go-grpc_opt=module=game "
            f"{proto_file}"
        )
        os.system(protoc_cmd)
        logging.info(f"[protoc] {protoc_cmd}")

        # 2) 只对 .pb.go 注入 tag
        pb_go = self._find_generated_pb_go(proto_file)
        if pb_go:
            inject_cmd = f"{self.protoc_go_inject} -input={pb_go}"
            os.system(inject_cmd)
            os.system(f"gofmt -w {pb_go}")
            # logging.info(f"[inject] {inject_cmd}")

    def _proto_path_flags(self) -> str:
        flags = [f"--proto_path={self.proto_dir}"]
        for d, dirs, _ in os.walk(self.proto_dir):
            for sub in dirs:
                flags.append(f"--proto_path={os.path.join(d, sub)}")
        return " ".join(flags)

    def _find_generated_pb_go(self, proto_file: str):
        pb_go_name = Path(proto_file).with_suffix(".pb.go").name
        for root, _, files in os.walk(self.tmp_dir):
            if pb_go_name in files:
                return os.path.join(root, pb_go_name)
        logging.warning(f"未找到生成文件: {pb_go_name}")
        return None

    def _copy_generated_files(self):
        generated_proto_dir = os.path.join(self.tmp_dir, "src", "proto")
        if not os.path.exists(generated_proto_dir):
            logging.error(f"生成目录不存在: {generated_proto_dir}")
            return

        for name in os.listdir(self.output_dir):
            dst = os.path.join(self.output_dir, name)
            if os.path.isdir(dst) and name.lower() != "docpb":
                shutil.rmtree(dst)

        for name in os.listdir(generated_proto_dir):
            src = os.path.join(generated_proto_dir, name)
            dst = os.path.join(self.output_dir, name)
            if os.path.isdir(src):
                if os.path.exists(dst):
                    shutil.rmtree(dst)
                shutil.copytree(src, dst)
            else:
                shutil.copy2(src, dst)

        shutil.rmtree(self.tmp_dir)
