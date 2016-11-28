package wallet

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
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
	REFRESH_ADDRESSES_TIME = time.Second * 3
	REFRESH_UTXO_TIME      = time.Second * 5
	BLOCKR_UTXO_ADDRESS    = "http://btc.blockr.io/api/v1/address/unspent/"
	BLOCKR_PUSHTX_ADDRESS  = "http://btc.blockr.io/api/v1/tx/push"
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
	Script        string `json:"script"`
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
	return fmt.Sprintf("%f", res)
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

func (utm *UnspentTransactionMonitor) GetTXinsForAddress(
	address string,
	amount int64,
) ([]*wire.TxIn, [][]byte, int64) {

	utm.RLock()
	var res []*wire.TxIn
	var scripts [][]byte

	if item, ok := utm.balances[address]; ok {
		if item.Balance < amount {
			return res, scripts, 0
		}

		var currentValue int64
		for _, utxo := range item.UnspentTransactions {
			if currentValue >= amount {
				break
			}
			hash, err := chainhash.NewHashFromStr(utxo.Tx)
			if err != nil {
				Error.Fatal(err)
			}
			txin := wire.NewTxIn(
				wire.NewOutPoint(
					hash, uint32(utxo.Idx),
				),
				[]byte{},
			)
			res = append(res, txin)

			byteScript, err := hex.DecodeString(utxo.Script)
			if err != nil {
				Error.Fatal(err)
			}

			scripts = append(scripts, byteScript)
			currentValue += utxo.Satoshis()
		}
		return res, scripts, currentValue
	}
	return res, scripts, -1
}

func (utm *UnspentTransactionMonitor) GetUTXOBalanceForAddress(address string) (int64, error) {
	utm.RLock()
	defer utm.RUnlock()
	if el, ok := utm.balances[address]; ok {
		return el.Balance, nil
	}
	return -1, errors.New(fmt.Sprintf("Address %s was not found\n", address))
}

func (utm *UnspentTransactionMonitor) Run() {
	for {
		select {
		case <-utm.fetchAddressesTicker.C:
			Info.Println("TICK")
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
				Info.Println(walletRequest)
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
						if record.Confirmations > 0 {
							balance += record.Satoshis()
						}
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
			Info.Println("TOCK")

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
