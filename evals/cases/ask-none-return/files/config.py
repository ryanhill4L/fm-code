import json
import os


def load_config(path):
    if not os.path.exists(path):
        return None
    with open(path) as f:
        return json.load(f)
