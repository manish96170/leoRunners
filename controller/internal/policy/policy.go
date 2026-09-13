package policy

type Mode string

const (
	CustomerFirst Mode = "customer-first"
	ManagedFirst  Mode = "managed-first"
	CustomerOnly  Mode = "customer-only"
	ManagedOnly   Mode = "managed-only"
	Fallback      Mode = "fallback"
)

type Pool struct {
	ID            string
	CustomerOwned bool
	Available     bool
}

func Select(mode Mode, pools []Pool) (Pool, bool) {
	eligible := func(owned bool) (Pool, bool) {
		for _, p := range pools {
			if p.Available && p.CustomerOwned == owned {
				return p, true
			}
		}
		return Pool{}, false
	}
	switch mode {
	case CustomerFirst:
		if p, ok := eligible(true); ok {
			return p, true
		}
		return eligible(false)
	case ManagedFirst:
		if p, ok := eligible(false); ok {
			return p, true
		}
		return eligible(true)
	case CustomerOnly:
		return eligible(true)
	case ManagedOnly:
		return eligible(false)
	case Fallback:
		for _, p := range pools {
			if p.Available {
				return p, true
			}
		}
	}
	return Pool{}, false
}
