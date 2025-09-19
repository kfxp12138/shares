#!/usr/bin/env python3
"""Fetch daily kline data via adata and upsert into shares_daily_tbl.

Example::

  python import_daily_history.py --start 2025-09-01 --end 2025-09-18 --limit 200 \
      --output shares/output/daily_import.json --log shares/output/daily_import.log

Requirements::

  pip install -r ../../adata/requirements.txt
  pip install -e ../../adata
  pip install pymysql pyyaml
"""

import argparse
import os
import sys
import json
from datetime import datetime
from typing import Optional

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '../../..'))
ADATA_REPO = os.path.join(ROOT, 'adata')
if ADATA_REPO not in sys.path:
    sys.path.insert(0, ADATA_REPO)

for key in ("http_proxy", "https_proxy", "HTTP_PROXY", "HTTPS_PROXY"):
    os.environ[key] = ""

try:
    import adata
    from adata.common.utils import code_utils
except Exception as exc:  # pragma: no cover
    sys.stderr.write(f"Failed to import adata: {exc}\n")
    sys.exit(2)

code_utils.exchange_suffix.setdefault('81', '.SH')
code_utils.exchange_suffix.setdefault('82', '.SZ')
code_utils.exchange_suffix.setdefault('88', '.SH')

try:
    import yaml
except ImportError:
    yaml = None

import pymysql

DEFAULT_OUTPUT = os.path.join(ROOT, 'shares', 'output', 'daily_import.json')
DEFAULT_LOG = os.path.join(ROOT, 'shares', 'output', 'daily_import.log')

CONFIG_PATH = os.path.join(ROOT, 'shares', 'shares', 'conf', 'config.yml')


def load_db_config():
    """Read DB config from config.yml or env variables."""
    username = os.getenv('DB_USERNAME', '')
    password = os.getenv('DB_PASSWORD', '')
    host = os.getenv('DB_HOST', 'localhost')
    port = int(os.getenv('DB_PORT', '3306'))
    database = os.getenv('DB_NAME', 'caoguo_dev')

    if yaml and os.path.exists(CONFIG_PATH):
        with open(CONFIG_PATH, 'r', encoding='utf-8') as fh:
            cfg = yaml.safe_load(fh) or {}
        db_info = (cfg.get('db_info') or {})
        cfg_user = db_info.get('username')
        if cfg_user not in (None, ''):
            username = cfg_user
        cfg_pwd = db_info.get('password')
        if cfg_pwd not in (None, ''):
            password = cfg_pwd
        host = db_info.get('host') or host
        port = db_info.get('port') or port
        database = db_info.get('database') or database

    if not username or not password:
        raise RuntimeError('DB username/password not configured. Set in config.yml or env.')

    return {
        'host': host,
        'port': int(port),
        'user': str(username),
        'password': str(password),
        'database': database,
        'charset': 'utf8mb4',
        'cursorclass': pymysql.cursors.DictCursor,
    }


def to_prefixed_code(code: str, exchange: str) -> str:
    code = code.strip()
    exch = exchange.upper()
    if exch.startswith('SH') or exch == 'SESH':
        return f'sh{code}'
    if exch.startswith('SZ') or exch == 'SESZ':
        return f'sz{code}'
    if exch.startswith('BJ'):
        return f'bj{code}'
    return code


def fetch_codes(limit: Optional[int]) -> list[str]:
    df = adata.stock.info.all_code()
    if limit:
        df = df.head(limit)
    return [to_prefixed_code(row.stock_code, row.exchange) for row in df.itertuples()]


