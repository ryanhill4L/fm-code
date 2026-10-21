import os
import sys
import json
import re


def parse(line):
    m = re.match(r"(\w+)=(\w+)", line)
    return m.groups() if m else None


def load(text):
    return json.loads(text)


def main():
    for line in ["a=1", "b=2", "bad"]:
        print(parse(line))
    print(load('{"x": 1}'))
    sys.exit(0)


if __name__ == "__main__":
    main()
