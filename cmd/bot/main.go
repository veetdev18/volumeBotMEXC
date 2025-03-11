package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	ccxt "github.com/ccxt/ccxt/go/v4"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

// Config holds all configuration for the application
type Config struct {
	APIKey          string
	APISecret       string
	Symbol          string
	MinOrderSize    float64
	MaxOrderSize    float64
	MinGap          float64
	MaxGap          float64
	OrderCount      int
	FillPercent     float64 // Percentage of orders to let execute (0-100)
	SelfTradeGap    float64 // Percentage gap between buy and sell orders in self-trades
	SelfTradeMethod string  // Method for self-trading: "limit" or "market"
	DryRun          bool
	LogLevel        string
}

func main() {
	// Set up logging
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.InfoLevel)

	// Load configuration
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Set log level from config
	level, err := logrus.ParseLevel(config.LogLevel)
	if err == nil {
		log.SetLevel(level)
	}

	// Print configuration (excluding sensitive data)
	log.Infof("Starting Volume Trading Bot for MEXC")
	log.Infof("Symbol: %s", config.Symbol)
	log.Infof("Min Order Size: %f", config.MinOrderSize)
	log.Infof("Max Order Size: %f", config.MaxOrderSize)
	log.Infof("Min Gap: %f%%", config.MinGap)
	log.Infof("Max Gap: %f%%", config.MaxGap)
	log.Infof("Orders per Cycle: %d", config.OrderCount)
	log.Infof("Fill Percent: %f%%", config.FillPercent)
	log.Infof("Self-Trade Gap: %f%%", config.SelfTradeGap)
	log.Infof("Self-Trade Method: %s", config.SelfTradeMethod)
	log.Infof("Dry Run Mode: %v", config.DryRun)

	// Initialize MEXC exchange
	exchange := ccxt.NewMexc(map[string]interface{}{
		"apiKey":          config.APIKey,
		"secret":          config.APISecret,
		"enableRateLimit": true,
	})

	// Set up signal handling for graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// Run the trading bot in a separate goroutine
	stopChan := make(chan struct{})
	go runTradingBot(exchange, config, stopChan)

	// Wait for interrupt signal
	<-signalChan
	log.Info("Shutdown signal received")

	// Trigger graceful shutdown
	close(stopChan)
	log.Info("Bot execution completed")
}

func loadConfig() (*Config, error) {
	// Load .env file if it exists
	_ = godotenv.Load()

	// Load required configuration from environment variables
	apiKey := os.Getenv("MEXC_API_KEY")
	apiSecret := os.Getenv("MEXC_API_SECRET")

	// Check for required fields
	if apiKey == "" || apiSecret == "" {
		return nil, fmt.Errorf("MEXC_API_KEY and MEXC_API_SECRET environment variables are required")
	}

	// Load optional configuration with defaults
	symbol := getEnvWithDefault("SYMBOL", "BTC/USDT")
	minOrderSize := getEnvAsFloat("MIN_ORDER_SIZE", 0.001)
	maxOrderSize := getEnvAsFloat("MAX_ORDER_SIZE", 0.01)
	minGap := getEnvAsFloat("MIN_GAP", 0.1)
	maxGap := getEnvAsFloat("MAX_GAP", 0.5)
	orderCount := getEnvAsInt("ORDER_COUNT", 5)
	fillPercent := getEnvAsFloat("FILL_PERCENT", 20.0)                 // Default 20% of orders execute
	selfTradeGap := getEnvAsFloat("SELF_TRADE_GAP", 0.05)              // Default 0.05% gap
	selfTradeMethod := getEnvWithDefault("SELF_TRADE_METHOD", "limit") // Default to limit orders
	dryRun := getEnvAsBool("DRY_RUN", true)
	logLevel := getEnvWithDefault("LOG_LEVEL", "info")

	return &Config{
		APIKey:          apiKey,
		APISecret:       apiSecret,
		Symbol:          symbol,
		MinOrderSize:    minOrderSize,
		MaxOrderSize:    maxOrderSize,
		MinGap:          minGap,
		MaxGap:          maxGap,
		OrderCount:      orderCount,
		FillPercent:     fillPercent,
		SelfTradeGap:    selfTradeGap,
		SelfTradeMethod: selfTradeMethod,
		DryRun:          dryRun,
		LogLevel:        logLevel,
	}, nil
}

