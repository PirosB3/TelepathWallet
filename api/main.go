package main

import (
	"encoding/json"
	"github.com/PirosB3/TelepathWallet"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"gopkg.in/redis.v5"
	"log"
	"net/http"
	"os"
	"time"
)

var (
	Client *redis.Client
	DB     *gorm.DB

	Info  *log.Logger
	Error *log.Logger

	usm     *wallet.UnspentTransactionMonitor
	reserve *wallet.ReserveService
	txMgr   *wallet.TransactionManager
	acctMgr *wallet.AccountManager
)

func init() {
	Info = log.New(os.Stdout,
		"INFO: ",
		log.Ldate|log.Ltime|log.Lshortfile)
	Error = log.New(os.Stdout,
		"ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	var err error
	DB, err = gorm.Open("sqlite3", "mainnet.db")
	if err != nil {
		panic("failed to connect database")
	}
	DB.AutoMigrate(&wallet.Reserve{})
	Client = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	DB.AutoMigrate(&wallet.Account{})
	DB.AutoMigrate(&wallet.Reserve{})
}

func SpendReserve(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	username := vars["user"]
	reserveId := vars["reserve"]

	payload := &struct {
		DestinationUser string `json:"account"`
	}{}
	err := json.NewDecoder(request.Body).Decode(payload)
	if err != nil {
		Error.Fatal(err)
	}

	frmPk, frmAddress := acctMgr.GetKeysForAddress(username)
	_, toAddress := acctMgr.GetKeysForAddress(payload.DestinationUser)
	tx, err := txMgr.SpendReserve(
		frmAddress.EncodeAddress(), reserveId,
		frmPk, toAddress.EncodeAddress(),
	)

	if err != nil {
		Error.Fatal(err)
	}

	response := &struct {
		Transaction string
	}{tx}

	json.NewEncoder(writer).Encode(response)
}

func MakeReserveHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	username := vars["user"]

	payload := &struct {
		Amount int64
	}{}
	err := json.NewDecoder(request.Body).Decode(payload)
	if err != nil {
		Error.Fatal(err)
	}

	_, address := acctMgr.GetKeysForAddress(username)
	utxoBalance, err := usm.GetUTXOBalanceForAddress(address.EncodeAddress())
	if err != nil {
		Error.Fatal(err)
	}

	totalReserve := reserve.GetAmountReservedForAddress(address.EncodeAddress())

	available := utxoBalance - totalReserve
	if payload.Amount > available {
		response := struct {
			Error string
		}{"Amount is too high"}
		json.NewEncoder(writer).Encode(&response)
		return
	}

	idResponse, err := reserve.AddReserveForAddress(address.EncodeAddress(), payload.Amount)
	if err != nil {
		Error.Fatal(err)
	}

	response := struct {
		ReserveId string
	}{idResponse}

	json.NewEncoder(writer).Encode(&response)
}

func AddressHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	username := vars["user"]
	_, address := acctMgr.GetKeysForAddress(username)
	amountReserved := reserve.GetAmountReservedForAddress(address.EncodeAddress())
	unspentTransactionBalance, err := usm.GetUTXOBalanceForAddress(address.EncodeAddress())
	if err != nil {
		Error.Print(err)
		unspentTransactionBalance = 0
	}

	response := struct {
		Address          string `json:"address"`
		AvailableToSpend int64  `json:"available_to_spend"`
		Reserved         int64  `json:"reserved"`
	}{
		Address:          address.EncodeAddress(),
		AvailableToSpend: unspentTransactionBalance,
		Reserved:         amountReserved,
	}

	json.NewEncoder(writer).Encode(&response)
}

func main() {
	acctMgr = wallet.NewAccountManager(DB, Client)
	reserve = wallet.NewReserverService(DB)
	usm = wallet.NewUnspentTransactionMonitor(Client)
	txMgr = wallet.NewTransactionManager(
		usm, reserve,
	)

	go usm.Run()

	r := mux.NewRouter()
	r.HandleFunc("/accounts/{user}/address", AddressHandler)
	r.HandleFunc("/accounts/{user}/reserve", MakeReserveHandler).Methods("POST")
	r.HandleFunc("/accounts/{user}/reserve/{reserve}/spend", SpendReserve).Methods("POST")

	srv := &http.Server{
		Handler: r,
		Addr:    "127.0.0.1:8000",
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	Error.Fatal(srv.ListenAndServe())
}
