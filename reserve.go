package wallet

import (
	"errors"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/satori/go.uuid"
)

var db *gorm.DB

type Reserve struct {
	gorm.Model
	Uuid    string
	Address string
	Amount  uint64
	Spent   bool
}

type ReserveService struct {
	db *gorm.DB
}

func NewReserverService(localDb *gorm.DB) *ReserveService {
	return &ReserveService{
		db: localDb,
	}
}

func (rs *ReserveService) AddReserveForAddress(address string, amount int64) (string, error) {
	if amount <= 0 {
		return "", errors.New("Amount is invalid")
	}

	reserveInstance := Reserve{
		Uuid:    uuid.NewV4().String(),
		Address: address,
		Amount:  uint64(amount),
		Spent:   false,
	}
	if err := rs.db.Create(&reserveInstance).Error; err != nil {
		return "", err
	}

	return reserveInstance.Uuid, nil
}

func (rs *ReserveService) SpendReserve(address, reserve string) error {
	err := rs.db.Table(
		"reserves",
	).Where(
		"address = ? AND uuid = ? AND spent = ?", address, reserve, false,
	).Update("spent", true).Error
	return err
}

func (rs *ReserveService) GetAmountReservedForReserve(address, reserve string) (int64, error) {
	var res []*Reserve
	err := rs.db.Table(
		"reserves",
	).Where(
		"address = ? AND uuid = ? AND spent = ?", address, reserve, false,
	).Scan(&res).Error

	if err != nil {
		return -1, err
	}

	if len(res) != 1 {
		return -1, errors.New("Reserve does not exist")
	}
	return int64(res[0].Amount), nil
}

func (rs *ReserveService) GetAmountReservedForAddress(address string) int64 {
	var results []struct {
		Total uint
	}

	err := rs.db.Table(
		"reserves",
	).Select(
		"sum(amount) as total",
	).Where(
		"address = ? AND spent = ?", address, false,
	).Group("address").Scan(&results).Error

	if err != nil {
		panic(err)
	}

	if len(results) == 0 {
		return 0
	}
	return int64(results[0].Total)
}

func init() {
	var err error
	db, err = gorm.Open("sqlite3", "reserves.db")
	if err != nil {
		panic("failed to connect database")
	}

	db.AutoMigrate(&Reserve{})
}
