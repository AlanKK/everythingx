package models

type ObjectType int

const (
	ItemIsFile ObjectType = iota
	ItemIsDir
	ItemIsSymlink
)

type EventAction int

const (
	ItemCreated EventAction = iota
	ItemDeleted
)

type EventRecord struct {
	Filename    string
	Path        string
	ObjectType  ObjectType
	EventID     uint64
	EventTime   int64
	EventAction EventAction
	FoundOnScan bool
}

func (e EventAction) String() string {
	switch e {
	case ItemCreated:
		return "ItemCreated"
	case ItemDeleted:
		return "ItemDeleted"
	default:
		return "Unknown"
	}
}
