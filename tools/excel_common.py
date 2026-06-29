import logging
import os
import subprocess

import utils


class ExportExcel:
    """
    Excel配置导出类：通过Luban工具导出配置数据和生成代码。
    支持输出数据格式如 json/bin，代码格式如 cs-simple-json 等。
    """

    def __init__(
        self,
        file: str,
        workspace: str,
        luban_dll: str,
        conf_root: str,
        output_data_dir: str,
        output_code_dir: str,
        target: str,
        data_target: str,
        code_target: str,
        other_param: str,
    ):
        """
        参数：
        workspace：为工作目录，一般为 Client、Server、Excel、Tool 同级目录
        luban_dll：为 luban.dll 文件路径，相对于 workspace
        conf_root: 为 luban.conf 目录，相对于 workspace
        output_data_dir: 为 数据输出目录，相对于 workspace
        output_code_dir: 为 代码输出目录，相对于 workspace
        target：输出目标，一般考虑 client、server、all
        data_target：输出数据格式，一般考虑 bin、json
        code_target：输出代码格式，C# 等，需要按要求配置
        other_param：其他参数
        """
        self.file = file
        # 相对路径转绝对路径
        # 工作目录
        self.workspace = os.path.abspath(workspace)
        # luban 文件
        self.luban_dll = os.path.join(self.workspace, luban_dll)
        # --conf
        self.conf_root = os.path.join(self.workspace, conf_root)
        # -x, --xargs  outputDataDir
        self.output_data_dir = os.path.join(self.workspace, output_data_dir)
        # -x, --xargs  outputCodeDir
        self.output_code_dir = os.path.join(self.workspace, output_code_dir)
        # -t, --target
        self.target = target
        # -d, --dataTarget
        self.data_target = data_target
        # -c, --codeTarget
        self.code_target = code_target
        # -x
        self.other_param = other_param
        utils.set_logger(file)

    # 主执行函数：构建并执行 luban 导出命令
    def run(self):
        # 打印当前信息
        logging.info(
            "==================================== 鲁班导出数据 ===================================="
        )
        utils.print_current_info(self.file)

        # 打印配置信息
        logging.info(
            "====================================== 常量值 ======================================"
        )
        logging.info(f"工作目录：{self.workspace}")
        logging.info(f"Luban.dll 文件：{self.luban_dll}")
        logging.info(f"luban.conf 目录：{self.conf_root}")
        logging.info(f"数据输出目录：{self.output_data_dir}")
        logging.info(f"代码输出目录：{self.output_code_dir}")
        logging.info(f"目标名(client、server、all)：{self.target}")
        logging.info(f"数据类型：{self.data_target}")
        logging.info(f"代码类型：{self.code_target}")
        logging.info(f"其他参数：{self.other_param}")

        # 参数处理，执行命令
        logging.info(
            "=================================== 参数处理，执行命令 ==================================="
        )
        self.check_output_dir()
        self.run_with_table_trace()

        # 结束
        logging.info(
            "===================================== 程序结束 ====================================="
        )

    # 清理输出目录下的旧文件
    def check_output_dir(self):
        # 清空 数据输出目录
        utils.check_and_clear_dir(self.output_data_dir)
        # 清空 代码输出目录
        utils.check_and_clear_dir(self.output_code_dir)

    # dotnet
    def luban_dll_cmd(self):
        return f"dotnet {self.luban_dll}"

    # --conf
    def conf_file_cmd(self):
        return f"--conf {self.conf_root}\\luban.conf"

    # -t
    def target_cmd(self):
        return f"-t {self.target}"

    # -d
    def data_target_cmd(self):
        return f"-d {self.data_target}"

    # -c
    def code_target_cmd(self):
        return f"-c {self.code_target}"

    # -x outputDataDir
    def out_put_data_dir_cmd(self):
        return f"-x outputDataDir={self.output_data_dir}"

    # -x outputCodeDir
    def out_put_code_dir_cmd(self):
        return f"-x outputCodeDir={self.output_code_dir}"

    # 构造完整命令字符串（包括各类参数）
    def cmd(self):
        return (
            f"{self.luban_dll_cmd()} {self.conf_file_cmd()} "
            f"{self.target_cmd()} {self.data_target_cmd()} {self.code_target_cmd()} "
            f"{self.out_put_data_dir_cmd()} {self.out_put_code_dir_cmd()} {self.other_param}"
        )

    def run_with_table_trace(self):
        tables = self._find_excel_files()
        if not tables:
            logging.error(f"未找到任何 Excel 文件：{self.output_data_dir}")
            return

        logging.info("========== 表清单开始 ==========")
        for i, table in enumerate(tables, 1):
            logging.info(f"表[{i}/{len(tables)}]：{table}")
        logging.info("========== 表清单结束 ==========")

        # 批处理：一次 Luban 进程处理整个目录，自动按 target group 过滤
        logging.info("========== 批处理模式开始 ==========")
        cmd_str = self.cmd()
        logging.info(f"执行命令：{cmd_str}")
        result = subprocess.run(
            cmd_str, shell=True, capture_output=True, text=True, encoding="utf-8"
        )
        if result.stdout:
            for line in result.stdout.strip().splitlines():
                if line:
                    logging.info(line)
        if result.stderr:
            for line in result.stderr.strip().splitlines():
                if line:
                    logging.error(line)
        if result.returncode != 0:
            logging.error(f"命令返回码：{result.returncode}")
        logging.info("========== 批处理模式结束 ==========")

    def cmd_for_single_table(self, table_path: str):
        # Luban appends filename to inputDataDir, so pass parent Datas/ dir
        parent_dir = os.path.dirname(table_path)
        return (
            f"{self.luban_dll_cmd()} {self.conf_file_cmd()} "
            f"{self.target_cmd()} {self.data_target_cmd()} {self.code_target_cmd()} "
            f"{self.out_put_data_dir_cmd()} {self.out_put_code_dir_cmd()} "
            f"-x inputDataDir={parent_dir} -x lubanGoModule=docpb"
        )

    def _find_excel_files(self):
        result = []
        for root, _, files in os.walk(
            os.path.join(self.workspace, "..", "config", "Excel", "Datas")
        ):
            for fn in files:
                if fn.lower().endswith(".xlsx") and not fn.startswith("~$"):
                    result.append(os.path.join(root, fn))
        result.sort()
        return result
