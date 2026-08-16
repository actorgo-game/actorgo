package httpactor

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestTimeoutPrecedence(t *testing.T) {
	handler := &Handler{options: Options{DefaultTimeout: 5 * time.Second, MaxTimeout: 10 * time.Second}}
	request := httptest.NewRequest("POST", "http://example.test/actor", nil)

	if got := handler.timeout(request); got != 5*time.Second {
		t.Fatalf("default timeout: got %s", got)
	}
	request.Header.Set(TimeoutHeader, "7000")
	if got := handler.timeout(request); got != 7*time.Second {
		t.Fatalf("client timeout: got %s", got)
	}
	request.Header.Set(TimeoutHeader, "30000")
	if got := handler.timeout(request); got != 10*time.Second {
		t.Fatalf("capped timeout: got %s", got)
	}
}
