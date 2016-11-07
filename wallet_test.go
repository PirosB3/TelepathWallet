package wallet

import "testing"
import "time"

import "gopkg.in/redis.v5"

var Client *redis.Client

func init() {
	Client = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	Client.Ping()
}

func TestUnspentTransactions(t *testing.T) {
	tx := NewUnspentTransactionMonitor(Client)
	tx.registerAddresses([]string{
		"myAddress", "hello", "world",
	})
	res := tx.makeWalletRequest(tx.GetAddresses())
	if res != "http://btc.blockr.io/api/v1/address/unspent/myAddress,hello,world" {
		t.Fail()
	}
}

func TestUnspentTransaction(t *testing.T) {
	b := BlockrUnspentItem{
		Amount: "0.00020000",
	}
	if b.Satoshis() != 20000 {
		t.Fail()
	}
}

func TestRedisAddressesSet(t *testing.T) {
	Client.ZAdd("seen_addresses", redis.Z{
		float64(time.Now().Unix()), "hello",
	})
	Client.ZAdd("seen_addresses", redis.Z{
		float64(time.Now().Unix()) - 3600, "world",
	})
	Client.ZAdd("seen_addresses", redis.Z{
		float64(0), "old",
	})

	tx := NewUnspentTransactionMonitor(Client)
	addresses := tx.getAddressesToMonitor()
	if len(addresses) != 2 {
		t.Fail()
	}
}
