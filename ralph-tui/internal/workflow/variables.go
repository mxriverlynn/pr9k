package workflow

// VariablePool is a simple in-memory key-value store for workflow variables.
// It is only accessed sequentially from Run()'s step loop (single goroutine),
// so no mutex is needed.
type VariablePool struct {
	vars map[string]string
}

// NewVariablePool returns an initialized, empty VariablePool.
func NewVariablePool() *VariablePool {
	return &VariablePool{vars: make(map[string]string)}
}

// Set stores value under name, overwriting any previous value.
func (vp *VariablePool) Set(name, value string) {
	vp.vars[name] = value
}

// Get retrieves the value stored under name. The second return value is false
// if no value has been set for name.
func (vp *VariablePool) Get(name string) (string, bool) {
	v, ok := vp.vars[name]
	return v, ok
}

// All returns a shallow copy of the pool's current key-value pairs. Mutations
// to the returned map do not affect the pool.
func (vp *VariablePool) All() map[string]string {
	out := make(map[string]string, len(vp.vars))
	for k, v := range vp.vars {
		out[k] = v
	}
	return out
}

// Clear removes the named keys from the pool. Keys that are not present are
// silently ignored.
func (vp *VariablePool) Clear(names []string) {
	for _, name := range names {
		delete(vp.vars, name)
	}
}
