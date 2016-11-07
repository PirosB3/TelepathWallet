package wallet

import (
	"gopkg.in/redis.v5"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	REFRESH_ADDRESSES_TIME = time.Minute * 1
	REFRESH_UTXO_TIME      = time.Minute * 2
	BLOCKR_UTXO_ADDRESS    = "http://btc.blockr.io/api/v1/address/unspent/"
)

var (
	Info  *log.Logger
	Error *log.Logger
)

func init() {
	Info = log.New(os.Stdout,
		"INFO: ",
		log.Ldate|log.Ltime|log.Lshortfile)
	Error = log.New(os.Stdout,
		"ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)
}

type UnspentTransactionMonitor struct {
	sync.RWMutex
	client                           *redis.Client
	addressList                      []string
	fetchAddressesTicker             *time.Ticker
	refreshUnspentTransactionsTicker *time.Ticker
}

func (utm *UnspentTransactionMonitor) makeWalletRequest(addresses []string) string {
	addressesStr := strings.Join(addresses, ",")
	return BLOCKR_UTXO_ADDRESS + addressesStr
}

func (utm *UnspentTransactionMonitor) GetAddresses() []string {
	return utm.addressList
}

func (utm *UnspentTransactionMonitor) registerAddresses(addresses []string) {
	utm.Lock()
	utm.addressList = addresses
	utm.Unlock()
}

func (utm *UnspentTransactionMonitor) getAddressesToMonitor() []string {
	start := time.Now()
	stop := time.Now().Add(-time.Hour * 24)
	results, err := utm.client.ZRangeByScore("seen_addresses", redis.ZRangeBy{
		Min: strconv.FormatInt(stop.Unix(), 10),
		Max: strconv.FormatInt(start.Unix(), 10),
	}).Result()
	if err != nil {
		Error.Fatal(err)
	}
	Info.Println(len(results))
	return results
}

func NewUnspentTransactionMonitor(client *redis.Client) *UnspentTransactionMonitor {
	return &UnspentTransactionMonitor{
		client:                           client,
		fetchAddressesTicker:             time.NewTicker(REFRESH_ADDRESSES_TIME),
		refreshUnspentTransactionsTicker: time.NewTicker(REFRESH_UTXO_TIME),
	}
}
