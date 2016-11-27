package main

import "github.com/PirosB3/TelepathWallet"
import "gopkg.in/redis.v5"

func main() {
	Client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	utxm := wallet.NewUnspentTransactionMonitor(Client)
	utxm.Run()
}
