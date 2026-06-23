package mongoclient

type PersisterMode uint8

const (
	PersisterModeDefault PersisterMode = iota
	PersisterModeReadOnly
	PersisterModeNoDefaultLoad
)

func (m PersisterMode) readOnly() bool {
	return m == PersisterModeReadOnly
}

func (m PersisterMode) noDefaultLoad() bool {
	return m == PersisterModeNoDefaultLoad
}
