package witness

import (
	"fmt"
	"os"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openTestDB(t *testing.T) *gorm.DB {
	dbUri := os.Getenv("TEST_MYSQL_DSN")
	if dbUri == "" {
		t.Skip("TEST_MYSQL_DSN not set")
	}
	db, err := gorm.Open(mysql.Open(dbUri), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func accountIdIndexNonUnique(t *testing.T, db *gorm.DB, table string) []int {
	var nonUnique []int
	err := db.Raw("SELECT NON_UNIQUE FROM information_schema.STATISTICS "+
		"WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?",
		table, AccountIdIndexName).Scan(&nonUnique).Error
	if err != nil {
		t.Fatal(err)
	}
	return nonUnique
}

// createLegacyUserProofTable creates a userproof table as older versions did:
// no index on account_id, optionally followed by a non-unique idx_str.
func createLegacyUserProofTable(t *testing.T, db *gorm.DB, table string, withNonUniqueIndex bool) {
	if err := db.Exec(fmt.Sprintf("CREATE TABLE `%s` (`account_index` int unsigned, `account_id` varchar(64), UNIQUE INDEX `idx_int` (`account_index`))", table)).Error; err != nil {
		t.Fatal(err)
	}
	if withNonUniqueIndex {
		if err := db.Exec(fmt.Sprintf("ALTER TABLE `%s` ADD INDEX `%s` (`account_id`)", table, AccountIdIndexName)).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestCreateAccountIdIndex(t *testing.T) {
	db := openTestDB(t)
	suffix := "_idx_test"
	table := UserProofTableNamePrefix + suffix
	m := NewUserProofModel(db, suffix)

	cases := []struct {
		name  string
		setup func()
	}{
		{"new table", func() {
			if err := m.CreateUserProofTable(); err != nil {
				t.Fatal(err)
			}
		}},
		{"legacy table without index", func() { createLegacyUserProofTable(t, db, table, false) }},
		{"legacy table with non-unique index", func() { createLegacyUserProofTable(t, db, table, true) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m.DropUserProofTable()
			defer m.DropUserProofTable()
			c.setup()
			// run twice: the second call must be a no-op
			for i := 0; i < 2; i++ {
				if err := m.CreateAccountIdIndex(); err != nil {
					t.Fatalf("call %d: %v", i, err)
				}
				if got := accountIdIndexNonUnique(t, db, table); len(got) != 1 || got[0] != 0 {
					t.Fatalf("call %d: expected one unique index entry, got NON_UNIQUE=%v", i, got)
				}
			}
		})
	}

	t.Run("legacy table with duplicate account ids", func(t *testing.T) {
		m.DropUserProofTable()
		defer m.DropUserProofTable()
		createLegacyUserProofTable(t, db, table, true)
		if err := db.Exec(fmt.Sprintf("INSERT INTO `%s` (account_index, account_id) VALUES (0, 'dup'), (1, 'dup')", table)).Error; err != nil {
			t.Fatal(err)
		}
		if err := m.CreateAccountIdIndex(); err == nil {
			t.Fatal("expected an error for duplicate account ids")
		}
		// the failed ALTER must leave the legacy index in place
		if got := accountIdIndexNonUnique(t, db, table); len(got) != 1 || got[0] != 1 {
			t.Fatalf("expected legacy non-unique index to remain, got NON_UNIQUE=%v", got)
		}
	})
}
