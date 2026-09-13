package policy

import "testing"

func TestPhase9SelectionModes(t *testing.T) {
	tests := []struct {
		name   string
		mode   Mode
		pools  []Pool
		wantID string
		wantOK bool
	}{
		{
			name:   "customer-first prefers an available customer pool",
			mode:   CustomerFirst,
			pools:  []Pool{{ID: "managed-a", Available: true}, {ID: "customer-a", CustomerOwned: true, Available: true}},
			wantID: "customer-a",
			wantOK: true,
		},
		{
			name:   "customer-first falls back to managed pool",
			mode:   CustomerFirst,
			pools:  []Pool{{ID: "managed-a", Available: true}, {ID: "customer-a", CustomerOwned: true, Available: false}},
			wantID: "managed-a",
			wantOK: true,
		},
		{
			name:   "managed-first prefers an available managed pool",
			mode:   ManagedFirst,
			pools:  []Pool{{ID: "customer-a", CustomerOwned: true, Available: true}, {ID: "managed-a", Available: true}},
			wantID: "managed-a",
			wantOK: true,
		},
		{
			name:   "managed-first falls back to customer pool",
			mode:   ManagedFirst,
			pools:  []Pool{{ID: "customer-a", CustomerOwned: true, Available: true}, {ID: "managed-a", Available: false}},
			wantID: "customer-a",
			wantOK: true,
		},
		{
			name:   "customer-only selects customer pool",
			mode:   CustomerOnly,
			pools:  []Pool{{ID: "managed-a", Available: true}, {ID: "customer-a", CustomerOwned: true, Available: true}},
			wantID: "customer-a",
			wantOK: true,
		},
		{
			name:   "customer-only does not use managed pool",
			mode:   CustomerOnly,
			pools:  []Pool{{ID: "managed-a", Available: true}},
			wantOK: false,
		},
		{
			name:   "managed-only selects managed pool",
			mode:   ManagedOnly,
			pools:  []Pool{{ID: "customer-a", CustomerOwned: true, Available: true}, {ID: "managed-a", Available: true}},
			wantID: "managed-a",
			wantOK: true,
		},
		{
			name:   "managed-only does not use customer pool",
			mode:   ManagedOnly,
			pools:  []Pool{{ID: "customer-a", CustomerOwned: true, Available: true}},
			wantOK: false,
		},
		{
			name:   "fallback selects the first available pool",
			mode:   Fallback,
			pools:  []Pool{{ID: "unavailable", Available: false}, {ID: "customer-a", CustomerOwned: true, Available: true}, {ID: "managed-a", Available: true}},
			wantID: "customer-a",
			wantOK: true,
		},
		{
			name:   "unavailable pools produce no selection",
			mode:   Fallback,
			pools:  []Pool{{ID: "customer-a", CustomerOwned: true, Available: false}, {ID: "managed-a", Available: false}},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Select(tt.mode, tt.pools)
			if ok != tt.wantOK {
				t.Fatalf("Select(%q) availability = %v, want %v; pool=%+v", tt.mode, ok, tt.wantOK, got)
			}
			if ok && got.ID != tt.wantID {
				t.Fatalf("Select(%q) pool ID = %q, want %q", tt.mode, got.ID, tt.wantID)
			}
		})
	}
}

func TestPhase9TieSelectionIsDeterministicInInputOrder(t *testing.T) {
	pools := []Pool{
		{ID: "customer-first", CustomerOwned: true, Available: true},
		{ID: "customer-second", CustomerOwned: true, Available: true},
	}

	for i := 0; i < 100; i++ {
		got, ok := Select(CustomerFirst, pools)
		if !ok || got.ID != "customer-first" {
			t.Fatalf("iteration %d selected %+v, available=%v; want customer-first", i, got, ok)
		}
	}

	reversed, ok := Select(CustomerFirst, []Pool{pools[1], pools[0]})
	if !ok || reversed.ID != "customer-second" {
		t.Fatalf("reversed input selected %+v, available=%v; want customer-second", reversed, ok)
	}
}
