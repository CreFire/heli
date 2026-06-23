import logging
import os
import subprocess


# 清理文件夹
def check_and_clear_dir(directory):
    logger = logging.getLogger()
    # 文件夹不存在，创建
    if not os.path.exists(directory):
        os.makedirs(directory)
        logger.info(f"目录不存在，创建目录 {directory}")
    # 文件夹存在，清空
    else:
        # shutil.rmtree(directory)
        # os.makedirs(directory)
        logger.info(f"清空目录 {directory}")


# 命令行输出输出到日志
def run_command_and_log_output(command):
    logger = logging.getLogger()
    try:
        result = subprocess.run(
            command, shell=True, capture_output=True, text=True, encoding="utf-8"
        )
        # 捕获标准输出和标准错误
        stdout = result.stdout
        stderr = result.stderr
        # 通过 logger 输出
        if stdout:
            logger.info("命令行输出：\n\n" + stdout)
        if stderr:
            logger.error("命令行错误：\n\n" + stderr)
    except UnicodeDecodeError as e:
        logger.error(f"UnicodeDecodeError: {e}")
    except Exception as e:
        logger.error(f"命令执行失败: {e}")


# 设置日志
def set_logger(file):
    script_log_name = os.path.splitext(os.path.basename(file))[0] + ".log"
    if os.path.exists(script_log_name):
        os.remove(script_log_name)
    logging.basicConfig(
        filename=script_log_name,  # 日志文件名
        level=logging.INFO,  # 日志级别
        format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",  # 日志格式
        encoding="utf-8",  # 指定编码为UTF-8
    )
    logger = logging.getLogger()
    logger.addHandler(logging.StreamHandler())


# 打印当前信息
def print_current_info(file):
    logging.info(f"当前脚本名：{os.path.basename(file)}")
    abs_path = os.path.dirname(os.path.abspath(__file__))
    logging.info(f"当前绝对路径：{abs_path}")
