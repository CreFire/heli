from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parent
WORKSPACE = ROOT.parent
EXCEL_DIR = Path(r'E:\work\heli\config\Excel')
LUBAN_DIR = ROOT / 'Luban'
CONF = EXCEL_DIR / 'luban.conf'
OUTPUT_DIR = WORKSPACE / 'src' / 'proto' / 'docpb'


def main() -> int:
    if not CONF.exists():
        print(f'未找到配置文件: {CONF}', file=sys.stderr)
        return 1
    if not LUBAN_DIR.exists():
        print(f'未找到 Luban 工具目录: {LUBAN_DIR}', file=sys.stderr)
        return 1
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)

    cmd = [
        str(LUBAN_DIR / 'Luban.exe'),
        '-v',
        '-j', 'cfg',
        '-d', str(EXCEL_DIR),
        '-c', str(CONF),
        '-t', 'server',
        '-o', str(OUTPUT_DIR),
    ]
    print('执行命令:')
    print(' '.join(f'\"{x}\"' if ' ' in x else x for x in cmd))
    return subprocess.call(cmd, cwd=str(ROOT))


if __name__ == '__main__':
    raise SystemExit(main())
