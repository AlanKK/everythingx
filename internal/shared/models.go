package shared

type ObjectType int

const (
	ItemIsFile ObjectType = iota
	ItemIsDir
	ItemIsSymlink
	ItemIsNamedPipe
	ItemIsSocket
	ItemIsCharDevice
	ItemIsBlockDevice
)

type SearchResult struct {
	Fullpath   string
	ObjectType ObjectType
}

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
	FoundOnScan bool
	IsRename    bool
	// IsDelete marks a record for an event that removed the path (Linux:
	// FAN_DELETE / FAN_MOVED_FROM). It is applied even if something new
	// exists at the path by flush time; that new object has its own later
	// event in the same queue and is inserted after the delete.
	IsDelete bool
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