def upsert_daily(cursor, code: str, row):
    day_str = row['trade_date']
    try:
        day0 = int(datetime.strptime(day_str, '%Y-%m-%d').timestamp())
    except ValueError:
        day0 = int(datetime.now().timestamp())

    sql = (
        "REPLACE INTO shares_daily_tbl "
        "(code, day0, day0_str, percent, price, close_price, volume, turnover, "
        "turnover_rate, macd, dea, dif, open_price, max, min, created_at) "
        "VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, NOW())"
    )

    percent = row.get('pct_chg', 0.0)
    close_price = row.get('close', 0.0)
    volume = (row.get('vol') or 0.0) / 100  # shares -> hands
    turnover = (row.get('amount') or 0.0) / 10000

    cursor.execute(
        sql,
        (
            code,
            day0,
            day_str,
            percent,
            close_price,
            close_price,
            volume,
            turnover,
            row.get('turnover_rate'),
            row.get('macd'),
            row.get('dea'),
            row.get('dif'),
            row.get('open'),
            row.get('high'),
            row.get('low'),
        ),
    )


def strip_prefix(code: str) -> str:
    if code.lower().startswith(('sh', 'sz', 'bj')):
        return code[2:]
    return code


def import_daily(start: str, end: Optional[str], limit: Optional[int],
                 output_path: Optional[str], log_path: Optional[str]):
    codes = fetch_codes(limit)
    db_cfg = load_db_config()
    conn = pymysql.connect(**db_cfg)
    cursor = conn.cursor()

    total_rows = 0
    log_entries = []
    for idx, code in enumerate(codes, start=1):
        raw_code = strip_prefix(code)
        df = adata.stock.market.get_market(stock_code=raw_code, start_date=start, end_date=end, k_type=1, adjust_type=1)
        if df is None or df.empty:
            msg = f"[{idx}/{len(codes)}] {code}: no data"
            print(msg)
            log_entries.append(msg)
            continue
        for row in df.to_dict('records'):
            row['trade_date'] = row.get('trade_date')
            row['close'] = row.get('close')
            row['pct_chg'] = row.get('change_pct')
            row['vol'] = row.get('volume')
            row['amount'] = row.get('amount')
            row['turnover_rate'] = row.get('turnover_ratio')
            row['macd'] = row.get('macd')
            row['dea'] = row.get('dea')
            row['dif'] = row.get('dif')
            upsert_daily(cursor, code, row)
            total_rows += 1
        if idx % 10 == 0:
            conn.commit()
            msg = f"[{idx}/{len(codes)}] {code}: committed"
            print(msg)
            log_entries.append(msg)

    conn.commit()
    cursor.close()
    conn.close()
    summary = {
        "start": start,
        "end": end,
        "codes": len(codes),
        "rows": total_rows,
        "run_at": datetime.now().isoformat(timespec='seconds'),
    }
    print(f"Import finished. affected rows: {total_rows}")

    if output_path:
        os.makedirs(os.path.dirname(output_path), exist_ok=True)
        with open(output_path, 'w', encoding='utf-8') as fh:
            json.dump(summary, fh, ensure_ascii=False, indent=2)
    if log_path:
        os.makedirs(os.path.dirname(log_path), exist_ok=True)
        with open(log_path, 'w', encoding='utf-8') as fh:
            for line in log_entries:
                fh.write(line + '\n')
            fh.write(json.dumps(summary, ensure_ascii=False) + '\n')


def main():
    parser = argparse.ArgumentParser(description='Import daily kline data using adata')
    parser.add_argument('--start', default='2025-09-01', help='start date YYYY-MM-DD (default: 2025-09-01)')
    parser.add_argument('--end', default=None, help='end date YYYY-MM-DD (default: today)')
    parser.add_argument('--limit', type=int, default=0, help='limit number of codes to import (default: 0)')
    parser.add_argument('--output', default=DEFAULT_OUTPUT, help='save summary json (default: shares/output/daily_import.json)')
    parser.add_argument('--log', default=DEFAULT_LOG, help='save execution log (default: shares/output/daily_import.log)')
    args = parser.parse_args()

    import_daily(args.start, args.end, args.limit, args.output, args.log)


if __name__ == '__main__':
    main()
