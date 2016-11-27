package wallet

import (
	"encoding/hex"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	"testing"
)

func TestSpendReserve(t *testing.T) {
	txmgr := NewTransactionManager(
		NewUnspentTransactionMonitor(Client),
		NewReserverService(testDB),
	)

	frmPK, _ := btcec.NewPrivateKey(btcec.S256())
	frmAddress, _ := btcutil.NewAddressPubKey(frmPK.PubKey().SerializeCompressed(), &chaincfg.MainNetParams)

	toPK, _ := btcec.NewPrivateKey(btcec.S256())
	toAddress, _ := btcutil.NewAddressPubKey(toPK.PubKey().SerializeCompressed(), &chaincfg.MainNetParams)

	p2pkhFrmAddress, _ := txmgr.makePayToPubkeyHashScript(frmAddress.EncodeAddress())
	p2pkhFrmAddressString := hex.EncodeToString(p2pkhFrmAddress)

	// Add some balances
	txmgr.unspentTransactionMonitorInstance.balances = map[string]*AddressBalanceMapping{
		frmAddress.EncodeAddress(): &AddressBalanceMapping{
			Balance: 200000000,
			UnspentTransactions: []BlockrUnspentItem{
				BlockrUnspentItem{
					Tx:     "aa631d3cb0c98ada8ddb3ec82f23de2a948819e841a00ad740794837b7fbd7e9",
					Idx:    0,
					Script: p2pkhFrmAddressString,
					Amount: "1.0",
				},
				BlockrUnspentItem{
					Tx:     "8787402b7eed22e236b5aaa9d33c8a52c7499d97b5fa93d354f55b78405db14f",
					Script: p2pkhFrmAddressString,
					Idx:    1,
					Amount: "1.0",
				},
			},
		},
	}

	reserve, _ := txmgr.reserveInstance.AddReserveForAddress(frmAddress.EncodeAddress(), 120000000)

	res, err := txmgr.MakeTransactionForReserve(frmAddress.EncodeAddress(), reserve, frmPK, toAddress.EncodeAddress())
	if err != nil {
		t.Fail()
	}
	t.Log(hex.EncodeToString(res))
}