// generateRandomOrderSize returns a random order size between min and max
func generateRandomOrderSize(min, max float64) float64 {
	return min + rand.Float64()*(max-min)
}

// TradeOrder represents an order with its details for tracking
type TradeOrder struct {
	ID        string
	Side      string
	Price     float64
	Size      float64
	ToExecute bool // Whether this order should be allowed to execute
}

// SelfTradePair represents a pair of orders that will trade with each other
type SelfTradePair struct {
	BuyOrderID  string
	SellOrderID string
	Price       float64
	Size        float64
}

func runTradingBot(exchange ccxt.Mexc, config *Config, stopChan <-chan struct{}) {
	// Initialize random number generator with a time-based seed
	rand.Seed(time.Now().UnixNano())

	// Variables to track our trading state
	executedBuyVolume := 0.0
	executedSellVolume := 0.0
	totalFilledVolume := 0.0

	// Load markets to ensure the exchange is ready
	<-exchange.LoadMarkets()
	log.Info("Markets loaded successfully")

	// Main loop for the trading bot
	for {
		select {
		case <-stopChan:
			log.Info("Stopping trading bot")
			return
		default:
			// Continue with trading strategy
		}

		// Step 1: Get order book
		log.Infof("Fetching order book for %s", config.Symbol)
		orderBook, err := exchange.FetchOrderBook(config.Symbol)
		if err != nil {
			log.Errorf("Failed to fetch order book: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		// Step 2: Find prices within the bid-ask spread
		log.Info("Analyzing bid-ask spread for order placement")
		buyPrices, sellPrices := findPricesInBidAskSpread(orderBook, config.OrderCount)

		if len(buyPrices) == 0 && len(sellPrices) == 0 {
			log.Info("No suitable prices found in the bid-ask spread. Waiting...")
			time.Sleep(5 * time.Second)
			continue
		}

		log.Infof("Found %d buy prices and %d sell prices in the bid-ask spread",
			len(buyPrices), len(sellPrices))

		// Track all placed orders
		placedOrders := make([]TradeOrder, 0)

		// Track pairs of orders that will be self-traded
		selfTradePairs := make([]SelfTradePair, 0)

		// Step 3: Place initial orders
		// Calculate how many orders to create for self-trading vs. market execution
		selfTradeCount := int(float64(config.OrderCount) * (config.FillPercent / 100.0))
		log.Infof("Will create %d self-trade pairs", selfTradeCount)

		// Place initial orders for self-trading
		for i := 0; i < selfTradeCount && i < min(len(buyPrices), len(sellPrices)); i++ {
			// Generate a random order size
			orderSize := generateRandomOrderSize(config.MinOrderSize, config.MaxOrderSize)

			// Pick prices from our calculated price levels
			basePrice := (buyPrices[i] + sellPrices[i]) / 2

			// Calculate price gap (used for limit order method)
			gapAmount := basePrice * (config.SelfTradeGap / 100.0)

			// Adjusted prices - for limit orders the buy price is higher to ensure execution
			sellPrice := basePrice - gapAmount
			buyPrice := basePrice + gapAmount

			if config.SelfTradeMethod == "limit" {
				log.Infof("Creating self-trade using LIMIT orders - Sell: %.8f, Buy: %.8f, Size: %.8f",
					sellPrice, buyPrice, orderSize)
			} else {
				log.Infof("Creating self-trade using MARKET order for buy side, Limit sell: %.8f, Size: %.8f",
					sellPrice, orderSize)
			}

			if config.DryRun {
				log.Info("Dry run mode: orders not actually placed")
				selfTradePairs = append(selfTradePairs, SelfTradePair{
					BuyOrderID:  fmt.Sprintf("dry-run-buy-%d", i),
					SellOrderID: fmt.Sprintf("dry-run-sell-%d", i),
					Price:       basePrice,
					Size:        orderSize,
				})
				continue
			}

			// First, place a sell order (always limit order)
			sellOrder, err := exchange.CreateOrder(
				config.Symbol,
				"limit",
				"sell",
				orderSize,
				ccxt.WithCreateOrderPrice(sellPrice),
				ccxt.WithCreateOrderParams(map[string]interface{}{
					"timeInForce": "GTC", // Good Till Canceled
				}),
			)

			if err != nil {
				log.Errorf("Failed to place sell order for self-trading: %v", err)
				continue
			}

			log.Infof("Placed self-trade sell order: %s at price %.8f", *sellOrder.Id, sellPrice)

			// Wait briefly to ensure the sell order is in the book
			time.Sleep(2 * time.Second)

			var buyOrder ccxt.Order

			// Use different order types based on configuration
			if config.SelfTradeMethod == "limit" {
				// Place a limit buy order at a higher price to execute against the sell
				buyOrderResult, err := exchange.CreateOrder(
					config.Symbol,
					"limit",
					"buy",
					orderSize,
					ccxt.WithCreateOrderPrice(buyPrice),
					ccxt.WithCreateOrderParams(map[string]interface{}{
						"timeInForce": "GTC", // Good Till Canceled
					}),
				)

				if err == nil {
					buyOrder = buyOrderResult
				}
			} else {
				// Place a market buy order to guarantee execution
				buyOrderResult, err := exchange.CreateOrder(
					config.Symbol,
					"market",
					"buy",
					orderSize,
				)

				if err == nil {
					buyOrder = buyOrderResult
				}
			}

			if err != nil {
				log.Errorf("Failed to place buy order for self-trading: %v", err)
				// Try to cancel the sell order since we couldn't place the matching buy
				_, _ = exchange.CancelOrder(*sellOrder.Id,
					ccxt.WithCancelOrderSymbol(config.Symbol),
					ccxt.WithCancelOrderParams(nil),
				)
				continue
			}

			if config.SelfTradeMethod == "limit" {
				log.Infof("Placed self-trade limit buy order: %s at price %.8f", *buyOrder.Id, buyPrice)
			} else {
				log.Infof("Placed self-trade market buy order: %s", *buyOrder.Id)
			}

			// Record this pair of orders
			selfTradePairs = append(selfTradePairs, SelfTradePair{
				BuyOrderID:  *buyOrder.Id,
				SellOrderID: *sellOrder.Id,
				Price:       basePrice,
				Size:        orderSize,
			})

			// Update our executed volumes
			executedBuyVolume += orderSize
			executedSellVolume += orderSize
			totalFilledVolume += orderSize

			// Add a longer delay between pairs to allow time for execution
			time.Sleep(3 * time.Second)
		}

		// Step 4: Place additional orders for market visibility (these will be cancelled)
		remainingBuySlots := len(buyPrices) - len(selfTradePairs)
		remainingSellSlots := len(sellPrices) - len(selfTradePairs)

		// Place remaining buy orders that will be cancelled
		for i := 0; i < remainingBuySlots; i++ {
			index := i + len(selfTradePairs)
			if index >= len(buyPrices) {
				break
			}

			price := buyPrices[index]
			orderSize := generateRandomOrderSize(config.MinOrderSize, config.MaxOrderSize)

			log.Infof("Placing additional buy order at price %.8f (will be cancelled)", price)

			if config.DryRun {
				log.Info("Dry run mode: order not actually placed")
				placedOrders = append(placedOrders, TradeOrder{
					ID:        fmt.Sprintf("dry-run-add-buy-%d", i),
					Side:      "buy",
					Price:     price,
					Size:      orderSize,
					ToExecute: false,
				})
				continue
			}

			order, err := exchange.CreateOrder(
				config.Symbol,
				"limit",
				"buy",
				orderSize,
				ccxt.WithCreateOrderPrice(price),
				ccxt.WithCreateOrderParams(map[string]interface{}{
					"timeInForce": "GTC",
				}),
			)

			if err != nil {
				log.Errorf("Failed to place additional buy order: %v", err)
				continue
			}

			log.Infof("Additional buy order placed: %s", *order.Id)
			placedOrders = append(placedOrders, TradeOrder{
				ID:        *order.Id,
				Side:      "buy",
				Price:     price,
				Size:      orderSize,
				ToExecute: false,
			})

			time.Sleep(500 * time.Millisecond)
		}

		// Place remaining sell orders that will be cancelled
		for i := 0; i < remainingSellSlots; i++ {
			index := i + len(selfTradePairs)
			if index >= len(sellPrices) {
				break
			}

			price := sellPrices[index]
			orderSize := generateRandomOrderSize(config.MinOrderSize, config.MaxOrderSize)

			log.Infof("Placing additional sell order at price %.8f (will be cancelled)", price)

			if config.DryRun {
				log.Info("Dry run mode: order not actually placed")
				placedOrders = append(placedOrders, TradeOrder{
					ID:        fmt.Sprintf("dry-run-add-sell-%d", i),
					Side:      "sell",
					Price:     price,
					Size:      orderSize,
					ToExecute: false,
				})
				continue
			}

			order, err := exchange.CreateOrder(
				config.Symbol,
				"limit",
				"sell",
				orderSize,
				ccxt.WithCreateOrderPrice(price),
				ccxt.WithCreateOrderParams(map[string]interface{}{
					"timeInForce": "GTC",
				}),
			)

			if err != nil {
				log.Errorf("Failed to place additional sell order: %v", err)
				continue
			}

			log.Infof("Additional sell order placed: %s", *order.Id)
			placedOrders = append(placedOrders, TradeOrder{
				ID:        *order.Id,
				Side:      "sell",
				Price:     price,
				Size:      orderSize,
				ToExecute: false,
			})

			time.Sleep(500 * time.Millisecond)
		}

		// Step 5: Wait a bit to let the self-trades execute
		waitSeconds := 10
		log.Infof("Waiting %d seconds to let self-trades execute...", waitSeconds)
		time.Sleep(time.Duration(waitSeconds) * time.Second)

		// Step 6: Cancel any remaining orders (additional orders that weren't self-traded)
		for _, order := range placedOrders {
			log.Infof("Cancelling order %s at price %.8f", order.ID, order.Price)

			if config.DryRun {
				log.Info("Dry run mode: order not actually cancelled")
				continue
			}

			// Cancel the order using the correct options pattern
			_, err := exchange.CancelOrder(order.ID,
				ccxt.WithCancelOrderSymbol(config.Symbol),
				ccxt.WithCancelOrderParams(nil),
			)
			if err != nil {
				log.Errorf("Failed to cancel order %s: %v", order.ID, err)
				continue
			}

			log.Infof("Order %s cancelled successfully", order.ID)

			// Add a small delay between cancellations to avoid rate limiting
			time.Sleep(500 * time.Millisecond)
		}

		// Step 7: Log information about what happened this cycle
		log.Infof("Current trading state - Total Volume Created: %.8f", totalFilledVolume)
		log.Infof("Executed Buy Volume: %.8f, Executed Sell Volume: %.8f, Difference: %.8f",
			executedBuyVolume, executedSellVolume, executedBuyVolume-executedSellVolume)

		// Wait a bit before starting the next cycle
		cycleWait := 30
		log.Infof("Cycle completed. Waiting %d seconds before next cycle...", cycleWait)
		time.Sleep(time.Duration(cycleWait) * time.Second)
	}
}

// findPricesInBidAskSpread finds prices within the bid-ask spread
// Returns prices that can be used for placing buy and sell orders
func findPricesInBidAskSpread(orderBook ccxt.OrderBook, numLevels int) (buyPrices []float64, sellPrices []float64) {
	// Check if we have valid bids and asks
	if orderBook.Bids == nil || len(orderBook.Bids) == 0 ||
		orderBook.Asks == nil || len(orderBook.Asks) == 0 {
		return nil, nil
	}

	// Get the highest bid and lowest ask
	highestBid := orderBook.Bids[0][0]
	lowestAsk := orderBook.Asks[0][0]

	// Calculate the spread
	spread := lowestAsk - highestBid

	// If the spread is too small, we can't place orders
	if spread <= 0 {
		return nil, nil
	}

	// Calculate number of price levels to create
	// Add 1 to include both endpoints if needed
	numPriceLevels := min(numLevels, 5) + 1

	// Calculate step size between price levels
	step := spread / float64(numPriceLevels)

	// Generate price levels between highest bid and lowest ask
	for i := 1; i < numPriceLevels; i++ {
		// Calculate price at this level (move up from highest bid)
		price := highestBid + (step * float64(i))

		// Add to appropriate list based on proximity
		if i < numPriceLevels/2 {
			// Closer to bid - use for buy orders
			buyPrices = append(buyPrices, price)
		} else {
			// Closer to ask - use for sell orders
			sellPrices = append(sellPrices, price)
		}
	}

	return buyPrices, sellPrices
}

// Helper functions for environment variables
func getEnvWithDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsFloat(key string, defaultValue float64) float64 {
	if value, exists := os.LookupEnv(key); exists {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
