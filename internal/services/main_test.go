package services

import (
	"os"
	"testing"

	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutil.Cleanup()
	os.Exit(code)
}
