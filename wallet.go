package wallet

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"sync"

	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

const BTC_FEE_IN_SATOSHIS = 20400

type TransactionManager struct {
	sync.Mutex
	unspentTransactionMonitorInstance *UnspentTransactionMonitor
	reserveInstance                   *ReserveService
	netClient                         *http.Client
}

func NewTransactionManager(
	unspentTransactionMonitorInstance *UnspentTransactionMonitor,
	reserveInstance *ReserveService,
) *TransactionManager {
	return &TransactionManager{
		unspentTransactionMonitorInstance: unspentTransactionMonitorInstance,
		reserveInstance:                   reserveInstance,
		netClient:                         &http.Client{},
	}
}

func (tm *TransactionManager) makePayToPubkeyHashScript(address string) ([]byte, error) {
	dstAddress, err := btcutil.DecodeAddress(address, &chaincfg.MainNetParams)
	if err != nil {
		return nil, err
	}
	p2pkh, err := txscript.PayToAddrScript(dstAddress)
	if err != nil {
		return nil, err
	}
	return p2pkh, nil
}

func (tm *TransactionManager) SpendReserve(
	address, reserve string,
	pk *btcec.PrivateKey,
	dstAddressString string,
) error {
	tm.Lock()
	defer tm.Unlock()

	// Encode the transaction
	var buffer bytes.Buffer
	txBytes, err := tm.MakeTransactionForReserve(address, reserve, pk, dstAddressString)
	if err != nil {
		return err
	}

	txHexString := hex.EncodeToString(txBytes)
	payload := struct {
		Hex string `json:"hex"`
	}{
		Hex: txHexString,
	}
	json.NewEncoder(&buffer).Encode(payload)
	res, err := tm.netClient.Post(BLOCKR_PUSHTX_ADDRESS, "application/json", &buffer)
	if err != nil {
		return err
	}

	// Decode response
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	Info.Println(string(body))

	err = tm.reserveInstance.SpendReserve(address, reserve)
	if err != nil {
		return err
	}
	return nil
}

func (tm *TransactionManager) MakeTransactionForReserve(
	address, reserve string,
	pk *btcec.PrivateKey,
	dstAddressString string,
) ([]byte, error) {

	// Get amount to spend
	amountToSpend, err := tm.reserveInstance.GetAmountReservedForReserve(address, reserve)
	if err != nil {
		return nil, err
	}

	// Get transactions for that amount
	txIns, scripts, totalSpent := tm.unspentTransactionMonitorInstance.GetTXinsForAddress(
		address, amountToSpend,
	)
	if totalSpent < amountToSpend {
		return nil, errors.New("Insufficient funds")
	}

	// Make out scripts
	dstScript, err := tm.makePayToPubkeyHashScript(dstAddressString)
	if err != nil {
		return nil, err
	}
	returnScript, err := tm.makePayToPubkeyHashScript(address)
	if err != nil {
		return nil, err
	}

	// Make Transaction
	tx := wire.NewMsgTx()
	for _, txin := range txIns {
		tx.AddTxIn(txin)
	}
	toDst := amountToSpend - BTC_FEE_IN_SATOSHIS
	tx.AddTxOut(wire.NewTxOut(
		toDst, dstScript,
	))
	remainder := totalSpent - toDst
	if remainder > 0 {
		tx.AddTxOut(wire.NewTxOut(
			remainder, returnScript,
		))
	}

	lookupKey := func(a btcutil.Address) (*btcec.PrivateKey, bool, error) {
		return pk, true, nil
	}

	// Sign transaction
	for idx, _ := range tx.TxIn {
		sigScript, err := txscript.SignTxOutput(&chaincfg.MainNetParams,
			tx, idx, scripts[idx], txscript.SigHashAll,
			txscript.KeyClosure(lookupKey), nil, nil)
		if err != nil {
			return nil, err
		}
		tx.TxIn[idx].SignatureScript = sigScript
	}

	// Post transaction
	byteBuffer := make([]byte, 0, tx.SerializeSize())
	buffer := bytes.NewBuffer(byteBuffer)
	err = tx.Serialize(buffer)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
