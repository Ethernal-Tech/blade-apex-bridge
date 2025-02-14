package cardanofw

import "github.com/Ethernal-Tech/cardano-infrastructure/sendtx"

type IExchangeService interface {
	GetExchangeRate(srcChain, dstChain string) []sendtx.ExchangeRateEntry
}

type TestExchangeServiceDummy struct {
	rates []sendtx.ExchangeRateEntry
}

func NewExchangeService() IExchangeService {
	return &TestExchangeServiceDummy{
		rates: []sendtx.ExchangeRateEntry{
			{
				SrcChainID: ChainIDPrime,
				DstChainID: ChainIDCardano,
				Value:      0.5,
			},
			{
				SrcChainID: ChainIDCardano,
				DstChainID: ChainIDPrime,
				Value:      2.0,
			},
		},
	}
}

func (es *TestExchangeServiceDummy) GetExchangeRate(srcChain, dstChain string) []sendtx.ExchangeRateEntry {
	exchangeRates := []sendtx.ExchangeRateEntry{}

	for i, rate := range es.rates {
		if rate.SrcChainID == srcChain && rate.DstChainID == dstChain {
			exchangeRates = append(exchangeRates, es.rates[i])
		}
	}

	if len(exchangeRates) == 0 {
		exchangeRates = append(exchangeRates, sendtx.ExchangeRateEntry{
			SrcChainID: srcChain,
			DstChainID: dstChain,
			Value:      1.0,
		})
	}

	return exchangeRates
}

var _ IExchangeService = (*TestExchangeServiceDummy)(nil)
