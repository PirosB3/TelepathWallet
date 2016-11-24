package wallet

import (
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"gopkg.in/redis.v5"
	"testing"
)

var Client *redis.Client
var testDB *gorm.DB
var rs *ReserveService

func TestMain(m *testing.M) {
	Client = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       1,
	})
	Client.Del("seen_addresses").Result()

	var err error
	testDB, err = gorm.Open("sqlite3", ":memory:")
	if err != nil {
		panic(err)
	}
	testDB.AutoMigrate(&Reserve{})
	rs = NewReserverService(testDB)
	m.Run()
}

func TestNewReserveCountForUser(t *testing.T) {
	res := rs.GetAmountReservedForAddress("myAddress")
	if res != 0 {
		t.Log(res)
		t.Fail()
	}

	reserve1, err1 := rs.AddReserveForAddress("myAddress", SATOSHI_IN_BITCOIN)
	reserve2, err2 := rs.AddReserveForAddress("myAddress", SATOSHI_IN_BITCOIN/2)
	res = rs.GetAmountReservedForAddress("myAddress")
	if err1 != nil || err2 != nil {
		t.Fail()
	}
	if reserve1 == reserve2 {
		t.Fail()
	}
	if res != 150000000 {
		t.Fail()
	}
}
