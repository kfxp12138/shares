#!/usr/bin/env python3
"""
Generate concepts mapping JSON using local 'adata' package and save to file.
Output shape:
  {"codes": [{"code": "sh600000", "concepts": ["人工智能", ...]}, ...]}

Env/Args (optional):
- OUTPUT_FILE: path to output JSON file (default: concepts.json)
- LIMIT_CONCEPTS: int, only process first N concepts
- START_FROM: int, start processing from this concept index (0-based)
- MAX_RETRIES: int, maximum retries per concept (default: 5)
- REQUEST_DELAY: float, delay between requests in seconds (default: 0.5)
"""
import json
import os
import sys
import time
import ssl
import requests
import traceback
from requests.adapters import HTTPAdapter
from urllib3.poolmanager import PoolManager
from collections import defaultdict
import signal

# 在导入任何库之前禁用所有代理
os.environ['NO_PROXY'] = '*'
os.environ['http_proxy'] = ''
os.environ['https_proxy'] = ''
os.environ['HTTP_PROXY'] = ''
os.environ['HTTPS_PROXY'] = ''

# 添加本地 adata 包到路径
ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '../../..'))
ADATA_REPO = os.path.join(ROOT, 'adata')
if ADATA_REPO not in sys.path:
    sys.path.insert(0, ADATA_REPO)

try:
    import adata
    from adata.stock.info import info
except Exception as e:
    sys.stderr.write(f"Failed to import adata: {e}\n")
    sys.exit(2)

# 自定义SSL适配器，强制使用TLSv1.2
class CustomSSLAdapter(HTTPAdapter):
    def init_poolmanager(self, connections, maxsize, block=False):
        self.poolmanager = PoolManager(
            num_pools=connections,
            maxsize=maxsize,
            block=block,
            ssl_version=ssl.PROTOCOL_TLSv1_2,
            ssl_context=ssl.create_default_context(ssl.Purpose.SERVER_AUTH),
        )

def to_prefixed_code(code: str) -> str:
    code = str(code).strip()
    if not code or len(code) < 2:
        return code
    p2 = code[:2]
    if p2 in ('60', '68', '90'):
        return 'sh' + code
    if p2 in ('00', '20', '30'):
        return 'sz' + code
    if p2 in ('83', '87', '43', '92'):
        return 'bj' + code
    return code

def get_concept_constituents_with_retry(concept_code, max_retries=5):
    """获取概念成分股，带重试机制和自定义SSL"""
    for attempt in range(max_retries):
        try:
            # 创建自定义会话
            session = requests.Session()
            adapter = CustomSSLAdapter()
            session.mount('https://', adapter)
            session.trust_env = False  # 忽略系统代理
            
            # 使用备用域名
            host = 'push2.eastmoney.com'
            if attempt % 2 == 1:  # 在重试时尝试备用域名
                host = 'push2delay.eastmoney.com'
            
            url = f"https://{host}/api/qt/clist/get"
            params = {
                'fid': 'f62',
                'po': '1',
                'pz': '200',
                'pn': '1',
                'np': '1',
                'fltt': '2',
                'invt': '2',
                'fs': f'b:{concept_code}',
                'fields': 'f12,f14'
            }
            
            # 设置更长的超时时间
            response = session.get(url, params=params, timeout=60)
            response.raise_for_status()
            data = response.json()
            
            if data.get('rc') != 0:
                raise Exception(f"API error: {data.get('message', 'unknown error')}")
                
            items = data.get('data', {}).get('diff', [])
            return [{'stock_code': item.get('f12'), 'name': item.get('f14')} for item in items]
            
        except Exception as e:
            sys.stderr.write(f"Attempt {attempt+1}/{max_retries} for {concept_code} failed: {e}\n")
            if attempt < max_retries - 1:
                # 指数退避 + 随机抖动
                sleep_time = (2 ** attempt) + (0.1 * attempt)
                sys.stderr.write(f"  Retrying in {sleep_time:.1f} seconds...\n")
                time.sleep(sleep_time)
    return None

# 信号处理函数，用于优雅退出
def signal_handler(sig, frame):
    sys.stderr.write("\n\nInterrupt received, saving progress...\n")
    save_progress()
    sys.exit(0)

def save_progress():
    """保存当前进度到文件"""
    global code_to_concepts, processed, rows, checkpoint_file
    
    if not code_to_concepts:
        sys.stderr.write("No progress to save\n")
        return
        
    checkpoint = {
        "last_processed": processed,
        "concepts": rows,
        "code_to_concepts": dict(code_to_concepts)
    }
    
    try:
        with open(checkpoint_file, 'w') as f:
            json.dump(checkpoint, f)
        sys.stderr.write(f"Progress saved to {checkpoint_file}\n")
    except Exception as e:
        sys.stderr.write(f"Failed to save progress: {e}\n")

