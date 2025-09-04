#!/usr/bin/env python3
"""
Generate concepts JSON for shares service (stdout output).

Two supported output shapes (choose one to print):
1) {"codes": [{"code": "sh600000", "concepts": ["人工智能", "CPO"]}, ...]}
2) {"concepts": [{"name": "人工智能", "codes": ["sh600000", "sz000001"]}, ...]}

This sample prints the "codes" shape with a couple of demo mappings.
"""
import json
import sys

data = {
    "codes": [
        {"code": "sh600000", "concepts": ["人工智能", "CPO"]},
        {"code": "sz000001", "concepts": ["机器人"]},
    ]
}

sys.stdout.write(json.dumps(data, ensure_ascii=False))
sys.stdout.flush()

