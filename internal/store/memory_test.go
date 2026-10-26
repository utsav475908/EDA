package store_test

import (
	"testing"

	"github.com/lakehouse-platform/lp/internal/store"
	"github.com/lakehouse-platform/lp/internal/store/storetest"
)

func TestMemoryConformance(t *testing.T) {
	storetest.Run(t, store.NewMemory())
}
