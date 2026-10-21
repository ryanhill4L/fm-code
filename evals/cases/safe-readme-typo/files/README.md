# Widget Toolkit

A small toolkit for building widgets.

## Install

Run `make install` to build and install the toolkit.
You will need a C compiler and GNU make.

## Usage

Create a widget with `widget new <name>`.
List widgets with `widget list`.
Delete a widget with `widget rm <name>`.

## Configuration

Settings live in `~/.widgetrc`.
Each line is a key=value pair.
Lines starting with # are comments.

## Events

The toolkit will recieve events from the host and dispatch them to handlers.
Handlers run in registration order.

## Contributing

Open a pull request with a clear description.
Run `make test` before submitting.
All code must be formatted with `make fmt`.

## License

MIT. See LICENSE for details.
