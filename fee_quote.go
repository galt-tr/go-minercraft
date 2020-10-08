package minercraft

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/bitcoinschema/go-bitcoin"
)

const (

	// FeeTypeData is the key corresponding to the data rate
	FeeTypeData = "data"

	// FeeTypeStandard is the key corresponding to the standard rate
	FeeTypeStandard = "standard"

	// FeeCategoryMining is the category corresponding to the mining rate
	FeeCategoryMining = "mining"

	// FeeCategoryRelay is the category corresponding to the relay rate
	FeeCategoryRelay = "relay"
)

/*
Example feeQuote response from Merchant API:

{
	"payload": "{\"apiVersion\":\"0.1.0\",\"timestamp\":\"2020-10-07T21:13:04.335Z\",\"expiryTime\":\"2020-10-07T21:23:04.335Z\",\"minerId\":\"0211ccfc29e3058b770f3cf3eb34b0b2fd2293057a994d4d275121be4151cdf087\",\"currentHighestBlockHash\":\"000000000000000000edb30c3bbbc8e6a07e522e85522e6a213f7e933e6e2d8d\",\"currentHighestBlockHeight\":655874,\"minerReputation\":null,\"fees\":[{\"feeType\":\"standard\",\"miningFee\":{\"satoshis\":500,\"bytes\":1000},\"relayFee\":{\"satoshis\":250,\"bytes\":1000}},{\"feeType\":\"data\",\"miningFee\":{\"satoshis\":500,\"bytes\":1000},\"relayFee\":{\"satoshis\":250,\"bytes\":1000}}]}",
	"signature": "304402206443bea5bdd98a16e23eb61c36b4b998bd68ceb9c84983c7e695e267b21a30440220191571e9b9632c8337d9196723ca20eefa63966ef6360170db0e57a04047453f",
	"publicKey": "0211ccfc29e3058b770f3cf3eb34b0b2fd2293057a994d4d275121be4151cdf087",
	"encoding": "UTF-8",
	"mimetype": "application/json"
}
*/

// FeeQuoteResponse is the raw response from the Merchant API request
//
// Specs: https://github.com/bitcoin-sv-specs/brfc-merchantapi/tree/v1.2-beta#get-fee-quote
type FeeQuoteResponse struct {
	Miner     *Miner      `json:"miner"` // Custom field for our internal Miner configuration
	Quote     *FeePayload `json:"quote"` // Custom field for unmarshalled payload data
	Payload   string      `json:"payload"`
	Validated bool        `json:"validated"` // Custom field if the signature has been validated
	Signature string      `json:"signature"`
	PublicKey string      `json:"publicKey"`
	Encoding  string      `json:"encoding"`
	MimeType  string      `json:"mimetype"`
}

/*
Example FeeQuoteResponse.Payload (unmarshalled):

{
  "apiVersion": "0.1.0",
  "timestamp": "2020-10-07T21:13:04.335Z",
  "expiryTime": "2020-10-07T21:23:04.335Z",
  "minerId": "0211ccfc29e3058b770f3cf3eb34b0b2fd2293057a994d4d275121be4151cdf087",
  "currentHighestBlockHash": "000000000000000000edb30c3bbbc8e6a07e522e85522e6a213f7e933e6e2d8d",
  "currentHighestBlockHeight": 655874,
  "minerReputation": null,
  "fees": [
    {
      "feeType": "standard",
      "miningFee": {
        "satoshis": 500,
        "bytes": 1000
      },
      "relayFee": {
        "satoshis": 250,
        "bytes": 1000
      }
    },
    {
      "feeType": "data",
      "miningFee": {
        "satoshis": 500,
        "bytes": 1000
      },
      "relayFee": {
        "satoshis": 250,
        "bytes": 1000
      }
    }
  ]
}
*/

