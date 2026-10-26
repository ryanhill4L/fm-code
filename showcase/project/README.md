# Bookshop

A tiny inventory tool for a used-book shop. It reads `inventory.json`, prints the total value of the stock, and warns about titles that are running low.

## Usage

    make build
    ./bookshop -f inventory.json -low 3

Run `python3 scripts/report.py` for a per-author summary, and `make test` to run the Go and JavaScript tests.

## Requirements

Go 1.18 or later. Python 3 and Node are only needed for the extras.

## Notes

When new stock arrives, staff should recieve it into `inventory.json` before the shop opens. Prices are in US dollars.
