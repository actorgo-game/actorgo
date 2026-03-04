package cnuid

import (
	"fmt"
	"testing"
)

func TestNUID(t *testing.T) {
	for range 10 {
		fmt.Println(id.Next())
	}
}

func BenchmarkNUID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		id.Next()
	}
}