// FeePayload is the unmarshalled version of the payload envelope
type FeePayload struct {
	APIVersion                string      `json:"apiVersion"`
	Timestamp                 string      `json:"timestamp"`
	ExpirationTime            string      `json:"expiryTime"`
	MinerID                   string      `json:"minerId"`
	CurrentHighestBlockHash   string      `json:"currentHighestBlockHash"`
	CurrentHighestBlockHeight uint64      `json:"currentHighestBlockHeight"`
	MinerReputation           interface{} `json:"minerReputation"` // Not sure what this value is
	Fees                      []*feeType  `json:"fees"`
}

// GetFee will return the fee for the given txBytes
// Type: "FeeTypeData" or "FeeTypeStandard"
// Category: "FeeCategoryMining" or "FeeCategoryRelay"
//
// If no fee is found or fee is 0, returns 1 & error
//
// Spec: https://github.com/bitcoin-sv-specs/brfc-misc/tree/master/feespec#deterministic-transaction-fee-calculation-dtfc
func (f *FeePayload) GetFee(feeCategory, feeType string, txBytes int64) (int64, error) {

	// Valid feeType?
	if !strings.EqualFold(feeType, FeeTypeData) && !strings.EqualFold(feeType, FeeTypeStandard) {
		return 0, fmt.Errorf("feeType %s is not recognized", feeType)
	} else if !strings.EqualFold(feeCategory, FeeCategoryMining) && !strings.EqualFold(feeCategory, FeeCategoryRelay) {
		return 0, fmt.Errorf("feeCategory %s is not recognized", feeCategory)
	}

	// Loop all fee types looking for feeType (data or standard)
	for _, fee := range f.Fees {

		// Detect the type (data or standard)
		if fee.FeeType == feeType {

			// Multiply & Divide
			var calcFee int64
			if strings.EqualFold(feeCategory, FeeCategoryMining) {
				calcFee = (fee.MiningFee.Satoshis * txBytes) / fee.MiningFee.Bytes
			} else {
				calcFee = (fee.RelayFee.Satoshis * txBytes) / fee.RelayFee.Bytes
			}

			// Check for zero
			if calcFee != 0 {
				return calcFee, nil
			}

			// todo: maybe the error is not needed here?
			return 1, fmt.Errorf("warning: fee calculation was 0")
		}
	}

	// No fee type found in the slice of fees
	return 1, fmt.Errorf("feeType %s is not found in fees", feeType)
}

/*
Example FeePayload.Fees type:
{
  "feeType": "standard",
  "miningFee": {
	"satoshis": 500,
	"bytes": 1000
  },
  "relayFee": {
	"satoshis": 250,
	"bytes": 1000
  }
}
*/

// feeType is the the corresponding type of fee (standard or data)
type feeType struct {
	FeeType   string     `json:"feeType"`
	MiningFee *feeAmount `json:"miningFee"`
	RelayFee  *feeAmount `json:"relayFee"`
}

// feeAmount is the actual fee for the given feeType
type feeAmount struct {
	Bytes    int64 `json:"bytes"`
	Satoshis int64 `json:"satoshis"`
}

// FeeQuote will fire a Merchant API request to retrieve the fees from a given miner
//
// This endpoint is used to get the different fees quoted by a miner.
// It returns a JSONEnvelope with a payload that contains the fees charged by a specific BSV miner.
// The purpose of the envelope is to ensure strict consistency in the message content for the purpose of signing responses.
//
// Specs: https://github.com/bitcoin-sv-specs/brfc-merchantapi/tree/v1.2-beta#get-fee-quote
func (c *Client) FeeQuote(miner *Miner) (*FeeQuoteResponse, error) {

	// Make sure we have a valid miner
	if miner == nil {
		return nil, errors.New("miner was nil")
	}

	// Make the HTTP request for the quote
	result := getQuote(c, miner)
	if result.Response.Error != nil {
		return nil, result.Response.Error
	}

	// Parse the response into a quote
	response, err := parseResponseIntoQuote(result)
	if err != nil {
		return nil, err
	}

	// Valid quotes?
	if response.Quote == nil || len(response.Quote.Fees) == 0 {
		return nil, errors.New("failed getting quotes from: " + miner.Name)
	}

	// Return the fully parsed response
	return &response, nil
}

