from pathlib import Path

from excel_common import ExportExcel

FILE = __file__
TOOLS_DIR = Path(__file__).resolve().parent
WORKSPACE = str(TOOLS_DIR.parent)
EXCEL_ROOT = Path(r"E:\work\heli\config\Excel")
LUBAN_DLL = "tools/Luban/Luban.dll"
CONF_ROOT = str(EXCEL_ROOT)
OUTPUT_DATA_DIR = "docconf"
OUTPUT_CODE_DIR = "src/proto/docpb"
TARGET = "server"
DATA_TARGET = "json"
CODE_TARGET = "go-json"
OTHER_PARAM = f"-x inputDataDir={EXCEL_ROOT}/Datas -x lubanGoModule=docpb"


if __name__ == "__main__":
    ExportExcel(
        file=FILE,
        workspace=WORKSPACE,
        luban_dll=LUBAN_DLL,
        conf_root=CONF_ROOT,
        output_data_dir=OUTPUT_DATA_DIR,
        output_code_dir=OUTPUT_CODE_DIR,
        target=TARGET,
        data_target=DATA_TARGET,
        code_target=CODE_TARGET,
        other_param=OTHER_PARAM,
    ).run()
