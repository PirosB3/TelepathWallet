package wallet

import "testing"
import "time"
import "gopkg.in/redis.v5"

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

func TestGetUnspentForBalance(t *testing.T) {
	tx := NewUnspentTransactionMonitor(Client)
	tx.balances["myAddress"] = &AddressBalanceMapping{
		Address: "myAddress",
		UnspentTransactions: []BlockrUnspentItem{
			BlockrUnspentItem{
				Tx:     "aa631d3cb0c98ada8ddb3ec82f23de2a948819e841a00ad740794837b7fbd7e9",
				Idx:    0,
				Amount: "1.0",
			},
			BlockrUnspentItem{
				Tx:     "8787402b7eed22e236b5aaa9d33c8a52c7499d97b5fa93d354f55b78405db14f",
				Idx:    1,
				Amount: "1.0",
			},
		},
		Balance: 200000000,
	}

	var txTests = []struct {
		amount     int64
		txCount    int
		totalSpent int64
	}{
		{120000000, 2, 200000000},
		{9000000, 1, 100000000},
		{0, 0, 0},
		{300000000, 0, 0},
	}

	for _, test := range txTests {
		res, scripts, count := tx.GetTXinsForAddress("myAddress", test.amount)
		if len(res) != test.txCount {
			t.Fail()
		}
		if len(res) != len(scripts) {
			t.Fail()
		}
		if count != test.totalSpent {
			t.Fail()
		}
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
