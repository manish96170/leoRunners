package policy

import "testing"

func TestCustomerFirst(t *testing.T) {
	p, ok := Select(CustomerFirst, []Pool{{ID: "managed", Available: true}, {ID: "customer", CustomerOwned: true, Available: true}})
	if !ok || p.ID != "customer" {
		t.Fatalf("%+v %v", p, ok)
	}
}
