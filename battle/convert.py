#!/usr/bin/env python
"""Convert battle CSV files to JSON for game server loading."""

import csv
import json
import os
import sys

SRC = os.path.dirname(os.path.abspath(__file__))
DST = os.path.join(os.path.dirname(SRC), 'docconf', 'battle')

CSV_FILES = [
    'enemies_actual_zh.csv',
    'towers_actual_zh.csv',
    'bosses_actual_zh.csv',
    'battle_settings_actual_zh.csv',
]


def to_value(s: str):
    s = s.strip()
    if s == '':
        return ''
    try:
        return int(s)
    except ValueError:
        pass
    try:
        return float(s)
    except ValueError:
        pass
    return s


def main():
    os.makedirs(DST, exist_ok=True)

    for fname in CSV_FILES:
        src = os.path.join(SRC, fname)
        base = fname.replace('_actual_zh.csv', '')
        dst = os.path.join(DST, f'battle_{base}.json')

        if not os.path.exists(src):
            print(f'SKIP: {src} not found')
            continue

        with open(src, 'r', encoding='utf-8-sig') as f:
            reader = csv.DictReader(f)
            rows = []
            for row in reader:
                converted = {k: to_value(v) for k, v in row.items()}
                rows.append(converted)

        with open(dst, 'w', encoding='utf-8') as f:
            json.dump(rows, f, ensure_ascii=False, indent=2)

        ncols = len(rows[0]) if rows else 0
        print(f'{fname} -> {os.path.basename(dst)} ({len(rows)} rows, {ncols} cols)')

    print('DONE')


if __name__ == '__main__':
    main()
