package witness

import (
	"fmt"
	"math/big"

	"context"
	"github.com/binance/zkmerkle-proof-of-solvency/src/utils"
	"gorm.io/gorm"
	"time"
)

const UserProofTableNamePrefix = "userproof"

// AccountIdIndexName is the name of the unique index on account_id.
const AccountIdIndexName = "idx_str"

type (
	UserProofModel interface {
		CreateUserProofTable() error
		DropUserProofTable() error
		CreateUserProofs(rows []UserProof) error
		// CreateAccountIdIndex ensures account_id has the unique index
		// AccountIdIndexName. New tables already get it from the UserProof
		// struct tag; this upgrades tables created by older versions, which
		// either lack the index or have it as non-unique. It fails if the
		// table already contains duplicate account_ids.
		CreateAccountIdIndex() error
		GetUserProofByIndex(id uint32) (*UserProof, error)
		GetUserProofById(id string) (*UserProof, error)
		GetLatestAccountIndex() (uint32, error)
		GetUserCounts() (int, error)
	}

	defaultUserProofModel struct {
		table string
		DB    *gorm.DB
	}

	// UserProof stores a per-user Merkle inclusion proof.
	UserProof struct {
		AccountIndex    uint32 `gorm:"primaryKey;autoIncrement:false"`
		AccountId       string `gorm:"type:varchar(64);index:idx_str,unique"`
		AccountLeafHash string
		TotalEquity     string
		TotalDebt       string
		TotalCollateral string
		Assets          string
		Proof           string
		Config          string
	}

	UserConfig struct {
		AccountIndex    uint32
		AccountIdHash   string
		TotalEquity     *big.Int
		TotalDebt       *big.Int
		TotalCollateral *big.Int
		Assets          []utils.AccountAsset
		Root            string
		Proof           [][]byte
	}
)

func (m *defaultUserProofModel) TableName() string {
	return m.table
}

func NewUserProofModel(db *gorm.DB, suffix string) UserProofModel {
	return &defaultUserProofModel{
		table: UserProofTableNamePrefix + suffix,
		DB:    db,
	}
}

func (m *defaultUserProofModel) CreateUserProofTable() error {
	return m.DB.Table(m.table).AutoMigrate(UserProof{})
}

func (m *defaultUserProofModel) DropUserProofTable() error {
	return m.DB.Migrator().DropTable(m.table)
}

func (m *defaultUserProofModel) CreateAccountIdIndex() error {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()
	sql := fmt.Sprintf("ALTER TABLE `%s` ADD UNIQUE INDEX `%s` (`account_id`)", m.table, AccountIdIndexName)
	if m.DB.Migrator().HasIndex(m.table, AccountIdIndexName) {
		var nonUnique []int
		err := m.DB.Raw("SELECT NON_UNIQUE FROM information_schema.STATISTICS "+
			"WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?",
			m.table, AccountIdIndexName).Scan(&nonUnique).Error
		if err != nil {
			return err
		}
		if len(nonUnique) > 0 && nonUnique[0] == 0 {
			return nil
		}
		// replace the legacy non-unique index in a single ALTER
		sql = fmt.Sprintf("ALTER TABLE `%s` DROP INDEX `%s`, ADD UNIQUE INDEX `%s` (`account_id`)",
			m.table, AccountIdIndexName, AccountIdIndexName)
	}
	return m.DB.WithContext(ctx).Exec(sql).Error
}

func (m *defaultUserProofModel) CreateUserProofs(rows []UserProof) error {
	dbTx := m.DB.Table(m.table).Create(rows)
	if dbTx.Error != nil {
		return dbTx.Error
	}
	return nil
}

func (m *defaultUserProofModel) GetUserProofByIndex(id uint32) (userproof *UserProof, err error) {
	userproof = &UserProof{}
	dbTx := m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Where("account_index = ?", id).Find(userproof)
	if dbTx.Error != nil {
		return nil, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	} else if dbTx.RowsAffected == 0 {
		return nil, utils.DbErrNotFound
	}
	return userproof, nil
}

func (m *defaultUserProofModel) GetUserProofById(id string) (userproof *UserProof, err error) {
	userproof = &UserProof{}
	dbTx := m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Where("account_id = ?", id).Find(userproof)
	if dbTx.Error != nil {
		return nil, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	} else if dbTx.RowsAffected == 0 {
		return nil, utils.DbErrNotFound
	}
	return userproof, nil
}

func (m *defaultUserProofModel) GetLatestAccountIndex() (uint32, error) {
	var row *UserProof
	dbTx := m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Order("account_index desc").Limit(1).Find(&row)
	if dbTx.Error != nil {
		return 0, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	} else if dbTx.RowsAffected == 0 {
		return 0, utils.DbErrNotFound
	}
	return row.AccountIndex, nil
}

func (m *defaultUserProofModel) GetUserCounts() (int, error) {
	var count int64 = 0
	dbTx := m.DB.Clauses(utils.MaxExecutionTimeHint).Table(m.table).Count(&count)
	if dbTx.Error != nil {
		return 0, utils.ConvertMysqlErrToDbErr(dbTx.Error)
	}
	return int(count), nil
}
