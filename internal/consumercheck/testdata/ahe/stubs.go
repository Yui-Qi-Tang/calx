package consumer

// These reader-free types deliberately cover only the pure compiler boundary.
type ConsistencyScope struct{}
type ConsistencyMember struct{ NodeID string }
type ConsistencyView struct{ Members []ConsistencyMember }
