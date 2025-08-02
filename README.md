# Loopia Domain Grabber

A command-line tool for working with domains from Loopia. It provides two main commands:

1. **dropcatch** - Snipes expiring domains the instant the registry releases them (usually at 04:00 UTC for .se/.nu domains).
2. **available** - Downloads domain lists, checks for domains expiring tomorrow, and evaluates them based on criteria like length and pronounceability.

You can find .se and .nu domains that will expire soon here: https://internetstiftelsen.se/domaner/registrera-ett-domannamn/se-och-nu-domaner-som-snart-kan-bli-lediga/

## Project Structure

The project follows standard Go project layout conventions:

```
loopiaDomainGrabber/
├── cmd/                    # Command-line applications
│   └── loopiaDomainGrabber/  # Main application entry point
├── internal/               # Private application code
│   ├── available/          # Available command implementation
│   └── dropcatch/          # Dropcatch command implementation
├── pkg/                    # Public libraries that can be imported
│   ├── api/                # Loopia API client
│   ├── config/             # Configuration handling
│   ├── domain/             # Domain-related models
│   └── util/               # Utility functions
├── build/                  # Build artifacts (created by Makefile)
├── Makefile                # Build automation
├── go.mod                  # Go module definition
├── go.sum                  # Go module checksums
├── config.json             # Configuration file
└── README.md               # This file
```

## Features

- **Dropcatch Command**:
  - Attempts to register domains at the exact moment they become available
  - Uses a sophisticated retry mechanism with fast retries and exponential back-off
  - Supports multiple domains with concurrent registration attempts
  - Keeps your computer awake during the registration process

- **Available Command**:
  - Downloads domain lists from Internetstiftelsen
  - Caches the lists for 24 hours to reduce bandwidth usage
  - Identifies domains expiring tomorrow
  - Evaluates domains based on length, pronounceability, and other factors
  - Ranks and displays the most valuable domains

- **General Features**:
  - Configurable via command-line flags or a JSON configuration file
  - Detailed logging of all operations
  - Respects API rate limits (60 calls per hour)
  - Automatically stops sending requests on authentication (401) or rate limit (429) errors

## Installation

### From Source

1. Clone the repository:
```bash
git clone https://github.com/yourusername/loopiaDomainGrabber.git
cd loopiaDomainGrabber
```

2. Build the application:
```bash
make build
```

This will create the executable in the `build` directory.

### Using Go Install

```bash
go install github.com/yourusername/loopiaDomainGrabber/cmd/loopiaDomainGrabber@latest
```

## Usage

The application provides a Makefile for common operations:

```bash
# Show available commands
make help

# Build the application
make build

# Run tests
make test

# Clean build artifacts
make clean
```

### Dropcatch Command

The dropcatch command attempts to register domains as they expire.

```bash
# Using the Makefile
make run-dropcatch ARGS="-domain example.se -keep-awake"

# Using the built executable
./build/loopiaDomainGrabber dropcatch -domain example.se -keep-awake

# Using environment variables for credentials
export LOOPIA_USERNAME="apiuser@loopiaapi"
export LOOPIA_PASSWORD="secret"
./build/loopiaDomainGrabber dropcatch -domain example.se -keep-awake
```

#### Dropcatch Command Flags

- `-domain string`: Domain to register (can be specified in addition to domains in config file)
- `-dry`: Simulate calls without touching the API
- `-now`: Start registration attempts immediately instead of waiting for drop time
- `-keep-awake`: Keep computer awake by moving mouse periodically
- `-config string`: Path to configuration file (default "config.json")

### Available Command

The available command downloads domain lists, checks for domains expiring tomorrow, and evaluates them based on various criteria.

```bash
# Using the Makefile
make run-available

# Using the built executable
./build/loopiaDomainGrabber available

# With a custom config file
./build/loopiaDomainGrabber available -config custom-config.json
```

#### Available Command Flags

- `-config string`: Path to configuration file (default "config.json")

### Using Configuration File

Create a `config.json` file:

```json
{
  "username": "apiuser@loopiaapi",
  "password": "your_password_here",
  "domains": [
    "domain.se",
    "domain.nu"
  ],
  "cache_dir": "cache"
}
```

The configuration file should be placed in the same directory as the executable. The file is automatically loaded when the program starts.

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

## API Limitations

The tool respects the Loopia API limitations and implements safeguards to prevent issues:

- **Rate Limiting**: The tool enforces a maximum of 60 API calls per hour across all domains
- **Error Handling**: If the tool receives a 401 (Unauthorized) or 429 (Too Many Requests) error, it will:
  - Log the error with details
  - Stop sending further API requests to prevent account lockout
  - Abort all pending domain registration attempts

## Notes

- The tool will wait until the next drop time (04:00 UTC) unless the `-now` flag is specified
- While waiting, the tool rechecks the time every 10 minutes to ensure accuracy even during long waits
- The `-keep-awake` flag will move the mouse a few pixels every minute to prevent the computer from sleeping
- Multiple domains can be registered concurrently using goroutines
- The tool will attempt to register domains for up to one hour before giving up

## Requirements

- Go 1.22+
- Loopia API credentials

## License

MIT License
