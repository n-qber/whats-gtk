# whats-gtk

A native WhatsApp desktop client built with Go, GTK4, and libadwaita, powered by [whatsmeow](https://github.com/tulir/whatsmeow).

## Features

- **Native UI**: Built with GTK4 and Adwaita for a modern, consistent Linux desktop experience.
- **Secure Login**: Integrated QR code login process displayed directly in the UI.
- **Powered by whatsmeow**: Uses the robust and feature-rich whatsmeow library for WhatsApp communication.
- **Fast and Lightweight**: Written in Go for optimal performance.
- **Media Support**: Support for images, stickers, audio, and documents.

## Prerequisites

- Go 1.21+
- GTK4 and libadwaita development libraries

## Building

To build the project, run:

```bash
go build -o whats-gtk ./cmd/whats-gtk
```

## Running

```bash
./whats-gtk
```

## Contributing

Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details on how to contribute.

## License

This project is licensed under the terms found in the [LICENSE](LICENSE) file.
