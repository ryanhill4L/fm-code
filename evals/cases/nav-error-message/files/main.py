from parse import parse_age

try:
    parse_age("abc")
except ValueError as e:
    print("error:", e)
