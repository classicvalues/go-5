package orderbook

import (
	"context"
	"flag"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stellar/go/amount"
	"github.com/stellar/go/support/errors"
	"github.com/stellar/go/xdr"
)

var (
	// offersFile should contain a list of offers
	// each line in the offers file is the base 64 encoding of an offer entry xdr
	offersFile = flag.String("offers", "", "offers file generated by the dump-orderbook tool")
)

// loadGraphFromFile reads an offers file generated by the dump-orderbook tool
// and returns an orderbook built from those offers
func loadGraphFromFile(filePath string) (*OrderBookGraph, error) {
	graph := NewOrderBookGraph()
	rawBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, errors.Wrap(err, "could not read file")
	}

	for _, line := range strings.Split(string(rawBytes), "\n") {
		offer := xdr.OfferEntry{}
		if err := xdr.SafeUnmarshalBase64(line, &offer); err != nil {
			return nil, errors.Wrap(err, "could not base64 decode entry")
		}

		graph.AddOffer(offer)
	}
	if err := graph.Apply(1); err != nil {
		return nil, err
	}

	return graph, nil
}

type request struct {
	src xdr.Asset
	amt xdr.Int64
	dst []xdr.Asset
}

func loadRequestsFromFile(filePath string) ([]request, error) {
	var requests []request
	rawBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, errors.Wrap(err, "could not read file")
	}
	for _, line := range strings.Split(string(rawBytes), "\n") {
		if line == "" {
			continue
		}
		var parsed *url.URL
		if parsed, err = url.Parse(line); err != nil {
			return nil, errors.Wrap(err, "could not parse url")
		}
		var r request
		if parsed.Query().Get("source_asset_type") == "native" {
			r.src = xdr.MustNewNativeAsset()
		} else {
			r.src = xdr.MustNewCreditAsset(
				parsed.Query().Get("source_asset_code"),
				parsed.Query().Get("source_asset_issuer"),
			)
		}
		if r.amt, err = amount.Parse(parsed.Query().Get("source_amount")); err != nil {
			return nil, errors.Wrap(err, "could not parse source amount")
		}
		for _, asset := range strings.Split(parsed.Query().Get("destination_assets"), ",") {
			var parsedAsset xdr.Asset
			if len(asset) == 0 {
				continue
			} else if asset == "native" {
				parsedAsset = xdr.MustNewNativeAsset()
			} else {
				parts := strings.Split(asset, ":")
				parsedAsset = xdr.MustNewCreditAsset(parts[0], parts[1])
			}
			r.dst = append(r.dst, parsedAsset)
		}
		requests = append(requests, r)
	}

	return requests, nil
}

// BenchmarkMultipleDestinationAssets benchmarks the path finding function
// on a request which has multiple destination assets. Most requests to the
// path finding endpoint only specify a single destination asset, so I
// wanted to have benchmark dedicated to this case because it could
// easily be overlooked.
func BenchmarkMultipleDestinationAssets(b *testing.B) {
	if *offersFile == "" {
		b.Skip("missing offers file")
	}
	graph, err := loadGraphFromFile(*offersFile)
	if err != nil {
		b.Fatalf("could not read graph from file: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := graph.FindFixedPaths(
			context.Background(),
			3,
			xdr.MustNewCreditAsset("USDT", "GCQTGZQQ5G4PTM2GL7CDIFKUBIPEC52BROAQIAPW53XBRJVN6ZJVTG6V"),
			amount.MustParse("554.2610400"),
			[]xdr.Asset{
				xdr.MustNewNativeAsset(),
				xdr.MustNewCreditAsset("yXLM", "GARDNV3Q7YGT4AKSDF25LT32YSCCW4EV22Y2TV3I2PU2MMXJTEDL5T55"),
				xdr.MustNewCreditAsset("USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"),
				xdr.MustNewCreditAsset("EURT", "GAP5LETOV6YIE62YAM56STDANPRDO7ZFDBGSNHJQIYGGKSMOZAHOOS2S"),
			},
			5,
		)
		if err != nil {
			b.Fatal("could not find path")
		}
	}
}

// BenchmarkTestData benchmarks the path finding function on a sample of 100 expensive path finding requests.
// The sample of requests was obtained from recent horizon production pubnet logs.
func BenchmarkTestData(b *testing.B) {
	if *offersFile == "" {
		b.Skip("missing offers file")
	}
	graph, err := loadGraphFromFile(*offersFile)
	if err != nil {
		b.Fatalf("could not read graph from file: %v", err)
	}

	requests, err := loadRequestsFromFile(filepath.Join("testdata", "sample-requests"))
	if err != nil {
		b.Fatalf("could not read requests from file: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, req := range requests {
			_, _, err := graph.FindFixedPaths(
				context.Background(),
				3,
				req.src,
				req.amt,
				req.dst,
				5,
			)
			if err != nil {
				b.Fatal("could not find path")
			}
		}
	}
}
