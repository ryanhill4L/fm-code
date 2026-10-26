"""Print a per-author summary of the bookshop inventory."""
import json
import sys
from collections import defaultdict


def load(path):
    with open(path) as f:
        return json.load(f)


def by_author(books):
    totals = defaultdict(int)
    for book in books:
        totals[book["author"]] += book["qty"]
    return dict(totals)


# TODO: handle an empty inventory.
def average_price(books):
    return sum(b["price"] for b in books) / len(books)


def main():
    path = sys.argv[1] if len(sys.argv) > 1 else "inventory.json"
    books = load(path)
    for author, count in sorted(by_author(books).items()):
        print(f"{author}: {count} copies")
    print(f"average price: ${average_price(books):.2f}")


if __name__ == "__main__":
    main()
