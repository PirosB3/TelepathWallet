package wallet

import (
	"encoding/json"
	"fmt"
	"gopkg.in/redis.v5"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	SATOSHI_IN_BITCOIN     = 100000000
	REFRESH_ADDRESSES_TIME = time.Second * 2
	REFRESH_UTXO_TIME      = time.Second * 3
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

type BlockrUnspentItem struct {
	Tx            string `json:"tx"`
	Amount        string `json:"amount"`
	Idx           int    `json:"n"`
	Confirmations int    `json:"confirmations"`
}

func (b *BlockrUnspentItem) Satoshis() int64 {
	amountFloat, err := strconv.ParseFloat(b.Amount, 64)
	if err != nil {
		Error.Fatal(err)
	}
	res := amountFloat * SATOSHI_IN_BITCOIN
	return int64(res)
}

type BlockrUnspentResponse struct {
	Status string `json:"status"`
	Data   []struct {
		Address string              `json:"address"`
		Unspent []BlockrUnspentItem `json:"unspent"`
	} `json:"data"`
}

type AddressBalanceMapping struct {
	Address             string
	Balance             int64
	UnspentTransactions []BlockrUnspentItem
}

func (abm *AddressBalanceMapping) ToBTC() string {
	res := float64(abm.Balance) / SATOSHI_IN_BITCOIN
	return fmt.Sprintf("%f\n", res)
}

type UnspentTransactionMonitor struct {
	sync.RWMutex
	balances                         map[string]*AddressBalanceMapping
	netClient                        *http.Client
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
	return results
}

func (utm *UnspentTransactionMonitor) Run() {
	for {
		select {
		case <-utm.fetchAddressesTicker.C:
			utm.Lock()
			addresses := utm.addressList
			balances := make(map[string]*AddressBalanceMapping)
			for len(addresses) > 0 {

				var slice uint
				if len(addresses) > 10 {
					slice = 10
				} else {
					slice = uint(len(addresses))
				}

				currentAddresses := addresses[:slice]
				addresses = addresses[slice:]

				walletRequest := utm.makeWalletRequest(currentAddresses)
				response, err := utm.netClient.Get(walletRequest)
				if err != nil {
					Error.Fatal(err)
				}

				var decodedResponse BlockrUnspentResponse
				decoder := json.NewDecoder(response.Body)
				decoder.Decode(&decodedResponse)

				if decodedResponse.Status != "success" {
					Error.Fatal("Decoded response status: " + decodedResponse.Status)
				}

				for _, entry := range decodedResponse.Data {

					address := entry.Address
					var balance int64
					for _, record := range entry.Unspent {
						balance += record.Satoshis()
					}
					balances[address] = &AddressBalanceMapping{
						Address:             address,
						Balance:             balance,
						UnspentTransactions: entry.Unspent,
					}

					Info.Printf("%s has a balance of %s with %d unspent transactions\n", address, balances[address].ToBTC(), len(entry.Unspent))
				}

			}
			utm.balances = balances
			utm.Unlock()

		case <-utm.refreshUnspentTransactionsTicker.C:
			utm.Lock()
			utm.addressList = utm.getAddressesToMonitor()
			Info.Println("Imported addresses:" + strings.Join(utm.addressList, ", "))
			utm.Unlock()
		}
	}
}

func NewUnspentTransactionMonitor(client *redis.Client) *UnspentTransactionMonitor {
	return &UnspentTransactionMonitor{
		balances:                         make(map[string]*AddressBalanceMapping),
		netClient:                        &http.Client{},
		client:                           client,
		fetchAddressesTicker:             time.NewTicker(REFRESH_ADDRESSES_TIME),
		refreshUnspentTransactionsTicker: time.NewTicker(REFRESH_UTXO_TIME),
	}
}