def load_progress():
    """从文件加载进度"""
    global code_to_concepts, processed, rows
    
    if not os.path.exists(checkpoint_file):
        return False
        
    try:
        with open(checkpoint_file, 'r') as f:
            checkpoint = json.load(f)
        
        processed = checkpoint.get("last_processed", 0)
        rows = checkpoint.get("concepts", [])
        code_to_concepts = defaultdict(list, checkpoint.get("code_to_concepts", {}))
        
        sys.stderr.write(f"Loaded progress from {checkpoint_file}, starting from index {processed}\n")
        return True
    except Exception as e:
        sys.stderr.write(f"Failed to load progress: {e}\n")
        return False

# 全局变量用于保存进度
code_to_concepts = defaultdict(list)
processed = 0
rows = []
checkpoint_file = os.path.join(os.path.dirname(__file__), 'concepts_checkpoint.json')

def main():
    global code_to_concepts, processed, rows
    
    # 注册信号处理
    signal.signal(signal.SIGINT, signal_handler)
    
    # 获取环境变量
    output_file = os.getenv('OUTPUT_FILE') or 'concepts.json'
    limit = int(os.getenv('LIMIT_CONCEPTS') or '0')
    start_from = int(os.getenv('START_FROM') or '0')
    max_retries = int(os.getenv('MAX_RETRIES') or '5')
    request_delay = float(os.getenv('REQUEST_DELAY') or '0.5')
    
    sys.stderr.write(f"Output will be saved to: {output_file}\n")
    
    # 尝试加载进度
    if not load_progress():
        # 获取概念列表
        try:
            concepts_df = info.all_concept_code_east()
        except Exception as e:
            sys.stderr.write(f"all_concept_code_east failed: {e}\n")
            sys.exit(3)
            
        if concepts_df is None or concepts_df.empty:
            sys.stderr.write("no concepts fetched\n")
            sys.exit(4)

        rows = concepts_df.to_dict(orient='records')
        if limit and limit > 0:
            rows = rows[:limit]
    
    total_concepts = len(rows)
    
    # 设置起始点
    if start_from > 0:
        processed = start_from
        sys.stderr.write(f"Starting from concept index {processed}\n")
    
    # 处理概念
    for i in range(processed, total_concepts):
        processed = i
        row = rows[i]
        cname = str(row.get('name') or '').strip()
        ccode = str(row.get('concept_code') or row.get('index_code') or '').strip()
        
        if not cname or not ccode:
            sys.stderr.write(f"Skipping row {i+1}/{total_concepts}: missing name or code\n")
            continue
            
        sys.stderr.write(f"\nProcessing concept {i+1}/{total_concepts}: {cname} ({ccode})\n")
        
        try:
            constituents = get_concept_constituents_with_retry(ccode, max_retries)
        except Exception as e:
            sys.stderr.write(f"Critical error for {cname} ({ccode}): {e}\n")
            traceback.print_exc()
            continue
            
        if not constituents:
            sys.stderr.write(f"  Failed to get constituents for {cname} ({ccode})\n")
            continue
            
        for item in constituents:
            sc = to_prefixed_code(item.get('stock_code'))
            if sc and cname not in code_to_concepts[sc]:
                code_to_concepts[sc].append(cname)
                
        sys.stderr.write(f"  Added {cname} to {len(constituents)} stocks\n")
        
        # 定期保存进度
        if (i + 1) % 10 == 0:
            save_progress()
        
        # 请求间隔
        time.sleep(request_delay)
    
    # 最终保存进度
    save_progress()
    
    # 输出结果到文件
    out = {"codes": [{"code": k, "concepts": v} for k, v in code_to_concepts.items()]}
    
    try:
        with open(output_file, 'w', encoding='utf-8') as f:
            json.dump(out, f, ensure_ascii=False, indent=2)
        sys.stderr.write(f"\nSuccessfully saved results to {output_file}\n")
    except Exception as e:
        sys.stderr.write(f"Failed to save results to {output_file}: {e}\n")
        sys.exit(5)
    
    # 清理检查点文件
    if os.path.exists(checkpoint_file):
        try:
            os.remove(checkpoint_file)
            sys.stderr.write(f"Checkpoint file {checkpoint_file} removed\n")
        except Exception as e:
            sys.stderr.write(f"Failed to remove checkpoint file: {e}\n")


if __name__ == '__main__':
    main()