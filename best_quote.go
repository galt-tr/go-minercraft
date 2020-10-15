package minercraft

import (
	"context"
	"sync"
)

// BestQuote will check all known miners and compare rates, returning the best rate/quote
//
// Note: this might return different results each time if miners have the same rates as
// it's a race condition on which results come back first
func (c *Client) BestQuote(feeCategory, feeType string) (*FeeQuoteResponse, error) {

	// Best rate & quote
	var bestRate uint64
	var bestQuote FeeQuoteResponse

	// The channel for the internal results
	resultsChannel := make(chan *internalResult, len(c.Miners))

	// Create a context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Loop each miner (break into a Go routine for each quote request)
	var wg sync.WaitGroup
	for _, miner := range c.Miners {
		wg.Add(1)
		go func(ctx context.Context, wg *sync.WaitGroup, client *Client, miner *Miner, resultsChannel chan *internalResult) {
			defer wg.Done()
			resultsChannel <- getQuote(ctx, client, miner)
		}(ctx, &wg, c, miner, resultsChannel)
	}

	// Waiting for all requests to finish
	wg.Wait()
	close(resultsChannel)

	// Loop the results of the channel
	var testRate uint64
	for result := range resultsChannel {

		// Check for error?
		if result.Response.Error != nil {
			return nil, result.Response.Error
		}

		// Parse the response
		quote, err := result.parseQuote()
		if err != nil {
			return nil, err
		}

		// Get a test rate
		if testRate, err = quote.Quote.CalculateFee(feeCategory, feeType, 1000); err != nil {
			return nil, err
		}

		// Never set (or better)
		if bestRate == 0 || testRate < bestRate {
			bestRate = testRate
			bestQuote = quote
		}
	}

	// Return the best quote found
	return &bestQuote, nil
}