// BestQuote will check all known miners and compare rates, returning the best rate/quote
//
// Note: this might return different results each time if miners have the same rates as
// it's a race condition on which results come back first
func (c *Client) BestQuote(feeCategory, feeType string) (*FeeQuoteResponse, error) {

	// Best rate & quote
	var bestRate int64
	var bestQuote FeeQuoteResponse

	// The channel for the internal results
	resultsChannel := make(chan *feeResult, len(c.Miners))

	// Loop each miner (break into a Go routine for each quote request)
	var wg sync.WaitGroup
	for _, miner := range c.Miners {
		wg.Add(1)
		go getQuoteRoutine(&wg, c, miner, resultsChannel)
	}

	// Waiting for all requests to finish
	wg.Wait()
	close(resultsChannel)

	// Loop the results of the channel
	var testRate int64
	for result := range resultsChannel {

		// Check for error?
		if result.Response.Error != nil {
			return nil, result.Response.Error
		}

		// Parse the response into a Quote
		quote, err := parseResponseIntoQuote(result)
		if err != nil {
			return nil, err
		}

		// Do we have a rate set?
		if bestRate == 0 {
			bestQuote = quote
			if bestRate, err = quote.Quote.GetFee(feeCategory, feeType, 1000); err != nil {
				return nil, err
			}
		} else { // Test the other quotes
			if testRate, err = quote.Quote.GetFee(feeCategory, feeType, 1000); err != nil {
				return nil, err
			}
			if testRate < bestRate {
				bestRate = testRate
				bestQuote = quote
			}
		}
	}

	// Return the best quote found
	return &bestQuote, nil
}

// feeResult is a shim for storing miner & http response data
type feeResult struct {
	Response *RequestResponse
	Miner    *Miner
}

// parseResponseIntoQuote will convert the HTTP response into a struct and also
// unmarshal the payload JSON data
func parseResponseIntoQuote(result *feeResult) (response FeeQuoteResponse, err error) {

	// Set the miner on the response
	response.Miner = result.Miner

	// Unmarshal the response
	if err = json.Unmarshal(result.Response.BodyContents, &response); err != nil {
		return
	}

	// If we have a valid payload
	if len(response.Payload) > 0 {

		// Remove all escaped slashes from payload envelope
		// Also needed for signature validation since it was signed before escaping
		response.Payload = strings.Replace(response.Payload, "\\", "", -1)
		if err = json.Unmarshal([]byte(response.Payload), &response.Quote); err != nil {
			return
		}
	}

	// Validate the signature if found
	if len(response.Signature) > 0 && len(response.PublicKey) > 0 {

		// Verify using DER format
		if response.Validated, err = bitcoin.VerifyMessageDER(
			sha256.Sum256([]byte(response.Payload)),
			response.PublicKey,
			response.Signature,
		); err != nil {
			return
		}
	}

	return
}

// getQuote will fire the HTTP request to retrieve the fee quote
func getQuote(client *Client, miner *Miner) (result *feeResult) {
	result = &feeResult{Miner: miner}
	result.Response = httpRequest(
		client,
		http.MethodGet,
		"https://"+miner.URL+"/mapi/feeQuote",
		miner.Token,
		nil,
		http.StatusOK,
	)
	return
}

// getQuoteRoutine will fire getQuote as part of a WaitGroup and return
// the results into a channel
func getQuoteRoutine(wg *sync.WaitGroup, client *Client, miner *Miner, resultsChannel chan *feeResult) {
	defer wg.Done()
	resultsChannel <- getQuote(client, miner)
}

// todo: add new method (FastestQuote) (tries all, cancels after first one succeeds)
