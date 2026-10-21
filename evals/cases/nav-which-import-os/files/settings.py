import os
import json


def load():
    return json.loads(os.environ.get("SETTINGS", "{}"))
