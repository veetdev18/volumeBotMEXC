# MEXC Volume Trading Bot

A trading bot designed to place sell orders in empty ranges of the MEXC exchange order book and buy back those orders to increase trading volume.

## Features

- Automatically identifies gaps in the order book
- Places sell orders in those gaps
- Buys back placed orders to generate trading volume
- Configurable order sizes and gaps
- Dry run mode for testing without actual trading

## Prerequisites

- Go 1.20 or higher
- Node.js (for CCXT library)
- MEXC exchange API credentials

## Installation

1. Clone this repository:
```bash
git clone https://github.com/yourusername/volumeBot.git
cd volumeBot
```

2. Install the required Go dependencies:
```bash
go mod tidy
```

3. Install the required Node.js dependencies:
```bash
npm install ccxt
```

4. Copy the example environment file and fill in your MEXC API credentials:
```bash
cp .env.example .env
# Edit .env with your favorite text editor
```

## Configuration

Configure the bot by editing the `.env` file or setting environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| MEXC_API_KEY | Your MEXC API key | - |
| MEXC_API_SECRET | Your MEXC API secret | - |
| SYMBOL | Trading pair | BTC/USDT |
| ORDER_SIZE | Size of each order | 0.001 |
| MIN_GAP | Minimum gap percentage to place orders | 0.5 |
| MAX_GAP | Maximum gap percentage to place orders | 2.0 |
| ORDER_COUNT | Number of orders to place per cycle | 5 |
| DRY_RUN | Run in test mode without actual trading | true |
| LOG_LEVEL | Logging level (debug, info, warn, error) | info |

## Usage

Build and run the bot:

```bash
go build -o volumebot cmd/bot/main.go
./volumebot
```

Or run directly:

```bash
go run cmd/bot/main.go
```

## Disclaimer

This bot is provided for educational and research purposes only. Using trading bots may violate exchange terms of service. Use at your own risk. The authors are not responsible for any financial losses incurred through the use of this software.

## License

MIT 