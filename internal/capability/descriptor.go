package capability

type Descriptor struct {
	Capability string
	Version    string
	Command    string
	Tool       string
	Summary    string
	Mutating   bool
}
