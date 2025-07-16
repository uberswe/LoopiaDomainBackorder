# Loopia Domain Grabber

A command-line tool to snipe expiring domains from Loopia. This tool attempts to register domains the instant Loopia releases them (usually at 04:00 UTC for .se/.nu domains).

The tool uses a sophisticated retry mechanism that fires the first order 30ms before the drop time, performs five ultra-fast retries, then switches to exponential back-off for up to one hour. This maximizes the chances of successfully registering high-demand domains.

## Features

- Attempts to register domains at the exact moment they become available
- Supports multiple domains with concurrent registration attempts
- Keeps your computer awake during the registration process
- Configurable via command-line flags or a JSON configuration file
- Detailed logging of all registration attempts

## Installation

```bash
go get github.com/yourusername/loopiaDomainGrabber
```

## Usage

### Using Command-line Flags

```bash
# Register a single domain using environment variables for credentials
export LOOPIA_USERNAME="apiuser@loopiaapi"
export LOOPIA_PASSWORD="secret"
go run main.go -domain example.se -keep-awake
```

### Using Configuration File

Create a `config.json` file:

```json
{
  "username": "apiuser@loopiaapi",
  "password": "your_password_here",
  "domains": [
    "maxgains.se",
    "996.se",
    "3dlab.se"
  ]
}
```

The configuration file should be placed in the same directory as the executable. The file is automatically loaded when the program starts.

Then run:

```bash
go run main.go -keep-awake
```

## Command-line Flags

- `-domain string`: Domain to register (can be specified in addition to domains in config file)
- `-dry`: Simulate calls without touching the API
- `-now`: Start registration attempts immediately instead of waiting for drop time
- `-keep-awake`: Keep computer awake by moving mouse periodically
- `-config string`: Path to configuration file (default "config.json")

## How It Works

### Retry Mechanism

The tool employs a sophisticated retry strategy to maximize the chances of successful domain registration:

1. **Initial Attempt**: The first registration attempt is made 30ms before the scheduled drop time (typically 04:00 UTC for .se/.nu domains)
2. **Fast Retry Phase**: If the initial attempt fails, the tool immediately makes 5 ultra-fast retries with 100ms intervals
3. **Exponential Backoff Phase**: After the fast retries, the tool switches to an exponential backoff strategy:
   - Starting with a 1-second delay
   - Doubling the delay after each failed attempt
   - Capping the maximum delay at 5 minutes
4. **Purchasing Window**: The tool continues attempting to register the domain for up to one hour before giving up

### Concurrent Registration

The tool uses Go's goroutines to attempt registration of multiple domains concurrently, maximizing efficiency when trying to register several domains at once.

## Notes

- The tool will wait until the next drop time (04:00 UTC) unless the `-now` flag is specified
- The `-keep-awake` flag will move the mouse a few pixels every minute to prevent the computer from sleeping
- Multiple domains can be registered concurrently using goroutines
- The tool will attempt to register domains for up to one hour before giving up

## Requirements

- Go 1.22+
- Loopia API credentials

## License

MIT License
